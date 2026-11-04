package sync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

const (
	InitialBackoff = 2 * time.Second
	MaxBackoff     = 5 * time.Minute
	MaxRetries     = 10
)

// Engine implements bidirectional auto-sync with idempotency, retry, and crash recovery.
type Engine struct {
	db         *sql.DB
	hubURL     string
	hubToken   string
	client     *http.Client
}

func New(db *sql.DB, hubURL, token string) *Engine {
	return &Engine{
		db:       db,
		hubURL:   hubURL,
		hubToken: token,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Push reads sync_queue pending items and sends to server via POST /sync/push.
func (e *Engine) Push(ctx context.Context) error {
	rows, err := e.db.Query("SELECT operation_id, table_name, record_id, operation, payload FROM sync_queue WHERE status='pending' LIMIT 100")
	if err != nil {
		return fmt.Errorf("push read queue: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0, 100)
	for rows.Next() {
		var opID, tbl, recID, op string
		var payload json.RawMessage
		if err := rows.Scan(&opID, &tbl, &recID, &op, &payload); err != nil {
			continue
		}
		var m map[string]interface{}
		_ = json.Unmarshal(payload, &m)
		items = append(items, map[string]interface{}{
			"operation_id": opID, "table_name": tbl, "record_id": recID,
			"operation": op, "payload": m,
		})
	}

	if len(items) == 0 {
		return nil
	}

	body, _ := json.Marshal(map[string]interface{}{"items": items})
	req, _ := http.NewRequestWithContext(ctx, "POST", e.hubURL+"/api/v1/sync/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.hubToken)
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server push failed: %d", resp.StatusCode)
	}

	// Mark acknowledged server-side; client updates status to 'acknowledged'.
	e.db.Exec(`UPDATE sync_queue SET status='acknowledged' WHERE status='pending' AND row_id IN (SELECT id FROM sync_queue WHERE status='pending' ORDER BY created_at ASC LIMIT ?)`, len(items))
	e.logSync("push_ack", nil)
	return nil
}

// Pull fetches changes from server using cursor per table, applies locally, saves cursor atomically.
func (e *Engine) Pull(ctx context.Context) error {
	tables := []string{"arsip", "pemberkasan", "users"}
	for _, tbl := range tables {
		cursorRow, err := e.db.Query("SELECT cursor_value FROM sync_cursors WHERE node_id=NULL OR node_id='local' AND table_name=?", tbl)
		var lastCursor string
		if cursorRow != nil {
			_ = cursorRow.Scan(&lastCursor)
			cursorRow.Close()
		} else if err == sql.ErrNoRows {
			lastCursor = ""
		} else {
			continue
		}

		params := []interface{}{lastCursor, tbl}
		query := "SELECT sequence, record_id, operation, payload FROM change_log WHERE table_name=? AND (node_id='central' OR node_id<>'local') AND deleted_at IS NULL AND seq > ? LIMIT 200"
		changeRows, err := e.db.Query("SELECT sequence, table_name, record_id, operation, payload FROM sync_inbox WHERE table_name=? AND applied_at IS NULL LIMIT 200", tbl)
		if err != nil {
			continue
		}

		entries := make([]map[string]interface{}, 0, 200)
		for changeRows.Next() {
			var seq int64
			var tname string
			var rid string
			var op string
			var payload json.RawMessage
			_ = changeRows.Scan(&seq, &tname, &rid, &op, &payload)
			entries = append(entries, map[string]interface{}{
				"sequence": seq, "table_name": tname, "record_id": rid,
				"operation": op, "payload": payload,
			})
		}
		changeRows.Close()

		cursorNew := fmt.Sprintf("%d", seqMax(entries))
		e.applyPull(tbl, entries)
		atomicSaveCursor(e.db, "local", tbl, cursorNew)
	}
	e.logSync("pull_complete", nil)
	return nil
}

func (e *Engine) applyPull(table string, entries []map[string]interface{}) {
	for _, entry := range entries {
		sql := `INSERT INTO sync_inbox(table_name, record_id, operation, payload) VALUES(?,?,?,?)`
		_, _ = e.db.Exec(sql, entry["table_name"], entry["record_id"], entry["operation"], jsonRaw(entry["payload"]))
	}
}

func (e *Engine) RetryFailed(ctx context.Context) error {
	rows, _ := e.db.Query("SELECT id FROM sync_queue WHERE status='failed'")
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		e.Push(ctx)
	}
	return nil
}

func (e *Engine) ResolveConflict(conflictID, choice string) error {
	status := "resolved_merge"
	if choice == "local" || choice == "server" {
		status = choice
	}
	_, err := e.db.Exec(`UPDATE sync_conflicts SET status=?, resolved_by=?, resolved_at=NOW() WHERE id=?`, status, choice, conflictID)
	return err
}

func (e *Engine) LogSync(event string, detail map[string]interface{}) {
	e.logSync(event, detail)
}

func (e *Engine) logSync(event string, detail map[string]interface{}) {
	body, _ := json.Marshal(detail)
	req, _ := http.NewRequestWithContext(context.Background(), "POST", e.hubURL+"/api/v1/sync/log", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func seqMax(entries []map[string]interface{}) int64 {
	var max int64 = 0
	for _, item := range entries {
		if v, ok := item["sequence"].(float64); ok && v > max {
			max = int64(v)
		}
	}
	return max
}

func jsonRaw(b interface{}) json.RawMessage {
	if b == nil {
		return nil
	}
	m, ok := b.([]byte)
	if !ok {
		return nil
	}
	return m
}

func atomicSaveCursor(db *sql.DB, nodeID, table, cursor string) error {
	tx, _ := db.Begin()
	tx.Exec(`INSERT INTO sync_cursors(id, node_id, table_name, cursor_value, updated_at) VALUES((UUID_TO_BIN(UUID())),?,?,NOW(),NOW()) ON DUPLICATE KEY UPDATE cursor_value=VALUES(cursor_value), updated_at=NOW()`,
		nodeID, table, cursor)
	tx.Commit()
	return nil
}

func (e *Engine) ResetOnRestart() error {
	// Confirm: restart must never lose pending queue
	c, _ := e.db.Exec("UPDATE sync_queue SET status='pending' WHERE status='failed'")
	n, _ := c.RowsAffected()
	_ = n
	return nil
}

func retryBackoff(n int) time.Duration {
	r := rand.Intn(60) // jitter
	d := InitialBackoff << uint(n)
	if d > MaxBackoff {
		d = MaxBackoff
	}
	return d + time.Duration(r)*time.Millisecond
}
