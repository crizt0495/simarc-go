package handlers

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"
	"arsippro/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

type BlockRecord struct {
	models.BlockchainAudit
	EntityName string
	UserName   string
}

// ── LAPORAN ───────────────────────────────────────────────────────────────────

type LaporanHandler struct{}

func (h *LaporanHandler) Index(c *gin.Context) {
	stats := gin.H{}
	var totalArsip, arsipAktif, arsipInaktif, totalPemusnahan, totalUnitKerja, totalBerkas int64
	database.DB.Raw(`SELECT
		(SELECT COUNT(*) FROM arsip WHERE deleted_at IS NULL) as total_arsip,
		(SELECT COUNT(*) FROM arsip WHERE deleted_at IS NULL AND status_arsip='aktif') as arsip_aktif,
		(SELECT COUNT(*) FROM arsip WHERE deleted_at IS NULL AND status_arsip='inaktif') as arsip_inaktif,
		(SELECT COUNT(*) FROM pemusnahan_arsip WHERE deleted_at IS NULL AND status='disetujui') as total_pemusnahan,
		(SELECT COUNT(*) FROM unit_kerja WHERE deleted_at IS NULL) as total_unit,
		(SELECT COUNT(*) FROM pemberkasan WHERE deleted_at IS NULL) as total_berkas
	`).Scan(&struct {
		TotalArsip     *int64 `gorm:"column:total_arsip"`
		ArsipAktif     *int64 `gorm:"column:arsip_aktif"`
		ArsipInaktif   *int64 `gorm:"column:arsip_inaktif"`
		TotalPemusnahan *int64 `gorm:"column:total_pemusnahan"`
		TotalUnitKerja *int64 `gorm:"column:total_unit"`
		TotalBerkas    *int64 `gorm:"column:total_berkas"`
	}{
		TotalArsip:     &totalArsip,
		ArsipAktif:     &arsipAktif,
		ArsipInaktif:   &arsipInaktif,
		TotalPemusnahan: &totalPemusnahan,
		TotalUnitKerja: &totalUnitKerja,
		TotalBerkas:    &totalBerkas,
	})
	var diberkaskanCount, layakMusnah, sudahMusnah int64
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'diberkaskan'").Count(&diberkaskanCount)
	database.DB.Model(&models.Arsip{}).Where("tanggal_retensi_berakhir < ? AND status_arsip != 'musnah'", time.Now()).Count(&layakMusnah)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'musnah'").Count(&sudahMusnah)
	stats = gin.H{
		"TotalArsip":      totalArsip,
		"ArsipAktif":      arsipAktif,
		"ArsipInaktif":    arsipInaktif,
		"ArsipDiberkaskan": diberkaskanCount,
		"LayakMusnah":     layakMusnah,
		"SudahDimusnahkan": sudahMusnah,
	}

	// Arsip per unit kerja
	type UnitStat struct {
		NamaUnit   string `gorm:"column:nama_unit"`
		JumlahArsip int64  `gorm:"column:total"`
	}
	var perUnit []UnitStat
	database.DB.Raw(`SELECT uk.nama_unit, COUNT(a.id) as total FROM arsip a
		JOIN unit_kerja uk ON a.unit_kerja_id = uk.id
		WHERE a.deleted_at IS NULL GROUP BY uk.nama_unit ORDER BY total DESC LIMIT 10`).Scan(&perUnit)

	// Arsip per status
	type StatusStat struct {
		Status string `gorm:"column:status_arsip"`
		Total  int64  `gorm:"column:total"`
	}
	var perStatus []StatusStat
	database.DB.Raw(`SELECT status_arsip, COUNT(*) as total FROM arsip WHERE deleted_at IS NULL GROUP BY status_arsip`).Scan(&perStatus)

	// Monthly trend
	type MonthlyStat struct {
		Month string `gorm:"column:month"`
		Total int64  `gorm:"column:total"`
	}
	var perBulan []MonthlyStat
	database.DB.Raw(`SELECT TO_CHAR(tanggal_dibuat, 'YYYY-MM') as month, COUNT(*) as total
		FROM arsip WHERE deleted_at IS NULL AND EXTRACT(YEAR FROM tanggal_dibuat) = ?
		GROUP BY month ORDER BY month`, time.Now().Year()).Scan(&perBulan)

	Render(c, 200, "laporan/index.html", gin.H{
		"title": "Laporan - SIMARC", "pageTitle": "Laporan & Statistik",
		"Stats": stats, "perUnit": perUnit, "perStatus": perStatus, "perBulan": perBulan,
	})
}

