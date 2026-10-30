package syncdb

// boundary.go — penanganan baris "boundary" (timestamp == watermark).
//
// Latar masalah: delta dulu dibaca dengan `ts >= (watermark - overlap)`, jadi
// baris yang timestamp-nya sama persis dengan watermark (khas data seed/bulk:
// ribuan baris ber-stempel identik) dibaca ulang dan dikirim ulang ke dua arah
// pada SETIAP run, selamanya — watermark tidak pernah maju karena maxSeen
// memang tetap di nilai tersebut.
//
// Perbaikan dua sisi:
//  1. Delta utama kini strict Greater-Than (`ts > watermark`). Baris tepat ada
//     di watermark tidak dibaca oleh danain utama.
//  2. Set baris boundary (ts == watermark) dibaca terpisah dan hanya
//     dikalakan (didorong/ditarik) bila HASH KONTEN barisnya berbeda dari
//     yang terakhir dikalakan — memo disimpan di sync_boundary (lokal saja).
//     Konten tak berubah → 0 baris perpindahan → watermark diam pada nilai
//     itu, dan penambahan data baru ber-stempel lebih baru tetap terbaca
//     normal oleh delta utama.
//
// Hash dihitung dari nilai yang discan dengan kanikalisasi sama untuk MySQL
// ([]byte/time.Time) dan SQLite (string), sehingga perbandingan antar run
// stabil dalam satu node. Memo berupa (table, key_hash, direction) → row_hash.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// readBoundary membaca baris yang change-timestamp-nya TEPAT sama dengan
// watermark. Untuk tabel soft-delete, baris yang deleted_at-nya tepat di
// watermark juga diikutkan (tombstone boundary).
func readBoundary(ctx context.Context, db *sql.DB, ti *tableInfo, wm time.Time) ([][]any, error) {
	rows := [][]any{}
	if wm.IsZero() {
		return rows, nil
	}
	tsCol := ti.timestampColumn()
	if tsCol == "" {
		// Tanpa kolom waktu: delta membaca seluruh tabel (tidak ada boundary).
		return rows, nil
	}
	cols := quotedColumns(ti)
	q := "SELECT " + cols + " FROM `" + ti.Name + "`"
	coalesce, _ := deltaExpr(ti)
	var args []any
	if ti.HasDeletedAt {
		q += " WHERE (" + coalesce + ") = ? OR (`deleted_at` IS NOT NULL AND `deleted_at` = ?)"
		args = []any{wm, wm}
	} else {
		q += " WHERE (" + coalesce + ") = ?"
		args = []any{wm}
	}
	got, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer got.Close()
	colNames, err := got.Columns()
	if err != nil {
		return nil, err
	}
	for got.Next() {
		vals := make([]any, len(colNames))
		ptrs := make([]any, len(colNames))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := got.Scan(ptrs...); err != nil {
			return nil, err
		}
		rows = append(rows, vals)
	}
	return rows, got.Err()
}

// fingerprintCell kanikalisasi satu nilai hasil scan agar hash stabil antar
// run dan antar dialek (MySQL mengembalikan []byte/time.Time, SQLite string).
func fingerprintCell(v any) string {
	switch x := v.(type) {
	case nil:
		return "\x00"
	case time.Time:
		return x.Format("2006-01-02 15:04:05.999999999")
	case []byte:
		return string(x)
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return "0"
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	default:
		return fmt.Sprint(x)
	}
}

// canonicalRow mengubah seluruh nilai baris menjadi format teks stabil untuk
// hash dan untuk penulis kunci.
func canonicalRow(vals []any) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = fingerprintCell(v)
	}
	return out
}

