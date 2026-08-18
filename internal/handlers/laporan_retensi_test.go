package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/google/uuid"
)

// TestLaporanRetensi_PageRenders verifies the Laporan Retensi page renders
// without template errors. Regression test: the report table previously
// accessed fields that do not exist on models.Arsip (NamaKlasifikasi,
// LamaSimpan, MasaRetensi, StatusRetensi), which caused an HTTP 500.
//
// A test arsip past its retention date is seeded so the table row (the
// previously broken cells) is actually executed — with an empty list Go
// templates skip the {{range}} body and the regression would go unnoticed.
func TestLaporanRetensi_PageRenders(t *testing.T) {
	// Register the laporan/laporan-retensi.html template set using the same
	// loading strategy as main.go / initTemplates.
	layoutFiles, _ := filepath.Glob("web/templates/layouts/*.html")
	compFiles, _ := filepath.Glob("web/templates/components/*.html")
	sharedFiles := append(layoutFiles, compFiles...)
	allFiles := append([]string{}, sharedFiles...)
	allFiles = append(allFiles, "web/templates/laporan/laporan-retensi.html")
	ts := template.Must(template.New("").Funcs(TemplateFuncs()).ParseFiles(allFiles...))
	for _, tt := range ts.Templates() {
		n := tt.Name()
		if !strings.Contains(n, "/") || strings.HasPrefix(n, "layouts/") || strings.HasPrefix(n, "components/") {
			continue
		}
		TemplateSets[n] = ts
	}

	// Seed an arsip past its retention end date so the report row renders.
	testArsip := models.Arsip{
		ID:                  uuid.New().String(),
		NomorArsip:          "[TEST] RET-001",
		NamaArsip:           "[TEST] Dokumen Retensi",
		KodeKlasifikasiID:   testKK.ID,
		UnitKerjaID:         testUnitKerja.ID,
		StatusArsip:         "aktif",
		TanggalRetensiAkhir: &[]time.Time{time.Now().AddDate(0, 0, -30)}[0],
		Jumlah:              1,
		Satuan:              "Berkas",
	}
	if err := database.DB.Create(&testArsip).Error; err != nil {
		t.Fatalf("Failed to seed test arsip: %v", err)
	}
	defer database.DB.Where("id = ?", testArsip.ID).Delete(&models.Arsip{})

	h := &LaporanHandler{}
	c, w := newTestContext("GET", "/laporan/retensi", nil)
	h.Retensi(c)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, "Template error") {
		t.Fatalf("Page rendered with template error: %s", body)
	}
	if !strings.Contains(body, "Laporan Retensi") {
		t.Error("Page should contain the 'Laporan Retensi' heading")
	}
	// The report row must render the seeded arsip (exercises the fixed cells).
	if !strings.Contains(body, testArsip.NamaArsip) {
		t.Error("Report table should render the seeded arsip name (row template not exercised?)")
	}
	if !strings.Contains(body, testKK.KodeKlasifikasi) {
		t.Error("Report table should render the kode klasifikasi of the seeded arsip")
	}
}
