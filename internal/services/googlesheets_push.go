package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"golang.org/x/oauth2/google"
)

// ── Google Sheets push (database → spreadsheet) ─────────────────────────────
//
// Exports the arsip table to a Google Sheet using a Google Cloud SERVICE
// ACCOUNT (server-side, no browser OAuth). Configuration resolution order:
//
//	Credentials JSON : env GOOGLE_SHEETS_CREDENTIALS_JSON → integration.api_key
//	Spreadsheet ID   : env GOOGLE_SHEETS_SPREADSHEET_ID  → integration.base_url
//
// The service-account email must be granted "Editor" access on the target
// spreadsheet (Share → add the @*.iam.gserviceaccount.com address).
//
// GOOGLE_SHEETS_SYNC_ENABLED=true|1 gates automatic/cron usage; manual pushes
// from the UI are always allowed when credentials resolve.

const sheetsScope = "https://www.googleapis.com/auth/spreadsheets"

// SheetPushResult summarises one push run.
type SheetPushResult struct {
	SpreadsheetID string
	SheetTitle    string
	Rows          int // data rows written (excluding header)
}

// SheetsPushEnabled reports whether automatic sheet sync is turned on.
func SheetsPushEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOOGLE_SHEETS_SYNC_ENABLED")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ResolveSheetsConfig returns (credentialsJSON, spreadsheetID) for an
// integration, preferring environment variables over stored fields.
func ResolveSheetsConfig(m *models.Integration) (string, string, error) {
	creds := strings.TrimSpace(os.Getenv("GOOGLE_SHEETS_CREDENTIALS_JSON"))
	if creds == "" {
		creds = strings.TrimSpace(m.ApiKey)
	}
	if creds == "" {
		return "", "", fmt.Errorf("kredensial service account tidak ditemukan — set env GOOGLE_SHEETS_CREDENTIALS_JSON atau isi kolom API Key integrasi")
	}
	ssID := strings.TrimSpace(os.Getenv("GOOGLE_SHEETS_SPREADSHEET_ID"))
	if ssID == "" {
		ssID = ExtractSheetID(m.BaseURL)
	}
	if ssID == "" {
		return "", "", fmt.Errorf("spreadsheet ID tidak ditemukan — set env GOOGLE_SHEETS_SPREADSHEET_ID atau isi URL sheet pada kolom Base URL")
	}
	return creds, ssID, nil
}

// normalizeServiceAccountJSON accepts the credential payload either as raw
// JSON or as base64 (Vercel env vars often mangle multiline JSON, so storing
// base64 is common). Returns compact JSON ready for JWTConfigFromJSON.
func normalizeServiceAccountJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("kredensial kosong")
	}
	if strings.HasPrefix(trimmed, "{") {
		var probe map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
			return "", fmt.Errorf("JSON kredensial tidak valid: %w", err)
		}
		return trimmed, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && strings.HasPrefix(strings.TrimSpace(string(decoded)), "{") {
		var probe map[string]interface{}
		if err := json.Unmarshal(decoded, &probe); err != nil {
			return "", fmt.Errorf("JSON kredensial (hasil decode base64) tidak valid: %w", err)
		}
		return string(decoded), nil
	}
	return "", fmt.Errorf("format kredensial tidak dikenali — isi GOOGLE_SHEETS_CREDENTIALS_JSON dengan JSON service account lengkap (mulai dengan '{') atau versi base64-nya")
}

// sheetsClient builds an authenticated HTTP client from service account JSON.
func sheetsClient(ctx context.Context, credsJSON string) (*http.Client, error) {
	normalized, err := normalizeServiceAccountJSON(credsJSON)
	if err != nil {
		return nil, err
	}
	conf, err := google.JWTConfigFromJSON([]byte(normalized), sheetsScope)
	if err != nil {
		return nil, fmt.Errorf("JSON kredensial tidak valid: %w", err)
	}
	return conf.Client(ctx), nil
}

// firstSheetTitle fetches the title of the spreadsheet's first visible tab so
// writes always land on it regardless of naming.
func firstSheetTitle(ctx context.Context, client *http.Client, ssID string) (string, error) {
	url := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s?fields=sheets(properties(title))", ssID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gagal membuka spreadsheet (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}
	var parsed struct {
		Sheets []struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Sheets) == 0 {
		return "", fmt.Errorf("spreadsheet tidak memiliki tab")
	}
	return parsed.Sheets[0].Properties.Title, nil
}

