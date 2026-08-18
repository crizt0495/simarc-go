package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"arsippro/internal/database"
	"arsippro/internal/models"
)

// ── CREATE handler tests ──────────────────────────────────────────────────────

func TestArsipCreate_WithoutLocation_ShowsEmptyLocationSelect(t *testing.T) {
	h := &ArsipHandler{}
	c, w := newTestContext("GET", "/arsip/create", nil)

	h.Create(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}

	body := w.Body.String()
	// Should contain the location <select> element
	if !strings.Contains(body, `<select name="lokasi_arsip_id"`) {
		t.Error("Should show location dropdown when not in location mode")
	}
	// Should contain Batal button (not Selesai)
	if !strings.Contains(body, "Batal") {
		t.Error("Should show Batal button when not in location mode")
	}
	// Should NOT contain hidden from_location field
	if strings.Contains(body, `name="from_location"`) {
		t.Error("Should NOT include hidden from_location field when not in location mode")
	}
}

func TestArsipCreate_WithLocation_ShowsLocationBadge(t *testing.T) {
	h := &ArsipHandler{}
	path := "/arsip/create?lokasi_arsip_id=" + testLokasi.ID
	c, w := newTestContext("GET", path, nil)

	h.Create(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	// Should show the location name
	if !strings.Contains(body, testLokasi.NamaLokasi) {
		t.Errorf("Should show location name %q when in location mode", testLokasi.NamaLokasi)
	}
	// Should contain hidden from_location field
	if !strings.Contains(body, `name="from_location"`) {
		t.Error("Should include hidden from_location field")
	}
	if !strings.Contains(body, `value="`+testLokasi.ID+`"`) {
		t.Error("from_location should have the location ID as value")
	}
	// Should contain Selesai button
	if !strings.Contains(body, "Selesai") {
		t.Error("Should show Selesai button when in location mode")
	}
	// Should contain Simpan & Tambah Lagi
	if !strings.Contains(body, "Tambah Lagi") {
		t.Error("Should show 'Simpan & Tambah Lagi' button when in location mode")
	}
	// Should NOT contain the location <select> dropdown
	if strings.Contains(body, `<select name="lokasi_arsip_id"`) {
		t.Error("Should NOT show location dropdown in location mode")
	}
}

func TestArsipCreate_WithUnknownLocation_StillShowsLocationMode(t *testing.T) {
	h := &ArsipHandler{}
	path := "/arsip/create?lokasi_arsip_id=nonexistent-id"
	c, w := newTestContext("GET", path, nil)

	h.Create(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `name="from_location"`) {
		t.Error("Should include hidden from_location field even with unknown lokasi_arsip_id")
	}
}

// ── STORE handler tests ───────────────────────────────────────────────────────

func TestArsipStore_WithFromLocation_OnSuccess_RedirectsToCreateForm(t *testing.T) {
	engine := newTestEngine()
	form := url.Values{
		"nama_arsip":          {"[TEST] Dokumen Dari Lokasi"},
		"nomor_arsip":         {"002/TEST/2024"},
		"kode_klasifikasi_id": {testKK.ID},
		"unit_kerja_id":       {testUnitKerja.ID},
		"lokasi_arsip_id":     {testLokasi.ID},
		"from_location":       {testLokasi.ID},
		"jumlah":              {"1"},
		"satuan":              {"Berkas"},
		"status_arsip":        {"aktif"},
	}
	w := performRequest(engine, "POST", "/arsip", form)

	if w.Code != http.StatusFound {
		t.Errorf("Expected redirect 302, got %d. Body: %s", w.Code, w.Body.String())
	}

	location := w.Header().Get("Location")
	expected := "/arsip/create?lokasi_arsip_id=" + testLokasi.ID
	if location != expected {
		t.Errorf("Expected redirect to %q, got %q", expected, location)
	}

	// Cleanup test arsip
	database.DB.Where("nomor_arsip = ?", "002/TEST/2024").Delete(&models.Arsip{})
}

func TestArsipStore_WithoutFromLocation_OnSuccess_RedirectsToArsipList(t *testing.T) {
	engine := newTestEngine()
	form := url.Values{
		"nama_arsip":          {"[TEST] Dokumen Biasa"},
		"nomor_arsip":         {"003/TEST/2024"},
		"kode_klasifikasi_id": {testKK.ID},
		"unit_kerja_id":       {testUnitKerja.ID},
		"lokasi_arsip_id":     {testLokasi.ID},
		"jumlah":              {"1"},
		"satuan":              {"Berkas"},
		"status_arsip":        {"aktif"},
	}
	w := performRequest(engine, "POST", "/arsip", form)

	if w.Code != http.StatusFound {
		t.Errorf("Expected redirect 302, got %d. Body: %s", w.Code, w.Body.String())
	}

	location := w.Header().Get("Location")
	if location != "/arsip" {
		t.Errorf("Expected redirect to /arsip, got %q", location)
	}

	// Cleanup
	database.DB.Where("nomor_arsip = ?", "003/TEST/2024").Delete(&models.Arsip{})
}

func TestArsipStore_WithFromLocation_ValidationError_PreservesLocation(t *testing.T) {
	engine := newTestEngine()
	// Missing required fields - should fail validation
	form := url.Values{
		"from_location": {testLokasi.ID},
	}
	w := performRequest(engine, "POST", "/arsip", form)

	if w.Code != http.StatusFound {
		t.Errorf("Expected redirect 302, got %d. Body: %s", w.Code, w.Body.String())
	}

	location := w.Header().Get("Location")
	expected := "/arsip/create?lokasi_arsip_id=" + testLokasi.ID
	if location != expected {
		t.Errorf("Expected redirect to %q (preserving location), got %q", expected, location)
	}
}

func TestArsipStore_WithoutFromLocation_ValidationError_DoesNotAppendLocation(t *testing.T) {
	engine := newTestEngine()
	// Missing required fields and no from_location
	form := url.Values{}
	w := performRequest(engine, "POST", "/arsip", form)

	if w.Code != http.StatusFound {
		t.Errorf("Expected redirect 302, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/arsip/create" {
		t.Errorf("Expected redirect to /arsip/create, got %q", location)
	}
}