func applyLaporanArsipFilters(db *gorm.DB, c *gin.Context) *gorm.DB {
	if v := c.Query("unit_kerja_id"); v != "" {
		db = db.Where("unit_kerja_id = ?", v)
	}

	status := c.Query("status_arsip")
	if status == "" {
		status = c.Query("status")
	}
	if status != "" {
		if status == "siap_penyusutan" {
			db = db.Where("status_arsip != 'musnah' AND tanggal_retensi_berakhir < ?", time.Now())
		} else {
			db = db.Where("status_arsip = ?", status)
		}
	}

	if v := c.Query("kode_klasifikasi_id"); v != "" {
		db = db.Where("kode_klasifikasi_id = ?", v)
	}

	if v := c.Query("lokasi_arsip_id"); v != "" {
		db = db.Where("lokasi_arsip_id = ?", v)
	}

	if v := c.Query("tahun"); v != "" {
		db = db.Where("EXTRACT(YEAR FROM tanggal_dibuat) = ?", v)
	}

	retensi := c.Query("retensi_filter")
	if retensi == "akan_berakhir" {
		db = db.Where("tanggal_retensi_berakhir BETWEEN ? AND ?", time.Now(), time.Now().AddDate(1, 0, 0))
	} else if retensi == "sudah_berakhir" {
		db = db.Where("tanggal_retensi_berakhir < ?", time.Now())
	}

	if v := c.Query("start_date"); v != "" {
		db = db.Where("tanggal_dibuat >= ?", v)
	}

	if v := c.Query("end_date"); v != "" {
		db = db.Where("tanggal_dibuat <= ?", v+" 23:59:59")
	}

	if q := c.Query("search"); q != "" {
		likePattern := "%" + q + "%"
		db = db.Where("(nama_arsip LIKE ? OR nomor_arsip LIKE ? OR uraian LIKE ?)", likePattern, likePattern, likePattern)
	}

	return db
}

func (h *LaporanHandler) Arsip(c *gin.Context) {
	const perPage = 25

	// ── Parse page ──
	page := 1
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	// ── Build filtered query ──
	db := database.DB.Model(&models.Arsip{}).
		Preload("KodeKlasifikasi").
		Preload("UnitKerja").
		Preload("LokasiArsip")
	db = applyLaporanArsipFilters(db, c)

	// ── Count total filtered records ──
	var totalFiltered int64
	countDB := db.Session(&gorm.Session{})
	countDB.Count(&totalFiltered)

	// ── Calculate pagination ──
	totalPages := int(math.Ceil(float64(totalFiltered) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage

	// ── Fetch paginated data ──
	var arsipList []models.Arsip
	db.Order("nomor_arsip").Offset(offset).Limit(perPage).Find(&arsipList)

	// ── Calculate display range ──
	firstItem := offset + 1
	lastItem := offset + len(arsipList)
	if lastItem > int(totalFiltered) {
		lastItem = int(totalFiltered)
	}
	hasPages := totalPages > 1

	// ── Build query string without "page" param (for pagination links & exports) ──
	rawQuery := c.Request.URL.RawQuery
	paginationQueryStr := removePageParam(rawQuery)
	exportQueryStr := ""
	if paginationQueryStr != "" {
		exportQueryStr = "?" + paginationQueryStr
	}

	// ── Generate pagination HTML ──
	var paginationHTML template.HTML
	if hasPages {
		paginationHTML = BuildPagination(page, totalPages, paginationQueryStr)
	}

	// ── Load filter options ──
	var unitKerjaOpts []models.UnitKerja
	var kodeKlasifikasiOpts []models.KodeKlasifikasi
	var lokasiArsipOpts []models.LokasiArsip
	database.DB.Order("nama_unit").Find(&unitKerjaOpts)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOpts)
	database.DB.Where("is_active = 1").Order("nama_lokasi").Find(&lokasiArsipOpts)

	var totalArsip, aktifArsip, inaktifArsip, diberkaskanArsip int64
	database.DB.Model(&models.Arsip{}).Count(&totalArsip)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'aktif'").Count(&aktifArsip)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'inaktif'").Count(&inaktifArsip)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'diberkaskan'").Count(&diberkaskanArsip)

	var tahunList []int
	database.DB.Model(&models.Arsip{}).
		Select("DISTINCT EXTRACT(YEAR FROM tanggal_dibuat) as yr").
		Where("tanggal_dibuat IS NOT NULL AND deleted_at IS NULL").
		Order("yr DESC").
		Pluck("yr", &tahunList)

	if tahunList == nil {
		tahunList = []int{}
	}

	hasFilters := c.Query("unit_kerja_id") != "" || c.Query("status") != "" || c.Query("status_arsip") != "" ||
		c.Query("kode_klasifikasi_id") != "" || c.Query("lokasi_arsip_id") != "" ||
		c.Query("tahun") != "" || c.Query("retensi_filter") != "" ||
		c.Query("start_date") != "" || c.Query("end_date") != "" || c.Query("search") != ""

	reqStatus := c.Query("status_arsip")
	if reqStatus == "" {
		reqStatus = c.Query("status")
	}

	Render(c, 200, "laporan/arsip.html", gin.H{
		"title": "Laporan Arsip - SIMARC", "pageTitle": "Laporan Arsip",
		"ArsipList":             arsipList,
		"UnitKerjaList":         unitKerjaOpts,
		"KodeKlasifikasiList":   kodeKlasifikasiOpts,
		"LokasiArsipList":       lokasiArsipOpts,
		"count":                 len(arsipList),
		"HasFilters":            hasFilters,
		"QueryString":           exportQueryStr,
		"RequestSearch":         c.Query("search"),
		"RequestUnitKerjaID":    c.Query("unit_kerja_id"),
		"RequestStatusArsip":    reqStatus,
		"RequestKodeKlasifikasiID": c.Query("kode_klasifikasi_id"),
		"RequestLokasiArsipID":  c.Query("lokasi_arsip_id"),
		"RequestTahun":          c.Query("tahun"),
		"RequestRetensiFilter":  c.Query("retensi_filter"),
		"RequestStartDate":      c.Query("start_date"),
		"RequestEndDate":        c.Query("end_date"),
		"TahunList":             tahunList,
		"SummaryStats": gin.H{
			"Total":       totalArsip,
			"Aktif":       aktifArsip,
			"Inaktif":     inaktifArsip,
			"Diberkaskan": diberkaskanArsip,
		},
		"ArsipListFirstItem": firstItem,
		"ArsipListLastItem":  lastItem,
		"ArsipListTotal":     int(totalFiltered),
		"ArsipListHasPages":  hasPages,
		"Pagination":        paginationHTML,
		"ShowUserUnitInfo":  false,
		"UserUnitKerja":     "",
	})
}