// fingerprintRow menghash konten baris (tanpa kolom sumber: hash dibangun dari
// nilai seluruh kolom sehingga berubah saat konten berubah).
func fingerprintRow(vals []any) string {
	h := sha256.New()
	for _, c := range canonicalRow(vals) {
		h.Write([]byte(c))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// boundaryKey returns the key hash (identik dengan pola tombstone) untuk baris.
func boundaryKey(ti *tableInfo, vals []any) string {
	idx := primaryKeyIndexes(ti)
	pvals := make([]string, 0, len(idx))
	for _, i := range idx {
		if i >= 0 && i < len(vals) {
			pvals = append(pvals, fingerprintCell(vals[i]))
		}
	}
	return hashKey(ti.Name, pvals)
}

// primaryKeyIndexes mengembalikan posisi kolom PK di dalam ti.Columns.
func primaryKeyIndexes(ti *tableInfo) []int {
	idx := make([]int, 0, len(ti.PK))
	for _, pk := range ti.PK {
		for i, c := range ti.Columns {
			if c == pk {
				idx = append(idx, i)
				break
			}
		}
	}
	return idx
}

// loadBoundaryMemos memuat seluruh memo boundary dari sync_boundary ke memori:
// hasil peta [kolom] -> [arah] -> peta [key_hash]row_hash].
func loadBoundaryMemos(ctx context.Context, db *sql.DB) (map[string]map[string]map[string]string, error) {
	out := map[string]map[string]map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT table_name, direction, key_hash, row_hash FROM sync_boundary`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var table, dir, kh, rh string
		if err := rows.Scan(&table, &dir, &kh, &rh); err != nil {
			return out, err
		}
		if out[table] == nil {
			out[table] = map[string]map[string]string{}
		}
		if out[table][dir] == nil {
			out[table][dir] = map[string]string{}
		}
		out[table][dir][kh] = rh
	}
	return out, rows.Err()
}

// boundaryMemo is the in-memory pending write buffer per run.
type boundaryPending struct {
	table string
	dir   string
	kh    string
	rh    string
}

// flushBoundaryMemos menyimpan seluruh memo baru/terbarui ke sync_boundary
// (lokal saja) dalam batch. Kegagalannya non-fatal: kala berikutnya baris
// boundary yang belum di-memo hanya baru dikalakan sekali lagi (lazim).
func flushBoundaryMemos(ctx context.Context, db *sql.DB, driver string, pending []boundaryPending, logf func(string, ...any)) error {
	if len(pending) == 0 {
		return nil
	}
	const batch = 400
	for i := 0; i < len(pending); i += batch {
		end := i + batch
		if end > len(pending) {
			end = len(pending)
		}
		chunk := pending[i:end]
		var sb string
		var args []any
		if driver == driverSQLite {
			sb = `INSERT INTO sync_boundary (table_name, key_hash, direction, row_hash) VALUES `
			for j := range chunk {
				if j > 0 {
					sb += ","
				}
				sb += "(?,?,?,?)"
				args = append(args, chunk[j].table, chunk[j].kh, chunk[j].dir, chunk[j].rh)
			}
			sb += ` ON CONFLICT (table_name, key_hash, direction) DO UPDATE SET row_hash=excluded.row_hash`
		} else {
			sb = `INSERT INTO sync_boundary (table_name, key_hash, direction, row_hash) VALUES `
			for j := range chunk {
				if j > 0 {
					sb += ","
				}
				sb += "(?,?,?,?)"
				args = append(args, chunk[j].table, chunk[j].kh, chunk[j].dir, chunk[j].rh)
			}
			sb += ` ON DUPLICATE KEY UPDATE row_hash=VALUES(row_hash)`
		}
		if _, err := db.ExecContext(ctx, sb, args...); err != nil {
			return err
		}
	}
	return nil
}

// boundaryBatch adalah kumpulan baris boundary yang perlu dikalakan beserta
// kunci memo-nya untuk dicatat setelah upsert berhasil.
type boundaryBatch struct {
	rows [][]any
	pend []boundaryPending
}

// prepareBoundary membaca baris yang TEPAT berada pada watermark dari sumber
// (lokal untuk push, hub untuk pull) lalu menyaringnya dengan memo: hanya
// baris yang kontennya berbeda (atau belum pernah di-memo) yang harus dikirim.
// Watermark nol (run pertama) menghasilkan wilayah kosong — run pertama tetap
// membaca seluruh tabel lewat delta utama.
func (e *Engine) prepareBoundary(ctx context.Context, db *sql.DB, ti *tableInfo, dir string, wm time.Time) (boundaryBatch, error) {
	var out boundaryBatch
	if wm.IsZero() {
		return out, nil
	}
	rows, err := readBoundary(ctx, db, ti, wm)
	if err != nil {
		return out, err
	}
	for _, vals := range rows {
		kh := boundaryKey(ti, vals)
		rh := fingerprintRow(vals)
		if e.bMemo[ti.Name][dir][kh] == rh {
			continue // konten sudah dikenal; tidak perlu dikirim ulang
		}
		out.rows = append(out.rows, vals)
		out.pend = append(out.pend, boundaryPending{table: ti.Name, dir: dir, kh: kh, rh: rh})
	}
	return out, nil
}

// markBoundary mencatat memo setelah upsert berhasil. Entri dicatat PER
// ARAH saja menggunakan hash dari scan sumber arah itu (lokal untuk push,
// hub untuk pull) — kanikalisasi antar dialek tidak selalu identik, dan
// waktu memo antar-arah justru membuat baris bermasalah konflik-kanikalisasi
// bergoyang tanpa henti tiap run. Setiap arah membandingkan scan dari
// sumber yang sama sehingga match-nya stabil. Memo diperbarui in-memory agar
// pembacaan berikutnya dalam run yang sama konsisten.
func (e *Engine) markBoundary(table string, pend []boundaryPending) {
	for _, p := range pend {
		dir := p.dir
		e.bPend = append(e.bPend, boundaryPending{table: table, dir: dir, kh: p.kh, rh: p.rh})
		if e.bMemo[table] == nil {
			e.bMemo[table] = map[string]map[string]string{}
		}
		if e.bMemo[table][dir] == nil {
			e.bMemo[table][dir] = map[string]string{}
		}
		e.bMemo[table][dir][p.kh] = p.rh
	}
}

// filterAppendRows menyaring baris delta tabel append yang tidak punya kolom
// waktu: readDelta membaca seluruh tabel setiap run dan wm tidak pernah maju,
// sehingga dulu seluruh log dikirim ulang dua arah setiap run. Baris append
// tidak pernah berubah, jadi memo konten cukup aman untuk melewatkan baris
// yang sudah pernah dikirim. Hasilnya baris yang perlu dikirim + memo barunya.
func (e *Engine) filterAppendRows(ti *tableInfo, dir string, rows [][]any) ([][]any, []boundaryPending) {
	kept := make([][]any, 0, len(rows))
	var pend []boundaryPending
	for _, vals := range rows {
		kh := boundaryKey(ti, vals)
		rh := fingerprintRow(vals)
		if e.bMemo[ti.Name][dir][kh] == rh {
			continue // sudah pernah dikirim; log append tidak berubah
		}
		kept = append(kept, vals)
		pend = append(pend, boundaryPending{table: ti.Name, dir: dir, kh: kh, rh: rh})
	}
	return kept, pend
}

// filterNullStamps menyaring baris delta yang tidak membawa waktu sama sekali
// (NULL di updated_at DAN created_at) — dulu klausa delta membuat baris ini
// SELALU dikirim pada setiap run ("always travel"), karena tak ada cara lama
// untuk mendeteksi perubahannya. Hash konten kini menyediakan cara itu: baris
// hanya dikirim bila memo belum mengenal isinya; konten berubah → hash
// berubah → tetap terkirim. Tabel tanpa kolom waktu sama sekali ditangani
// filterAppendRows oleh penelepon.
func (e *Engine) filterNullStamps(ti *tableInfo, dir string, rows [][]any) ([][]any, []boundaryPending) {
	tsName := ti.timestampColumn()
	if tsName == "" {
		return rows, nil
	}
	tsIdx, createdIdx := -1, -1
	for i, c := range ti.Columns {
		if c == tsName && tsIdx < 0 {
			tsIdx = i
		}
		if c == "created_at" && createdIdx < 0 {
			createdIdx = i
		}
	}
	kept := make([][]any, 0, len(rows))
	var pend []boundaryPending
	dropped := 0
	for _, vals := range rows {
		ok := false
		if tsIdx >= 0 && tsIdx < len(vals) {
			_, ok = asTime(vals[tsIdx])
		}
		if !ok && createdIdx >= 0 && createdIdx < len(vals) {
			_, ok = asTime(vals[createdIdx])
		}
		if ok { // baris berwaktu: jalur delta normal
			kept = append(kept, vals)
			continue
		}
		// Baris null-stamp: kirim hanya bila kontennya belum dikenal.
		kh := boundaryKey(ti, vals)
		rh := fingerprintRow(vals)
		if e.bMemo[ti.Name][dir][kh] == rh {
			dropped++
			continue
		}
		kept = append(kept, vals)
		pend = append(pend, boundaryPending{table: ti.Name, dir: dir, kh: kh, rh: rh})
	}
	if dropped > 0 {
		e.logf("[sync] %s: %d baris tanpa timestamp sudah dikenal — tidak dikirim ulang", ti.Name, dropped)
	}
	return kept, pend
}
