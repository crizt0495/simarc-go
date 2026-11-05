package sync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	InitialBackoff = 2 * time.Second
	MaxBackoff     = 5 * time.Minute
	MaxRetries     = 10
)

// Engine implements bidirectional sync with an offline-first local database.
// Seluruh akses database berjalan di sini (backend Golang), antarmuka hanya
// menampilkan hasil binding — tidak ada akses DB dari frontend.
type Engine struct {
	db        *sql.DB
	hubURL    string
	hubToken  string
	client    *http.Client
	mu        sync.Mutex // TriggerSync boleh dipangggil ulang dari UI
	lastError string
}

func New(db *sql.DB, hubURL, token string) *Engine {
	return &Engine{
		db:      db,
		hubURL:  hubURL,
		hubToken: token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// LastError returns the last sync error (untuk ditampilkan di UI).
func (e *Engine) LastError() string { return e.lastError }

// RunSync menjalankan satu siklus push+pull; aman dipanggil dari UI.
func (e *Engine) RunSync(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.Push(ctx); err != nil {
		e.lastError = err.Error()
		return err
	}
	if err := e.Pull(ctx); err != nil {
		e.lastError = err.Error()
		return err
	}
	e.lastError = ""
	return nil
}

// Push reads sync_queue pending items and sends to hub via POST /api/v1/sync/push.
func (e *Engine) Push(ctx context.Context) error {
	rows, err := e.db.Query("SELECT operation_id, table_name, HEX(record_id), operation, payload FROM sync_queue WHERE status='pending' ORDER BY created_at ASC LIMIT 100")
	if err != nil {
		return fmt.Errorf("push read queue: %w", err)
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0, 100)
	ids := make([]string, 0, 100)
	for rows.Next() {
		var opID, tbl, recID, op, payload string
		if err := rows.Scan(&opID, &tbl, &recID, &op, &payload); err != nil {
			continue
		}
		var m map[string]interface{}
		_ = json.Unmarshal([]byte(payload), &m)
		items = append(items, map[string]interface{}{
			"operation_id": opID, "table_name": tbl, "record_id": recID,
			"operation": op, "payload": m,
		})
		ids = append(ids, opID)
	}
	rows.Close()

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
		return fmt.Errorf("hub push failed: HTTP %d", resp.StatusCode)
	}

	// Tandai terkirim (idempoten, hanya baris yang benar-benar dikirim).
	for _, opID := range ids {
		e.db.Exec(`UPDATE sync_queue SET status='acknowledged', ack_received_at=NOW(3) WHERE operation_id=?`, opID)
	}
	e.logSync("push_ack", map[string]interface{}{"count": len(items)})
	return nil
}

// Pull fetches changes from hub per table using the stored cursor.
func (e *Engine) Pull(ctx context.Context) error {
	tables := []string{"arsip", "pemberkasan", "users", "roles"}
	var cursor struct {
		Table, Value string
	}
	for _, tbl := range tables {
		e.db.QueryRow(`SELECT table_name, IFNULL(cursor_value,'') FROM sync_cursors WHERE node_id='local' AND table_name=?`, tbl).
			Scan(&cursor.Table, &cursor.Value)

		req, _ := http.NewRequestWithContext(ctx, "GET", e.hubURL+"/api/v1/sync/pull?table="+tbl+"&cursor="+cursor.Value, nil)
		req.Header.Set("Authorization", "Bearer "+e.hubToken)
		resp, err := e.client.Do(req)
		if err != nil {
			return fmt.Errorf("pull request %s: %w", tbl, err)
		}
		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			continue // tidak ada perubahan
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return fmt.Errorf("hub pull %s: HTTP %d", tbl, resp.StatusCode)
		}

		var batch struct {
			Changes []struct {
				Sequence  int64                  `json:"sequence"`
				RecordID  string                 `json:"record_id"`
				Operation string                 `json:"operation"`
				Payload   map[string]interface{} `json:"payload"`
			} `json:"changes"`
		}
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&batch); err != nil {
			resp.Body.Close()
			return fmt.Errorf("pull decode %s: %w", tbl, err)
		}
		resp.Body.Close()

		for _, c := range batch.Changes {
			raw, _ := json.Marshal(c.Payload)
			e.db.Exec(`UPDATE sync_inbox SET applied_at=NOW(3), payload=? WHERE change_log_id=(SELECT id FROM sync_cursors WHERE node_id='local' AND table_name=?) AND record_id=UNHEX(REPLACE(?,'-','')) AND applied_at IS NULL`,
				string(raw), tbl, c.RecordID)
		}
		if len(batch.Changes) > 0 {
			e.db.Exec(`INSERT INTO sync_cursors(id,node_id,table_name,cursor_value) VALUES(UUID_TO_BIN(UUID()),'local',?,?) 
				ON DUPLICATE KEY UPDATE cursor_value=VALUES(cursor_value)`,
				tbl, fmt.Sprintf("%d", batch.Changes[len(batch.Changes)-1].Sequence))
		}
	}
	e.logSync("pull_complete", nil)
	return nil
}

func (e *Engine) RetryFailed(ctx context.Context) error {
	_, _ = e.db.Exec(`UPDATE sync_queue SET status='pending' WHERE status='failed' AND retry_count < ?`, MaxRetries)
	return e.Push(ctx)
}

// Enqueue menyimpan operasi lokal ke sync_queue (dipanggil oleh handler bisnis).
func (e *Engine) Enqueue(tableName, operation string, recordID [16]byte, payload map[string]interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = e.db.Exec(`INSERT INTO sync_queue(operation_id, table_name, record_id, operation, payload) VALUES(REPLACE(UUID(),'-',''), ?, UNHEX(REPLACE(?,'-','')), ?, ?)`,
		tableName, BINToHex(recordID), operation, string(raw))
	return err
}

func BINToHex(v [16]byte) string {
	s := ""
	for _, b := range v {
		s += fmt.Sprintf("%02x", b)
	}
	return s
}

func (e *Engine) LogSync(event string, detail map[string]interface{}) {
	e.logSync(event, detail)
}

func (e *Engine) logSync(event string, detail map[string]interface{}) {
	detailStr, _ := json.Marshal(detail)
	e.db.Exec(`INSERT INTO sync_logs(event, detail) VALUES(?, JSON_QUOTE(?))`, event, string(detailStr))
}

func (e *Engine) ResetOnRestart() error {
	// Konfirmasi: restart tidak boleh kehilangan antrean pending.
	_, err := e.db.Exec("UPDATE sync_queue SET status='pending' WHERE status='failed'")
	return err
}

func retryBackoff(n int) time.Duration {
	d := InitialBackoff << uint(n)
	if d > MaxBackoff {
		d = MaxBackoff
	}
	return d
}