func (h *LaporanHandler) Retensi(c *gin.Context) {
	const perPage = 25

	page := 1
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	db := database.DB.Model(&models.Arsip{}).Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("tanggal_retensi_berakhir < ? AND status_arsip != 'musnah'", time.Now())

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
	db.Order("tanggal_retensi_berakhir ASC").Offset(offset).Limit(perPage).Find(&list)

	// Build enhanced list with computed retention fields for the report table
	now := time.Now()
	enhancedList := make([]retensiArsipItem, 0, len(list))
	for _, a := range list {
		totalRetensi := 0
		if a.KodeKlasifikasi != nil {
			totalRetensi = a.KodeKlasifikasi.RetensiAktif + a.KodeKlasifikasi.RetensiInaktif
		}
		expired := a.TanggalRetensiAkhir != nil && a.TanggalRetensiAkhir.Before(now)
		status := "Masih Disimpan"
		if expired {
			status = "Siap Dimusnahkan"
		}
		lamaSimpan := 0
		if a.TanggalDibuat != nil && a.TanggalDibuat.Before(now) {
			lamaSimpan = int(now.Sub(*a.TanggalDibuat).Hours() / 24 / 365.25)
		}
		enhancedList = append(enhancedList, retensiArsipItem{
			Arsip:                         a,
			TotalRetensi:                  totalRetensi,
			TanggalRetensiBerakhirExpired: expired,
			StatusRetensi:                 status,
			LamaSimpan:                    lamaSimpan,
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

	var paginationHTML template.HTML
	if hasPages {
		paginationHTML = BuildPagination(page, totalPages, paginationQueryStr)
	}

	var totalArsip, siapMusnah, segeraMusnah int64
	database.DB.Model(&models.Arsip{}).Count(&totalArsip)
	database.DB.Model(&models.Arsip{}).Where("tanggal_retensi_berakhir < ? AND status_arsip != 'musnah'", time.Now()).Count(&siapMusnah)
	database.DB.Model(&models.Arsip{}).Where("tanggal_retensi_berakhir BETWEEN ? AND ?", time.Now(), time.Now().AddDate(1, 0, 0)).Count(&segeraMusnah)

	Render(c, 200, "laporan/laporan-retensi.html", gin.H{
		"title": "Laporan Retensi - SIMARC", "pageTitle": "Laporan Retensi Arsip",
		"List":                  enhancedList,
		"ArsipRetensi":          enhancedList,
		"ArsipListFirstItem":    firstItem,
		"ArsipListLastItem":     lastItem,
		"ArsipListTotal":        int(totalFiltered),
		"ArsipListHasPages":     hasPages,
		"Pagination":            paginationHTML,
		"Stats": gin.H{
			"Total":        totalArsip,
			"SiapMusnah":   siapMusnah,
			"SegeraMusnah": segeraMusnah,
		},
	})
}

func (h *LaporanHandler) Pemusnahan(c *gin.Context) {
	const perPage = 25

	page := 1
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	db := database.DB.Model(&models.PemusnahanArsip{}).Preload("Creator").Preload("Arsip")
	if v := c.Query("status"); v != "" {
		db = db.Where("status = ?", v)
	}

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

	var list []models.PemusnahanArsip
	db.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&list)

	firstItem := offset + 1
	lastItem := offset + len(list)
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

	Render(c, 200, "laporan/pemusnahan.html", gin.H{
		"title": "Laporan Pemusnahan - SIMARC", "pageTitle": "Laporan Pemusnahan",
		"PemusnahanList":           list,
		"PemusnahanListFirstItem":  firstItem,
		"PemusnahanListLastItem":   lastItem,
		"PemusnahanListTotal":      int(totalFiltered),
		"PemusnahanListHasPages":   hasPages,
		"Pagination":               paginationHTML,
		"RequestStatus":            c.Query("status"),
	})
}

// ── QR CODE ───────────────────────────────────────────────────────────────────

type QrCodeHandler struct{}

func (h *QrCodeHandler) Index(c *gin.Context) {
	var total, active, totalScans, totalBoxes, totalArchives int64
	database.DB.Model(&models.QrCode{}).Count(&total)
	database.DB.Model(&models.QrCode{}).Where("is_active = 1").Count(&active)
	database.DB.Model(&models.QrScanLog{}).Count(&totalScans)
	database.DB.Model(&models.QrCode{}).Where("qr_type = 'box'").Count(&totalBoxes)
	database.DB.Model(&models.QrCode{}).Where("qr_type = 'arsip'").Count(&totalArchives)

	var recentScans []models.QrScanLog
	database.DB.Order("scanned_at DESC").Limit(10).Find(&recentScans)

	var qrCodes []models.QrCode
	database.DB.Preload("Arsip").Order("created_at DESC").Limit(50).Find(&qrCodes)

	Render(c, 200, "qrcode/index.html", gin.H{
		"title": "QR Code - SIMARC", "pageTitle": "Manajemen QR Code",
		"recentScans": recentScans, "qrCodes": qrCodes,
		"Stats": gin.H{
			"TotalQr":    total,
			"Active":     active,
			"TotalScans": totalScans,
			"Boxes":      totalBoxes,
			"Archives":   totalArchives,
		},
	})
}

func (h *QrCodeHandler) Scanner(c *gin.Context) {
	Render(c, 200, "qrcode/scanner.html", gin.H{
		"title": "QR Scanner", "pageTitle": "QR Code Scanner",
	})
}

func (h *QrCodeHandler) Generate(c *gin.Context) {
	arsipID := c.Param("arsipId")
	if arsipID == "" {
		arsipID = c.Param("id")
	}
	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", arsipID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Arsip tidak ditemukan"})
		return
	}

	qrDir := config.QRCodeDir()
	os.MkdirAll(qrDir, 0755)
	filename := fmt.Sprintf("qr_%s.png", arsip.ID)
	path := filepath.Join(qrDir, filename)

	qrData := fmt.Sprintf("/arsip/%s", arsip.ID)
	if err := qrcode.WriteFile(qrData, qrcode.Medium, 256, path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membuat QR Code"})
		return
	}

	// Save to DB
	database.DB.Where("arsip_id = ?", arsipID).Delete(&models.QrCode{})
	qr := models.QrCode{
		ID: uuid.New().String(), ArsipID: &arsipID,
		QrType: "arsip", QrCodePath: path, QrData: qrData, IsActive: true,
	}
	database.DB.Create(&qr)

	middleware.SetFlash(c, "success", "QR Code berhasil dibuat.")
	c.Redirect(http.StatusFound, "/arsip/"+arsipID)
}

