package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/google/uuid"
)

// ── Google Sheets integration ────────────────────────────────────────────────
//
// Reads a public (anyone-with-the-link) Google Sheet through the CSV export
// endpoint and imports the rows into the `arsip` table. No OAuth/API key is
// required as long as the sheet is link-shared; ApiKey on the integration is
// optional and unused by this provider.

var (
	sheetIDPattern = regexp.MustCompile(`spreadsheets?:?/*d/([a-zA-Z0-9-_]{20,})`)
	gidPattern     = regexp.MustCompile(`gid=([0-9]+)`)
)

// ExtractSheetID accepts either a full Google Sheets URL or a raw spreadsheet
// ID and returns the ID (empty string when nothing matches).
func ExtractSheetID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := sheetIDPattern.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	// Raw ID: 33-ish chars of [a-zA-Z0-9-_].
	if len(s) >= 25 && !strings.ContainsAny(s, "/?# ") {
		return s
	}
	return ""
}

// ExtractGid pulls the gid (tab index/id) from a pasted URL. Defaults to 0.
func ExtractGid(s string) int64 {
	if m := gidPattern.FindStringSubmatch(s); len(m) > 1 {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return v
		}
	}
	return 0
}

// FetchSheetCSV downloads the CSV export of the given tab.
func FetchSheetCSV(ctx context.Context, client *http.Client, sheetID string, gid int64) ([]byte, error) {
	if sheetID == "" {
		return nil, fmt.Errorf("spreadsheet ID kosong")
	}
	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%d", sheetID, gid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // cap at 10 MB
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d — pastikan sheet dibagikan 'Siapa saja yang memiliki link dapat melihat'", resp.StatusCode)
	}
	return body, nil
}

// ParseCSV returns the header row and all data rows.
func ParseCSV(data []byte) (headers []string, rows [][]string, err error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	all, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("sheet kosong")
	}
	headers = make([]string, len(all[0]))
	for i, h := range all[0] {
		headers[i] = normalizeHeader(h)
	}
	rows = all[1:]
	return headers, rows, nil
}

func normalizeHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.ReplaceAll(h, " ", "_")
	return h
}

// firstOf returns the value for the first matching header alias.
func colValue(headers []string, row []string, aliases ...string) string {
	for _, a := range aliases {
		for i, h := range headers {
			if h == a && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
	}
	return ""
}

// SheetSyncResult summarises one sync run.
type SheetSyncResult struct {
	TotalRows int
	Created   int
	Updated   int
	Skipped   int
}

// SyncGoogleSheetToArsip fetches the sheet linked to the integration and
// upserts its rows into the arsip table.
//
// Recognised headers (case/space insensitive):
//
//	nomor_arsip | nama_arsip | uraian | jenis_arsip | kode_klasifikasi |
//	unit_kerja | tanggal | jumlah | satuan | status
//
// Rows are matched by nomor_arsip: existing numbers are updated, new ones
// inserted. Rows without nama_arsip are skipped.
func SyncGoogleSheetToArsip(ctx context.Context, m *models.Integration) (*SheetSyncResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	gid := ExtractGid(m.Config + " " + m.BaseURL)
	data, err := FetchSheetCSV(ctx, client, ExtractSheetID(m.BaseURL), gid)
	if err != nil {
		return nil, err
	}
	headers, rows, err := ParseCSV(data)
	if err != nil {
		return nil, err
	}

	res := &SheetSyncResult{TotalRows: len(rows)}
	now := time.Now()
	for _, row := range rows {
		nama := colValue(headers, row, "nama_arsip", "nama")
		if nama == "" {
			res.Skipped++
			continue
		}
		nomor := colValue(headers, row, "nomor_arsip", "nomor", "no_arsip")

		var m2 models.Arsip
		found := false
		if nomor != "" {
			if err := database.DB.Where("nomor_arsip = ?", nomor).First(&m2).Error; err == nil {
				found = true
			}
		}
		if !found {
			m2 = models.Arsip{
				ID:          uuid.New().String(),
				Jumlah:      1,
				Satuan:      "Berkas",
				StatusArsip: "aktif",
			}
		}

		m2.NomorArsip = nomor
		m2.NamaArsip = truncate(nama, 500)
		m2.Uraian = colValue(headers, row, "uraian", "keterangan", "deskripsi")
		m2.JenisArsip = truncate(colValue(headers, row, "jenis_arsip", "jenis"), 50)

		// Kode klasifikasi: find by code, create when absent.
		if kk := colValue(headers, row, "kode_klasifikasi", "kode"); kk != "" {
			var k models.KodeKlasifikasi
			if err := database.DB.Where("kode_klasifikasi = ?", kk).First(&k).Error; err != nil {
				k = models.KodeKlasifikasi{
					ID:              uuid.New().String(),
					KodeKlasifikasi: truncate(kk, 50),
					NamaKlasifikasi: truncate(kk, 255),
					IsActive:        true,
				}
				database.DB.Create(&k)
			}
			m2.KodeKlasifikasiID = k.ID
		} else if m2.KodeKlasifikasiID == "" {
			// Required column — fall back to an "IMPORT-SHEET" bucket code.
			var k models.KodeKlasifikasi
			if err := database.DB.Where("kode_klasifikasi = ?", "IMPORT-SHEET").First(&k).Error; err != nil {
				k = models.KodeKlasifikasi{
					ID:              uuid.New().String(),
					KodeKlasifikasi: "IMPORT-SHEET",
					NamaKlasifikasi: "Import Google Sheet",
					IsActive:        true,
				}
				database.DB.Create(&k)
			}
			m2.KodeKlasifikasiID = k.ID
		}

		// Unit kerja: find/create by name; default bucket otherwise.
		unit := colValue(headers, row, "unit_kerja", "unit")
		if unit == "" {
			unit = "Import Google Sheet"
		}
		var u models.UnitKerja
		if err := database.DB.Where("nama_unit = ?", unit).First(&u).Error; err != nil {
			u = models.UnitKerja{ID: uuid.New().String(), NamaUnit: truncate(unit, 255)}
			database.DB.Create(&u)
		}
		m2.UnitKerjaID = u.ID

		// Optional numeric fields.
		if j := colValue(headers, row, "jumlah"); j != "" {
			if v, err := strconv.Atoi(j); err == nil && v > 0 {
				m2.Jumlah = v
			}
		}
		if s := colValue(headers, row, "satuan"); s != "" {
			m2.Satuan = truncate(s, 30)
		}
		if st := colValue(headers, row, "status"); st != "" {
			m2.StatusArsip = truncate(st, 50)
		}
		if t := parseFlexibleDate(colValue(headers, row, "tanggal", "tanggal_dibuat", "tanggal_dokumen")); t != nil {
			m2.TanggalDibuat = t
		}

		if found {
			database.DB.Save(&m2)
			res.Updated++
		} else {
			database.DB.Create(&m2)
			res.Created++
		}
	}
	_ = now
	return res, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// parseFlexibleDate understands the common sheet date layouts.
func parseFlexibleDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	layouts := []string{
		"2006-01-02", "02/01/2006", "01/02/2006", "2/1/2006", "1/2/2006",
		"02-01-2006", "2006/01/02", "02 Jan 2006", "2 January 2006",
		"01/02/06", "2006-01-02T15:04:05Z07:00",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}
