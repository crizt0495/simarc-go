package handlers

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"arsippro/internal/cache"
	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"
	"arsippro/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ── SMART DISPOSAL ──────────────────────────────────────────────────────────

type DisposalHandler struct{}

func (h *DisposalHandler) Index(c *gin.Context) {
	var klasifikasiList []models.KodeKlasifikasi
	var total int64
	db := database.DB.Model(&models.KodeKlasifikasi{}).Where("is_active = 1")
	db.Count(&total)
	perPage := 20
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (int(total) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	db.Preload("Children").Order("kode_klasifikasi").Limit(perPage).Offset(offset).Find(&klasifikasiList)
	type ClassStat struct {
		KodeKlasifikasiID string `gorm:"column:kk_id"`
		Kode              string `gorm:"column:kode"`
		Nama              string `gorm:"column:nama"`
		TotalEligible     int64  `gorm:"column:total"`
	}
	var stats []ClassStat
	database.DB.Raw(`SELECT kk.id as kk_id, kk.kode_klasifikasi as kode, kk.nama_klasifikasi as nama,
		COUNT(a.id) as total FROM kode_klasifikasi kk
		LEFT JOIN arsip a ON a.kode_klasifikasi_id = kk.id AND a.deleted_at IS NULL
		AND a.status_arsip NOT IN ('musnah','siap_penyusutan','permanen')
		AND LOWER(TRIM(kk.penyusutan_arsip)) = 'musnah'
		AND kk.is_active = 1
		AND (kk.retensi_aktif + kk.retensi_inaktif) > 0
		AND a.tanggal_dibuat IS NOT NULL
		AND DATE_ADD(a.tanggal_dibuat, INTERVAL (kk.retensi_aktif + kk.retensi_inaktif) YEAR) < CURDATE()
		WHERE kk.is_active = 1 GROUP BY kk.id, kk.kode_klasifikasi, kk.nama_klasifikasi
		HAVING total > 0 ORDER BY total DESC`).Scan(&stats)
	Render(c, 200, "disposal/index.html", gin.H{
		"title": "Smart Disposal", "pageTitle": "Smart Disposal - Berdasarkan Klasifikasi",
		"klasifikasiList": klasifikasiList, "stats": stats,
		"Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(klasifikasiList),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}

func (h *DisposalHandler) ShowByClassification(c *gin.Context) {
	var kk models.KodeKlasifikasi
	if err := database.DB.First(&kk, "id = ?", c.Param("kodeKlasifikasi")).Error; err != nil {
		c.Redirect(http.StatusFound, "/disposal")
		return
	}
	var arsipList []models.Arsip
	var total int64
	db := database.DB.Model(&models.Arsip{}).
		Where("arsip.status_arsip NOT IN ('musnah','siap_penyusutan','permanen')").
		Where("DATE_ADD(arsip.tanggal_dibuat, INTERVAL (kode_klasifikasi.retensi_aktif + kode_klasifikasi.retensi_inaktif) YEAR) < CURDATE()").
		Where("arsip.tanggal_dibuat IS NOT NULL").
		Where("kode_klasifikasi.id = ?", kk.ID)
	db.Count(&total)
	perPage := 20
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (int(total) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	db.Preload("UnitKerja").Preload("LokasiArsip").
		Order("tanggal_retensi_berakhir ASC").Limit(perPage).Offset(offset).Find(&arsipList)
	Render(c, 200, "disposal/index.html", gin.H{
		"title": "Disposal - " + kk.KodeKlasifikasi, "pageTitle": "Disposal: " + kk.NamaKlasifikasi,
		"ArsipList": arsipList, "kk": kk,
		"Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(arsipList),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}

func (h *DisposalHandler) ListArchivesForDisposal(c *gin.Context) {
	var arsipList []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Joins("INNER JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id").
		Where("arsip.status_arsip NOT IN ('musnah','siap_penyusutan','permanen')").
		Where("DATE_ADD(arsip.tanggal_dibuat, INTERVAL (kode_klasifikasi.retensi_aktif + kode_klasifikasi.retensi_inaktif) YEAR) < CURDATE()").
		Where("arsip.tanggal_dibuat IS NOT NULL").
		Where("kode_klasifikasi.id = ?", c.Param("kodeKlasifikasi")).
		Order("arsip.tanggal_dibuat ASC").Find(&arsipList)
	c.JSON(http.StatusOK, gin.H{"data": arsipList, "count": len(arsipList)})
}

func (h *DisposalHandler) CreateSchedule(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	arsipIDs := c.PostFormArray("arsip_ids[]")
	action := c.PostForm("action")
	if action == "" {
		action = "musnah"
	}
	scheduledDate := time.Now().AddDate(0, 1, 0)
	if v := c.PostForm("scheduled_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			scheduledDate = t
		}
	}
	count := 0
	for _, aid := range arsipIDs {
		var arsip models.Arsip
		if database.DB.First(&arsip, "id = ?", aid).Error == nil {
			sched := models.DisposalSchedule{
				ID: uuid.New().String(), KodeKlasifikasiID: arsip.KodeKlasifikasiID,
				ArsipID: aid, ScheduledDate: scheduledDate, Action: action,
				Status: "pending", CreatedBy: &user.ID,
			}
			database.DB.Create(&sched)
			count++
		}
	}
	middleware.SetFlash(c, "success", fmt.Sprintf("%d jadwal disposal berhasil dibuat.", count))
	c.Redirect(http.StatusFound, "/disposal/schedules")
}

func (h *DisposalHandler) Schedules(c *gin.Context) {
	var schedules []models.DisposalSchedule
	var total int64
	db := database.DB.Model(&models.DisposalSchedule{})
	if v := c.Query("status"); v != "" {
		db = db.Where("status = ?", v)
	}
	db.Count(&total)
	perPage := 20
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (int(total) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	db.Preload("KodeKlasifikasi").Preload("Arsip").Preload("Arsip.KodeKlasifikasi").Preload("Arsip.UnitKerja").
		Order("scheduled_date ASC").Limit(perPage).Offset(offset).Find(&schedules)
	Render(c, 200, "disposal/index.html", gin.H{
		"title": "Jadwal Disposal", "pageTitle": "Jadwal Disposal", "schedules": schedules, "scheduleMode": true,
		"Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(schedules),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}

func (h *DisposalHandler) ShowSchedule(c *gin.Context) {
	var sched models.DisposalSchedule
	if err := database.DB.Preload("KodeKlasifikasi").Preload("Arsip").Preload("Arsip.KodeKlasifikasi").Preload("Arsip.UnitKerja").
		First(&sched, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/disposal/schedules")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sched})
}

func (h *DisposalHandler) AutoCreateSchedules(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	var eligible []models.Arsip
	database.DB.
		Where("status_arsip NOT IN ('musnah','siap_penyusutan','permanen')").
		Where("kode_klasifikasi_id IS NOT NULL").
		Where("tanggal_dibuat IS NOT NULL").
		Where("DATE_ADD(tanggal_dibuat, INTERVAL (kode_klasifikasi.retensi_aktif + kode_klasifikasi.retensi_inaktif) YEAR) < CURDATE()").Find(&eligible)
	count := 0
	for _, a := range eligible {
		var existing models.DisposalSchedule
		if database.DB.Where("arsip_id = ? AND status = 'pending'", a.ID).First(&existing).Error != nil {
			sched := models.DisposalSchedule{
				ID: uuid.New().String(), KodeKlasifikasiID: a.KodeKlasifikasiID,
				ArsipID: a.ID, ScheduledDate: time.Now().AddDate(0, 1, 0),
				Action: "musnah", Status: "pending", CreatedBy: &user.ID,
			}
			database.DB.Create(&sched)
			count++
		}
	}
	middleware.SetFlash(c, "success", fmt.Sprintf("%d jadwal disposal otomatis berhasil dibuat.", count))
	c.Redirect(http.StatusFound, "/disposal/schedules")
}

func (h *DisposalHandler) PreviewRecommendations(c *gin.Context) {
	svc := &services.SmartDisposalService{}
	eligible := svc.GetEligibleArsip()
	c.JSON(http.StatusOK, gin.H{"data": eligible, "count": len(eligible)})
}

// ── ADVANCED DASHBOARD ──────────────────────────────────────────────────────

type AdvancedDashboardHandler struct{}

func (h *AdvancedDashboardHandler) Index(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	var stats struct {
		Total          int64 `gorm:"column:total"`
		Aktif          int64 `gorm:"column:aktif"`
		TotalItems     int64 `gorm:"column:total_items"`
		Inaktif        int64 `gorm:"column:inaktif"`
		SiapPenyusutan int64 `gorm:"column:siap_penyusutan"`
		Musnah         int64 `gorm:"column:musnah"`
		Digital        int64 `gorm:"column:digital"`
	}
	database.DB.Raw(`SELECT
		SUM(jumlah) as total_items,
		COUNT(*) as total,
		SUM(CASE WHEN status_arsip='aktif' OR status_arsip='diberkaskan' THEN 1 ELSE 0 END) as aktif,
		SUM(CASE WHEN status_arsip='inaktif' THEN 1 ELSE 0 END) as inaktif,
		SUM(CASE WHEN status_arsip='siap_penyusutan' THEN 1 ELSE 0 END) as siap_penyusutan,
		SUM(CASE WHEN status_arsip='musnah' THEN 1 ELSE 0 END) as musnah,
		SUM(CASE WHEN file_path IS NOT NULL AND file_path != '' THEN 1 ELSE 0 END) as digital
		FROM arsip WHERE deleted_at IS NULL`).Scan(&stats)

	var widgets []models.DashboardWidget
	database.DB.Where("user_id = ?", user.ID).Order("position ASC").Find(&widgets)

	// Recent archives for activity table
	type RecentArchive struct {
		NamaArsip   string `gorm:"column:nama_arsip"`
		UnitKerja   string `gorm:"column:unit_kerja"`
		CreatedAt   string `gorm:"column:created_at"`
		StatusArsip string `gorm:"column:status_arsip"`
	}
	var recentArchives []RecentArchive
	database.DB.Raw(`
		SELECT a.nama_arsip, COALESCE(uk.nama_unit,'-') as unit_kerja,
		       TO_CHAR(a.created_at, 'DD Mon YYYY') as created_at, a.status_arsip
		FROM arsip a
		LEFT JOIN unit_kerja uk ON a.unit_kerja_id = uk.id
		WHERE a.deleted_at IS NULL
		ORDER BY a.created_at DESC LIMIT 10
	`).Scan(&recentArchives)

	// Monthly growth chart
	type MonthlyStat struct {
		Month string `gorm:"column:month"`
		Total int64  `gorm:"column:total"`
	}
	var monthlyStats []MonthlyStat
	database.DB.Raw(`SELECT TO_CHAR(created_at, 'YYYY-MM') as month, COUNT(*) as total
		FROM arsip WHERE deleted_at IS NULL GROUP BY month ORDER BY month ASC LIMIT 12`).Scan(&monthlyStats)

	var growthLabels []string
	var growthData []int64
	for _, m := range monthlyStats {
		growthLabels = append(growthLabels, m.Month)
		growthData = append(growthData, m.Total)
	}

	// Top 5 classification distribution
	type DistRow struct {
		Kode  string `gorm:"column:kode"`
		Total int64  `gorm:"column:total"`
	}
	var distRows []DistRow
	database.DB.Raw(`
		SELECT COALESCE(kk.kode_klasifikasi,'TANPA KLASIFIKASI') as kode, COUNT(a.id) as total
		FROM arsip a
		LEFT JOIN kode_klasifikasi kk ON a.kode_klasifikasi_id = kk.id
		WHERE a.deleted_at IS NULL
		GROUP BY kode ORDER BY total DESC LIMIT 5
	`).Scan(&distRows)

	var distLabels []string
	var distData []int64
	for _, d := range distRows {
		distLabels = append(distLabels, d.Kode)
		distData = append(distData, d.Total)
	}

	// Document types (jenis arsip)
	type DocTypeRow struct {
		Jenis string `gorm:"column:jenis"`
		Total int64  `gorm:"column:total"`
	}
	var docTypeRows []DocTypeRow
	database.DB.Raw(`
		SELECT COALESCE(NULLIF(a.jenis_arsip,''),'Lainnya') as jenis, COUNT(a.id) as total
		FROM arsip a WHERE a.deleted_at IS NULL
		GROUP BY jenis ORDER BY total DESC LIMIT 5
	`).Scan(&docTypeRows)

	var docTypeLabels []string
	var docTypeData []int64
	for _, d := range docTypeRows {
		docTypeLabels = append(docTypeLabels, d.Jenis)
		docTypeData = append(docTypeData, d.Total)
	}

	// Retention lifecycle (aktif, inaktif, siap_penyusutan)
	retentionLabels := []string{"Aktif", "Inaktif", "Siap Musnah"}
	retentionData := []int64{stats.Aktif, stats.Inaktif, stats.SiapPenyusutan}

	// AkurasiSistem - health score based on digital ratio + status distribution
	akurasiSistem := 100.0
	if stats.Total > 0 {
		unclassified := stats.Total - stats.Aktif - stats.Inaktif - stats.SiapPenyusutan - stats.Musnah
		if unclassified < 0 {
			unclassified = 0
		}
		knownRatio := float64(stats.Total-unclassified) / float64(stats.Total)
		digitalRatio := float64(stats.Digital) / float64(stats.Total)
		akurasiSistem = (knownRatio*0.6 + digitalRatio*0.4) * 100
	}

	Render(c, 200, "dashboard/advanced.html", gin.H{
		"title": "Advanced Dashboard", "pageTitle": "Advanced Dashboard",
		"Stats": gin.H{
			"TotalDokumen":   stats.Total,
			"DokumenAktif":   stats.Aktif,
			"DokumenInaktif": stats.Inaktif,
			"SiapMusnah":     stats.SiapPenyusutan,
			"AkurasiSistem":  int64(akurasiSistem),
		},
		"widgets": widgets,
		"RecentActivity": gin.H{
			"RecentArchives": recentArchives,
		},
		"ChartsData": gin.H{
			"Growth": gin.H{
				"Data":   growthData,
				"Labels": growthLabels,
			},
			"Distribution": gin.H{
				"Data":   distData,
				"Labels": distLabels,
			},
			"DocTypes": gin.H{
				"Data":   docTypeData,
				"Labels": docTypeLabels,
			},
			"Retention": gin.H{
				"Data":   retentionData,
				"Labels": retentionLabels,
			},
		},
	})
}

func (h *AdvancedDashboardHandler) GetWidgetData(c *gin.Context) {
	key := c.Param("widgetKey")
	data := map[string]interface{}{}
	switch key {
	case "arsip_growth":
		svc := &services.AnalyticsService{}
		data["growth"] = svc.GetArsipGrowth(12)
	case "forecast":
		svc := &services.DataScienceService{}
		data["forecast"] = svc.ForecastGrowth(6)
	case "anomalies":
		svc := &services.DataScienceService{}
		data["anomalies"] = svc.DetectSpjAnomalies()
	case "leaderboard":
		svc := &services.ArchivalSupervisionService{}
		data["leaderboard"] = svc.GetLeaderboard()
	default:
		data["message"] = "Widget not found"
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *AdvancedDashboardHandler) SaveWidgetConfig(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	var body []struct {
		WidgetKey string `json:"widget_key"`
		Position  int    `json:"position"`
		IsVisible bool   `json:"is_visible"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	database.DB.Where("user_id = ?", user.ID).Delete(&models.DashboardWidget{})
	for _, w := range body {
		widget := models.DashboardWidget{
			ID: uuid.New().String(), UserID: user.ID, WidgetKey: w.WidgetKey,
			Position: w.Position, IsVisible: w.IsVisible,
		}
		database.DB.Create(&widget)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── INTEGRATION HUB ─────────────────────────────────────────────────────────

type IntegrationHandler struct{}

func (h *IntegrationHandler) Index(c *gin.Context) {
	var list []models.Integration
	var totalInt int64
	db := database.DB.Model(&models.Integration{})
	db.Count(&totalInt)
	perPage := 15
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (int(totalInt) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	db.Order("name").Limit(perPage).Offset(offset).Find(&list)

	// External spreadsheet links for google_sheets integrations.
	sheetLinks := make(map[string]string, len(list))
	for i := range list {
		if list[i].Type == "google_sheets" {
			if u := services.SheetsWebURL(&list[i]); u != "" {
				sheetLinks[list[i].ID] = u
			}
		}
	}

	var total, active, inactive, errorCount int64
	database.DB.Model(&models.Integration{}).Count(&total)
	database.DB.Model(&models.Integration{}).Where("is_active = 1").Count(&active)
	database.DB.Model(&models.Integration{}).Where("is_active = 0").Count(&inactive)
	database.DB.Model(&models.Integration{}).Where("last_status = 'error'").Count(&errorCount)
	Render(c, 200, "integrations/index.html", gin.H{
		"title": "Integrasi", "pageTitle": "Integration Hub", "List": list,
		"SheetLinks": sheetLinks,
		"Stats": gin.H{
			"Total":    total,
			"Active":   active,
			"Inactive": inactive,
			"Error":    errorCount,
		},
		"Total": totalInt, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(list),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}

func (h *IntegrationHandler) Create(c *gin.Context) {
	Render(c, 200, "integrations/index.html", gin.H{"title": "Tambah Integrasi", "pageTitle": "Tambah Integrasi", "createForm": true})
}

func (h *IntegrationHandler) Store(c *gin.Context) {
	m := models.Integration{
		ID: uuid.New().String(), Name: c.PostForm("name"), Type: c.PostForm("type"),
		BaseURL: c.PostForm("base_url"), ApiKey: c.PostForm("api_key"),
		IsActive: c.PostForm("is_active") == "on",
	}
	if m.Type == "google_sheets" {
		m.BaseURL = services.ExtractSheetID(m.BaseURL)
		gid := services.ExtractGid(c.PostForm("base_url"))
		if cfg, err := json.Marshal(map[string]int64{"gid": gid}); err == nil {
			m.Config = string(cfg)
		}
	}
	if err := database.DB.Create(&m).Error; err != nil {
		log.Printf("[INTEGRASI] Gagal menyimpan integrasi: %v", err)
		middleware.SetFlash(c, "error", "Gagal menyimpan integrasi: "+err.Error())
		c.Redirect(http.StatusFound, "/advanced/integrations")
		return
	}
	middleware.SetFlash(c, "success", "Integrasi berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/advanced/integrations")
}

func (h *IntegrationHandler) Show(c *gin.Context) {
	var m models.Integration
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/advanced/integrations")
		return
	}
	var logs []models.IntegrationLog
	database.DB.Where("integration_id = ?", m.ID).Order("created_at DESC").Limit(20).Find(&logs)
	c.JSON(http.StatusOK, gin.H{"data": m, "logs": logs})
}

func (h *IntegrationHandler) Edit(c *gin.Context) {
	var m models.Integration
	database.DB.First(&m, "id = ?", c.Param("id"))
	Render(c, 200, "integrations/index.html", gin.H{"title": "Edit Integrasi", "pageTitle": "Edit Integrasi", "m": m, "editForm": true})
}

func (h *IntegrationHandler) Update(c *gin.Context) {
	var m models.Integration
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Integrasi tidak ditemukan.")
		c.Redirect(http.StatusFound, "/advanced/integrations")
		return
	}
	m.Name = c.PostForm("name")
	m.Type = c.PostForm("type")
	m.BaseURL = c.PostForm("base_url")
	m.ApiKey = c.PostForm("api_key")
	m.IsActive = c.PostForm("is_active") == "on"
	if m.Type == "google_sheets" {
		gid := services.ExtractGid(c.PostForm("base_url"))
		if old := services.ExtractGid(m.Config); gid == 0 && old != 0 {
			gid = old
		}
		m.BaseURL = services.ExtractSheetID(m.BaseURL)
		if cfg, err := json.Marshal(map[string]int64{"gid": gid}); err == nil {
			m.Config = string(cfg)
		}
	}
	database.DB.Save(&m)
	middleware.SetFlash(c, "success", "Integrasi berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/advanced/integrations")
}

func (h *IntegrationHandler) Destroy(c *gin.Context) {
	database.DB.Delete(&models.Integration{}, "id = ?", c.Param("id"))
	database.DB.Where("integration_id = ?", c.Param("id")).Delete(&models.IntegrationLog{})
	middleware.SetFlash(c, "success", "Integrasi berhasil dihapus.")
	c.Redirect(http.StatusFound, "/advanced/integrations")
}

func (h *IntegrationHandler) Test(c *gin.Context) {
	var m models.Integration
	database.DB.First(&m, "id = ?", c.Param("id"))

	start := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 35*time.Second)
	defer cancel()

	status, statusCode, detail := "success", http.StatusOK, ""
	if m.Type != "google_sheets" {
		// Generic integrations: only check that a URL is configured.
		if m.BaseURL == "" {
			status, statusCode, detail = "error", 400, "URL belum diisi"
		}
	} else if res, err := testGoogleSheet(ctx, &m); err != nil {
		status, statusCode, detail = "error", 502, err.Error()
	} else {
		detail = fmt.Sprintf("%d baris, %d kolom", res["rows"], res["cols"])
	}

	durationMs := int(time.Since(start).Milliseconds())
	logEntry := models.IntegrationLog{
		IntegrationID: m.ID, Action: "test", Status: status, StatusCode: statusCode,
		DurationMs: durationMs,
	}
	if len(detail) > 500 {
		detail = detail[:500]
	}
	logEntry.ResponseBody = detail
	database.DB.Create(&logEntry)

	c.JSON(http.StatusOK, gin.H{
		"success":     status == "success",
		"status_code": statusCode,
		"detail":      detail,
	})
}

// testGoogleSheet validates connectivity and parses the sheet structure.
func testGoogleSheet(ctx context.Context, m *models.Integration) (map[string]int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	gid := services.ExtractGid(m.Config)
	data, err := services.FetchSheetCSV(ctx, client, services.ExtractSheetID(m.BaseURL), gid)
	if err != nil {
		return nil, err
	}
	headers, rows, err := services.ParseCSV(data)
	if err != nil {
		return nil, err
	}
	return map[string]int{"rows": len(rows), "cols": len(headers)}, nil
}

func (h *IntegrationHandler) Sync(c *gin.Context) {
	var m models.Integration
	database.DB.First(&m, "id = ?", c.Param("id"))
	now := time.Now()

	start := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	statusCode := http.StatusOK
	var detail string
	if m.Type == "google_sheets" {
		res, err := services.SyncGoogleSheetToArsip(ctx, &m)
		if err != nil {
			statusCode = 502
			detail = err.Error()
			logEntry := models.IntegrationLog{
				IntegrationID: m.ID, Action: "sync", Status: "error",
				StatusCode: statusCode, DurationMs: int(time.Since(start).Milliseconds()),
				ResponseBody: truncateString(detail, 500),
			}
			database.DB.Create(&logEntry)
			database.DB.Model(&m).Updates(map[string]interface{}{
				"last_sync_at": now, "last_status": "error",
			})
			middleware.SetFlash(c, "error", "Sinkronisasi gagal: "+detail)
			c.Redirect(http.StatusFound, "/advanced/integrations")
			return
		}
		detail = fmt.Sprintf("Total %d baris — dibuat %d, diperbarui %d, dilewati %d",
			res.TotalRows, res.Created, res.Updated, res.Skipped)
		middleware.SetFlash(c, "success", "Sinkronisasi berhasil. "+detail)
	} else {
		middleware.SetFlash(c, "success", "Sinkronisasi berhasil.")
	}

	logEntry := models.IntegrationLog{
		IntegrationID: m.ID, Action: "sync", Status: "success", StatusCode: statusCode,
		DurationMs: int(time.Since(start).Milliseconds()), ResponseBody: truncateString(detail, 500),
	}
	database.DB.Create(&logEntry)
	database.DB.Model(&m).Updates(map[string]interface{}{"last_sync_at": now, "last_status": "synced"})
	c.Redirect(http.StatusFound, "/advanced/integrations")
}

// PushToSheet exports the database arsip table INTO the configured Google
// Sheet (database → spreadsheet), using the service-account credentials from
// env/integration. The target tab is replaced with current data.
func (h *IntegrationHandler) PushToSheet(c *gin.Context) {
	var m models.Integration
	database.DB.First(&m, "id = ?", c.Param("id"))
	now := time.Now()
	start := time.Now()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 55*time.Second)
	defer cancel()

	statusCode := http.StatusOK
	if m.Type != "google_sheets" {
		h.logIntegration(m.ID, "push", "error", http.StatusBadRequest, start, "Integrasi bukan google_sheets")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Push hanya tersedia untuk integrasi google_sheets"})
		return
	}

	res, err := services.PushArsipToSheet(ctx, &m)
	detail := ""
	if err != nil {
		statusCode = http.StatusBadGateway
		detail = err.Error()
		h.logIntegration(m.ID, "push", "error", statusCode, start, detail)
		database.DB.Model(&m).Updates(map[string]interface{}{"last_sync_at": now, "last_status": "push_error"})
		c.JSON(statusCode, gin.H{"success": false, "error": detail})
		return
	}
	detail = fmt.Sprintf("%d arsip dikirim ke tab \"%s\" (%s)", res.Rows, res.SheetTitle, res.SpreadsheetID)
	h.logIntegration(m.ID, "push", "success", statusCode, start, detail)
	database.DB.Model(&m).Updates(map[string]interface{}{"last_sync_at": now, "last_status": "pushed"})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Database berhasil dikirim ke Google Sheet. " + detail,
	})
}

// logIntegration records one integration event and swallows logging errors.
func (h *IntegrationHandler) logIntegration(id, action, status string, code int, start time.Time, body string) {
	database.DB.Create(&models.IntegrationLog{
		IntegrationID: id, Action: action, Status: status,
		StatusCode: code, DurationMs: int(time.Since(start).Milliseconds()),
		ResponseBody: truncateString(body, 500),
	})
}

// ViewSheet renders a read-only preview of the connected Google Sheet tab.
func (h *IntegrationHandler) ViewSheet(c *gin.Context) {
	var m models.Integration
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Integrasi tidak ditemukan.")
		c.Redirect(http.StatusFound, "/advanced/integrations")
		return
	}
	if m.Type != "google_sheets" {
		middleware.SetFlash(c, "error", "Pratinjau sheet hanya tersedia untuk integrasi google_sheets.")
		c.Redirect(http.StatusFound, "/advanced/integrations")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	tbl, err := services.FetchSheetTable(ctx, &m)
	if err != nil {
		Render(c, http.StatusBadGateway, "integrations/view.html", gin.H{
			"title": "Pratinjau Sheet", "pageTitle": "Pratinjau Google Sheet", "Sheet": nil, "Err": err.Error(), "Integration": m,
		})
		return
	}
	Render(c, 200, "integrations/view.html", gin.H{
		"title": "Pratinjau Sheet", "pageTitle": "Pratinjau Google Sheet",
		"Sheet": tbl, "Err": "", "Integration": m,
	})
}

func truncateString(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (h *IntegrationHandler) ShowLog(c *gin.Context) {
	var log models.IntegrationLog
	database.DB.First(&log, "id = ?", c.Param("logId"))
	c.JSON(http.StatusOK, gin.H{"data": log})
}

func (h *IntegrationHandler) Status(c *gin.Context) {
	var m models.Integration
	database.DB.First(&m, "id = ?", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"is_active": m.IsActive, "last_status": m.LastStatus, "last_sync_at": m.LastSyncAt})
}

// ── IMPORT/EXPORT ────────────────────────────────────────────────────────────

type ImportExportHandler struct{}

func (h *ImportExportHandler) Index(c *gin.Context) {
	var jobs []models.ImportExportJob
	var total int64
	db := database.DB.Model(&models.ImportExportJob{})
	db.Count(&total)
	perPage := 20
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (int(total) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	db.Order("created_at DESC").Limit(perPage).Offset(offset).Find(&jobs)
	var totalJobs, pending, processing, completed, failed int64
	database.DB.Model(&models.ImportExportJob{}).Count(&totalJobs)
	database.DB.Model(&models.ImportExportJob{}).Where("status = 'pending'").Count(&pending)
	database.DB.Model(&models.ImportExportJob{}).Where("status = 'processing'").Count(&processing)
	database.DB.Model(&models.ImportExportJob{}).Where("status = 'completed'").Count(&completed)
	database.DB.Model(&models.ImportExportJob{}).Where("status = 'failed'").Count(&failed)
	Render(c, 200, "import-export/index.html", gin.H{
		"title": "Import/Export", "pageTitle": "Import/Export Data", "jobs": jobs,
		"Stats": gin.H{
			"TotalJobs":  totalJobs,
			"Pending":    pending,
			"Processing": processing,
			"Completed":  completed,
			"Failed":     failed,
		},
		"Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(jobs),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}

func (h *ImportExportHandler) ShowImportForm(c *gin.Context) {
	Render(c, 200, "import-export/index.html", gin.H{"title": "Import Data", "pageTitle": "Import Data", "importForm": true})
}

func (h *ImportExportHandler) ProcessImport(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	file, err := c.FormFile("file")
	if err != nil {
		middleware.SetFlash(c, "error", "File wajib diunggah.")
		c.Redirect(http.StatusFound, "/advanced/import-export/import")
		return
	}
	entityType := c.PostForm("entity_type")
	uploadDir := filepath.Join(config.StorageDir(), "imports")
	os.MkdirAll(uploadDir, 0755)
	dst := filepath.Join(uploadDir, fmt.Sprintf("import_%d%s", time.Now().Unix(), filepath.Ext(file.Filename)))
	c.SaveUploadedFile(file, dst)
	job := models.ImportExportJob{
		ID: uuid.New().String(), UserID: user.ID, Type: "import",
		EntityType: entityType, Status: "processing", InputFile: dst,
	}
	database.DB.Create(&job)
	middleware.SetFlash(c, "success", "Import sedang diproses.")
	c.Redirect(http.StatusFound, "/advanced/import-export")
}

func (h *ImportExportHandler) ShowExportForm(c *gin.Context) {
	Render(c, 200, "import-export/index.html", gin.H{"title": "Export Data", "pageTitle": "Export Data", "exportForm": true})
}

func (h *ImportExportHandler) ProcessExport(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	entityType := c.PostForm("entity_type")
	format := c.PostForm("format")
	if format == "" {
		format = "csv"
	}
	exportDir := filepath.Join(config.StorageDir(), "exports")
	os.MkdirAll(exportDir, 0755)
	filename := fmt.Sprintf("export_%s_%d.%s", entityType, time.Now().Unix(), format)
	path := filepath.Join(exportDir, filename)
	job := models.ImportExportJob{
		ID: uuid.New().String(), UserID: user.ID, Type: "export",
		EntityType: entityType, Status: "processing", OutputFile: path,
	}
	database.DB.Create(&job)
	go func() {
		f, err := os.Create(path)
		if err != nil {
			database.DB.Model(&job).Updates(map[string]interface{}{"status": "failed"})
			return
		}
		defer f.Close()
		w := csv.NewWriter(f)
		switch entityType {
		case "arsip":
			w.Write([]string{"Nomor Arsip", "Nama Arsip", "Kode Klasifikasi", "Unit Kerja", "Status"})
			var list []models.Arsip
			database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Find(&list)
			for _, a := range list {
				kk, uk := "", ""
				if a.KodeKlasifikasi != nil {
					kk = a.KodeKlasifikasi.KodeKlasifikasi
				}
				if a.UnitKerja != nil {
					uk = a.UnitKerja.NamaUnit
				}
				w.Write([]string{a.NomorArsip, a.NamaArsip, kk, uk, a.StatusArsip})
			}
			job.TotalRows = len(list)
		case "users":
			w.Write([]string{"Username", "Name", "Role", "Active"})
			var list []models.User
			database.DB.Preload("Role").Find(&list)
			for _, u := range list {
				role := ""
				if u.Role != nil {
					role = u.Role.Name
				}
				active := "false"
				if u.IsActive {
					active = "true"
				}
				w.Write([]string{u.Username, u.Name, role, active})
			}
			job.TotalRows = len(list)
		}
		w.Flush()
		now := time.Now()
		database.DB.Model(&job).Updates(map[string]interface{}{"status": "completed", "completed_at": now, "total_rows": job.TotalRows})
	}()
	middleware.SetFlash(c, "success", "Export sedang diproses.")
	c.Redirect(http.StatusFound, "/advanced/import-export")
}

func (h *ImportExportHandler) DownloadTemplate(c *gin.Context) {
	entityType := c.Param("type")
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=template_%s.csv", entityType))
	w := csv.NewWriter(c.Writer)
	switch entityType {
	case "arsip":
		w.Write([]string{"nomor_arsip", "nama_arsip", "kode_klasifikasi", "unit_kerja", "status_arsip", "uraian"})
	case "users":
		w.Write([]string{"username", "name", "password", "role", "is_active"})
	default:
		w.Write([]string{"column1", "column2", "column3"})
	}
	w.Flush()
}

func (h *ImportExportHandler) ShowJob(c *gin.Context) {
	var job models.ImportExportJob
	database.DB.First(&job, "id = ?", c.Param("jobId"))
	c.JSON(http.StatusOK, gin.H{"data": job})
}

func (h *ImportExportHandler) DownloadResult(c *gin.Context) {
	var job models.ImportExportJob
	if err := database.DB.First(&job, "id = ?", c.Param("jobId")).Error; err != nil || job.OutputFile == "" {
		c.String(http.StatusNotFound, "File tidak ditemukan")
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+filepath.Base(job.OutputFile))
	c.File(job.OutputFile)
}

func (h *ImportExportHandler) Progress(c *gin.Context) {
	var job models.ImportExportJob
	database.DB.First(&job, "id = ?", c.Param("jobId"))
	c.JSON(http.StatusOK, gin.H{
		"status": job.Status, "total_rows": job.TotalRows,
		"processed_rows": job.ProcessedRows, "error_rows": job.ErrorRows,
	})
}

func (h *ImportExportHandler) Retry(c *gin.Context) {
	database.DB.Model(&models.ImportExportJob{}).Where("id = ?", c.Param("jobId")).Updates(map[string]interface{}{
		"status": "pending", "processed_rows": 0, "error_rows": 0, "error_log": "",
	})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── LAPORAN EXPORT ──────────────────────────────────────────────────────────

type LaporanExportHandler struct{}

func (h *LaporanExportHandler) ArsipPDF(c *gin.Context) {
	var list []models.Arsip
	db := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip")
	db = applyLaporanArsipFilters(db, c)
	db.Order("nomor_arsip").Find(&list)
	headers := []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Klasifikasi", "Unit Kerja", "Lokasi", "Status", "Tgl Dibuat", "Retensi"}
	rows := [][]string{}
	for i, a := range list {
		kk, uk, lokasi := "", "", ""
		if a.KodeKlasifikasi != nil {
			kk = a.KodeKlasifikasi.KodeKlasifikasi + " - " + a.KodeKlasifikasi.NamaKlasifikasi
		}
		if a.UnitKerja != nil {
			uk = a.UnitKerja.NamaUnit
		}
		if a.LokasiArsip != nil {
			lokasi = a.LokasiArsip.NamaLokasi
		}
		tglDibuat := "-"
		if a.TanggalDibuat != nil {
			tglDibuat = a.TanggalDibuat.Format("02 Jan 2006")
		}
		retensi := "-"
		if a.TanggalRetensiAkhir != nil {
			retensi = a.TanggalRetensiAkhir.Format("02 Jan 2006")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), a.NomorArsip, a.NamaArsip, a.Uraian, kk, uk, lokasi, a.StatusArsip, tglDibuat, retensi})
	}
	exportPDF(c, "Laporan-Arsip-"+time.Now().Format("2006-01-02"), "Laporan Data Arsip", headers, rows)
}

func (h *LaporanExportHandler) ArsipExcel(c *gin.Context) {
	var list []models.Arsip
	db := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip")
	db = applyLaporanArsipFilters(db, c)
	db.Order("nomor_arsip").Find(&list)
	rows := [][]string{}
	for i, a := range list {
		kk, uk, lokasi := "", "", ""
		if a.KodeKlasifikasi != nil {
			kk = a.KodeKlasifikasi.KodeKlasifikasi
		}
		if a.UnitKerja != nil {
			uk = a.UnitKerja.NamaUnit
		}
		if a.LokasiArsip != nil {
			lokasi = a.LokasiArsip.NamaLokasi
		}
		tglDibuat := "-"
		if a.TanggalDibuat != nil {
			tglDibuat = a.TanggalDibuat.Format("2006-01-02")
		}
		retensi := "-"
		if a.TanggalRetensiAkhir != nil {
			retensi = a.TanggalRetensiAkhir.Format("2006-01-02")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), a.NomorArsip, a.NamaArsip, a.Uraian, kk, uk, lokasi, a.StatusArsip, tglDibuat, retensi})
	}
	exportXLSX(c, "Laporan-Arsip-"+time.Now().Format("2006-01-02"), []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Unit Kerja", "Lokasi", "Status", "Tgl Dibuat", "Retensi"}, rows)
}

func (h *LaporanExportHandler) DigitalPDF(c *gin.Context) {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("file_path IS NOT NULL AND file_path != ''").Order("nomor_arsip").Find(&list)
	headers := []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Unit Kerja", "Status", "Berkas", "Tgl Dibuat"}
	rows := [][]string{}
	for i, a := range list {
		uk := ""
		if a.UnitKerja != nil {
			uk = a.UnitKerja.NamaUnit
		}
		berkas := a.FileName
		if berkas == "" && a.FilePath != "" {
			berkas = a.FilePath
		}
		tgl := "-"
		if a.TanggalDibuat != nil {
			tgl = a.TanggalDibuat.Format("02 Jan 2006")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), a.NomorArsip, a.NamaArsip, a.Uraian, uk, a.StatusArsip, berkas, tgl})
	}
	exportPDF(c, "Laporan-Arsip-Digital-"+time.Now().Format("2006-01-02"), "Laporan Arsip Digital", headers, rows)
}

func (h *LaporanExportHandler) DigitalExcel(c *gin.Context) {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("file_path IS NOT NULL AND file_path != ''").Order("nomor_arsip").Find(&list)
	rows := [][]string{}
	for i, a := range list {
		uk := ""
		if a.UnitKerja != nil {
			uk = a.UnitKerja.NamaUnit
		}
		berkas := a.FileName
		if berkas == "" && a.FilePath != "" {
			berkas = a.FilePath
		}
		tgl := "-"
		if a.TanggalDibuat != nil {
			tgl = a.TanggalDibuat.Format("2006-01-02")
		}
		// Format kode klasifikasi
		kk := ""
		if a.KodeKlasifikasi != nil {
			kk = a.KodeKlasifikasi.KodeKlasifikasi + " - " + a.KodeKlasifikasi.NamaKlasifikasi
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), a.NomorArsip, a.NamaArsip, a.Uraian, kk, uk, a.StatusArsip, berkas, tgl})
	}
	exportXLSX(c, "Laporan-Arsip-Digital-"+time.Now().Format("2006-01-02"), []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Unit Kerja", "Status", "Berkas", "Tgl Dibuat"}, rows)
}

func (h *LaporanExportHandler) RetensiPDF(c *gin.Context) {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Joins("INNER JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id AND kode_klasifikasi.is_active = 1").
		Where("arsip.tanggal_dibuat IS NOT NULL AND arsip.status_arsip NOT IN ('musnah','siap_penyusutan','permanen') AND DATE_ADD(arsip.tanggal_dibuat, INTERVAL (kode_klasifikasi.retensi_aktif + kode_klasifikasi.retensi_inaktif) YEAR) < CURDATE()").
		Order("arsip.tanggal_dibuat").Find(&list)
	headers := []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Tgl Perolehan", "Retensi Berakhir", "Status"}
	rows := [][]string{}
	for i, a := range list {
		kk := ""
		if a.KodeKlasifikasi != nil {
			kk = a.KodeKlasifikasi.KodeKlasifikasi + " - " + a.KodeKlasifikasi.NamaKlasifikasi
		}
		ret := "-"
		if a.TanggalRetensiAkhir != nil {
			ret = a.TanggalRetensiAkhir.Format("02 Jan 2006")
		}
		tgl := "-"
		if a.TanggalDibuat != nil {
			tgl = a.TanggalDibuat.Format("02 Jan 2006")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), a.NomorArsip, a.NamaArsip, a.Uraian, kk, tgl, ret, a.StatusArsip})
	}
	exportPDF(c, "Laporan-Retensi-"+time.Now().Format("2006-01-02"), "Laporan Retensi Arsip", headers, rows)
}

func (h *LaporanExportHandler) RetensiExcel(c *gin.Context) {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Joins("INNER JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id AND kode_klasifikasi.is_active = 1").
		Where("arsip.tanggal_dibuat IS NOT NULL AND arsip.status_arsip NOT IN ('musnah','siap_penyusutan','permanen') AND DATE_ADD(arsip.tanggal_dibuat, INTERVAL (kode_klasifikasi.retensi_aktif + kode_klasifikasi.retensi_inaktif) YEAR) < CURDATE()").
		Order("arsip.tanggal_dibuat").Find(&list)
	rows := [][]string{}
	for i, a := range list {
		kk := ""
		if a.KodeKlasifikasi != nil {
			kk = a.KodeKlasifikasi.KodeKlasifikasi + " - " + a.KodeKlasifikasi.NamaKlasifikasi
		}
		ret := "-"
		if a.TanggalRetensiAkhir != nil {
			ret = a.TanggalRetensiAkhir.Format("2006-01-02")
		}
		tgl := "-"
		if a.TanggalDibuat != nil {
			tgl = a.TanggalDibuat.Format("2006-01-02")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), a.NomorArsip, a.NamaArsip, a.Uraian, kk, tgl, ret, a.StatusArsip})
	}
	exportXLSX(c, "Laporan-Retensi-"+time.Now().Format("2006-01-02"), []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Tgl Perolehan", "Retensi Berakhir", "Status"}, rows)
}

func (h *LaporanExportHandler) PemusnahanPDF(c *gin.Context) {
	var list []models.PemusnahanArsip
	database.DB.Preload("UserPengaju").Preload("UserApprove").Preload("Arsip").Order("created_at DESC").Find(&list)
	headers := []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Alasan", "Status", "Tgl Pengajuan", "Pengaju", "Persetujuan"}
	rows := [][]string{}
	for i, p := range list {
		arsipNomor := "-"
		arsipNama := "-"
		arsipUraian := "-"
		if p.Arsip != nil {
			arsipNomor = p.Arsip.NomorArsip
			arsipNama = p.Arsip.NamaArsip
			arsipUraian = p.Arsip.Uraian
		}
		pengaju := "-"
		if p.UserPengaju != nil {
			pengaju = p.UserPengaju.Name
		}
		approver := "-"
		if p.UserApprove != nil {
			approver = p.UserApprove.Name
		}
		tglPengajuan := "-"
		if p.TanggalPengajuan != nil {
			tglPengajuan = p.TanggalPengajuan.Format("02 Jan 2006")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), arsipNomor, arsipNama, arsipUraian, p.AlasanPengajuan, p.Status, tglPengajuan, pengaju, approver})
	}
	exportPDF(c, "Laporan-Pemusnahan-"+time.Now().Format("2006-01-02"), "Laporan Pemusnahan Arsip", headers, rows)
}

func (h *LaporanExportHandler) PemusnahanExcel(c *gin.Context) {
	var list []models.PemusnahanArsip
	database.DB.Preload("UserPengaju").Preload("UserApprove").Preload("Arsip").Order("created_at DESC").Find(&list)
	rows := [][]string{}
	for i, p := range list {
		arsipNomor := "-"
		arsipNama := "-"
		arsipUraian := "-"
		if p.Arsip != nil {
			arsipNomor = p.Arsip.NomorArsip
			arsipNama = p.Arsip.NamaArsip
			arsipUraian = p.Arsip.Uraian
		}
		pengaju := "-"
		if p.UserPengaju != nil {
			pengaju = p.UserPengaju.Name
		}
		approver := "-"
		if p.UserApprove != nil {
			approver = p.UserApprove.Name
		}
		tglPengajuan := "-"
		if p.TanggalPengajuan != nil {
			tglPengajuan = p.TanggalPengajuan.Format("2006-01-02")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), arsipNomor, arsipNama, arsipUraian, p.AlasanPengajuan, p.Status, tglPengajuan, pengaju, approver})
	}
	exportXLSX(c, "Laporan-Pemusnahan-"+time.Now().Format("2006-01-02"), []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Alasan", "Status", "Tgl Pengajuan", "Pengaju", "Persetujuan"}, rows)
}

func (h *LaporanExportHandler) PemberkasanPDF(c *gin.Context) {
	var list []models.Pemberkasan
	database.DB.Preload("Creator").Preload("UnitKerja").Preload("KodeKlasifikasi").Preload("Arsip").Order("created_at DESC").Find(&list)
	headers := []string{"No", "Kode Berkas", "Nama Pemberkasan", "Unit Kerja", "Kode Klasifikasi", "Tahun", "Jumlah Arsip", "Status", "Tgl Dibuat"}
	rows := [][]string{}
	for i, p := range list {
		uk := ""
		if p.UnitKerja != nil {
			uk = p.UnitKerja.NamaUnit
		}
		kk := ""
		if p.KodeKlasifikasi != nil {
			kk = p.KodeKlasifikasi.KodeKlasifikasi + " - " + p.KodeKlasifikasi.NamaKlasifikasi
		}
		tahun := ""
		if p.Tahun > 0 {
			tahun = strconv.Itoa(p.Tahun)
		}
		jmlArsip := 0
		if p.Arsip != nil {
			jmlArsip = len(p.Arsip)
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), p.KodeBerkas, p.NamaPemberkasan, uk, kk, tahun, strconv.Itoa(jmlArsip), p.StatusBerkas, p.CreatedAt.Format("02 Jan 2006")})
	}
	exportPDF(c, "Laporan-Pemberkasan-"+time.Now().Format("2006-01-02"), "Laporan Pemberkasan", headers, rows)
}

func (h *LaporanExportHandler) PemberkasanExcel(c *gin.Context) {
	var list []models.Pemberkasan
	database.DB.Preload("Creator").Preload("UnitKerja").Preload("KodeKlasifikasi").Preload("Arsip").Order("created_at DESC").Find(&list)
	rows := [][]string{}
	for i, p := range list {
		uk := ""
		if p.UnitKerja != nil {
			uk = p.UnitKerja.NamaUnit
		}
		kk := ""
		if p.KodeKlasifikasi != nil {
			kk = p.KodeKlasifikasi.KodeKlasifikasi + " - " + p.KodeKlasifikasi.NamaKlasifikasi
		}
		tahun := ""
		if p.Tahun > 0 {
			tahun = strconv.Itoa(p.Tahun)
		}
		jmlArsip := 0
		if p.Arsip != nil {
			jmlArsip = len(p.Arsip)
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), p.KodeBerkas, p.NamaPemberkasan, uk, kk, tahun, strconv.Itoa(jmlArsip), p.StatusBerkas, p.CreatedAt.Format("2006-01-02")})
	}
	exportXLSX(c, "Laporan-Pemberkasan-"+time.Now().Format("2006-01-02"), []string{"No", "Kode Berkas", "Nama Pemberkasan", "Unit Kerja", "Kode Klasifikasi", "Tahun", "Jumlah Arsip", "Status", "Tgl Dibuat"}, rows)
}

func (h *LaporanExportHandler) StatistikPDF(c *gin.Context) {
	var total, aktif, inaktif, musnah int64
	database.DB.Model(&models.Arsip{}).Count(&total)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'aktif' AND deleted_at IS NULL").Count(&aktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'inaktif' AND deleted_at IS NULL").Count(&inaktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'musnah' AND deleted_at IS NULL").Count(&musnah)
	headers := []string{"Kategori", "Jumlah"}
	rows := [][]string{
		{"Total Arsip", strconv.FormatInt(total, 10)},
		{"Arsip Aktif", strconv.FormatInt(aktif, 10)},
		{"Arsip Inaktif", strconv.FormatInt(inaktif, 10)},
		{"Arsip Musnah", strconv.FormatInt(musnah, 10)},
	}
	exportPDF(c, "Laporan-Statistik-"+time.Now().Format("2006-01-02"), "Laporan Statistik Arsip", headers, rows)
}

func (h *LaporanExportHandler) StatistikExcel(c *gin.Context) {
	var total, aktif, inaktif, musnah int64
	database.DB.Model(&models.Arsip{}).Count(&total)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'aktif' AND deleted_at IS NULL").Count(&aktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'inaktif' AND deleted_at IS NULL").Count(&inaktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'musnah' AND deleted_at IS NULL").Count(&musnah)
	rows := [][]string{
		{"Total Arsip", strconv.FormatInt(total, 10)},
		{"Arsip Aktif", strconv.FormatInt(aktif, 10)},
		{"Arsip Inaktif", strconv.FormatInt(inaktif, 10)},
		{"Arsip Musnah", strconv.FormatInt(musnah, 10)},
	}
	exportXLSX(c, "Laporan-Statistik-"+time.Now().Format("2006-01-02"), []string{"Kategori", "Jumlah"}, rows)
}

func (h *LaporanExportHandler) KlasifikasiDetail(c *gin.Context) {
	requestKlasifikasiID := c.Query("kode_klasifikasi_id")
	requestSearch := c.Query("search")
	requestUnitKerja := c.Query("unit_kerja_id")
	requestTahun := c.Query("tahun")
	requestStatus := c.Query("status")

	var allKlasifikasi []models.KodeKlasifikasi
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&allKlasifikasi)

	var totalArsipTerklasifikasi, totalArsipTanpaKlasifikasi int64
	database.DB.Model(&models.Arsip{}).Where("kode_klasifikasi_id IS NOT NULL AND deleted_at IS NULL").Count(&totalArsipTerklasifikasi)
	database.DB.Model(&models.Arsip{}).Where("kode_klasifikasi_id IS NULL AND deleted_at IS NULL").Count(&totalArsipTanpaKlasifikasi)

	type KlasifikasiWithCount struct {
		models.KodeKlasifikasi
		ArsipCount int `json:"arsip_count"`
	}
	var klasifikasiList []KlasifikasiWithCount

	if len(allKlasifikasi) > 0 {
		klasifikasiIDs := make([]string, len(allKlasifikasi))
		for i, kk := range allKlasifikasi {
			klasifikasiIDs[i] = kk.ID
		}
		type countRow struct {
			KodeKlasifikasiID string
			Count             int
		}
		var counts []countRow
		database.DB.Model(&models.Arsip{}).
			Select("kode_klasifikasi_id, COUNT(*) as count").
			Where("kode_klasifikasi_id IN ? AND deleted_at IS NULL", klasifikasiIDs).
			Group("kode_klasifikasi_id").
			Scan(&counts)
		countMap := make(map[string]int)
		for _, r := range counts {
			countMap[r.KodeKlasifikasiID] = r.Count
		}

		for _, kk := range allKlasifikasi {
			klasifikasiList = append(klasifikasiList, KlasifikasiWithCount{
				KodeKlasifikasi: kk,
				ArsipCount:      countMap[kk.ID],
			})
		}
	}

	// Top 5 klasifikasi by count
	sort.Slice(klasifikasiList, func(i, j int) bool {
		return klasifikasiList[i].ArsipCount > klasifikasiList[j].ArsipCount
	})
	topKlasifikasi := klasifikasiList
	if len(topKlasifikasi) > 5 {
		topKlasifikasi = topKlasifikasi[:5]
	}
	// Restore original sort
	sort.Slice(klasifikasiList, func(i, j int) bool {
		return klasifikasiList[i].KodeKlasifikasi.KodeKlasifikasi < klasifikasiList[j].KodeKlasifikasi.KodeKlasifikasi
	})

	// Build query string
	queryParams := url.Values{}
	if requestKlasifikasiID != "" {
		queryParams.Set("kode_klasifikasi_id", requestKlasifikasiID)
	}
	queryString := ""
	if len(queryParams) > 0 {
		queryString = "?" + queryParams.Encode()
	}

	// Default empty values
	data := gin.H{
		"title":                      "Laporan Klasifikasi",
		"pageTitle":                  "Detail Per Klasifikasi",
		"AllKlasifikasi":             allKlasifikasi,
		"TotalArsipTerklasifikasi":   totalArsipTerklasifikasi,
		"TotalArsipTanpaKlasifikasi": totalArsipTanpaKlasifikasi,
		"KlasifikasiList":            klasifikasiList,
		"TopKlasifikasi":             topKlasifikasi,
		"RequestKodeKlasifikasiID":   requestKlasifikasiID,
		"RequestSearch":              requestSearch,
		"RequestUnitKerjaID":         requestUnitKerja,
		"RequestTahun":               requestTahun,
		"RequestStatusArsip":         requestStatus,
		"QueryString":                queryString,
		"FilterShow":                 requestKlasifikasiID != "",
		"AdditionalParams":           "",
		"UnitKerjaList":              []models.UnitKerja{},
		"TahunList":                  []int{},
		"StatusList":                 []string{"aktif", "inaktif", "siap_penyusutan", "permanen"},
		"SelectedKlasifikasi":        nil,
		"SelectedKlasifikasiID":      "",
		"KlasifikasiListPagination":  "",
		"ArsipDetailTotal":           0,
		"ArsipDetail":                []interface{}{},
		"ArsipDetailPagination":      "",
		"ArsipStats":                 gin.H{"Aktif": 0, "Inaktif": 0, "SiapPenyusutan": 0, "Permanen": 0, "PersenAktif": 0.0, "PersenInaktif": 0.0, "PersenSusut": 0.0, "PersenPermanen": 0.0},
		"ChartData":                  []interface{}{},
	}

	// If a specific classification is selected, load details
	if requestKlasifikasiID != "" {
		var selected models.KodeKlasifikasi
		if err := database.DB.First(&selected, "id = ?", requestKlasifikasiID).Error; err == nil {
			data["SelectedKlasifikasi"] = selected
			data["SelectedKlasifikasiID"] = selected.ID
			data["SelectedKlasifikasi.KlasifikasiKeamanan"] = selected.KlasifikasiKeamanan

			// Arsip stats for this classification
			var aktif, inaktif, siapSusut, permanen int64
			database.DB.Model(&models.Arsip{}).Where("kode_klasifikasi_id = ? AND deleted_at IS NULL AND status_arsip = ?", requestKlasifikasiID, "aktif").Count(&aktif)
			database.DB.Model(&models.Arsip{}).Where("kode_klasifikasi_id = ? AND deleted_at IS NULL AND status_arsip = ?", requestKlasifikasiID, "inaktif").Count(&inaktif)
			database.DB.Model(&models.Arsip{}).Where("kode_klasifikasi_id = ? AND deleted_at IS NULL AND status_arsip IN (?)", requestKlasifikasiID, []string{"siap_penyusutan", "siap_musnah", "siap_retensi"}).Count(&siapSusut)
			database.DB.Model(&models.Arsip{}).Where("kode_klasifikasi_id = ? AND deleted_at IS NULL AND status_arsip = ?", requestKlasifikasiID, "permanen").Count(&permanen)
			total := aktif + inaktif + siapSusut + permanen
			if total == 0 {
				total = 1
			}
			data["ArsipStats"] = gin.H{
				"Aktif": aktif, "Inaktif": inaktif,
				"SiapPenyusutan": siapSusut, "Permanen": permanen,
				"PersenAktif":    float64(aktif) / float64(total) * 100,
				"PersenInaktif":  float64(inaktif) / float64(total) * 100,
				"PersenSusut":    float64(siapSusut) / float64(total) * 100,
				"PersenPermanen": float64(permanen) / float64(total) * 100,
			}
			data["ArsipDetailTotal"] = total

			// Arsip list with filters
			db := database.DB.Where("kode_klasifikasi_id = ? AND deleted_at IS NULL", requestKlasifikasiID)
			if requestSearch != "" {
				db = db.Where("(nama_arsip LIKE ? OR nomor_arsip LIKE ? OR uraian LIKE ?)",
					"%"+requestSearch+"%", "%"+requestSearch+"%", "%"+requestSearch+"%")
			}
			if requestStatus != "" {
				db = db.Where("status_arsip = ?", requestStatus)
			}
			if requestUnitKerja != "" {
				db = db.Where("unit_kerja_id = ?", requestUnitKerja)
			}
			var totalArsip int64
			db.Model(&models.Arsip{}).Count(&totalArsip)
			data["ArsipDetailTotal"] = totalArsip

			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			if page < 1 {
				page = 1
			}
			perPage := 15
			offset := (page - 1) * perPage

			var arsipList []models.Arsip
			db.Preload("KodeKlasifikasi").Preload("UnitKerja").
				Order("created_at DESC").Limit(perPage).Offset(offset).Find(&arsipList)

			type ArsipRow struct {
				ID          string
				NomorArsip  string
				NamaArsip   string
				Uraian      string
				UnitKerja   string
				StatusText  string
				StatusClass string
			}
			var detailRows []ArsipRow
			for _, a := range arsipList {
				unitNama := ""
				if a.UnitKerja != nil {
					unitNama = a.UnitKerja.NamaUnit
				}
				statusText := a.StatusArsip
				statusClass := "primary"
				switch a.StatusArsip {
				case "aktif":
					statusText = "Aktif"
					statusClass = "success"
				case "inaktif":
					statusText = "Inaktif"
					statusClass = "warning"
				case "siap_penyusutan", "siap_musnah", "siap_retensi":
					statusText = "Siap Susut"
					statusClass = "danger"
				case "permanen":
					statusText = "Permanen"
					statusClass = "info"
				}
				detailRows = append(detailRows, ArsipRow{
					ID: a.ID, NomorArsip: a.NomorArsip, NamaArsip: a.NamaArsip,
					Uraian: a.Uraian, UnitKerja: unitNama,
					StatusText: statusText, StatusClass: statusClass,
				})
			}
			data["ArsipDetail"] = detailRows

			// Pagination
			totalPages := int(totalArsip) / perPage
			if int(totalArsip)%perPage > 0 {
				totalPages++
			}
			pagination := `<nav><ul class="pagination pagination-sm justify-content-end mb-0">`
			if page > 1 {
				pagination += fmt.Sprintf(`<li class="page-item"><a class="page-link" href="?kode_klasifikasi_id=%s&page=%d">«</a></li>`, requestKlasifikasiID, page-1)
			} else {
				pagination += `<li class="page-item disabled"><span class="page-link">«</span></li>`
			}
			for p := 1; p <= totalPages; p++ {
				active := ""
				if p == page {
					active = " active"
				}
				pagination += fmt.Sprintf(`<li class="page-item%s"><a class="page-link" href="?kode_klasifikasi_id=%s&page=%d">%d</a></li>`, active, requestKlasifikasiID, p, p)
			}
			if page < totalPages {
				pagination += fmt.Sprintf(`<li class="page-item"><a class="page-link" href="?kode_klasifikasi_id=%s&page=%d">»</a></li>`, requestKlasifikasiID, page+1)
			} else {
				pagination += `<li class="page-item disabled"><span class="page-link">»</span></li>`
			}
			pagination += `</ul></nav>`
			data["ArsipDetailPagination"] = pagination

			// Unit kerja list for filter
			var unitKerjaList []models.UnitKerja
			database.DB.Order("nama_unit").Find(&unitKerjaList)
			data["UnitKerjaList"] = unitKerjaList
		}
	}

	Render(c, 200, "laporan/klasifikasi-detail.html", data)
}
func (h *LaporanExportHandler) LokasiIndex(c *gin.Context) {
	// Data untuk dropdown
	var lokasiList []models.LokasiArsip
	database.DB.Where("is_active = ?", true).Order("nama_lokasi ASC").Find(&lokasiList)

	var unitKerjaList []models.UnitKerja
	database.DB.Order("nama_unit ASC").Find(&unitKerjaList)

	// Query params filter
	lokasiID := c.Query("lokasi_id")
	unitKerjaID := c.Query("unit_kerja_id")
	statusArsip := c.Query("status_arsip")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	search := c.Query("search")

	// QueryString untuk export links (semua params kecuali lokasi_id)
	var qsParts []string
	if unitKerjaID != "" {
		qsParts = append(qsParts, "unit_kerja_id="+unitKerjaID)
	}
	if statusArsip != "" {
		qsParts = append(qsParts, "status_arsip="+statusArsip)
	}
	if startDate != "" {
		qsParts = append(qsParts, "start_date="+startDate)
	}
	if endDate != "" {
		qsParts = append(qsParts, "end_date="+endDate)
	}
	if search != "" {
		qsParts = append(qsParts, "search="+search)
	}
	qs := strings.Join(qsParts, "&")

	data := gin.H{
		"title":              "Laporan Lokasi",
		"pageTitle":          "Laporan Per Lokasi",
		"LokasiList":         lokasiList,
		"UnitKerjaList":      unitKerjaList,
		"RequestLokasiID":    lokasiID,
		"RequestUnitKerjaID": unitKerjaID,
		"RequestStatusArsip": statusArsip,
		"RequestStartDate":   startDate,
		"RequestEndDate":     endDate,
		"RequestSearch":      search,
		"QueryString":        qs,
		"ArsipListFirstItem": 0,
		"ArsipListLastItem":  0,
		"ArsipListTotal":     0,
		"ArsipListHasPages":  false,
		"Pagination":         template.HTML(""),
	}

	if lokasiID != "" {
		// Ambil data lokasi terpilih
		var lokasi models.LokasiArsip
		if err := database.DB.First(&lokasi, "id = ?", lokasiID).Error; err == nil {
			data["Lokasi"] = &lokasi
		}

		// Statistik arsip berdasarkan lokasi terpilih
		type LokasiStats struct {
			Total       int64 `gorm:"column:total"`
			Aktif       int64 `gorm:"column:aktif"`
			Inaktif     int64 `gorm:"column:inaktif"`
			Diberkaskan int64 `gorm:"column:diberkaskan"`
			Musnah      int64 `gorm:"column:musnah"`
			Permanen    int64 `gorm:"column:permanen"`
		}
		var lokasiStats LokasiStats
		database.DB.Raw(`
			SELECT
				COUNT(*) as total,
				SUM(CASE WHEN a.status_arsip IN ('aktif','diberkaskan') THEN 1 ELSE 0 END) as aktif,
				SUM(CASE WHEN a.status_arsip = 'inaktif' THEN 1 ELSE 0 END) as inaktif,
				SUM(CASE WHEN a.status_arsip = 'diberkaskan' THEN 1 ELSE 0 END) as diberkaskan,
				SUM(CASE WHEN a.status_arsip = 'musnah' THEN 1 ELSE 0 END) as musnah,
				SUM(CASE WHEN a.status_arsip = 'permanen' THEN 1 ELSE 0 END) as permanen
			FROM arsip a
			WHERE a.lokasi_arsip_id = ? AND a.deleted_at IS NULL`, lokasiID).Scan(&lokasiStats)
		data["Stats"] = &lokasiStats

		// Query arsip dengan filter
		db := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").
			Where("lokasi_arsip_id = ?", lokasiID)
		if unitKerjaID != "" {
			db = db.Where("unit_kerja_id = ?", unitKerjaID)
		}
		if statusArsip != "" {
			db = db.Where("status_arsip = ?", statusArsip)
		}
		if search != "" {
			searchLike := "%" + search + "%"
			db = db.Where("(nomor_arsip LIKE ? OR nama_arsip LIKE ?)", searchLike, searchLike)
		}
		if startDate != "" {
			db = db.Where("tanggal_dibuat >= ?", startDate)
		}
		if endDate != "" {
			db = db.Where("tanggal_dibuat <= ?", endDate)
		}

		// ── Pagination for Lokasi arsip list ──
		perPage := 25
		page := 1
		if p := c.Query("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}

		var totalFiltered int64
		dbCount := db.Session(&gorm.Session{})
		dbCount.Count(&totalFiltered)

		totalPages := int(math.Ceil(float64(totalFiltered) / float64(perPage)))
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}
		offset := (page - 1) * perPage

		var arsipList []models.Arsip
		db.Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) ASC").Offset(offset).Limit(perPage).Find(&arsipList)

		firstItem := offset + 1
		lastItem := offset + len(arsipList)
		if lastItem > int(totalFiltered) {
			lastItem = int(totalFiltered)
		}
		hasPages := totalPages > 1

		rawQuery := c.Request.URL.RawQuery
		paginationQueryStr := removePageParam(rawQuery)

		var paginationHTML template.HTML
		if hasPages {
			paginationHTML = BuildPagination(page, totalPages, paginationQueryStr)
		}

		data["ArsipList"] = arsipList
		data["ArsipListFirstItem"] = firstItem
		data["ArsipListLastItem"] = lastItem
		data["ArsipListTotal"] = int(totalFiltered)
		data["ArsipListHasPages"] = hasPages
		data["Pagination"] = paginationHTML
	}

	Render(c, 200, "laporan/lokasi/index.html", data)
}

func (h *LaporanExportHandler) LokasiFilter(c *gin.Context) {
	// Redirect ke halaman index dengan query params untuk render HTML
	qs := c.Request.URL.RawQuery
	if qs != "" {
		c.Redirect(http.StatusFound, "/laporan/lokasi?"+qs)
	} else {
		c.Redirect(http.StatusFound, "/laporan/lokasi")
	}
}

func (h *LaporanExportHandler) LokasiPDF(c *gin.Context) {
	lokasiID := c.Query("lokasi_id")
	var (
		headers []string
		rows    [][]string
		title   string
	)
	if lokasiID != "" {
		// Export per lokasi
		var lokasi models.LokasiArsip
		if err := database.DB.First(&lokasi, "id = ?", lokasiID).Error; err == nil {
			title = "Laporan Arsip - " + lokasi.NamaLokasi
		} else {
			title = "Laporan Arsip Per Lokasi"
		}
		var arsipList []models.Arsip
		database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").
			Where("lokasi_arsip_id = ?", lokasiID).Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) ASC").Find(&arsipList)
		headers = []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Unit Kerja", "Tanggal Arsip", "Status", "Lokasi"}
		for i, a := range arsipList {
			status := a.StatusArsip
			tgl := ""
			if a.TanggalDibuat != nil {
				tgl = a.TanggalDibuat.Format("2006-01-02")
			}
			unitKerja := ""
			if a.UnitKerja != nil {
				unitKerja = a.UnitKerja.NamaUnit
			}
			kk := ""
			if a.KodeKlasifikasi != nil {
				kk = a.KodeKlasifikasi.KodeKlasifikasi
			}
			lokasiNama := ""
			if a.LokasiArsip != nil {
				lokasiNama = a.LokasiArsip.NamaLokasi
			}
			rows = append(rows, []string{
				strconv.Itoa(i + 1),
				a.NomorArsip,
				a.NamaArsip,
				a.Uraian,
				kk,
				unitKerja,
				tgl,
				status,
				lokasiNama,
			})
		}
	} else {
		// Export semua lokasi (aggregate)
		var stats []struct {
			NamaLokasi string `gorm:"column:nama_lokasi"`
			Total      int64  `gorm:"column:total"`
		}
		database.DB.Raw(`SELECT la.nama_lokasi, COUNT(a.id) as total
			FROM lokasi_arsips la LEFT JOIN arsip a ON a.lokasi_arsip_id = la.id AND a.deleted_at IS NULL
			GROUP BY la.id, la.nama_lokasi ORDER BY total DESC`).Scan(&stats)
		title = "Laporan Arsip Per Lokasi"
		headers = []string{"Lokasi", "Jumlah Arsip"}
		for _, s := range stats {
			rows = append(rows, []string{s.NamaLokasi, strconv.FormatInt(s.Total, 10)})
		}
	}
	exportPDF(c, "Laporan-Lokasi-"+time.Now().Format("2006-01-02"), title, headers, rows)
}

func (h *LaporanExportHandler) LokasiExcel(c *gin.Context) {
	lokasiID := c.Query("lokasi_id")
	var (
		headers []string
		rows    [][]string
	)
	if lokasiID != "" {
		// Export per lokasi
		var arsipList []models.Arsip
		database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").
			Where("lokasi_arsip_id = ?", lokasiID).Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) ASC").Find(&arsipList)
		headers = []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Unit Kerja", "Tanggal Arsip", "Status", "Lokasi"}
		for i, a := range arsipList {
			status := a.StatusArsip
			tgl := ""
			if a.TanggalDibuat != nil {
				tgl = a.TanggalDibuat.Format("2006-01-02")
			}
			unitKerja := ""
			if a.UnitKerja != nil {
				unitKerja = a.UnitKerja.NamaUnit
			}
			kk := ""
			if a.KodeKlasifikasi != nil {
				kk = a.KodeKlasifikasi.KodeKlasifikasi
			}
			lokasiNama := ""
			if a.LokasiArsip != nil {
				lokasiNama = a.LokasiArsip.NamaLokasi
			}
			rows = append(rows, []string{
				strconv.Itoa(i + 1),
				a.NomorArsip,
				a.NamaArsip,
				a.Uraian,
				kk,
				unitKerja,
				tgl,
				status,
				lokasiNama,
			})
		}
	} else {
		// Export semua lokasi (aggregate)
		var stats []struct {
			NamaLokasi string `gorm:"column:nama_lokasi"`
			Total      int64  `gorm:"column:total"`
		}
		database.DB.Raw(`SELECT la.nama_lokasi, COUNT(a.id) as total
			FROM lokasi_arsips la LEFT JOIN arsip a ON a.lokasi_arsip_id = la.id AND a.deleted_at IS NULL
			GROUP BY la.id, la.nama_lokasi ORDER BY total DESC`).Scan(&stats)
		headers = []string{"Lokasi", "Jumlah Arsip"}
		for _, s := range stats {
			rows = append(rows, []string{s.NamaLokasi, strconv.FormatInt(s.Total, 10)})
		}
	}
	exportXLSX(c, "Laporan-Lokasi-"+time.Now().Format("2006-01-02"), headers, rows)
}

func (h *LaporanExportHandler) LokasiStatistik(c *gin.Context) {
	lokasiID := c.Query("lokasi_id")
	var stats []struct {
		NamaLokasi string `gorm:"column:nama_lokasi"`
		Total      int64  `gorm:"column:total"`
	}
	if lokasiID != "" {
		database.DB.Raw(`SELECT la.nama_lokasi, COUNT(a.id) as total
			FROM lokasi_arsips la LEFT JOIN arsip a ON a.lokasi_arsip_id = la.id AND a.deleted_at IS NULL
			WHERE a.lokasi_arsip_id = ?
			GROUP BY la.id, la.nama_lokasi ORDER BY total DESC`, lokasiID).Scan(&stats)
	} else {
		database.DB.Raw(`SELECT la.nama_lokasi, COUNT(a.id) as total
			FROM lokasi_arsips la LEFT JOIN arsip a ON a.lokasi_arsip_id = la.id AND a.deleted_at IS NULL
			GROUP BY la.id, la.nama_lokasi ORDER BY total DESC`).Scan(&stats)
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

func (h *LaporanExportHandler) Digital(c *gin.Context) {
	const perPage = 25

	page := 1
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	db := database.DB.Model(&models.Arsip{}).Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("file_path IS NOT NULL AND file_path != ''")

	var totalFiltered int64
	countDB := db.Session(&gorm.Session{})
	countDB.Count(&totalFiltered)

	totalPages := int(math.Ceil(float64(totalFiltered) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage

	var list []models.Arsip
	db.Order("nomor_arsip").Offset(offset).Limit(perPage).Find(&list)

	firstItem := offset + 1
	lastItem := offset + len(list)
	if lastItem > int(totalFiltered) {
		lastItem = int(totalFiltered)
	}
	hasPages := totalPages > 1

	rawQuery := c.Request.URL.RawQuery
	paginationQueryStr := removePageParam(rawQuery)
	exportQueryStr := ""
	if paginationQueryStr != "" {
		exportQueryStr = "?" + paginationQueryStr
	}

	var paginationHTML template.HTML
	if hasPages {
		paginationHTML = BuildPagination(page, totalPages, paginationQueryStr)
	}

	// Load filter options
	var unitKerjaOpts []models.UnitKerja
	var kodeKlasifikasiOpts []models.KodeKlasifikasi
	database.DB.Order("nama_unit").Find(&unitKerjaOpts)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOpts)

	var tahunList []int
	database.DB.Model(&models.Arsip{}).
		Select("DISTINCT EXTRACT(YEAR FROM tanggal_dibuat) as yr").
		Where("tanggal_dibuat IS NOT NULL AND deleted_at IS NULL").
		Order("yr DESC").
		Pluck("yr", &tahunList)
	if tahunList == nil {
		tahunList = []int{}
	}

	Render(c, 200, "laporan/digital.html", gin.H{
		"title": "Laporan Arsip Digital", "pageTitle": "Laporan Arsip Digital",
		"ArsipList":                list,
		"count":                    len(list),
		"ArsipListFirstItem":       firstItem,
		"ArsipListLastItem":        lastItem,
		"ArsipListTotal":           int(totalFiltered),
		"ArsipListHasPages":        hasPages,
		"Pagination":               paginationHTML,
		"QueryString":              exportQueryStr,
		"UnitKerjaList":            unitKerjaOpts,
		"KodeKlasifikasiList":      kodeKlasifikasiOpts,
		"TahunList":                tahunList,
		"RequestSearch":            c.Query("search"),
		"RequestUnitKerjaID":       c.Query("unit_kerja_id"),
		"RequestStatusArsip":       c.Query("status_arsip"),
		"RequestKodeKlasifikasiID": c.Query("kode_klasifikasi_id"),
		"RequestTahun":             c.Query("tahun"),
	})
}

func (h *LaporanExportHandler) Pemberkasan(c *gin.Context) {
	const perPage = 25

	page := 1
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	db := database.DB.Model(&models.Pemberkasan{}).Preload("Creator").Preload("UnitKerja").Preload("KodeKlasifikasi").Preload("Arsip")

	if v := c.Query("unit_kerja_id"); v != "" {
		db = db.Where("unit_kerja_id = ?", v)
	}
	if v := c.Query("status_berkas"); v != "" {
		db = db.Where("status_berkas = ?", v)
	}
	if v := c.Query("tahun"); v != "" {
		db = db.Where("EXTRACT(YEAR FROM tahun) = ?", v)
	}

	hasFilters := c.Query("unit_kerja_id") != "" || c.Query("status_berkas") != "" || c.Query("tahun") != ""

	var totalFiltered int64
	countDB := db.Session(&gorm.Session{})
	countDB.Count(&totalFiltered)

	totalPages := int(math.Ceil(float64(totalFiltered) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage

	var list []models.Pemberkasan
	db.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&list)

	type PemberkasanWithCount struct {
		models.Pemberkasan
		ArsipCount int `json:"arsip_count"`
	}
	var enrichedList []PemberkasanWithCount
	for _, p := range list {
		enrichedList = append(enrichedList, PemberkasanWithCount{
			Pemberkasan: p,
			ArsipCount:  len(p.Arsip),
		})
	}

	firstItem := offset + 1
	lastItem := offset + len(list)
	if lastItem > int(totalFiltered) {
		lastItem = int(totalFiltered)
	}
	hasPages := totalPages > 1

	rawQuery := c.Request.URL.RawQuery
	paginationQueryStr := removePageParam(rawQuery)
	exportQueryStr := ""
	if paginationQueryStr != "" {
		exportQueryStr = "?" + paginationQueryStr
	}

	var paginationHTML template.HTML
	if hasPages {
		paginationHTML = BuildPagination(page, totalPages, paginationQueryStr)
	}

	// Load filter options
	var unitKerjaOpts []models.UnitKerja
	database.DB.Order("nama_unit").Find(&unitKerjaOpts)

	var tahunList []int
	database.DB.Model(&models.Pemberkasan{}).
		Select("DISTINCT tahun as yr").
		Where("tahun IS NOT NULL AND tahun > 0").
		Order("yr DESC").
		Pluck("yr", &tahunList)
	if tahunList == nil {
		tahunList = []int{}
	}

	Render(c, 200, "laporan/pemberkasan.html", gin.H{
		"title": "Laporan Pemberkasan", "pageTitle": "Laporan Pemberkasan",
		"PemberkasanList":          enrichedList,
		"PemberkasanListFirstItem": firstItem,
		"PemberkasanListLastItem":  lastItem,
		"PemberkasanListTotal":     int(totalFiltered),
		"PemberkasanListHasPages":  hasPages,
		"Pagination":               paginationHTML,
		"UnitKerjaList":            unitKerjaOpts,
		"TahunList":                tahunList,
		"HasFilters":               hasFilters,
		"QueryString":              exportQueryStr,
		"RequestUnitKerjaID":       c.Query("unit_kerja_id"),
		"RequestStatusBerkas":      c.Query("status_berkas"),
		"RequestTahun":             c.Query("tahun"),
	})
}

func (h *LaporanExportHandler) Statistik(c *gin.Context) {
	var stats struct {
		Total          int64 `gorm:"column:total"`
		Aktif          int64 `gorm:"column:aktif"`
		Inaktif        int64 `gorm:"column:inaktif"`
		SiapPenyusutan int64 `gorm:"column:siap_penyusutan"`
		Musnah         int64 `gorm:"column:musnah"`
		Permanen       int64 `gorm:"column:permanen"`
		WithFile       int64 `gorm:"column:with_file"`
		WithoutFile    int64 `gorm:"column:without_file"`
		Spj            int64 `gorm:"column:spj"`
		NonSpj         int64 `gorm:"column:non_spj"`
		Unclassified   int64 `gorm:"column:unclassified"`
	}

	database.DB.Raw(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status_arsip IN ('aktif','diberkaskan') THEN 1 ELSE 0 END) as aktif,
			SUM(CASE WHEN status_arsip = 'inaktif' THEN 1 ELSE 0 END) as inaktif,
			SUM(CASE WHEN status_arsip = 'siap_penyusutan' THEN 1 ELSE 0 END) as siap_penyusutan,
			SUM(CASE WHEN status_arsip = 'musnah' THEN 1 ELSE 0 END) as musnah,
			SUM(CASE WHEN status_arsip = 'permanen' THEN 1 ELSE 0 END) as permanen,
			SUM(CASE WHEN file_path IS NOT NULL AND file_path != '' THEN 1 ELSE 0 END) as with_file,
			SUM(CASE WHEN file_path IS NULL OR file_path = '' THEN 1 ELSE 0 END) as without_file,
			SUM(CASE WHEN a.jenis_arsip = 'SPJ' THEN 1 ELSE 0 END) as spj,
			SUM(CASE WHEN a.jenis_arsip = 'Non SPJ' THEN 1 ELSE 0 END) as non_spj,
			SUM(CASE WHEN NULLIF(a.jenis_arsip,'') IS NULL OR a.jenis_arsip NOT IN ('SPJ','Non SPJ') THEN 1 ELSE 0 END) as unclassified
		FROM arsip a
		WHERE a.deleted_at IS NULL
	`).Scan(&stats)

	type UnitStat struct {
		NamaUnit    string `gorm:"column:nama_unit"`
		JumlahArsip int64  `gorm:"column:total"`
	}
	var arsipPerUnit []UnitStat
	database.DB.Raw(`SELECT uk.nama_unit, COUNT(a.id) as total FROM arsip a
		LEFT JOIN unit_kerja uk ON a.unit_kerja_id = uk.id
		WHERE a.deleted_at IS NULL GROUP BY uk.nama_unit ORDER BY total DESC LIMIT 10`).Scan(&arsipPerUnit)

	type KlasifikasiStat struct {
		NamaKlasifikasi string `gorm:"column:nama_klasifikasi"`
		JumlahArsip     int64  `gorm:"column:total"`
	}
	var arsipPerKlasifikasi []KlasifikasiStat
	database.DB.Raw(`SELECT kk.nama_klasifikasi, COUNT(a.id) as total FROM arsip a
		LEFT JOIN kode_klasifikasi kk ON a.kode_klasifikasi_id = kk.id
		WHERE a.deleted_at IS NULL GROUP BY kk.nama_klasifikasi ORDER BY total DESC LIMIT 10`).Scan(&arsipPerKlasifikasi)

	var pemusnahanTotal int64
	database.DB.Model(&models.PemusnahanArsip{}).Count(&pemusnahanTotal)
	var pemusnahanDisetujui int64
	database.DB.Model(&models.PemusnahanArsip{}).Where("status = 'disetujui'").Count(&pemusnahanDisetujui)
	var pemusnahanPending int64
	if pemusnahanTotal > pemusnahanDisetujui {
		pemusnahanPending = pemusnahanTotal - pemusnahanDisetujui
	}

	digitalPercent := 0
	if stats.Total > 0 {
		digitalPercent = int(float64(stats.WithFile) * 100 / float64(stats.Total))
	}

	Render(c, 200, "laporan/statistik.html", gin.H{
		"title": "Statistik", "pageTitle": "Statistik Arsip",
		"TotalArsip":               stats.Total,
		"TotalArsipAktif":          stats.Aktif,
		"TotalArsipInaktif":        stats.Inaktif,
		"TotalSiapMusnah":          stats.SiapPenyusutan,
		"TotalArsipMusnah":         stats.Musnah,
		"TotalPemusnahan":          pemusnahanTotal,
		"TotalArsipPermanen":       stats.Permanen,
		"TotalArsipDigital":        stats.WithFile,
		"TotalArsipBelumDigital":   stats.WithoutFile,
		"PersenDigitalisasi":       digitalPercent,
		"TotalArsipSpj":            stats.Spj,
		"TotalArsipNonSpj":         stats.NonSpj,
		"TotalArsipBelumTerklas":   stats.Unclassified,
		"TotalPemusnahanDisetujui": pemusnahanDisetujui,
		"TotalPemusnahanPending":   pemusnahanPending,
		"ArsipPerUnit":             arsipPerUnit,
		"ArsipPerKlasifikasi":      arsipPerKlasifikasi,
	})
}

// ── BACKUP ADVANCED ─────────────────────────────────────────────────────────

type BackupAdvancedHandler struct{}

// restoreGuard blocks destructive database restores unless the caller is an
// admin AND explicitly confirmed the action. Restores pipe a mysqldump-style
// SQL file (containing DROP TABLE statements) straight into the live
// database, so both checks are mandatory.
func restoreGuard(c *gin.Context) (bool, string) {
	user := middleware.GetCurrentUser(c)
	if user == nil || !user.IsAdmin() {
		return false, "Hanya admin yang dapat melakukan restore database."
	}
	return true, ""
}

func (h *BackupAdvancedHandler) Restore(c *gin.Context) {
	isJSON := strings.Contains(c.ContentType(), "application/json")

	if ok, msg := restoreGuard(c); !ok {
		if isJSON {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": msg})
		} else {
			middleware.SetFlash(c, "error", msg)
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	var restoreFilePath string
	var filename string

	if isJSON {
		var req struct {
			BackupFile  string `json:"backup_file"`
			RestoreMode string `json:"restore_mode"`
			Confirm     bool   `json:"confirm"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Request payload tidak valid"})
			return
		}
		if req.BackupFile == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "File backup tidak ditentukan"})
			return
		}
		if !req.Confirm {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Restore harus dikirim dengan konfirmasi (confirm=true)"})
			return
		}
		filename = req.BackupFile
		// Look up backup log first
		var log models.BackupLog
		if err := database.DB.Where("filename = ?", filename).First(&log).Error; err == nil {
			restoreFilePath = log.FilePath
		} else {
			restoreFilePath = filepath.Join(config.BackupDir(), filename)
		}
	} else {
		if c.PostForm("confirm") != "1" {
			middleware.SetFlash(c, "error", "Restore harus dikirim dengan konfirmasi (confirm=1).")
			c.Redirect(http.StatusFound, "/backup")
			return
		}
		// Multipart Form Upload (Try backup_file first, then file)
		file, err := c.FormFile("backup_file")
		if err != nil {
			file, err = c.FormFile("file")
		}
		if err != nil {
			middleware.SetFlash(c, "error", "File backup wajib diunggah: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
			return
		}
		filename = file.Filename
		uploadDir := filepath.Join(config.StorageDir(), "restore")
		os.MkdirAll(uploadDir, 0755)
		restoreFilePath = filepath.Join(uploadDir, filename)
		if err := c.SaveUploadedFile(file, restoreFilePath); err != nil {
			middleware.SetFlash(c, "error", "Gagal menyimpan file upload: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
			return
		}
		defer os.Remove(restoreFilePath)
	}

	if _, err := os.Stat(restoreFilePath); os.IsNotExist(err) {
		if isJSON {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "File backup tidak ditemukan: " + restoreFilePath})
		} else {
			middleware.SetFlash(c, "error", "File backup tidak ditemukan.")
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	targetSQLPath := restoreFilePath

	// Decrypt if .enc
	if strings.HasSuffix(restoreFilePath, ".enc") {
		decryptedPath := strings.TrimSuffix(restoreFilePath, ".enc")
		key := getEncryptionKey()
		if err := decryptFile(restoreFilePath, decryptedPath, key); err != nil {
			if isJSON {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal mendekripsi backup: " + err.Error()})
			} else {
				middleware.SetFlash(c, "error", "Gagal mendekripsi backup: "+err.Error())
				c.Redirect(http.StatusFound, "/backup")
			}
			return
		}
		targetSQLPath = decryptedPath
		defer os.Remove(decryptedPath)
	}

	// Decompress if .sql.gz
	if strings.HasSuffix(targetSQLPath, ".gz") {
		decompressedPath := strings.TrimSuffix(targetSQLPath, ".gz")
		if err := decompressGzip(targetSQLPath, decompressedPath); err != nil {
			if isJSON {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal mendekompresi backup: " + err.Error()})
			} else {
				middleware.SetFlash(c, "error", "Gagal mendekompresi backup: "+err.Error())
				c.Redirect(http.StatusFound, "/backup")
			}
			return
		}
		defer os.Remove(decompressedPath)
		targetSQLPath = decompressedPath
	}

	// Run mysql restore
	// mysql command is not available on Vercel serverless
	if !config.CanRestore() {
		if isJSON {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Database restore tidak tersedia di Vercel atau mysql tidak ditemukan di sistem. Gunakan restore dari dashboard database provider"})
		} else {
			middleware.SetFlash(c, "error", "Database restore tidak tersedia di Vercel atau mysql tidak ditemukan di sistem.")
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	dbUser := os.Getenv("DB_USERNAME")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_DATABASE")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	if dbHost == "" {
		dbHost = "127.0.0.1"
	}
	if dbPort == "" {
		dbPort = "3306"
	}
	args := []string{"--host=" + dbHost, "--port=" + dbPort, "--user=" + dbUser, dbName}
	cmd := exec.Command("mysql", args...)
	if dbPass != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+dbPass)
	}
	inFile, err := os.Open(targetSQLPath)
	if err != nil {
		if isJSON {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal membuka file restore: " + err.Error()})
		} else {
			middleware.SetFlash(c, "error", "Gagal membuka file restore: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}
	defer inFile.Close()
	var errBufRestore strings.Builder
	cmd.Stderr = &errBufRestore

	// Optimasi: disable FK & autocommit untuk restore cepat
	optReader := strings.NewReader("SET FOREIGN_KEY_CHECKS=0;\nSET unique_checks=0;\nSET autocommit=0;\n")
	finalReader := strings.NewReader("\nCOMMIT;\nSET FOREIGN_KEY_CHECKS=1;\nSET unique_checks=1;\nSET autocommit=1;\n")
	cmd.Stdin = io.MultiReader(optReader, inFile, finalReader)

	if err := cmd.Run(); err != nil {
		errMsg := err.Error()
		if stderrOut := errBufRestore.String(); stderrOut != "" {
			errMsg += " — " + strings.TrimSpace(stderrOut)
		}
		if isJSON {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Restore MySQL gagal: " + errMsg})
		} else {
			middleware.SetFlash(c, "error", "Restore MySQL gagal: "+errMsg)
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	if user := middleware.GetCurrentUser(c); user != nil {
		logActivity(user.ID, "restore", "Restore database berhasil menggunakan file: "+filename, "backup", "", c.ClientIP(), c.GetHeader("User-Agent"))
	}

	if isJSON {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Database berhasil di-restore dari " + filename})
	} else {
		middleware.SetFlash(c, "success", "Database berhasil di-restore.")
		c.Redirect(http.StatusFound, "/backup")
	}
}

func (h *BackupAdvancedHandler) ImportSQL(c *gin.Context) {
	isJSON := strings.Contains(c.ContentType(), "application/json")

	if ok, msg := restoreGuard(c); !ok {
		if isJSON {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": msg})
		} else {
			middleware.SetFlash(c, "error", msg)
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	confirm := c.PostForm("confirm")
	if isJSON {
		var req struct {
			Confirm bool `json:"confirm"`
		}
		if err := c.ShouldBindJSON(&req); err == nil && req.Confirm {
			confirm = "1"
		}
	}
	if confirm != "1" {
		if isJSON {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Import SQL harus dikirim dengan konfirmasi (confirm=1)"})
		} else {
			middleware.SetFlash(c, "error", "Import SQL harus dikirim dengan konfirmasi (confirm=1)")
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	file, err := c.FormFile("sql_file")
	if err != nil {
		if isJSON {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "File SQL wajib diunggah: " + err.Error()})
		} else {
			middleware.SetFlash(c, "error", "File SQL wajib diunggah: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	if !strings.HasSuffix(strings.ToLower(file.Filename), ".sql") {
		if isJSON {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Hanya file dengan ekstensi .sql yang didukung"})
		} else {
			middleware.SetFlash(c, "error", "Hanya file dengan ekstensi .sql yang didukung")
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	uploadDir := filepath.Join(config.StorageDir(), "restore")
	os.MkdirAll(uploadDir, 0755)
	tempFilePath := filepath.Join(uploadDir, "import_"+uuid.New().String()+".sql")

	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		if isJSON {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menyimpan file upload: " + err.Error()})
		} else {
			middleware.SetFlash(c, "error", "Gagal menyimpan file upload: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}
	defer os.Remove(tempFilePath)

	// Modify the SQL content to temporarily disable constraints during import
	sqlContent, err := os.ReadFile(tempFilePath)
	if err != nil {
		if isJSON {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal membaca file SQL: " + err.Error()})
		} else {
			middleware.SetFlash(c, "error", "Gagal membaca file SQL: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	modifiedSQLPath := tempFilePath + ".modified.sql"
	modifiedContent := "SET FOREIGN_KEY_CHECKS=0;\nSET unique_checks=0;\nSET autocommit=0;\n" + string(sqlContent) + "\nCOMMIT;\nSET FOREIGN_KEY_CHECKS=1;\nSET unique_checks=1;\nSET autocommit=1;\n"
	if err := os.WriteFile(modifiedSQLPath, []byte(modifiedContent), 0644); err != nil {
		if isJSON {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menyiapkan file import: " + err.Error()})
		} else {
			middleware.SetFlash(c, "error", "Gagal menyiapkan file import: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}
	defer os.Remove(modifiedSQLPath)

	// Run mysql restore
	// mysql CLI is not available on Vercel serverless
	if !config.CanRestore() {
		if isJSON {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "SQL import tidak tersedia di Vercel atau mysql tidak ditemukan di sistem. Gunakan Aiven Console untuk import"})
		} else {
			middleware.SetFlash(c, "error", "SQL import tidak tersedia di Vercel atau mysql tidak ditemukan di sistem.")
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	dbUser := os.Getenv("DB_USERNAME")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_DATABASE")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	if dbHost == "" {
		dbHost = "127.0.0.1"
	}
	if dbPort == "" {
		dbPort = "3306"
	}
	args := []string{"--host=" + dbHost, "--port=" + dbPort, "--user=" + dbUser, dbName}
	cmd := exec.Command("mysql", args...)
	if dbPass != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+dbPass)
	}

	inFile, err := os.Open(modifiedSQLPath)
	if err != nil {
		if isJSON {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal membuka file import: " + err.Error()})
		} else {
			middleware.SetFlash(c, "error", "Gagal membuka file import: "+err.Error())
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}
	defer inFile.Close()
	var errBufRestore strings.Builder
	cmd.Stderr = &errBufRestore
	cmd.Stdin = inFile
	if err := cmd.Run(); err != nil {
		errMsg := err.Error()
		if stderrOut := errBufRestore.String(); stderrOut != "" {
			errMsg += " — " + strings.TrimSpace(stderrOut)
		}
		if isJSON {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal mengimpor database: " + errMsg})
		} else {
			middleware.SetFlash(c, "error", "Gagal mengimpor database: "+errMsg)
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	if user := middleware.GetCurrentUser(c); user != nil {
		logActivity(user.ID, "restore", "Import database .sql berhasil menggunakan file: "+file.Filename, "backup", "", c.ClientIP(), c.GetHeader("User-Agent"))
	}

	if isJSON {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Database berhasil di-import dari " + file.Filename})
	} else {
		middleware.SetFlash(c, "success", "Database berhasil di-import dari "+file.Filename)
		c.Redirect(http.StatusFound, "/backup")
	}
}

func (h *BackupAdvancedHandler) Cleanup(c *gin.Context) {
	isJSON := strings.Contains(c.ContentType(), "application/json")
	days := 30

	if isJSON {
		var req struct {
			Days int `json:"days"`
		}
		if err := c.ShouldBindJSON(&req); err == nil && req.Days > 0 {
			days = req.Days
		}
	} else {
		if v := c.PostForm("days"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				days = d
			}
		}
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var old []models.BackupLog
	database.DB.Where("created_at < ?", cutoff).Find(&old)
	for _, log := range old {
		os.Remove(log.FilePath)
		database.DB.Delete(&log)
	}
	if isJSON {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("%d backup lama berhasil dibersihkan.", len(old))})
	} else {
		middleware.SetFlash(c, "success", fmt.Sprintf("%d backup lama berhasil dibersihkan.", len(old)))
		c.Redirect(http.StatusFound, "/backup")
	}
}

type BlockchainAdvancedHandler struct{}

func (h *BlockchainAdvancedHandler) Show(c *gin.Context) {
	var record models.BlockchainAudit
	if err := database.DB.Where("block_number = ?", c.Param("blockNumber")).First(&record).Error; err != nil {
		c.Redirect(http.StatusFound, "/blockchain")
		return
	}

	// Resolve entity name
	entityName := record.EntityID
	if record.EntityType == "arsip" && record.EntityID != "" {
		var name string
		database.DB.Table("arsip").Select("nama_arsip").Where("id = ?", record.EntityID).Take(&name)
		if name != "" {
			entityName = name
		}
	}

	// Resolve user name
	userName := ""
	if record.UserID != nil && *record.UserID != "" {
		var name string
		database.DB.Table("users").Select("name").Where("id = ?", *record.UserID).Take(&name)
		userName = name
	}

	// Get adjacent blocks for navigation
	var prevBlock, nextBlock *models.BlockchainAudit
	var temp models.BlockchainAudit
	if record.BlockNumber > 1 {
		if err := database.DB.Where("block_number = ?", record.BlockNumber-1).First(&temp).Error; err == nil {
			prevBlock = &temp
		}
	}
	if err := database.DB.Where("block_number = ?", record.BlockNumber+1).First(&temp).Error; err == nil {
		nextBlock = &temp
	}

	// Verify this block's integrity
	blockValid := true
	if record.PreviousHash == "" && record.BlockNumber > 1 {
		blockValid = false
	}
	if record.PreviousHash != "" {
		var prev models.BlockchainAudit
		if err := database.DB.Where("block_number = ?", record.BlockNumber-1).First(&prev).Error; err == nil {
			if record.PreviousHash != prev.CurrentHash {
				blockValid = false
			}
		}
	}

	Render(c, 200, "blockchain/show.html", gin.H{
		"title": "Block Detail", "pageTitle": "Detail Block",
		"Block":         record,
		"EntityName":    entityName,
		"UserName":      userName,
		"BlockValid":    blockValid,
		"PreviousBlock": prevBlock,
		"NextBlock":     nextBlock,
	})
}

func (h *BlockchainAdvancedHandler) Verify(c *gin.Context) {
	svc := &services.BlockchainAuditService{}
	result := svc.VerifyChain()

	isJSON := c.GetHeader("Content-Type") == "application/json" || strings.Contains(c.GetHeader("Accept"), "application/json")
	if isJSON {
		c.JSON(http.StatusOK, gin.H{
			"success": result["is_valid"],
			"data":    result,
		})
		return
	}

	middleware.SetFlash(c, "success", fmt.Sprintf("Verifikasi selesai. Valid: %v", result["is_valid"]))
	c.Redirect(http.StatusFound, "/blockchain")
}

func (h *BlockchainAdvancedHandler) SearchByHash(c *gin.Context) {
	hash := c.Query("hash")
	if hash == "" {
		c.JSON(http.StatusOK, gin.H{"found": false})
		return
	}
	var record models.BlockchainAudit
	if err := database.DB.Where("current_hash = ?", hash).First(&record).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"found": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"found":        true,
		"block_number": record.BlockNumber,
		"action":       record.Action,
		"entity_type":  record.EntityType,
		"entity_id":    record.EntityID,
		"timestamp":    record.Timestamp,
	})
}

func (h *BlockchainAdvancedHandler) Export(c *gin.Context) {
	var records []models.BlockchainAudit
	database.DB.Order("block_number ASC").Find(&records)
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=blockchain-audit.json")
	json.NewEncoder(c.Writer).Encode(records)
}

func (h *BlockchainAdvancedHandler) EntityAudit(c *gin.Context) {
	entityType := c.Param("entityType")
	entityId := c.Param("entityId")

	var records []models.BlockchainAudit
	database.DB.Where("entity_type = ? AND entity_id = ?", entityType, entityId).Order("block_number DESC").Find(&records)

	// We need to pass the raw records and let template parse JSON or parse it here
	type ParsedAudit struct {
		models.BlockchainAudit
		ParsedDetails map[string]interface{}
	}

	var parsedRecords []ParsedAudit
	for _, r := range records {
		var details map[string]interface{}
		json.Unmarshal([]byte(r.Details), &details)
		parsedRecords = append(parsedRecords, ParsedAudit{
			BlockchainAudit: r,
			ParsedDetails:   details,
		})
	}

	Render(c, 200, "blockchain/entity-audit.html", gin.H{
		"title": "Audit Trail - SIMARC", "pageTitle": "Entity Audit Trail",
		"Records":    parsedRecords,
		"EntityType": entityType,
		"EntityID":   entityId,
	})
}

// ── JADWAL RETENSI ADVANCED ─────────────────────────────────────────────────

type JadwalRetensiAdvancedHandler struct{}

func (h *JadwalRetensiAdvancedHandler) Calendar(c *gin.Context) {
	var list []models.JadwalRetensi
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Find(&list)
	type CalendarEvent struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Date  string `json:"date"`
		Kode  string `json:"kode"`
	}
	var events []CalendarEvent
	for _, j := range list {
		kode := ""
		if j.KodeKlasifikasi != nil {
			kode = j.KodeKlasifikasi.KodeKlasifikasi
		}
		events = append(events, CalendarEvent{ID: j.ID, Title: j.NamaJadwal, Date: j.CreatedAt.Format("2006-01-02"), Kode: kode})
	}
	Render(c, 200, "jadwal-retensi/calendar.html", gin.H{
		"title": "Kalender Retensi", "pageTitle": "Kalender Retensi", "events": events, "List": list,
	})
}

func (h *JadwalRetensiAdvancedHandler) AutoCreate(c *gin.Context) {
	var klasifikasiList []models.KodeKlasifikasi
	database.DB.Where("is_active = 1 AND (retensi_aktif > 0 OR retensi_inaktif > 0)").Find(&klasifikasiList)
	user := middleware.GetCurrentUser(c)
	var userID *string
	if user != nil {
		userID = &user.ID
	}
	count := 0
	for _, kk := range klasifikasiList {
		var existing models.JadwalRetensi
		if database.DB.Where("kode_klasifikasi_id = ?", kk.ID).First(&existing).Error != nil {
			jr := models.JadwalRetensi{
				ID: uuid.New().String(), NamaJadwal: "JRA " + kk.KodeKlasifikasi,
				KodeKlasifikasiID: &kk.ID, RetensiAktif: kk.RetensiAktif, RetensiInaktif: kk.RetensiInaktif,
				Nasib:       kk.PenyusutanArsip,
				Deskripsi:   "Auto-created from classification: " + kk.NamaKlasifikasi,
				Status:      "draft",
				JenisJadwal: "penyusutan",
				CreatedBy:   userID,
			}
			database.DB.Create(&jr)
			count++
		}
	}
	middleware.SetFlash(c, "success", fmt.Sprintf("%d jadwal retensi otomatis berhasil dibuat.", count))
	c.Redirect(http.StatusFound, "/jadwal-retensi")
}

func (h *JadwalRetensiAdvancedHandler) SearchArsip(c *gin.Context) {
	q := c.Query("q")
	var results []models.Arsip
	if q != "" {
		database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
			Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'')) @@ plainto_tsquery('simple', ?))", q).Limit(20).Find(&results)
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}

func (h *JadwalRetensiAdvancedHandler) Schedule(c *gin.Context) {
	var jr models.JadwalRetensi
	database.DB.First(&jr, "id = ?", c.Param("id"))
	arsipIDs := c.PostFormArray("arsip_ids[]")
	for _, aid := range arsipIDs {
		ja := models.JadwalRetensiArsip{
			ID: uuid.New().String(), JadwalRetensiID: jr.ID, ArsipID: aid,
			Status: "pending",
		}
		database.DB.Create(&ja)
	}
	middleware.SetFlash(c, "success", fmt.Sprintf("%d arsip ditambahkan ke jadwal retensi.", len(arsipIDs)))
	c.Redirect(http.StatusFound, "/jadwal-retensi/"+jr.ID)
}

func (h *JadwalRetensiAdvancedHandler) StartExecution(c *gin.Context) {
	var jr models.JadwalRetensi
	database.DB.First(&jr, "id = ?", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Eksekusi retensi dimulai untuk: " + jr.NamaJadwal})
}

func (h *JadwalRetensiAdvancedHandler) ExecuteDisposal(c *gin.Context) {
	var items []models.JadwalRetensiArsip
	database.DB.Where("jadwal_retensi_id = ?", c.Param("id")).Find(&items)
	now := time.Now()
	count := 0
	for _, item := range items {
		database.DB.Model(&models.Arsip{}).Where("id = ?", item.ArsipID).Update("status_arsip", "musnah")
		count++
	}
	database.DB.Model(&models.JadwalRetensiArsip{}).Where("jadwal_retensi_id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "processed", "processed_at": now,
	})
	database.DB.Model(&models.JadwalRetensi{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "completed", "tanggal_selesai": now,
	})
	middleware.SetFlash(c, "success", fmt.Sprintf("%d arsip berhasil dimusnahkan.", count))
	c.Redirect(http.StatusFound, "/jadwal-retensi/"+c.Param("id"))
}

func (h *JadwalRetensiAdvancedHandler) Cancel(c *gin.Context) {
	database.DB.Where("jadwal_retensi_id = ?", c.Param("id")).Delete(&models.JadwalRetensiArsip{})
	middleware.SetFlash(c, "success", "Jadwal retensi dibatalkan.")
	c.Redirect(http.StatusFound, "/jadwal-retensi")
}

func (h *JadwalRetensiAdvancedHandler) ProcessArchive(c *gin.Context) {
	var ja models.JadwalRetensiArsip
	database.DB.First(&ja, "id = ?", c.Param("jadwalArsipId"))
	now := time.Now()
	database.DB.Model(&models.JadwalRetensiArsip{}).Where("id = ?", c.Param("jadwalArsipId")).Updates(map[string]interface{}{
		"status": "processed", "processed_at": now,
	})
	database.DB.Model(&models.Arsip{}).Where("id = ?", ja.ArsipID).Update("status_arsip", "siap_penyusutan")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *JadwalRetensiAdvancedHandler) Show(c *gin.Context) {
	var m models.JadwalRetensi
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("Creator").First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/jadwal-retensi")
		return
	}
	var items []models.JadwalRetensiArsip
	database.DB.Preload("Arsip").Preload("Arsip.UnitKerja").Where("jadwal_retensi_id = ?", m.ID).Find(&items)

	pending, processed, skipped := 0, 0, 0
	for _, item := range items {
		switch item.Status {
		case "pending":
			pending++
		case "processed":
			processed++
		case "skipped":
			skipped++
		}
	}

	Render(c, 200, "jadwal-retensi/show.html", gin.H{
		"title": m.NamaJadwal, "pageTitle": "Detail Jadwal Retensi",
		"Item":            m,
		"ArsipList":       items,
		"ArsipTotal":      len(items),
		"ArsipFirstItem":  1,
		"ArsipPagination": "",
		"Stats": gin.H{
			"Pending":   pending,
			"Processed": processed,
			"Skipped":   skipped,
		},
	})
}

func (h *JadwalRetensiAdvancedHandler) Create(c *gin.Context) {
	var kkOpts []models.KodeKlasifikasi
	var ukOpts []models.UnitKerja
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kkOpts)
	database.DB.Order("nama_unit").Find(&ukOpts)
	Render(c, 200, "jadwal-retensi/create.html", gin.H{
		"title": "Tambah Jadwal Retensi", "pageTitle": "Tambah Jadwal Retensi", "kkOpts": kkOpts, "ukOpts": ukOpts,
	})
}

func (h *JadwalRetensiAdvancedHandler) Edit(c *gin.Context) {
	var m models.JadwalRetensi
	database.DB.First(&m, "id = ?", c.Param("id"))
	var kkOpts []models.KodeKlasifikasi
	var ukOpts []models.UnitKerja
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kkOpts)
	database.DB.Order("nama_unit").Find(&ukOpts)
	Render(c, 200, "jadwal-retensi/edit.html", gin.H{
		"title": "Edit Jadwal Retensi", "pageTitle": "Edit Jadwal Retensi", "m": m, "kkOpts": kkOpts, "ukOpts": ukOpts,
	})
}

func (h *JadwalRetensiAdvancedHandler) AdvancedStore(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	ra, _ := strconv.Atoi(c.PostForm("retensi_aktif"))
	ri, _ := strconv.Atoi(c.PostForm("retensi_inaktif"))
	m := models.JadwalRetensi{
		ID: uuid.New().String(), NamaJadwal: c.PostForm("nama_jadwal"),
		Deskripsi: c.PostForm("deskripsi"), JenisJadwal: c.PostForm("jenis_jadwal"),
		Status: "draft", RetensiAktif: ra, RetensiInaktif: ri,
		Nasib: c.PostForm("nasib"), Keterangan: c.PostForm("keterangan"),
		Catatan: c.PostForm("catatan"),
	}
	if v := c.PostForm("kode_klasifikasi_id"); v != "" {
		m.KodeKlasifikasiID = &v
	}
	if v := c.PostForm("unit_kerja_id"); v != "" {
		m.UnitKerjaID = &v
	}
	if v := c.PostForm("tanggal_jadwal"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			m.TanggalJadwal = &t
		}
	}
	if user != nil {
		m.CreatedBy = &user.ID
	}
	database.DB.Create(&m)
	middleware.SetFlash(c, "success", "Jadwal retensi berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/advanced/retention")
}

func (h *JadwalRetensiAdvancedHandler) AdvancedUpdate(c *gin.Context) {
	var m models.JadwalRetensi
	database.DB.First(&m, "id = ?", c.Param("id"))
	ra, _ := strconv.Atoi(c.PostForm("retensi_aktif"))
	ri, _ := strconv.Atoi(c.PostForm("retensi_inaktif"))
	m.NamaJadwal = c.PostForm("nama_jadwal")
	m.Deskripsi = c.PostForm("deskripsi")
	m.JenisJadwal = c.PostForm("jenis_jadwal")
	m.Status = c.PostForm("status")
	m.RetensiAktif = ra
	m.RetensiInaktif = ri
	m.Nasib = c.PostForm("nasib")
	m.Keterangan = c.PostForm("keterangan")
	m.Catatan = c.PostForm("catatan")
	if v := c.PostForm("tanggal_jadwal"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			m.TanggalJadwal = &t
		}
	}
	database.DB.Save(&m)
	middleware.SetFlash(c, "success", "Jadwal retensi berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/advanced/retention")
}

func (h *JadwalRetensiAdvancedHandler) InitiateDestruction(c *gin.Context) {
	var m models.JadwalRetensi
	database.DB.First(&m, "id = ?", c.Param("id"))
	database.DB.Model(&m).Update("status", "in_progress")
	var items []models.JadwalRetensiArsip
	database.DB.Where("jadwal_retensi_id = ?", m.ID).Find(&items)
	for _, item := range items {
		database.DB.Model(&models.JadwalRetensiArsip{}).Where("id = ?", item.ID).Update("status", "processed")
		database.DB.Model(&models.Arsip{}).Where("id = ?", item.ArsipID).Update("status_arsip", "siap_penyusutan")
	}
	middleware.SetFlash(c, "success", "Destruksi dimulai.")
	c.Redirect(http.StatusFound, "/advanced/retention")
}

func (h *JadwalRetensiAdvancedHandler) ExecuteDestruction(c *gin.Context) {
	var items []models.JadwalRetensiArsip
	database.DB.Where("jadwal_retensi_id = ?", c.Param("id")).Find(&items)
	now := time.Now()
	count := 0
	for _, item := range items {
		database.DB.Model(&models.Arsip{}).Where("id = ?", item.ArsipID).Update("status_arsip", "musnah")
		count++
	}
	database.DB.Model(&models.JadwalRetensiArsip{}).Where("jadwal_retensi_id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "processed", "processed_at": now,
	})
	database.DB.Model(&models.JadwalRetensi{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "completed", "tanggal_selesai": now,
	})
	middleware.SetFlash(c, "success", fmt.Sprintf("%d arsip berhasil dimusnahkan.", count))
	c.Redirect(http.StatusFound, "/advanced/retention")
}

func (h *JadwalRetensiAdvancedHandler) CancelRetention(c *gin.Context) {
	var m models.JadwalRetensi
	database.DB.First(&m, "id = ?", c.Param("id"))
	database.DB.Model(&m).Update("status", "cancelled")
	database.DB.Where("jadwal_retensi_id = ?", m.ID).Delete(&models.JadwalRetensiArsip{})
	middleware.SetFlash(c, "success", "Jadwal retensi dibatalkan.")
	c.Redirect(http.StatusFound, "/advanced/retention")
}

func (h *JadwalRetensiAdvancedHandler) ApproveRetention(c *gin.Context) {
	middleware.SetFlash(c, "success", "Retensi berhasil disetujui.")
	c.Redirect(http.StatusFound, "/advanced/retention")
}

func (h *JadwalRetensiAdvancedHandler) DailyRetentionCheck(c *gin.Context) {
	svc := &services.RetentionSchedulerService{}
	count, _ := svc.CheckAndUpdateRetention()
	middleware.SetFlash(c, "success", fmt.Sprintf("Pengecekan harian selesai. %d arsip diperbarui.", count))
	c.Redirect(http.StatusFound, "/advanced/retention")
}

// ── PEMINJAMAN ADVANCED ─────────────────────────────────────────────────────

type PeminjamanAdvancedHandler struct{}

func (h *PeminjamanAdvancedHandler) Create(c *gin.Context) {
	var arsipOpts []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Where("status_arsip = 'aktif'").Order("nomor_arsip").Find(&arsipOpts)
	Render(c, 200, "peminjaman/create.html", gin.H{
		"title": "Ajukan Peminjaman", "pageTitle": "Ajukan Peminjaman Arsip", "arsipOpts": arsipOpts,
	})
}

func (h *PeminjamanAdvancedHandler) Reject(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	database.DB.Model(&models.PeminjamanArsip{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "ditolak", "approved_by": user.ID,
	})
	middleware.SetFlash(c, "success", "Peminjaman ditolak.")
	c.Redirect(http.StatusFound, "/peminjaman")
}

// ── SETTINGS / THEME ────────────────────────────────────────────────────────

type SettingsThemeHandler struct{}

func (h *SettingsThemeHandler) Index(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	var themeMap map[string]string
	if user.ThemeSettings != "" {
		json.Unmarshal([]byte(user.ThemeSettings), &themeMap)
	}
	if themeMap == nil {
		themeMap = map[string]string{
			"primary_color":   "#1e3a8a",
			"secondary_color": "#2563eb",
			"accent_color":    "#06b6d4",
			"sidebar_theme":   "dark",
			"header_theme":    "light",
			"border_radius":   "12",
		}
	}
	Render(c, 200, "settings/index.html", gin.H{
		"title": "Pengaturan", "pageTitle": "Pengaturan Tampilan",
		"themeSettings": themeMap,
	})
}

func (h *SettingsThemeHandler) UpdateTheme(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	theme := map[string]string{
		"primary_color":   c.PostForm("primary_color"),
		"secondary_color": c.PostForm("secondary_color"),
		"accent_color":    c.PostForm("accent_color"),
		"sidebar_theme":   c.PostForm("sidebar_theme"),
		"header_theme":    c.PostForm("header_theme"),
		"border_radius":   c.PostForm("border_radius"),
	}
	if theme["primary_color"] == "" {
		theme["primary_color"] = "#1e3a8a"
	}
	if theme["secondary_color"] == "" {
		theme["secondary_color"] = "#2563eb"
	}
	if theme["accent_color"] == "" {
		theme["accent_color"] = "#06b6d4"
	}
	if theme["sidebar_theme"] == "" {
		theme["sidebar_theme"] = "dark"
	}
	if theme["header_theme"] == "" {
		theme["header_theme"] = "light"
	}
	if theme["border_radius"] == "" {
		theme["border_radius"] = "12"
	}
	data, _ := json.Marshal(theme)
	database.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("theme_settings", string(data))
	middleware.SetFlash(c, "success", "Tema berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/settings")
}

func (h *SettingsThemeHandler) ResetTheme(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	database.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("theme_settings", "")
	middleware.SetFlash(c, "success", "Tema berhasil direset.")
	c.Redirect(http.StatusFound, "/settings")
}

// ── PENGATURAN ADVANCED ─────────────────────────────────────────────────────

type PengaturanAdvancedHandler struct{}

func (h *PengaturanAdvancedHandler) Update(c *gin.Context) {
	appName := c.PostForm("app_name")
	timezone := c.PostForm("app_timezone")
	if appName == "" {
		appName = config.App.AppName
	}
	if timezone == "" {
		timezone = "Asia/Jakarta"
	}
	if err := saveAppSettings(appName, timezone); err != nil {
		middleware.SetFlash(c, "error", "Gagal menyimpan pengaturan ke file .env: "+err.Error())
	} else {
		middleware.SetFlash(c, "success", "Pengaturan berhasil diperbarui.")
	}
	c.Redirect(http.StatusFound, "/pengaturan")
}

func (h *PengaturanAdvancedHandler) ClearCache(c *gin.Context) {
	database.DB.Where("1 = 1").Delete(&models.SavedSearch{})
	middleware.SetFlash(c, "success", "Cache berhasil dibersihkan.")
	c.Redirect(http.StatusFound, "/pengaturan")
}

func (h *PengaturanAdvancedHandler) SystemInfo(c *gin.Context) {
	hostname, _ := os.Hostname()

	// Ambil versi database
	var dbVersion string
	database.DB.Raw("SELECT VERSION()").Scan(&dbVersion)

	// Hitung statistik
	var totalArsip, totalUsers, totalUnitKerja, totalRoles, totalPermissions int64
	database.DB.Model(&models.Arsip{}).Count(&totalArsip)
	database.DB.Model(&models.User{}).Count(&totalUsers)
	database.DB.Model(&models.UnitKerja{}).Count(&totalUnitKerja)
	database.DB.Model(&models.Role{}).Count(&totalRoles)
	database.DB.Model(&models.Permission{}).Count(&totalPermissions)

	// Ambil pengaturan dari file
	settings := loadAppSettings()
	timezone := "Asia/Jakarta"
	if tz, ok := settings["app_timezone"].(string); ok && tz != "" {
		timezone = tz
	}

	// Info memori
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	info := gin.H{
		"go_version":         runtime.Version(),
		"go_os":              runtime.GOOS,
		"go_arch":            runtime.GOARCH,
		"hostname":           hostname,
		"app_name":           config.App.AppName,
		"app_url":            config.App.AppURL,
		"app_port":           config.App.AppPort,
		"app_debug":          config.App.AppDebug,
		"timezone":           timezone,
		"db_driver":          "MySQL (MariaDB)",
		"db_version":         dbVersion,
		"db_name":            config.App.DBName,
		"cpu_cores":          runtime.NumCPU(),
		"goroutines":         runtime.NumGoroutine(),
		"memory_alloc":       formatBytes(int64(memStats.Alloc)),
		"memory_total_alloc": formatBytes(int64(memStats.TotalAlloc)),
		"memory_sys":         formatBytes(int64(memStats.Sys)),
	}

	Render(c, 200, "pengaturan/system.html", gin.H{
		"title": "Info Sistem", "pageTitle": "Informasi Sistem",
		"Info": info,
		"Stats": gin.H{
			"TotalUsers":       totalUsers,
			"TotalArsip":       totalArsip,
			"TotalUnitKerja":   totalUnitKerja,
			"TotalRoles":       totalRoles,
			"TotalPermissions": totalPermissions,
		},
	})
}

// ── MOBILE API ──────────────────────────────────────────────────────────────

type MobileAPIHandler struct{}

func (h *MobileAPIHandler) Create(c *gin.Context) {
	var unitKerjaOpts []models.UnitKerja
	var kkOpts []models.KodeKlasifikasi
	database.DB.Order("nama_unit").Find(&unitKerjaOpts)
	database.DB.Where("is_active = 1").Find(&kkOpts)
	Render(c, 200, "mobile/create.html", gin.H{
		"title": "Tambah Arsip", "pageTitle": "Tambah Arsip", "unitKerjaOpts": unitKerjaOpts, "kkOpts": kkOpts,
	})
}

func (h *MobileAPIHandler) Store(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	arsip := models.Arsip{
		ID: uuid.New().String(), NamaArsip: c.PostForm("nama_arsip"),
		NomorArsip: c.PostForm("nomor_arsip"), Uraian: c.PostForm("uraian"),
		StatusArsip: "aktif", KodeKlasifikasiID: c.PostForm("kode_klasifikasi_id"),
		UnitKerjaID: c.PostForm("unit_kerja_id"),
	}
	if file, err := c.FormFile("file"); err == nil {
		uploadDir := filepath.Join(config.UploadDir(), "arsip")
		os.MkdirAll(uploadDir, 0755)
		ext := filepath.Ext(file.Filename)
		filename := fmt.Sprintf("%d_%s%s", time.Now().Unix(), arsip.ID[:8], ext)
		dst := filepath.Join(uploadDir, filename)
		if err := c.SaveUploadedFile(file, dst); err == nil {
			arsip.FilePath = dst
		}
	}
	database.DB.Create(&arsip)
	logActivity(user.ID, "create", "Menambah arsip (mobile): "+arsip.NamaArsip, "arsip", arsip.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Arsip berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/mobile/archives")
}

func (h *MobileAPIHandler) ScanQR(c *gin.Context) {
	qrData := c.PostForm("qr_data")
	if qrData == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "QR data kosong"})
		return
	}
	var qr models.QrCode
	if err := database.DB.Preload("Arsip").Where("qr_data = ?", qrData).First(&qr).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "QR Code tidak ditemukan"})
		return
	}
	log := models.QrScanLog{QrCodeID: qr.ID, IPAddress: c.ClientIP(), ScannedAt: time.Now()}
	database.DB.Create(&log)
	c.JSON(http.StatusOK, gin.H{"success": true, "arsip": qr.Arsip})
}

func (h *MobileAPIHandler) SearchAPI(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		q = c.PostForm("q")
	}
	var results []models.Arsip
	if q != "" {
		database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
			Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'')) @@ plainto_tsquery('simple', ?))", q).Limit(20).Find(&results)
	}
	c.JSON(http.StatusOK, gin.H{"data": results, "count": len(results)})
}

func (h *MobileAPIHandler) ArchiveAPI(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").
		First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Arsip tidak ditemukan"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": arsip})
}

func (h *MobileAPIHandler) OfflineData(c *gin.Context) {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Order("created_at DESC").Limit(100).Find(&list)
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list)})
}

func (h *MobileAPIHandler) UpdateSettings(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	theme := c.PostForm("theme")
	database.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("theme_settings", theme)
	middleware.SetFlash(c, "success", "Pengaturan berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/mobile/settings")
}

// ── GOOGLE APPS SCRIPT API ──────────────────────────────────────────────────

type AppsScriptAPIHandler struct{}

func (h *AppsScriptAPIHandler) GetArchiveSummary(c *gin.Context) {
	var total, aktif, inaktif, musnah int64
	database.DB.Model(&models.Arsip{}).Count(&total)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'aktif' AND deleted_at IS NULL").Count(&aktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'inaktif' AND deleted_at IS NULL").Count(&inaktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'musnah' AND deleted_at IS NULL").Count(&musnah)
	c.JSON(http.StatusOK, gin.H{
		"total": total, "aktif": aktif, "inaktif": inaktif, "musnah": musnah,
	})
}

func (h *AppsScriptAPIHandler) GetClassificationDistribution(c *gin.Context) {
	type Row struct {
		Kode  string `gorm:"column:kode"`
		Nama  string `gorm:"column:nama"`
		Total int64  `gorm:"column:total"`
	}
	var rows []Row
	database.DB.Raw(`SELECT kk.kode_klasifikasi as kode, kk.nama_klasifikasi as nama, COUNT(a.id) as total
		FROM kode_klasifikasi kk LEFT JOIN arsip a ON a.kode_klasifikasi_id = kk.id AND a.deleted_at IS NULL
		GROUP BY kk.id, kk.kode_klasifikasi, kk.nama_klasifikasi ORDER BY total DESC`).Scan(&rows)
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

func (h *AppsScriptAPIHandler) GetArchivesByClassification(c *gin.Context) {
	kode := c.Param("kode")
	var kk models.KodeKlasifikasi
	if err := database.DB.Where("kode_klasifikasi = ?", kode).First(&kk).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Klasifikasi tidak ditemukan"})
		return
	}
	var list []models.Arsip
	database.DB.Preload("UnitKerja").Where("kode_klasifikasi_id = ?", kk.ID).Order("nomor_arsip").Find(&list)
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list)})
}

func (h *AppsScriptAPIHandler) GetTopStatistics(c *gin.Context) {
	type UnitRow struct {
		Nama  string `gorm:"column:nama"`
		Total int64  `gorm:"column:total"`
	}
	var units []UnitRow
	database.DB.Raw(`SELECT uk.nama_unit as nama, COUNT(a.id) as total FROM arsip a
		JOIN unit_kerja uk ON a.unit_kerja_id = uk.id WHERE a.deleted_at IS NULL
		GROUP BY uk.nama_unit ORDER BY total DESC LIMIT 5`).Scan(&units)
	var totalArsip, totalUsers int64
	database.DB.Model(&models.Arsip{}).Count(&totalArsip)
	database.DB.Model(&models.User{}).Count(&totalUsers)
	c.JSON(http.StatusOK, gin.H{
		"total_arsip": totalArsip, "total_users": totalUsers, "top_units": units,
	})
}

func (h *AppsScriptAPIHandler) SearchArchives(c *gin.Context) {
	q := c.Query("q")
	var results []models.Arsip
	if q != "" {
		like := "%" + q + "%"
		database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
			Where("(nama_arsip LIKE ? OR nomor_arsip LIKE ? OR uraian LIKE ? OR ocr_text LIKE ? OR tags LIKE ?)",
				like, like, like, like, like).
			Limit(50).Find(&results)
	}
	c.JSON(http.StatusOK, gin.H{"data": results, "count": len(results)})
}

func (h *AppsScriptAPIHandler) GetAllClassificationCodes(c *gin.Context) {
	var list []models.KodeKlasifikasi
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&list)
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// ── PREMIUM API V1 ──────────────────────────────────────────────────────────

type PremiumAPIHandler struct{}

func (h *PremiumAPIHandler) SmartSearch(c *gin.Context) {
	q := c.PostForm("q")
	if q == "" {
		var body struct {
			Query string `json:"q"`
		}
		c.ShouldBindJSON(&body)
		q = body.Query
	}
	svc := &services.DataScienceService{}
	results := svc.SemanticSearch(q)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": results, "count": len(results)})
}

func (h *PremiumAPIHandler) Analytics(c *gin.Context) {
	var total, aktif, inaktif, musnah, digital int64
	database.DB.Model(&models.Arsip{}).Count(&total)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'aktif' AND deleted_at IS NULL").Count(&aktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'inaktif' AND deleted_at IS NULL").Count(&inaktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'musnah' AND deleted_at IS NULL").Count(&musnah)
	database.DB.Model(&models.Arsip{}).Where("file_path IS NOT NULL AND file_path != '' AND deleted_at IS NULL").Count(&digital)
	svc := &services.AnalyticsService{}
	growth := svc.GetArsipGrowth(12)
	c.JSON(http.StatusOK, gin.H{
		"total": total, "aktif": aktif, "inaktif": inaktif, "musnah": musnah,
		"digital": digital, "growth": growth,
	})
}

func (h *PremiumAPIHandler) VerifyBlockchain(c *gin.Context) {
	svc := &services.BlockchainAuditService{}
	result := svc.VerifyChain()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *PremiumAPIHandler) AuditTrail(c *gin.Context) {
	entityType := c.Param("entityType")
	entityID := c.Param("entityId")
	var records []models.BlockchainAudit
	database.DB.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Order("id ASC").Find(&records)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": records, "count": len(records)})
}

// ── OCR ADVANCED ────────────────────────────────────────────────────────────

type OcrAdvancedHandler struct{}

func (h *OcrAdvancedHandler) Download(c *gin.Context) {
	text := c.PostForm("text")
	filename := c.PostForm("filename")
	if text == "" && c.GetHeader("Content-Type") == "application/json" {
		var req struct {
			Text     string `json:"text"`
			Filename string `json:"filename"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			text = req.Text
			filename = req.Filename
		}
	}
	if filename == "" {
		filename = "ocr-result"
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.txt", filename))
	c.Writer.WriteString(text)
}

func (h *OcrAdvancedHandler) Status(c *gin.Context) {
	var total, completed int64
	database.DB.Model(&models.OcrLog{}).Count(&total)
	database.DB.Model(&models.OcrLog{}).Where("status = 'completed'").Count(&completed)
	tessAvailable := isTesseractAvailable()
	status := "healthy"
	details := gin.H{"tesseract": "available"}
	if !tessAvailable {
		status = "limited"
		details = gin.H{"tesseract": "missing"}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": status, "details": details,
		"total": total, "completed": completed, "pending": total - completed,
		"tesseract_available": tessAvailable,
	})
}

func isTesseractAvailable() bool {
	_, err := exec.LookPath("tesseract")
	return err == nil
}

// ── SEARCH ADVANCED ─────────────────────────────────────────────────────────

type SearchAdvancedHandler struct{}

func (h *SearchAdvancedHandler) Suggestions(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}
	var results []struct {
		ID         string `json:"id"`
		Text       string `json:"text"`
		NomorArsip string `json:"nomor_arsip"`
	}
	database.DB.Model(&models.Arsip{}).
		Select("id, nama_arsip as text, nomor_arsip").
		Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'')) @@ plainto_tsquery('simple', ?))", q).
		Limit(10).Find(&results)
	c.JSON(http.StatusOK, results)
}

// ── ARCHIVAL SUPERVISION ────────────────────────────────────────────────────

type ArchivalSupervisionHandler struct{}

func (h *ArchivalSupervisionHandler) DownloadCertificate(c *gin.Context) {
	arsipID := c.Param("id")
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").First(&arsip, "id = ?", arsipID).Error; err != nil {
		c.String(http.StatusNotFound, "Arsip tidak ditemukan")
		return
	}
	user := middleware.GetCurrentUser(c)
	Render(c, 200, "supervision/certificate.html", gin.H{
		"title": "Sertifikat Arsip", "arsip": arsip, "User": user,
		"certificateDate": time.Now().Format("02 January 2006"),
		"certificateID":   fmt.Sprintf("CERT-%s-%d", arsipID[:8], time.Now().Unix()),
	})
}

// ── DASHBOARD PREMIUM / CHAMPION ────────────────────────────────────────────

func DashboardPremium(c *gin.Context) {
	var totalArsip, digitalArsip, unitKerja int64
	database.DB.Model(&models.Arsip{}).Count(&totalArsip)
	database.DB.Model(&models.Arsip{}).Where("jenis_arsip = 'Digital'").Count(&digitalArsip)
	database.DB.Model(&models.UnitKerja{}).Count(&unitKerja)
	var recentActivities []models.ActivityLog
	database.DB.Preload("User").Order("created_at DESC").Limit(5).Find(&recentActivities)
	Render(c, 200, "dashboard/ultra-premium-simple.html", gin.H{
		"title": "Dashboard - Ultra Premium", "totalArsip": totalArsip,
		"digitalArsip": digitalArsip, "peminjamanAktif": 0,
		"unitKerja": unitKerja, "recentActivities": recentActivities,
	})
}

func DashboardChampion(c *gin.Context) {
	var stats struct {
		Total          int64 `gorm:"column:total"`
		TotalItems     int64 `gorm:"column:total_items"`
		Aktif          int64 `gorm:"column:aktif"`
		Inaktif        int64 `gorm:"column:inaktif"`
		SiapPenyusutan int64 `gorm:"column:siap_penyusutan"`
		Musnah         int64 `gorm:"column:musnah"`
		Permanen       int64 `gorm:"column:permanen"`
		WithFile       int64 `gorm:"column:with_file"`
		WithoutFile    int64 `gorm:"column:without_file"`
	}
	database.DB.Raw(`
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(jumlah), 0) as total_items,
			SUM(CASE WHEN status_arsip IN ('aktif','diberkaskan') THEN 1 ELSE 0 END) as aktif,
			SUM(CASE WHEN status_arsip = 'inaktif' THEN 1 ELSE 0 END) as inaktif,
			SUM(CASE WHEN status_arsip = 'siap_penyusutan' THEN 1 ELSE 0 END) as siap_penyusutan,
			SUM(CASE WHEN status_arsip = 'musnah' THEN 1 ELSE 0 END) as musnah,
			SUM(CASE WHEN status_arsip = 'permanen' THEN 1 ELSE 0 END) as permanen,
			SUM(CASE WHEN file_path IS NOT NULL AND file_path != '' THEN 1 ELSE 0 END) as with_file,
			SUM(CASE WHEN file_path IS NULL OR file_path = '' THEN 1 ELSE 0 END) as without_file
		FROM arsip WHERE deleted_at IS NULL
	`).Scan(&stats)

	digitalPercent := 0.0
	if stats.Total > 0 {
		digitalPercent = float64(stats.WithFile) / float64(stats.Total) * 100
	}

	var blockchainCount int64
	database.DB.Table("blockchain_audits").Count(&blockchainCount)

	var recentActivities []struct {
		Action    string    `gorm:"column:action"`
		UserName  string    `gorm:"column:user_name"`
		CreatedAt time.Time `gorm:"column:created_at"`
		Desc      string    `gorm:"column:description"`
	}
	database.DB.Raw(`
		SELECT al.action, u.name as user_name, al.created_at, al.description
		FROM activity_logs al
		LEFT JOIN users u ON al.user_id = u.id
		ORDER BY al.created_at DESC LIMIT 5
	`).Scan(&recentActivities)

	var monthlyStats []struct {
		Month string `gorm:"column:month"`
		Total int64  `gorm:"column:total"`
	}
	database.DB.Raw(`
		SELECT TO_CHAR(created_at, 'YYYY-MM') as month, COUNT(*) as total
		FROM arsip WHERE deleted_at IS NULL
		GROUP BY month ORDER BY month ASC LIMIT 12
	`).Scan(&monthlyStats)

	supervisionSvc := &services.ArchivalSupervisionService{}
	leaderboard := supervisionSvc.GetLeaderboard()
	averageCompliance := supervisionSvc.GetAverageCompliance()

	Render(c, 200, "dashboard/premium-champion.html", gin.H{
		"title":             "Dashboard Juara 1 - SIMARC",
		"Stats":             stats,
		"digitalPercent":    digitalPercent,
		"blockchainCount":   blockchainCount,
		"recentActivities":  recentActivities,
		"monthlyStats":      monthlyStats,
		"leaderboard":       leaderboard,
		"averageCompliance": averageCompliance,
	})
}

func DashboardRefresh(c *gin.Context) {
	cache.InvalidatePrefix("dashboard:")
	middleware.SetFlash(c, "success", "Dashboard berhasil di-refresh.")
	c.Redirect(http.StatusFound, "/dashboard")
}

// ── UNIT KERJA SHOW / USERS SHOW / KODE KLASIFIKASI SHOW ────────────────────

func (h *UnitKerjaHandler) Show(c *gin.Context) {
	var m models.UnitKerja
	database.DB.First(&m, "id = ?", c.Param("id"))
	var arsipList []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Where("unit_kerja_id = ?", m.ID).Order("nomor_arsip").Find(&arsipList)
	Render(c, 200, "unit-kerja/show.html", gin.H{
		"title": m.NamaUnit, "pageTitle": "Detail Unit Kerja", "m": m, "ArsipList": arsipList,
	})
}

func (h *UserHandler) Show(c *gin.Context) {
	var user models.User
	if err := database.DB.Preload("Role").Preload("UnitKerja").First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/users")
		return
	}
	Render(c, 200, "users/show.html", gin.H{"title": user.Name, "pageTitle": "Detail Pengguna", "User": user})
}

func (h *KodeKlasifikasiHandler) Show(c *gin.Context) {
	var m models.KodeKlasifikasi
	if err := database.DB.Preload("Parent").Preload("Children").First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/kode-klasifikasi")
		return
	}
	var arsipList []models.Arsip
	database.DB.Preload("UnitKerja").Where("kode_klasifikasi_id = ?", m.ID).Order("nomor_arsip").Find(&arsipList)
	Render(c, 200, "kode-klasifikasi/show.html", gin.H{
		"title": m.NamaKlasifikasi, "pageTitle": "Detail Kode Klasifikasi", "m": m, "ArsipList": arsipList,
	})
}

// ── PEMBERKASAN ISI ─────────────────────────────────────────────────────────

func (h *PemberkasanHandler) ShowIsi(c *gin.Context) {
	var m models.Pemberkasan
	if err := database.DB.Preload("Creator").Preload("UnitKerja").Preload("KodeKlasifikasi").
		Preload("Arsip").Preload("Arsip.KodeKlasifikasi").Preload("Arsip.UnitKerja").
		First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/pemberkasan")
		return
	}
	Render(c, 200, "pemberkasan/isi.html", gin.H{
		"title": "Isi Pemberkasan", "pageTitle": "Isi Berkas: " + m.NamaPemberkasan, "Item": m,
		"Stats": gin.H{
			"TotalArsip":   len(m.Arsip),
			"ArsipAktif":   0,
			"ArsipInaktif": 0,
		},
	})
}

// ── PEMUSNAHAN ADVANCED ─────────────────────────────────────────────────────

func (h *PemusnahanHandler) Show(c *gin.Context) {
	var m models.PemusnahanArsip
	if err := database.DB.Preload("Creator").
		Preload("UserPengaju").Preload("UserApprove").
		Preload("Items.Arsip.KodeKlasifikasi").Preload("Items.Arsip.UnitKerja").
		First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/pemusnahan")
		return
	}
	Render(c, 200, "pemusnahan/show.html", gin.H{"title": "Detail Pemusnahan", "pageTitle": "Detail Pemusnahan", "Item": m})
}

func (h *PemusnahanHandler) ExportExcel(c *gin.Context) {
	var list []models.PemusnahanArsip
	database.DB.Preload("Items.Arsip.KodeKlasifikasi").Preload("Items.Arsip.UnitKerja").Preload("Creator").Preload("UserPengaju").Preload("UserApprove").Order("created_at DESC").Find(&list)
	rows := [][]string{}
	for _, p := range list {
		pengaju := "-"
		if p.UserPengaju != nil {
			pengaju = p.UserPengaju.Name
		} else if p.Creator != nil {
			pengaju = p.Creator.Name
		}
		approver := "-"
		if p.UserApprove != nil {
			approver = p.UserApprove.Name
		}
		tgl := "-"
		if p.TanggalPengajuan != nil {
			tgl = p.TanggalPengajuan.Format("2006-01-02")
		}
		status := p.Status
		alasan := p.AlasanPengajuan
		if len(p.Items) > 0 {
			for _, item := range p.Items {
				arsipNomor := "-"
				arsipNama := "-"
				uraian := "-"
				kk := ""
				uk := ""
				if item.Arsip != nil {
					arsipNomor = item.Arsip.NomorArsip
					arsipNama = item.Arsip.NamaArsip
					uraian = item.Arsip.Uraian
					if item.Arsip.KodeKlasifikasi != nil {
						kk = item.Arsip.KodeKlasifikasi.KodeKlasifikasi
					}
					if item.Arsip.UnitKerja != nil {
						uk = item.Arsip.UnitKerja.NamaUnit
					}
				}
				rows = append(rows, []string{p.NamaKegiatan, arsipNomor, arsipNama, uraian, kk, uk, alasan, status, tgl, pengaju, approver})
			}
		} else {
			// Fallback for empty items (backward compat with legacy single arsip)
			rows = append(rows, []string{p.NamaKegiatan, "-", "-", "-", "", "", alasan, status, tgl, pengaju, approver})
		}
	}
	exportXLSX(c, "Daftar-Pemusnahan-Arsip-"+time.Now().Format("2006-01-02"), []string{"Kegiatan", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Unit Kerja", "Alasan", "Status", "Tgl Pengajuan", "Pengaju", "Persetujuan"}, rows)
}

func (h *PemusnahanHandler) ExportPDF(c *gin.Context) {
	var list []models.PemusnahanArsip
	database.DB.Preload("Items.Arsip.KodeKlasifikasi").Preload("Items.Arsip.UnitKerja").Preload("Creator").Preload("UserPengaju").Preload("UserApprove").Order("created_at DESC").Find(&list)
	headers := []string{"No", "Kegiatan", "Nomor Arsip", "Nama Arsip", "Klasifikasi", "Unit Kerja", "Status", "Tgl Pengajuan"}
	rows := [][]string{}
	rowNum := 1
	for _, p := range list {
		tgl := "-"
		if p.TanggalPengajuan != nil {
			tgl = p.TanggalPengajuan.Format("02 Jan 2006")
		}
		if len(p.Items) > 0 {
			for _, item := range p.Items {
				arsipNomor := "-"
				arsipNama := "-"
				kk := ""
				uk := ""
				if item.Arsip != nil {
					arsipNomor = item.Arsip.NomorArsip
					arsipNama = item.Arsip.NamaArsip
					if item.Arsip.KodeKlasifikasi != nil {
						kk = item.Arsip.KodeKlasifikasi.KodeKlasifikasi
					}
					if item.Arsip.UnitKerja != nil {
						uk = item.Arsip.UnitKerja.NamaUnit
					}
				}
				rows = append(rows, []string{strconv.Itoa(rowNum), p.NamaKegiatan, arsipNomor, arsipNama, kk, uk, p.Status, tgl})
				rowNum++
			}
		} else {
			rows = append(rows, []string{strconv.Itoa(rowNum), p.NamaKegiatan, "-", "-", "", "", p.Status, tgl})
			rowNum++
		}
	}
	exportPDF(c, "Daftar-Pemusnahan-Arsip-"+time.Now().Format("2006-01-02"), "Daftar Pemusnahan Arsip", headers, rows)
}

func (h *PemusnahanHandler) SearchArsip(c *gin.Context) {
	q := c.Query("q")
	var results []models.Arsip
	if q != "" {
		database.DB.Preload("KodeKlasifikasi").
			Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'')) @@ plainto_tsquery('simple', ?)) AND status_arsip != 'musnah'", q).
			Limit(20).Find(&results)
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}

func (h *PemusnahanHandler) GetArsipDetail(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Arsip tidak ditemukan"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": arsip})
}

// ── PROFIL ADVANCED ─────────────────────────────────────────────────────────

func (h *ProfilHandler) EditPassword(c *gin.Context) {
	Render(c, 200, "profil/password.html", gin.H{"title": "Ubah Password", "pageTitle": "Ubah Password"})
}

func (h *ProfilHandler) UpdatePassword(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	currentPw := c.PostForm("current_password")
	newPw := c.PostForm("new_password")
	confirmPw := c.PostForm("confirm_password")
	if newPw != confirmPw {
		middleware.SetFlash(c, "error", "Password baru tidak cocok.")
		c.Redirect(http.StatusFound, "/profil/password")
		return
	}
	if len(newPw) < 8 {
		middleware.SetFlash(c, "error", "Password minimal 8 karakter.")
		c.Redirect(http.StatusFound, "/profil/password")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPw)); err != nil {
		middleware.SetFlash(c, "error", "Password lama salah.")
		c.Redirect(http.StatusFound, "/profil/password")
		return
	}
	hashed, _ := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	database.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("password", string(hashed))
	middleware.SetFlash(c, "success", "Password berhasil diubah.")
	c.Redirect(http.StatusFound, "/profil")
}

func decompressGzip(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gr.Close()

	_, err = io.Copy(out, gr)
	return err
}