func (h *QrCodeHandler) Download(c *gin.Context) {
	id := c.Param("id")
	var qr models.QrCode
	if err := database.DB.First(&qr, "id = ?", id).Error; err != nil || qr.QrCodePath == "" {
		c.String(http.StatusNotFound, "QR Code tidak ditemukan")
		return
	}
	if _, err := os.Stat(qr.QrCodePath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "File QR Code tidak ditemukan di server")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=qr_%s.png", qr.ID))
	c.Header("Content-Type", "image/png")
	c.File(qr.QrCodePath)
}

func (h *QrCodeHandler) DownloadByArsip(c *gin.Context) {
	arsipID := c.Param("id")
	var qr models.QrCode
	if err := database.DB.First(&qr, "arsip_id = ?", arsipID).Error; err != nil || qr.QrCodePath == "" {
		c.String(http.StatusNotFound, "QR Code tidak ditemukan")
		return
	}
	if _, err := os.Stat(qr.QrCodePath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "File QR Code tidak ditemukan")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=qr_%s.png", arsipID))
	c.Header("Content-Type", "image/png")
	c.File(qr.QrCodePath)
}

func (h *QrCodeHandler) Show(c *gin.Context) {
	id := c.Param("id")
	var qr models.QrCode
	if err := database.DB.Preload("Arsip").First(&qr, "id = ?", id).Error; err != nil {
		c.Redirect(http.StatusFound, "/qrcode")
		return
	}
	var scanLogs []models.QrScanLog
	database.DB.Where("qr_code_id = ?", qr.ID).Order("scanned_at DESC").Limit(20).Find(&scanLogs)
	Render(c, 200, "qrcode/show.html", gin.H{
		"title": "Detail QR Code", "pageTitle": "Detail QR Code",
		"qr": qr, "scanLogs": scanLogs,
	})
}

func (h *QrCodeHandler) Scan(c *gin.Context) {
	// Log scan
	qrID := c.Param("id")
	var qr models.QrCode
	if err := database.DB.Preload("Arsip").First(&qr, "id = ?", qrID).Error; err != nil {
		Render404(c)
		return
	}

	user := middleware.GetCurrentUser(c)
	userID := ""
	if user != nil {
		userID = user.ID
	}

	now := time.Now()
	log := models.QrScanLog{
		QrCodeID:  qr.ID,
		UserID:    &userID,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		ScannedAt: now,
		CreatedAt: now,
	}
	if c.Query("action") != "" {
		log.Action = c.Query("action")
	}
	database.DB.Create(&log)

	// Update scan count
	database.DB.Model(&qr).Updates(map[string]interface{}{
		"scan_count":      qr.ScanCount + 1,
		"last_scanned_at": now,
		"last_scanned_by": &userID,
	})

	if qr.ArsipID != nil {
		c.Redirect(http.StatusFound, "/arsip/"+*qr.ArsipID)
	} else {
		c.Redirect(http.StatusFound, "/dashboard")
	}
}

func (h *QrCodeHandler) ScanAPI(c *gin.Context) {
	action := c.PostForm("action")
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

	user := middleware.GetCurrentUser(c)
	userID := ""
	if user != nil {
		userID = user.ID
	}

	now := time.Now()
	log := models.QrScanLog{
		QrCodeID:  qr.ID,
		UserID:    &userID,
		Action:    action,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		ScannedAt: now,
		CreatedAt: now,
	}
	database.DB.Create(&log)

	database.DB.Model(&qr).Updates(map[string]interface{}{
		"scan_count":      qr.ScanCount + 1,
		"last_scanned_at": now,
		"last_scanned_by": &userID,
	})

	result := gin.H{"success": true, "qr": qr}
	if qr.Arsip != nil {
		result["arsip"] = qr.Arsip
	}
	c.JSON(http.StatusOK, result)
}

