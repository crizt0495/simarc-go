package app

import (
	"bytes"
	"testing"

	"arsippro/internal/handlers"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
)

func TestViewerTemplatesRender(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	buildTemplates(r)

	for _, name := range []string{"viewer/index.html", "viewer/detail.html"} {
		if _, ok := handlers.TemplateSets[name]; !ok {
			t.Fatalf("template %s not registered", name)
		}
	}

	list := []models.Arsip{{
		ID:          "abc-123",
		NomorArsip:  "001/ARS/2026",
		NamaArsip:   "Contoh Arsip",
		StatusArsip: "aktif",
		Uraian:      "Uraian contoh",
	}}

	data := gin.H{
		"title":                  "Direktori Arsip",
		"AppName":                "SIMARC",
		"Year":                   2026,
		"CSRFToken":              "tok",
		"AssetVersion":           "v1",
		"arsipList":              list,
		"unitKerjaOptions":       []models.UnitKerja{},
		"kodeKlasifikasiOptions": []models.KodeKlasifikasi{},
		"jenisArsipOptions":      []models.JenisArsip{},
		"totalArsip":             1, "aktifArsip": 1, "inaktifArsip": 0,
		"CurrentPage": 1, "TotalPages": 1, "PageNumbers": []int{1},
		"QueryString": "", "PerPage": 12, "StartIndex": 1,
		"searchKeyword": "", "TotalResults": 1,
		"HasFilters": false,
	}
	var buf bytes.Buffer
	if err := handlers.TemplateSets["viewer/index.html"].ExecuteTemplate(&buf, "viewer/index.html", data); err != nil {
		t.Fatalf("viewer/index.html execute: %v", err)
	}

	a := models.Arsip{ID: "abc-123", NamaArsip: "Contoh", NomorArsip: "001", StatusArsip: "aktif", Jumlah: 1, Satuan: "Berkas"}
	ddata := gin.H{"title": "Contoh", "arsip": a, "AppName": "SIMARC", "Year": 2026, "CSRFToken": "tok", "AssetVersion": "v1"}
	var dbuf bytes.Buffer
	if err := handlers.TemplateSets["viewer/detail.html"].ExecuteTemplate(&dbuf, "viewer/detail.html", ddata); err != nil {
		t.Fatalf("viewer/detail.html execute: %v", err)
	}
	t.Log("viewer templates OK")
}
