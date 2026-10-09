package handlers

import (
	"html/template"
	"strconv"
	"strings"

	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ViewerIndex renders the public, read-only archive directory (/viewer).
//
// It is intentionally free of authentication: both the web deployment and the
// desktop build serve this exact handler through the same Go server, so one
// page covers both targets. Only archive metadata is shown — there are no
// create/edit/delete/download actions here.
func (h *ArsipHandler) ViewerIndex(c *gin.Context) {
	if !database.Connected() {
		Render(c, 503, "errors/503.html", gin.H{
			"title":     "Database Tidak Tersedia",
			"pageTitle": "Database Tidak Tersedia",
			"message":   "Direktori arsip sedang tidak dapat diakses. Silakan coba beberapa saat lagi.",
		})
		return
	}

	db := database.DB.Model(&models.Arsip{}).
		Joins("LEFT JOIN unit_kerja ON unit_kerja.id = arsip.unit_kerja_id AND unit_kerja.deleted_at IS NULL").
		Joins("LEFT JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id AND kode_klasifikasi.deleted_at IS NULL").
		Preload("KodeKlasifikasi").
		Preload("UnitKerja").
		Preload("Pemberkasan").
		Preload("LokasiArsip").
		Preload("JenisArsipRel")

	// Free-text search across the same columns as the authenticated list.
	if q := c.Query("search"); q != "" {
		terms := strings.Fields(q)
		for _, term := range terms {
			likePattern := "%" + term + "%"
			db = db.Where(
				"(arsip.nama_arsip LIKE ? OR arsip.nomor_arsip LIKE ? OR arsip.uraian LIKE ? OR arsip.ocr_text LIKE ? OR arsip.tags LIKE ? OR arsip.jenis_arsip LIKE ? OR unit_kerja.nama_unit LIKE ? OR kode_klasifikasi.kode_klasifikasi LIKE ? OR kode_klasifikasi.nama_klasifikasi LIKE ?)",
				likePattern, likePattern, likePattern, likePattern, likePattern, likePattern, likePattern, likePattern, likePattern,
			)
		}
	}

	if v := c.Query("unit_kerja_id"); v != "" {
		db = db.Where("arsip.unit_kerja_id = ?", v)
	}
	if v := c.Query("kode_klasifikasi_id"); v != "" {
		db = db.Where("arsip.kode_klasifikasi_id = ?", v)
	}
	if v := c.Query("jenis_arsip_id"); v != "" {
		db = db.Where("arsip.jenis_arsip_id = ?", v)
	}
	if v := c.Query("start_date"); v != "" {
		db = db.Where("arsip.tanggal_dibuat >= ?", v)
	}
	if v := c.Query("end_date"); v != "" {
		db = db.Where("arsip.tanggal_dibuat <= ?", v+" 23:59:59")
	}

	db = db.Where("arsip.deleted_at IS NULL")

	// Stats are computed before the status filter so the hero cards always
	// reflect the current search across all statuses.
	baseStats := db.Session(&gorm.Session{})
	var total, aktif, inaktif int64
	baseStats.Session(&gorm.Session{}).Count(&total)
	baseStats.Session(&gorm.Session{}).Where("arsip.status_arsip = ?", "aktif").Count(&aktif)
	baseStats.Session(&gorm.Session{}).Where("arsip.status_arsip = ?", "inaktif").Count(&inaktif)

	if s := c.Query("status"); s != "" {
		db = db.Where("arsip.status_arsip = ?", s)
	}

	// Pagination
	perPage := 12
	if pp, err := strconv.Atoi(c.Query("per_page")); err == nil && pp > 0 && pp <= 100 {
		perPage = pp
	}
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	// Sorting (whitelist to avoid SQL injection)
	sortBy := c.Query("sort_by")
	sortOrder := strings.ToUpper(c.Query("sort_order"))
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "ASC"
	}
	switch sortBy {
	case "nomor_arsip":
		db = db.Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) " + sortOrder)
	case "nama_arsip":
		db = db.Order("arsip.nama_arsip " + sortOrder)
	case "tanggal_dibuat":
		db = db.Order("arsip.tanggal_dibuat " + sortOrder)
	case "unit_kerja":
		db = db.Order("unit_kerja.nama_unit " + sortOrder)
	default:
		db = db.Order("arsip.tanggal_dibuat DESC")
	}

	var arsipList []models.Arsip
	if err := db.Limit(perPage).Offset(offset).Find(&arsipList).Error; err != nil {
		arsipList = []models.Arsip{}
	}

	// Filter option lists
	var unitKerjaOptions []models.UnitKerja
	var kodeKlasifikasiOptions []models.KodeKlasifikasi
	var jenisArsipOptions []models.JenisArsip
	database.DB.Order("nama_unit").Find(&unitKerjaOptions)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOptions)
	database.DB.Order("nama_jenis").Find(&jenisArsipOptions)

	// Windowed page numbers
	var pageNumbers []int
	startPage := page - 3
	if startPage < 1 {
		startPage = 1
	}
	endPage := page + 3
	if endPage > totalPages {
		endPage = totalPages
	}
	for i := startPage; i <= endPage; i++ {
		pageNumbers = append(pageNumbers, i)
	}

	// Query string without "page" so pagination links keep active filters.
	qParams := c.Request.URL.Query()
	qParams.Del("page")
	queryString := template.URL(qParams.Encode())
	if queryString != "" {
		queryString = template.URL("&" + string(queryString))
	}

	hasFilters := c.Query("search") != "" || c.Query("status") != "" ||
		c.Query("unit_kerja_id") != "" || c.Query("kode_klasifikasi_id") != "" ||
		c.Query("jenis_arsip_id") != "" || c.Query("start_date") != "" ||
		c.Query("end_date") != "" || c.Query("sort_by") != ""

	Render(c, 200, "viewer/index.html", gin.H{
		"title":                  "Direktori Arsip - SIMARC",
		"pageTitle":              "Direktori Arsip",
		"arsipList":              arsipList,
		"unitKerjaOptions":       unitKerjaOptions,
		"kodeKlasifikasiOptions": kodeKlasifikasiOptions,
		"jenisArsipOptions":      jenisArsipOptions,
		"totalArsip":             total,
		"aktifArsip":             aktif,
		"inaktifArsip":           inaktif,
		"CurrentPage":            page,
		"TotalPages":             totalPages,
		"PageNumbers":            pageNumbers,
		"QueryString":            queryString,
		"PerPage":                perPage,
		"StartIndex":             offset + 1,
		"searchKeyword":          c.Query("search"),
		"TotalResults":           total,
		"FilterUnitKerjaID":      c.Query("unit_kerja_id"),
		"FilterStatus":           c.Query("status"),
		"FilterKlasifikasiID":    c.Query("kode_klasifikasi_id"),
		"FilterJenisArsipID":     c.Query("jenis_arsip_id"),
		"FilterStartDate":        c.Query("start_date"),
		"FilterEndDate":          c.Query("end_date"),
		"FilterSortBy":           c.Query("sort_by"),
		"FilterSortOrder":        sortOrder,
		"HasFilters":             hasFilters,
	})
}

// ViewerDetail renders a read-only archive detail page for the public viewer.
func (h *ArsipHandler) ViewerDetail(c *gin.Context) {
	if !database.Connected() {
		Render(c, 503, "errors/503.html", gin.H{
			"title":     "Database Tidak Tersedia",
			"pageTitle": "Database Tidak Tersedia",
			"message":   "Detail arsip sedang tidak dapat diakses. Silakan coba beberapa saat lagi.",
		})
		return
	}

	var arsip models.Arsip
	if err := database.DB.
		Preload("KodeKlasifikasi").
		Preload("UnitKerja").
		Preload("Pemberkasan").
		Preload("LokasiArsip").
		Preload("JenisArsipRel").
		First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		Render404(c)
		return
	}

	Render(c, 200, "viewer/detail.html", gin.H{
		"title": arsip.NamaArsip + " - SIMARC",
		"arsip": arsip,
	})
}