func (h *QrCodeHandler) BulkGenerate(c *gin.Context) {
	arsipIDs := c.PostFormArray("arsip_ids[]")
	if len(arsipIDs) == 0 {
		middleware.SetFlash(c, "error", "Pilih minimal satu arsip.")
		c.Redirect(http.StatusFound, "/qrcode")
		return
	}

	qrDir := config.QRCodeDir()
	os.MkdirAll(qrDir, 0755)

	count := 0
	for _, arsipID := range arsipIDs {
		var arsip models.Arsip
		if database.DB.First(&arsip, "id = ?", arsipID).Error != nil {
			continue
		}

		filename := fmt.Sprintf("qr_%s.png", arsip.ID)
		path := filepath.Join(qrDir, filename)
		qrData := fmt.Sprintf("/arsip/%s", arsip.ID)

		if err := qrcode.WriteFile(qrData, qrcode.Medium, 256, path); err != nil {
			continue
		}

		database.DB.Where("arsip_id = ?", arsipID).Delete(&models.QrCode{})
		qr := models.QrCode{
			ID: uuid.New().String(), ArsipID: &arsipID,
			QrType: "arsip", QrCodePath: path, QrData: qrData, IsActive: true,
		}
		database.DB.Create(&qr)
		count++
	}

	middleware.SetFlash(c, "success", fmt.Sprintf("%d QR Code berhasil dibuat.", count))
	c.Redirect(http.StatusFound, "/qrcode")
}

func (h *QrCodeHandler) Deactivate(c *gin.Context) {
	id := c.Param("id")
	database.DB.Model(&models.QrCode{}).Where("id = ?", id).Update("is_active", false)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *QrCodeHandler) CheckLocation(c *gin.Context) {
	arsipID := c.Param("arsipId")
	var qr models.QrCode
	if err := database.DB.Where("arsip_id = ?", arsipID).First(&qr).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "QR Code tidak ditemukan"})
		return
	}
	location := map[string]interface{}{
		"box_number":    qr.BoxNumber,
		"shelf":         qr.ShelfLocation,
		"room":          qr.RoomLocation,
		"location_data": qr.LocationData,
		"scan_count":    qr.ScanCount,
		"last_scanned":  qr.LastScannedAt,
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "location": location})
}

func (h *QrCodeHandler) GetByLocation(c *gin.Context) {
	boxNumber := c.Query("box")
	shelf := c.Query("shelf")
	db := database.DB.Model(&models.QrCode{}).Preload("Arsip")
	if boxNumber != "" {
		db = db.Where("box_number = ?", boxNumber)
	}
	if shelf != "" {
		db = db.Where("shelf_location = ?", shelf)
	}
	var list []models.QrCode
	db.Find(&list)
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// ── OCR ───────────────────────────────────────────────────────────────────────

type OcrHandler struct{}

func (h *OcrHandler) Index(c *gin.Context) {
	languages := []gin.H{
		{"Code": "ind", "Name": "Bahasa Indonesia"},
		{"Code": "eng", "Name": "English"},
		{"Code": "ind+eng", "Name": "Indonesia + English"},
	}
	Render(c, 200, "ocr/index.html", gin.H{
		"title": "OCR - Text Extraction - SIMARC", "pageTitle": "OCR - Ekstraksi Teks",
		"supportedFormats": []string{"JPG", "JPEG", "PNG", "PDF"},
		"Languages": languages,
	})
}

func (h *OcrHandler) Process(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "File wajib diunggah."})
		return
	}

	uploadDir := filepath.Join(config.StorageDir(), "ocr-temp")
	os.MkdirAll(uploadDir, 0755)
	ext := filepath.Ext(file.Filename)
	filename := fmt.Sprintf("ocr_%s%s", uuid.New().String(), ext)
	dst := filepath.Join(uploadDir, filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Gagal menyimpan file."})
		return
	}
	defer os.Remove(dst)

	startTime := time.Now()
	lang := c.PostForm("language")
	if lang == "" {
		lang = "ind+eng"
	}
	out, err := exec.Command("tesseract", dst, "stdout", "-l", lang).Output()
	elapsed := time.Since(startTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Tesseract tidak tersedia. Install: sudo apt install tesseract-ocr"})
		return
	}

	text := string(out)
	words := len(strings.Fields(text))
	lines := len(strings.Split(text, "\n"))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"text": text,
		},
		"stats": gin.H{
			"characters":     len(text),
			"words":          words,
			"lines":          lines,
			"pages":          1,
			"processing_time": elapsed.Milliseconds(),
		},
		"filename": file.Filename,
	})
}

// ── BLOCKCHAIN ────────────────────────────────────────────────────────────────

type BlockchainHandler struct{}

