package handlers

import (
	"net/http"
	"strings"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
)

// CmdkResult is a single "live data" hit for the command palette omnibox
// (Ctrl+K). Static navigation commands live in the front-end; this endpoint
// only returns real records so the user can jump straight to a document,
// classification code, unit, or storage location.
type CmdkResult struct {
	Type     string `json:"type"`     // arsip | kode-klasifikasi | unit-kerja | lokasi
	ID       string `json:"id"`       // record id (may be empty for code-based rows)
	Title    string `json:"title"`    // primary label
	Subtitle string `json:"subtitle"` // secondary label (code / unit / etc.)
	URL      string `json:"url"`      // destination link
}

// CmdkSearch powers live database search in the command palette. It searches
// the main archival datasets and returns a small ranked list of direct links.
// Kept deliberately cheap (indexed LIKE prefixes + tight LIMITs) so it can be
// called on every keystroke.
func CmdkSearch(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len([]rune(q)) < 2 {
		c.JSON(http.StatusOK, gin.H{"data": []CmdkResult{}})
		return
	}

	const perSection = 5
	results := make([]CmdkResult, 0, perSection*2)

	// 1) Arsip — the primary dataset.
	cond, sargs := arsipSearchLike(q, "nama_arsip", "nomor_arsip")
	var arsip []models.Arsip
	database.DB.Model(&models.Arsip{}).
		Select("id, nama_arsip, nomor_arsip").
		Where(cond, sargs...).
		Order("created_at DESC").
		Limit(perSection).Find(&arsip)
	for _, a := range arsip {
		sub := a.NomorArsip
		if strings.TrimSpace(sub) == "" {
			sub = "Arsip"
		}
		results = append(results, CmdkResult{
			Type: "arsip", ID: a.ID, Title: a.NamaArsip, Subtitle: sub,
			URL: "/arsip/" + a.ID,
		})
	}

	// 2) Kode klasifikasi.
	like := "%" + q + "%"
	var kk []models.KodeKlasifikasi
	database.DB.Model(&models.KodeKlasifikasi{}).
		Select("id, kode_klasifikasi, nama_klasifikasi").
		Where("COALESCE(kode_klasifikasi,'') LIKE ? OR COALESCE(nama_klasifikasi,'') LIKE ?", like, like).
		Order("kode_klasifikasi ASC").
		Limit(perSection).Find(&kk)
	for _, k := range kk {
		results = append(results, CmdkResult{
			Type: "kode-klasifikasi", ID: k.ID, Title: k.NamaKlasifikasi, Subtitle: k.KodeKlasifikasi,
			URL: "/kode-klasifikasi/" + k.ID,
		})
	}

	// 3) Unit kerja.
	var units []models.UnitKerja
	database.DB.Model(&models.UnitKerja{}).
		Select("id, nama_unit").
		Where("COALESCE(nama_unit,'') LIKE ?", like).
		Order("nama_unit ASC").
		Limit(perSection).Find(&units)
	for _, u := range units {
		results = append(results, CmdkResult{
			Type: "unit-kerja", ID: u.ID, Title: u.NamaUnit, Subtitle: "Unit Kerja",
			URL: "/unit-kerja/" + u.ID,
		})
	}

	// 4) Lokasi arsip.
	var lokasi []models.LokasiArsip
	database.DB.Model(&models.LokasiArsip{}).
		Select("id, nama_lokasi").
		Where("COALESCE(nama_lokasi,'') LIKE ?", like).
		Order("nama_lokasi ASC").
		Limit(perSection).Find(&lokasi)
	for _, l := range lokasi {
		results = append(results, CmdkResult{
			Type: "lokasi", ID: l.ID, Title: l.NamaLokasi, Subtitle: "Lokasi Arsip",
			URL: "/lokasi-arsip/" + l.ID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": results, "count": len(results)})
}