type arsipSheetRow struct {
	NomorArsip  string
	NamaArsip   string
	Uraian      string
	JenisArsip  string
	Kode        string
	Unit        string
	Tanggal     *time.Time
	Jumlah      int64
	Satuan      string
	StatusArsip string
}

// loadArsipRows reads all non-deleted archives with their classification and
// unit names resolved.
func loadArsipRows() ([]arsipSheetRow, error) {
	var rows []arsipSheetRow
	err := database.DB.Table("arsip").
		Select(`arsip.nomor_arsip, arsip.nama_arsip, COALESCE(arsip.uraian,'') AS uraian,
			COALESCE(arsip.jenis_arsip,'') AS jenis_arsip,
			COALESCE(kode_klasifikasi.kode_klasifikasi,'') AS kode,
			COALESCE(unit_kerja.nama_unit,'') AS unit,
			arsip.tanggal_dibuat, arsip.jumlah, COALESCE(arsip.satuan,'') AS satuan,
			COALESCE(arsip.status_arsip,'') AS status_arsip`).
		Joins("LEFT JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id AND kode_klasifikasi.deleted_at IS NULL").
		Joins("LEFT JOIN unit_kerja ON unit_kerja.id = arsip.unit_kerja_id AND unit_kerja.deleted_at IS NULL").
		Where("arsip.deleted_at IS NULL").
		Order("arsip.created_at ASC").
		Find(&rows).Error
	return rows, err
}

func (r arsipSheetRow) toCells() []interface{} {
	tanggal := ""
	if r.Tanggal != nil {
		tanggal = r.Tanggal.Format("2006-01-02")
	}
	jumlah := r.Jumlah
	if jumlah < 1 {
		jumlah = 1
	}
	return []interface{}{
		r.NomorArsip, r.NamaArsip, r.Uraian, r.JenisArsip,
		r.Kode, r.Unit, tanggal, jumlah, r.Satuan, r.StatusArsip,
	}
}

var sheetHeader = []interface{}{
	"nomor_arsip", "nama_arsip", "uraian", "jenis_arsip",
	"kode_klasifikasi", "unit_kerja", "tanggal", "jumlah", "satuan", "status",
}

// writeValues writes a block of rows starting at A1 (first call) or appends.
func writeValues(ctx context.Context, client *http.Client, ssID, sheetTitle string, values [][]interface{}, appendMode bool) error {
	if len(values) == 0 {
		return nil
	}
	var (
		url    string
		method string
	)
	payload := map[string]interface{}{"values": values}
	if appendMode {
		method = http.MethodPost
		url = fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s!A1:M1:append?valueInputOption=RAW&insertDataOption=OVERWRITE", ssID, sheetTitle)
	} else {
		method = http.MethodPut
		url = fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s!A1?valueInputOption=RAW", ssID, sheetTitle)
	}
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("penulisan gagal (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// clearSheet empties every cell of the given tab.
func clearSheet(ctx context.Context, client *http.Client, ssID, sheetTitle string) error {
	url := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s!A1:ZZ1000000:clear", ssID, sheetTitle)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("pembersihan sheet gagal (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// PushArsipToSheet exports the full arsip table to the configured spreadsheet.
// The target tab is cleared first, then the header and all rows are written in
// chunks. Existing content of the tab is REPLACED — that is the point of a
// mirror export; nothing in the database is modified.
func PushArsipToSheet(ctx context.Context, m *models.Integration) (*SheetPushResult, error) {
	credsJSON, ssID, err := ResolveSheetsConfig(m)
	if err != nil {
		return nil, err
	}
	client, err := sheetsClient(ctx, credsJSON)
	if err != nil {
		return nil, err
	}
	sheetTitle, err := firstSheetTitle(ctx, client, ssID)
	if err != nil {
		return nil, err
	}

	rows, err := loadArsipRows()
	if err != nil {
		return nil, fmt.Errorf("gagal membaca data arsip: %w", err)
	}

	if err := clearSheet(ctx, client, ssID, sheetTitle); err != nil {
		return nil, err
	}

	values := make([][]interface{}, 0, len(rows)+1)
	values = append(values, sheetHeader)
	for _, r := range rows {
		values = append(values, r.toCells())
	}

	const chunk = 1000
	for start := 0; start < len(values); start += chunk {
		end := start + chunk
		if end > len(values) {
			end = len(values)
		}
		if err := writeValues(ctx, client, ssID, sheetTitle, values[start:end], start > 0); err != nil {
			return nil, err
		}
	}

	return &SheetPushResult{SpreadsheetID: ssID, SheetTitle: sheetTitle, Rows: len(rows)}, nil
}