func (h *BlockchainHandler) Index(c *gin.Context) {
	var records []models.BlockchainAudit
	database.DB.Order("block_number DESC").Limit(50).Find(&records)
	var total int64
	database.DB.Model(&models.BlockchainAudit{}).Count(&total)

	var todayCount int64
	database.DB.Model(&models.BlockchainAudit{}).Where("DATE(created_at) = CURDATE()").Count(&todayCount)

	// Enrich records with entity names and user names
	arsipIDs := []string{}
	userIDs := []string{}
	for _, r := range records {
		if r.EntityType == "arsip" && r.EntityID != "" {
			arsipIDs = append(arsipIDs, r.EntityID)
		}
		if r.UserID != nil && *r.UserID != "" {
			userIDs = append(userIDs, *r.UserID)
		}
	}

	// Batch load entity names
	arsipMap := map[string]string{}
	if len(arsipIDs) > 0 {
		var arsipList []struct{ ID, NamaArsip string }
		database.DB.Table("arsip").Where("id IN ?", arsipIDs).Find(&arsipList)
		for _, a := range arsipList {
			arsipMap[a.ID] = a.NamaArsip
		}
	}

	userMap := map[string]string{}
	if len(userIDs) > 0 {
		var userList []struct{ ID, Name string }
		database.DB.Table("users").Where("id IN ?", userIDs).Find(&userList)
		for _, u := range userList {
			userMap[u.ID] = u.Name
		}
	}

	enriched := make([]BlockRecord, len(records))
	for i, r := range records {
		enriched[i] = BlockRecord{BlockchainAudit: r}
		if r.EntityType == "arsip" {
			enriched[i].EntityName = arsipMap[r.EntityID]
		} else {
			enriched[i].EntityName = r.EntityID
		}
		if r.UserID != nil {
			enriched[i].UserName = userMap[*r.UserID]
		}
	}

	// Verify chain integrity
	integrityOK := true
	for i := 1; i < len(records); i++ {
		if records[i].PreviousHash != records[i-1].CurrentHash {
			integrityOK = false
			break
		}
	}

	Render(c, 200, "blockchain/index.html", gin.H{
		"title": "Blockchain - SIMARC", "pageTitle": "Blockchain Audit Trail",
		"records": enriched, "Blocks": enriched, "total": total, "integrityOK": integrityOK,
		"Integrity": gin.H{"IsValid": integrityOK},
		"Stats": gin.H{
			"TotalBlocks":    total,
			"VerifiedBlocks": total,
			"TodayBlocks":    todayCount,
		},
	})
}

// Helper encryption/decryption functions shared within handlers package
func getEncryptionKey() []byte {
	keyStr := config.App.SessionKey
	if keyStr == "" {
		keyStr = "simarc-default-secret-key-32-bytes"
	}
	h := sha256.Sum256([]byte(keyStr))
	return h[:]
}

func encryptFile(src, dst string, key []byte) error {
	plaintext, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return os.WriteFile(dst, ciphertext, 0644)
}

func decryptFile(src, dst string, key []byte) error {
	ciphertext, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, plaintext, 0644)
}

func formatBytes(bytesCount int64) string {
	const (
		B  = 1
		KB = 1024 * B
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytesCount >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytesCount)/GB)
	case bytesCount >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytesCount)/MB)
	case bytesCount >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytesCount)/KB)
	default:
		return fmt.Sprintf("%d B", bytesCount)
	}
}

type BackupHandler struct{}

func (h *BackupHandler) Index(c *gin.Context) {
	var logs []models.BackupLog
	database.DB.Order("created_at DESC").Find(&logs)

	var totalBackups = len(logs)
	var totalSize int64 = 0
	var lastBackupAge = "Tidak ada"
	for _, b := range logs {
		if b.Status == "success" {
			totalSize += b.FileSize
		}
	}

	if totalBackups > 0 {
		var lastSuccess *models.BackupLog
		for _, b := range logs {
			if b.Status == "success" {
				lastSuccess = &b
				break
			}
		}
		if lastSuccess != nil {
			duration := time.Since(lastSuccess.CreatedAt)
			if duration.Hours() >= 24 {
				lastBackupAge = fmt.Sprintf("%.0f hari lalu", duration.Hours()/24)
			} else if duration.Hours() >= 1 {
				lastBackupAge = fmt.Sprintf("%.0f jam lalu", duration.Hours())
			} else if duration.Minutes() >= 1 {
				lastBackupAge = fmt.Sprintf("%.0f menit lalu", duration.Minutes())
			} else {
				lastBackupAge = "Baru saja"
			}
		}
	}

	totalSizeFormatted := formatBytes(totalSize)

	var avgSize int64 = 0
	var successCount int64 = 0
	for _, b := range logs {
		if b.Status == "success" {
			successCount++
		}
	}
	if successCount > 0 {
		avgSize = totalSize / successCount
	}
	avgSizeFormatted := formatBytes(avgSize)

	Render(c, 200, "backup/index.html", gin.H{
		"title": "Backup - SIMARC", "pageTitle": "Backup & Restore",
		"Backups": logs,
		"Statistics": gin.H{
			"TotalBackups":       totalBackups,
			"TotalSizeFormatted": totalSizeFormatted,
			"LastBackupAge":      lastBackupAge,
			"AverageSize":        avgSizeFormatted,
		},
		"GDriveClientID": config.App.GoogleDriveClientID,
		"GDriveFolderID": config.App.GoogleDriveFolderID,
		"CanBackup":      config.CanBackup(),
		"CanRestore":     config.CanRestore(),
		"IsVercel":       config.IsVercel(),
	})
}

type CreateBackupRequest struct {
	Type             string `json:"type"`
	Encrypt          bool   `json:"encrypt"`
	AutoUploadGDrive bool   `json:"auto_upload_gdrive"`
}

func (h *BackupHandler) Create(c *gin.Context) {
	var req CreateBackupRequest
	isJSON := strings.Contains(c.ContentType(), "application/json")

	if isJSON {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
			return
		}
	} else {
		req.Type = c.PostForm("type")
		req.Encrypt = c.PostForm("encrypt") == "true" || c.PostForm("encrypt") == "on"
		req.AutoUploadGDrive = c.PostForm("auto_upload_gdrive") == "true" || c.PostForm("auto_upload_gdrive") == "on"
	}

	if req.Type == "" {
		req.Type = "database"
	}

	user := middleware.GetCurrentUser(c)
	timestamp := time.Now().Format("2006-01-02_150405")

	// If mysqldump is available, use it (faster, more complete).
	// Otherwise fall back to GORM-based export (works on Vercel).
	if config.CanBackup() {
		backupSQL, err := backupWithMysqldump(isJSON)
		if err != nil {
			respondError(c, isJSON, err.Error())
			return
		}
		saveBackupAndRespond(c, isJSON, timestamp, backupSQL, user)
		return
	}

	// GORM-based export fallback (Vercel serverless compatible)
	if err := respondProgress(c, isJSON, "Mengekspor database via GORM..."); err != nil {
		// ignore — progress is optional
	}
	backupSQL, err := services.ExportDatabaseAsSQL()
	if err != nil {
		respondError(c, isJSON, "Gagal mengekspor database: "+err.Error())
		return
	}

	saveBackupAndRespond(c, isJSON, timestamp, backupSQL, user)
}

// backupWithMysqldump runs mysqldump and returns the SQL bytes.
func backupWithMysqldump(isJSON bool) ([]byte, error) {
	// ── Dump database ke memory ──
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

	args := []string{
		"--host=" + dbHost,
		"--port=" + dbPort,
		"--user=" + dbUser,
		"--no-tablespaces",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		dbName,
	}

	cmd := exec.Command("mysqldump", args...)
	if dbPass != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+dbPass)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("gagal membuat pipe: %w", err)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("gagal menjalankan mysqldump: %w", err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, stdout)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca output mysqldump: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		errMsg := err.Error()
		if stderrOut := errBuf.String(); stderrOut != "" {
			errMsg += " — " + strings.TrimSpace(stderrOut)
		}
		return nil, fmt.Errorf("mysqldump gagal: %s", errMsg)
	}

	return buf.Bytes(), nil
}

// saveBackupAndRespond saves the backup to disk, logs it, and responds.
func saveBackupAndRespond(c *gin.Context, isJSON bool, timestamp string, sqlData []byte, user *models.User) {
	filename := fmt.Sprintf("backup_%s.sql", timestamp)

	localPath := filepath.Join(config.BackupDir(), filename)
	os.MkdirAll(filepath.Dir(localPath), 0755)
	if err := os.WriteFile(localPath, sqlData, 0644); err != nil {
		respondError(c, isJSON, "Gagal menyimpan backup lokal: "+err.Error())
		return
	}

	log := models.BackupLog{
		FileName:    filename,
		FilePath:    localPath,
		FileSize:    int64(len(sqlData)),
		BackupType:  "database",
		Status:      "success",
		CompletedAt: &[]time.Time{time.Now()}[0],
	}
	database.DB.Create(&log)

	if user != nil {
		logActivity(user.ID, "backup", "Backup database berhasil: "+filename, "backup", log.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	}

	if isJSON {
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"filename": filename,
		})
	} else {
		middleware.SetFlash(c, "success", "Backup berhasil dibuat: "+filename)
		c.Redirect(http.StatusFound, "/backup")
	}
}

func (h *BackupHandler) Download(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		filename = c.Query("filename")
	}
	var log models.BackupLog
	if err := database.DB.Where("filename = ?", filename).First(&log).Error; err != nil {
		c.String(http.StatusNotFound, "Backup tidak ditemukan")
		return
	}
	// Coba file lokal
	if log.FilePath != "" {
		if _, err := os.Stat(log.FilePath); err == nil {
			c.Header("Content-Disposition", "attachment; filename="+log.FileName)
			c.File(log.FilePath)
			return
		}
	}
	c.String(http.StatusNotFound, "File backup tidak ditemukan (mungkin sudah dihapus)")
}

func (h *BackupHandler) Delete(c *gin.Context) {
	var filename string
	isJSON := false

	var req struct {
		Filename string `json:"filename"`
	}
	if strings.Contains(c.ContentType(), "application/json") {
		if err := c.ShouldBindJSON(&req); err == nil && req.Filename != "" {
			filename = req.Filename
			isJSON = true
		}
	}

	if filename == "" {
		filename = c.Param("filename")
	}

	if filename == "" {
		if isJSON {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Filename kosong"})
		} else {
			middleware.SetFlash(c, "error", "Filename kosong")
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	var log models.BackupLog
	if err := database.DB.Where("filename = ?", filename).First(&log).Error; err != nil {
		if isJSON {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Backup tidak ditemukan"})
		} else {
			middleware.SetFlash(c, "error", "Backup tidak ditemukan")
			c.Redirect(http.StatusFound, "/backup")
		}
		return
	}

	os.Remove(log.FilePath)
	database.DB.Delete(&log)

	if isJSON {
		c.JSON(http.StatusOK, gin.H{"success": true})
	} else {
		middleware.SetFlash(c, "success", "Backup berhasil dihapus.")
		c.Redirect(http.StatusFound, "/backup")
	}
}

// SaveGDriveSettings saves the Google Drive OAuth client ID and target folder
// (used by the client-side JavaScript upload) into the .env file and the
// in-memory config so changes apply immediately.
func (h *BackupHandler) SaveGDriveSettings(c *gin.Context) {
	var req struct {
		ClientID string `json:"client_id"`
		FolderID string `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Payload tidak valid"})
		return
	}

	updates := map[string]string{
		"GOOGLE_DRIVE_CLIENT_ID": strings.TrimSpace(req.ClientID),
		"GOOGLE_DRIVE_FOLDER_ID": strings.TrimSpace(req.FolderID),
	}
	if err := config.UpdateEnv(updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menyimpan pengaturan: " + err.Error()})
		return
	}

	// Reflect immediately in the in-memory config
	config.App.GoogleDriveClientID = updates["GOOGLE_DRIVE_CLIENT_ID"]
	config.App.GoogleDriveFolderID = updates["GOOGLE_DRIVE_FOLDER_ID"]

	if user := middleware.GetCurrentUser(c); user != nil {
		logActivity(user.ID, "backup", "Pengaturan Google Drive backup diperbarui", "backup", "", c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "client_id": config.App.GoogleDriveClientID, "folder_id": config.App.GoogleDriveFolderID})
}

// SaveGDriveFile records the Google Drive file ID returned by the client-side
// upload onto the matching backup log so the UI can show the Drive link.
func (h *BackupHandler) SaveGDriveFile(c *gin.Context) {
	var req struct {
		Filename          string `json:"filename"`
		GoogleDriveFileID string `json:"google_drive_file_id"`
		GoogleDriveURL    string `json:"google_drive_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Payload tidak valid"})
		return
	}
	if req.Filename == "" || req.GoogleDriveFileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Filename dan Google Drive file ID wajib diisi"})
		return
	}

	var log models.BackupLog
	if err := database.DB.Where("filename = ?", req.Filename).First(&log).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Backup tidak ditemukan"})
		return
	}

	database.DB.Model(&log).Updates(map[string]interface{}{
		"google_drive_file_id": req.GoogleDriveFileID,
		"google_drive_url":    req.GoogleDriveURL,
		"cloud_provider":      "google_drive",
	})

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── MONITORING ────────────────────────────────────────────────────────────────

type MonitoringHandler struct{}

type retensiArsipItem struct {
	models.Arsip
	TotalRetensi                  int     `json:"total_retensi"`
	TanggalRetensiBerakhirExpired bool    `json:"tanggal_retensi_berakhir_expired"`
	SisaRetensi                   float64 `json:"sisa_retensi"`
	LamaSimpan                    int     `json:"lama_simpan"`
	StatusRetensi                 string  `json:"status_retensi"`
}

func (h *MonitoringHandler) Retensi(c *gin.Context) {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("tanggal_retensi_berakhir BETWEEN ? AND ?", time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, 30)).
		Order("tanggal_retensi_berakhir").Find(&list)
	var sudahBerakhir, akanBerakhir, masihAktif int64
	database.DB.Model(&models.Arsip{}).Where("tanggal_retensi_berakhir < ?", time.Now()).Count(&sudahBerakhir)
	database.DB.Model(&models.Arsip{}).Where("tanggal_retensi_berakhir BETWEEN ? AND ?", time.Now(), time.Now().AddDate(1, 0, 0)).Count(&akanBerakhir)
	database.DB.Model(&models.Arsip{}).Where("(tanggal_retensi_berakhir IS NULL OR tanggal_retensi_berakhir > ?) AND status_arsip != 'musnah'", time.Now().AddDate(1, 0, 0)).Count(&masihAktif)
	
	// Build list with computed fields
	now := time.Now()
	enhancedList := make([]retensiArsipItem, 0, len(list))
	for _, a := range list {
		totalRetensi := 0
		if a.KodeKlasifikasi != nil {
			totalRetensi = a.KodeKlasifikasi.RetensiAktif + a.KodeKlasifikasi.RetensiInaktif
		}
		expired := a.TanggalRetensiAkhir != nil && a.TanggalRetensiAkhir.Before(now)
		sisaRetensi := 0.0
		if a.TanggalRetensiAkhir != nil && !expired {
			sisaRetensi = a.TanggalRetensiAkhir.Sub(now).Hours() / 24 / 365.25
		}
		enhancedList = append(enhancedList, retensiArsipItem{
			Arsip:                       a,
			TotalRetensi:                totalRetensi,
			TanggalRetensiBerakhirExpired: expired,
			SisaRetensi:                 sisaRetensi,
		})
	}
	
	Render(c, 200, "monitoring/retensi.html", gin.H{
		"title": "Monitoring Retensi - SIMARC", "pageTitle": "Monitoring Retensi", "List": enhancedList,
		"HasPages": false, "FirstItem": 1,
		"Stats": gin.H{
			"SudahBerakhir": sudahBerakhir,
			"AkanBerakhir":  akanBerakhir,
			"MasihAktif":    masihAktif,
		},
	})
}

// ── HEALTH CHECK ──────────────────────────────────────────────────────────────

func HealthCheck(c *gin.Context) {
	var count int64
	err := database.DB.Raw("SELECT 1").Scan(&count).Error
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "db": "down"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339), "app": config.App.AppName})
}



// ── BACKUP HELPERS ────────────────────────────────────────────────────────────

func respondProgress(c *gin.Context, isJSON bool, msg string) error {
	if isJSON {
		c.JSON(http.StatusOK, gin.H{"success": true, "progress": msg})
	}
	return nil
}

func respondError(c *gin.Context, isJSON bool, msg string) {
	if isJSON {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": msg})
	} else {
		middleware.SetFlash(c, "error", msg)
		c.Redirect(http.StatusFound, "/backup")
	}
}

func removePageParam(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	params := strings.Split(rawQuery, "&")
	var filtered []string
	for _, p := range params {
		if strings.HasPrefix(p, "page=") {
			continue
		}
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, "&")
}
