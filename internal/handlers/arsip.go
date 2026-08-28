package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"html/template"
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
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type ArsipHandler struct{}

func (h *ArsipHandler) Index(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	db := database.DB.Model(&models.Arsip{}).
		Joins("LEFT JOIN unit_kerja ON unit_kerja.id = arsip.unit_kerja_id AND unit_kerja.deleted_at IS NULL").
		Joins("LEFT JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id AND kode_klasifikasi.deleted_at IS NULL").
		Preload("KodeKlasifikasi").
		Preload("UnitKerja").
		Preload("Pemberkasan").
		Preload("LokasiArsip").
		Preload("JenisArsipRel")

	// User scoping - non-admin only sees own unit (disabled in debug mode for testing)
	if !user.IsAdmin() && user.UnitKerjaID != nil && config.App.AppDebug != "true" {
		db = db.Where("arsip.unit_kerja_id = ?", *user.UnitKerjaID)
	}

	// Filters
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

	if v := c.Query("unit_kerja_id"); v != "" && user.IsAdmin() {
		db = db.Where("arsip.unit_kerja_id = ?", v)
	}
	if v := c.Query("kode_klasifikasi_id"); v != "" {
		db = db.Where("arsip.kode_klasifikasi_id = ?", v)
	}
	if v := c.Query("lokasi_arsip_id"); v != "" {
		db = db.Where("arsip.lokasi_arsip_id = ?", v)
	}
	if v := c.Query("jenis_arsip_id"); v != "" {
		db = db.Where("arsip.jenis_arsip_id = ?", v)
	}
	if v := c.Query("jenis_arsip"); v != "" {
		db = db.Where("arsip.jenis_arsip = ?", v)
	}
	if v := c.Query("start_date"); v != "" {
		db = db.Where("arsip.tanggal_dibuat >= ?", v)
	}
	if v := c.Query("end_date"); v != "" {
		db = db.Where("arsip.tanggal_dibuat <= ?", v+" 23:59:59")
	}
	if v := c.Query("status_berkas_digital"); v != "" {
		if v == "sudah_ada" {
			db = db.Where("arsip.file_path IS NOT NULL AND arsip.file_path != ''")
		} else {
			db = db.Where("arsip.file_path IS NULL OR arsip.file_path = ''")
		}
	}
	if v := c.Query("nilai_min"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			db = db.Where("arsip.nilai_anggaran >= ?", n)
		}
	}
	if v := c.Query("nilai_max"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			db = db.Where("arsip.nilai_anggaran <= ?", n)
		}
	}
	if v := c.Query("pemberkasan_id"); v != "" {
		db = db.Where("arsip.pemberkasan_id = ?", v)
	}
	// Save the base query session before applying the status filter (for search-aware stats)
	baseStatsDb := db.Session(&gorm.Session{})
	baseStatsDb = baseStatsDb.Where("arsip.deleted_at IS NULL")

	if s := c.Query("status"); s != "" {
		db = db.Where("arsip.status_arsip = ?", s)
	}

	// Count for stats
	var total, aktif, inaktif int64
	db = db.Where("arsip.deleted_at IS NULL")
	db.Session(&gorm.Session{}).Count(&total)

	// Gunakan Session() terpisah agar kondisi tidak terakumulasi
	baseStatsDb.Session(&gorm.Session{}).Where("arsip.status_arsip = 'aktif'").Count(&aktif)
	baseStatsDb.Session(&gorm.Session{}).Where("arsip.status_arsip = 'inaktif'").Count(&inaktif)

	// Pagination
	perPage := 10
	if pp, err := strconv.Atoi(c.Query("per_page")); err == nil && pp > 0 {
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

	// Validate page number BEFORE fetching data
	if page > totalPages {
		page = totalPages
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	var arsipList []models.Arsip
	// Ensure deleted_at filter is always applied
	db = db.Where("arsip.deleted_at IS NULL")
	// Sorting
	sortBy := c.Query("sort_by")
	sortOrder := c.Query("sort_order")
	if sortOrder == "" {
		sortOrder = "ASC"
	} else {
		// Validate sort order to prevent SQL injection
		sortOrderUpper := strings.ToUpper(sortOrder)
		if sortOrderUpper != "ASC" && sortOrderUpper != "DESC" {
			sortOrder = "ASC"
		} else {
			sortOrder = sortOrderUpper
		}
	}
	switch sortBy {
	case "nomor_arsip":
		db = db.Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) " + sortOrder)
	case "nama_arsip":
		db = db.Order("arsip.nama_arsip " + sortOrder)
	case "tanggal_dibuat":
		db = db.Order("arsip.tanggal_dibuat " + sortOrder)
	case "status":
		db = db.Order("arsip.status_arsip " + sortOrder)
	case "unit_kerja":
		db = db.Order("unit_kerja.nama_unit " + sortOrder)
	default:
		db = db.Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) ASC")
	}
	if err := db.Limit(perPage).Offset(offset).Find(&arsipList).Error; err != nil {
		// Silently handle error - show empty results
		arsipList = []models.Arsip{}
	}

	// Options for filters
	var unitKerjaOptions []models.UnitKerja
	var kodeKlasifikasiOptions []models.KodeKlasifikasi
	var lokasiArsipOptions []models.LokasiArsip
	var jenisArsipOptions []models.JenisArsip
	var pemberkasanOptions []models.Pemberkasan

	database.DB.Order("nama_unit").Find(&unitKerjaOptions)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOptions)
	database.DB.Where("is_active = 1").Order("nama_lokasi").Find(&lokasiArsipOptions)
	database.DB.Order("nama_jenis").Find(&jenisArsipOptions)
	database.DB.Where("deleted_at IS NULL").Order("nama_pemberkasan").Find(&pemberkasanOptions)

	// Generate page numbers slice
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

	// Query string without "page" parameter for pagination links persistence
	qParams := c.Request.URL.Query()
	qParams.Del("page")
	queryString := template.URL(qParams.Encode())
	if queryString != "" {
		queryString = template.URL("&" + string(queryString))
	}

	hasFilters := c.Query("search") != "" || c.Query("status") != "" ||
		c.Query("unit_kerja_id") != "" || c.Query("kode_klasifikasi_id") != "" ||
		c.Query("lokasi_arsip_id") != "" || c.Query("jenis_arsip_id") != "" ||
		c.Query("jenis_arsip") != "" || c.Query("start_date") != "" ||
		c.Query("end_date") != "" || c.Query("status_berkas_digital") != "" ||
		c.Query("nilai_min") != "" || c.Query("nilai_max") != "" ||
		c.Query("pemberkasan_id") != "" ||
		c.Query("sort_by") != "" ||
		c.Query("sort_order") != ""

	Render(c, 200, "arsip/index.html", gin.H{
		"title":                  "Daftar Arsip - SIMARC",
		"pageTitle":              "Manajemen Arsip",
		"arsipList":              arsipList,
		"unitKerjaOptions":       unitKerjaOptions,
		"kodeKlasifikasiOptions": kodeKlasifikasiOptions,
		"lokasiArsipOptions":     lokasiArsipOptions,
		"jenisArsipOptions":      jenisArsipOptions,
		"pemberkasanOptions":     pemberkasanOptions,
		"totalArsip":             total,
		"aktifArsip":             aktif,
		"inaktifArsip":           inaktif,
		"CurrentPage":            page,
		"TotalPages":             totalPages,
		"PageNumbers":            pageNumbers,
		"QueryString":            queryString,
		"PerPage":                perPage,
		"StartIndex":             offset + 1,
		"queryParams":            c.Request.URL.RawQuery,
		"searchKeyword":          c.Query("search"),
		"SearchQuery":            c.Query("search"),
		"TotalResults":           total,
		"CanSeeAllUnits":         user.IsAdmin(),
		"CanEdit":                true,
		"CanDelete":              true,
		"CanCreate":              true,
		"CanImport":              true,
		"CanOcr":                 true,
		"CanBerkaskan":           true,
		"FilterUnitKerjaID":      c.Query("unit_kerja_id"),
		"FilterStatus":           c.Query("status"),
		"FilterKlasifikasiID":    c.Query("kode_klasifikasi_id"),
		"FilterLokasiID":         c.Query("lokasi_arsip_id"),
		"FilterJenisArsipID":     c.Query("jenis_arsip_id"),
		"FilterJenisArsip":       c.Query("jenis_arsip"),
		"FilterStartDate":        c.Query("start_date"),
		"FilterEndDate":          c.Query("end_date"),
		"FilterBerkasDigital":    c.Query("status_berkas_digital"),

		"FilterNilaiMin":         c.Query("nilai_min"),
		"FilterNilaiMax":         c.Query("nilai_max"),
		"FilterPemberkasanID":    c.Query("pemberkasan_id"),
		"FilterSortBy":           c.Query("sort_by"),
		"FilterSortOrder":        c.Query("sort_order"),
		"AuthUser":               user,
		"HasFilters":             hasFilters,
		"FilterSearch":           c.Query("search"),
	})
}

func (h *ArsipHandler) Create(c *gin.Context) {
	var unitKerjaOptions []models.UnitKerja
	var kodeKlasifikasiOptions []models.KodeKlasifikasi
	var lokasiArsipOptions []models.LokasiArsip
	var jenisArsipOptions []models.JenisArsip
	var pemberkasanOptions []models.Pemberkasan

	database.DB.Order("nama_unit").Find(&unitKerjaOptions)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOptions)
	database.DB.Where("is_active = 1").Order("nama_lokasi").Find(&lokasiArsipOptions)
	database.DB.Order("nama_jenis").Find(&jenisArsipOptions)
	database.DB.Where("status_berkas = 'aktif'").Order("nama_pemberkasan").Find(&pemberkasanOptions)

	today := time.Now().Format("2006-01-02")

	// Check if coming from location-based flow
	fromLocation := c.Query("lokasi_arsip_id")
	selectedLokasiID := ""
	selectedLokasiName := ""
	if fromLocation != "" {
		selectedLokasiID = fromLocation
		var lokasi models.LokasiArsip
		if database.DB.First(&lokasi, "id = ?", fromLocation).Error == nil {
			selectedLokasiName = lokasi.NamaLokasi
		}
	}

	Render(c, 200, "arsip/create.html", gin.H{
		"title":                  "Tambah Arsip - SIMARC",
		"pageTitle":              "Tambah Arsip",
		"unitKerjaOptions":       unitKerjaOptions,
		"kodeKlasifikasiOptions": kodeKlasifikasiOptions,
		"lokasiArsipOptions":     lokasiArsipOptions,
		"jenisArsipOptions":      jenisArsipOptions,
		"pemberkasanOptions":     pemberkasanOptions,
		"PrefilledName":          "",
		"PrefilledUraian":        "",
		"TodayDate":              today,
		"FromLocation":           fromLocation != "",
		"SelectedLokasiID":       selectedLokasiID,
		"SelectedLokasiName":     selectedLokasiName,
	})
}

func validasiFileUpload(c *gin.Context) (string, bool) {
	file, err := c.FormFile("file")
	if err != nil {
		return "", true // file opsional, tidak wajib
	}
	
	// Validasi ukuran file (maks 32MB)
	const maxSize int64 = 32 * 1024 * 1024
	if file.Size > maxSize {
		return fmt.Sprintf("Ukuran file terlalu besar (%.1f MB). Maksimal 32 MB.", float64(file.Size)/(1024*1024)), false
	}
	
	// Validasi tipe file
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExt := map[string]bool{".pdf": true, ".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".tiff": true, ".tif": true, ".bmp": true}
	if !allowedExt[ext] {
		return fmt.Sprintf("Tipe file %s tidak diizinkan. Hanya: PDF, JPG, PNG, GIF, TIFF", ext), false
	}
	
	return "", true
}

func (h *ArsipHandler) Store(c *gin.Context) {
	user := middleware.GetCurrentUser(c)

	arsip := models.Arsip{
		ID:          uuid.New().String(),
		NamaArsip:   c.PostForm("nama_arsip"),
		Uraian:      c.PostForm("uraian"),
		NomorArsip:  c.PostForm("nomor_arsip"),
		StatusArsip: c.PostForm("status_arsip"),
		JenisArsip:  autoClassifySPJFromName(c.PostForm("nama_arsip"), c.PostForm("uraian"), c.PostForm("nomor_arsip")),
	}

	// Capture from_location early for error redirects
	fromLocation := c.PostForm("from_location")
	createURL := "/arsip/create"
	if fromLocation != "" {
		createURL = "/arsip/create?lokasi_arsip_id=" + fromLocation
	}

	if arsip.NamaArsip == "" || c.PostForm("kode_klasifikasi_id") == "" || c.PostForm("unit_kerja_id") == "" {
		middleware.SetFlash(c, "error", "Nama arsip, kode klasifikasi, dan unit kerja wajib diisi.")
		c.Redirect(http.StatusFound, createURL)

	// Validasi No. SPM wajib untuk kategori SPJ
	if c.PostForm("nomor_spm") == "" {
		// Cek apakah jenis arsip yang dipilih memiliki kode SPJ
		var selectedJenis models.JenisArsip
		if v := c.PostForm("jenis_arsip_id"); v != "" {
			if id, err := strconv.ParseUint(v, 10, 64); err == nil {
				if database.DB.First(&selectedJenis, id).Error == nil && selectedJenis.KodeJenis == "SPJ" {
					middleware.SetFlash(c, "error", "Nomor SPM wajib diisi untuk arsip SPJ.")
					c.Redirect(http.StatusFound, createURL)
					return
				}
			}
		}
	}
		return
	}
	
	// Validasi file upload
	if msg, ok := validasiFileUpload(c); !ok {
		middleware.SetFlash(c, "error", "Upload gagal: "+msg)
		c.Redirect(http.StatusFound, createURL)
		return
	}

	arsip.KodeKlasifikasiID = c.PostForm("kode_klasifikasi_id")
	arsip.UnitKerjaID = c.PostForm("unit_kerja_id")

	if v := c.PostForm("lokasi_arsip_id"); v != "" {
		arsip.LokasiArsipID = &v
	}
	if v := c.PostForm("pemberkasan_id"); v != "" {
		arsip.PemberkasanID = &v
	}
	if v := c.PostForm("jenis_arsip_id"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			uid := uint(id)
			arsip.JenisArsipID = &uid
		}
	}
	if v := c.PostForm("tanggal_dibuat"); v != "" {
		// Fix: handle multiple date formats (d-m-Y, Y-m-d, d/m/Y)
		v = strings.ReplaceAll(v, "/", "-")
		var t time.Time
		var err error
		if strings.Count(v, "-") == 2 {
			parts := strings.Split(v, "-")
			if len(parts[0]) == 4 {
				t, err = time.Parse("2006-01-02", v) // Y-m-d
			} else {
				t, err = time.Parse("02-01-2006", v) // d-m-Y
			}
		} else {
			t, err = time.Parse("2006-01-02", v)
		}
		if err == nil {
			arsip.TanggalDibuat = &t
		}
	}
	if v := c.PostForm("tanggal_retensi_berakhir"); v != "" {
		v = strings.ReplaceAll(v, "/", "-")
		var t time.Time
		var err error
		if strings.Count(v, "-") == 2 {
			parts := strings.Split(v, "-")
			if len(parts[0]) == 4 {
				t, err = time.Parse("2006-01-02", v)
			} else {
				t, err = time.Parse("02-01-2006", v)
			}
		} else {
			t, err = time.Parse("2006-01-02", v)
		}
		if err == nil {
			arsip.TanggalRetensiAkhir = &t
		}
	}
	if v := c.PostForm("nilai_anggaran"); v != "" {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64); err == nil {
			arsip.NilaiAnggaran = &f
		}
	}

	arsip.NomorSPM = c.PostForm("nomor_spm")

	arsip.TingkatPerkembangan = c.PostForm("tingkat_perkembangan")
	arsip.Jumlah = 1
	if v := c.PostForm("jumlah"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			arsip.Jumlah = n
		}
	}
	arsip.Satuan = c.PostForm("satuan")
	if arsip.Satuan == "Lainnya" {
		if v := c.PostForm("satuan_lainnya"); v != "" {
			arsip.Satuan = v
		}
	}
	if arsip.Satuan == "" {
		arsip.Satuan = "Berkas"
	}

	// Auto-calculate tanggal_retensi_berakhir from KodeKlasifikasi if not manually set
	if arsip.TanggalRetensiAkhir == nil && arsip.KodeKlasifikasiID != "" {
		var kk models.KodeKlasifikasi
		if database.DB.Where("id = ?", arsip.KodeKlasifikasiID).First(&kk).Error == nil {
			baseDate := time.Now()
			if arsip.TanggalDibuat != nil {
				baseDate = *arsip.TanggalDibuat
			}
			arsip.TanggalRetensiAkhir = models.HitungRetensiBerakhir(baseDate, &kk)
		}
	}

	// Handle file upload
	if file, err := c.FormFile("file"); err == nil {
		uploadDir := filepath.Join(config.UploadDir(), "arsip")
		os.MkdirAll(uploadDir, 0755)
		ext := filepath.Ext(file.Filename)
		filename := fmt.Sprintf("%d_%s%s", time.Now().Unix(), arsip.ID[:8], ext)
		dst := filepath.Join(uploadDir, filename)
		if err := c.SaveUploadedFile(file, dst); err != nil {
			middleware.SetFlash(c, "error", "Gagal menyimpan file: "+err.Error())
			c.Redirect(http.StatusFound, createURL)
			return
		}
		arsip.FilePath = dst
	}

	// Process file (image/PDF)
	if arsip.FilePath != "" {
		fileSvc := &services.FileProcessingService{}
		if fileSvc.IsImage(arsip.FilePath) {
			if meta, err := fileSvc.ProcessImage(arsip.FilePath); err == nil {
				if thumbPath, ok := meta["thumbnail_path"].(string); ok {
					arsip.FilePath = thumbPath
				}
			}
		} else if fileSvc.IsPDF(arsip.FilePath) {
			if meta, err := fileSvc.ProcessPDF(arsip.FilePath); err == nil {
				if text, ok := meta["text"].(string); ok && text != "" {
					arsip.OcrText = text
				}
			}
		}
	}

	// Defense-in-depth: guarantee ID is never empty before hitting MySQL.
	// MySQL rejects INSERT without id when column has no DEFAULT (Error 1364).
	if arsip.ID == "" {
		arsip.ID = uuid.New().String()
	}

if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&arsip).Error; err != nil {
			return err
		}
		
		// Log activity — failure here must NOT block arsip creation.
		actLog := models.ActivityLog{
			UserID:      &user.ID,
			Action:      "create",
			Description: "Menambah arsip: " + arsip.NamaArsip,
			ModelType:   "arsip",
			ModelID:     arsip.ID,
		}
		if err := tx.Create(&actLog).Error; err != nil {
			// Activity log failure is non-fatal — arsip was already saved.
			log.Printf("[WARN] Gagal log aktivitas untuk arsip %s: %v", arsip.ID, err)
		}
		
		// Note: blockchain audit isn't part of logActivity if we bypass it, but logActivity internally uses global DB.
		// So we will just use the tx instead of calling logActivity.
		return nil
	}); err != nil {

		middleware.SetFlash(c, "error", "Gagal menyimpan arsip: "+err.Error())
		c.Redirect(http.StatusFound, createURL)
		return
	}
	
	// Record audit block on the blockchain (after transaction committed)
	_ = (&services.BlockchainAuditService{}).RecordAudit("arsip", arsip.ID, "create", "Menambah arsip: "+arsip.NamaArsip, user.ID, c.ClientIP(), c.GetHeader("User-Agent"))

	msg := "Arsip berhasil ditambahkan."
	if arsip.NomorArsip != "" {
		msg = "Arsip #" + arsip.NomorArsip + " berhasil ditambahkan."
	}
	middleware.SetFlash(c, "success", msg)

	// If coming from location-based flow, redirect back to create form to add more
	if fromLocation != "" {
		middleware.SetFlash(c, "success", msg+" Silakan tambah arsip lainnya, atau klik Selesai jika sudah selesai.")
		c.Redirect(http.StatusFound, createURL)
		return
	}
	cache.InvalidatePrefix("dashboard:")
	c.Redirect(http.StatusFound, "/arsip")
}

func (h *ArsipHandler) Show(c *gin.Context) {
	id := c.Param("id")
	var arsip models.Arsip
	if err := database.DB.
		Preload("KodeKlasifikasi").
		Preload("UnitKerja").
		Preload("Pemberkasan").
		Preload("LokasiArsip").
		Preload("JenisArsipRel").
		First(&arsip, "id = ?", id).Error; err != nil {
		Render404(c)
		return
	}

	// Arsip versions
	var versions []models.ArsipVersion
	database.DB.Where("arsip_id = ?", id).Order("nomor_versi DESC").Find(&versions)

	// QR Code
	var qrCode models.QrCode
	database.DB.Where("arsip_id = ?", id).First(&qrCode)

	// File info
	fileExists := arsip.FilePath != ""
	fileExt := strings.TrimPrefix(filepath.Ext(arsip.FilePath), ".")
	if fileExt == "" {
		fileExt = "pdf"
	}
	var fileType string
	switch fileExt {
	case "pdf":
		fileType = "PDF Document"
	case "jpg", "jpeg":
		fileType = "JPEG Image"
	case "png":
		fileType = "PNG Image"
	case "doc", "docx":
		fileType = "Word Document"
	case "xls", "xlsx":
		fileType = "Excel Document"
	default:
		fileType = "Unknown"
	}

	daysSinceCreated := int(time.Since(arsip.CreatedAt).Hours() / 24)
	daysSinceUpdated := int(time.Since(arsip.UpdatedAt).Hours() / 24)

	var fileSize int64
	if fileExists {
		fullPath := arsip.FilePath
		if !filepath.IsAbs(fullPath) && !strings.HasPrefix(fullPath, "storage/") && !strings.HasPrefix(fullPath, "/tmp/") {
			fullPath = "storage/" + fullPath
		}
		if info, err := os.Stat(fullPath); err == nil {
			fileSize = info.Size()
		}
	}

	// User permissions
	user := middleware.GetCurrentUser(c)
	canEdit := user != nil && (user.IsAdmin() || (user.Role != nil && user.Role.Name == "Editor"))
	canDelete := user != nil && user.IsAdmin()

	Render(c, 200, "arsip/show.html", gin.H{
		"title":            arsip.NamaArsip + " - SIMARC",
		"pageTitle":        "Detail Arsip",
		"arsip":            arsip,
		"versions":         versions,
		"qrCode":           qrCode,
		"FileType":         fileType,
		"Extension":        fileExt,
		"FileSize":         fileSize,
		"IsOcrProcessed":   arsip.OcrProcessed,
		"DaysSinceCreated": daysSinceCreated,
		"DaysSinceUpdated": daysSinceUpdated,
		"CanEdit":          canEdit,
		"CanDelete":        canDelete,
		"AuthUser":         user,
		"VersionCount":     len(versions),
		"GDriveClientID":   config.App.GoogleDriveClientID,
		"GDriveFolderID":   config.App.GoogleDriveFolderID,
		"FileInfo": gin.H{
			"Exists":   fileExists,
			"Filename": arsip.FileName,
			"OcrText":  arsip.OcrFullText,
		},
	})
}

func (h *ArsipHandler) Edit(c *gin.Context) {
	id := c.Param("id")
	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", id).Error; err != nil {
		Render404(c)
		return
	}

	var unitKerjaOptions []models.UnitKerja
	var kodeKlasifikasiOptions []models.KodeKlasifikasi
	var lokasiArsipOptions []models.LokasiArsip
	var jenisArsipOptions []models.JenisArsip
	var pemberkasanOptions []models.Pemberkasan

	database.DB.Order("nama_unit").Find(&unitKerjaOptions)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOptions)
	database.DB.Where("is_active = 1").Order("nama_lokasi").Find(&lokasiArsipOptions)
	database.DB.Order("nama_jenis").Find(&jenisArsipOptions)
	database.DB.Order("nama_pemberkasan").Find(&pemberkasanOptions)

	Render(c, 200, "arsip/edit.html", gin.H{
		"title":                  "Edit Arsip - SIMARC",
		"pageTitle":              "Edit Arsip",
		"arsip":                  arsip,
		"unitKerjaOptions":       unitKerjaOptions,
		"kodeKlasifikasiOptions": kodeKlasifikasiOptions,
		"lokasiArsipOptions":     lokasiArsipOptions,
		"jenisArsipOptions":      jenisArsipOptions,
		"pemberkasanOptions":     pemberkasanOptions,
	})
}

func (h *ArsipHandler) Update(c *gin.Context) {
	id := c.Param("id")
	user := middleware.GetCurrentUser(c)

	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", id).Error; err != nil {
		middleware.SetFlash(c, "error", "Arsip tidak ditemukan.")
		c.Redirect(http.StatusFound, "/arsip")
		return
	}

	// Validasi No. SPM wajib untuk kategori SPJ
	if c.PostForm("nomor_spm") == "" {
		// Cek apakah jenis arsip yang dipilih memiliki kode SPJ
		var selectedJenis models.JenisArsip
		if v := c.PostForm("jenis_arsip_id"); v != "" {
			if idJenis, err := strconv.ParseUint(v, 10, 64); err == nil {
				if database.DB.First(&selectedJenis, idJenis).Error == nil && selectedJenis.KodeJenis == "SPJ" {
					middleware.SetFlash(c, "error", "Nomor SPM wajib diisi untuk arsip SPJ.")
					c.Redirect(http.StatusFound, "/arsip/"+id+"/edit")
					return
				}
			}
		}
	}

	arsip.NamaArsip = c.PostForm("nama_arsip")
	arsip.Uraian = c.PostForm("uraian")
	arsip.NomorArsip = c.PostForm("nomor_arsip")
	arsip.JenisArsip = autoClassifySPJFromName(c.PostForm("nama_arsip"), c.PostForm("uraian"), c.PostForm("nomor_arsip"))
	arsip.TingkatPerkembangan = c.PostForm("tingkat_perkembangan")
	arsip.Jumlah = 1
	if v := c.PostForm("jumlah"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			arsip.Jumlah = n
		}
	}
	arsip.Satuan = c.PostForm("satuan")
	if arsip.Satuan == "Lainnya" {
		if v := c.PostForm("satuan_lainnya"); v != "" {
			arsip.Satuan = v
		}
	}
	if arsip.Satuan == "" {
		arsip.Satuan = "Berkas"
	}
	arsip.KodeKlasifikasiID = c.PostForm("kode_klasifikasi_id")
	arsip.UnitKerjaID = c.PostForm("unit_kerja_id")

	if v := c.PostForm("lokasi_arsip_id"); v != "" {
		arsip.LokasiArsipID = &v
	} else {
		arsip.LokasiArsipID = nil
	}
	if v := c.PostForm("pemberkasan_id"); v != "" {
		arsip.PemberkasanID = &v
	} else {
		arsip.PemberkasanID = nil
	}
	if v := c.PostForm("jenis_arsip_id"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			uid := uint(id)
			arsip.JenisArsipID = &uid
		}
	} else {
		arsip.JenisArsipID = nil
	}
	if v := c.PostForm("tanggal_dibuat"); v != "" {
		// Fix: handle multiple date formats (d-m-Y, Y-m-d, d/m/Y)
		v = strings.ReplaceAll(v, "/", "-")
		var t time.Time
		var err error
		if strings.Count(v, "-") == 2 {
			parts := strings.Split(v, "-")
			if len(parts[0]) == 4 {
				t, err = time.Parse("2006-01-02", v) // Y-m-d
			} else {
				t, err = time.Parse("02-01-2006", v) // d-m-Y
			}
		} else {
			t, err = time.Parse("2006-01-02", v)
		}
		if err == nil {
			arsip.TanggalDibuat = &t
		}
	}
	if v := c.PostForm("tanggal_retensi_berakhir"); v != "" {
		v = strings.ReplaceAll(v, "/", "-")
		var t time.Time
		var err error
		if strings.Count(v, "-") == 2 {
			parts := strings.Split(v, "-")
			if len(parts[0]) == 4 {
				t, err = time.Parse("2006-01-02", v)
			} else {
				t, err = time.Parse("02-01-2006", v)
			}
		} else {
			t, err = time.Parse("2006-01-02", v)
		}
		if err == nil {
			arsip.TanggalRetensiAkhir = &t
		}
	}
	if v := c.PostForm("nilai_anggaran"); v != "" {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64); err == nil {

	arsip.NomorSPM = c.PostForm("nomor_spm")
			arsip.NilaiAnggaran = &f
		}
	}

	// Auto-calculate tanggal_retensi_berakhir from KodeKlasifikasi if not manually set
	if arsip.TanggalRetensiAkhir == nil && arsip.KodeKlasifikasiID != "" {
		var kk models.KodeKlasifikasi
		if database.DB.Where("id = ?", arsip.KodeKlasifikasiID).First(&kk).Error == nil {
			baseDate := time.Now()
			if arsip.TanggalDibuat != nil {
				baseDate = *arsip.TanggalDibuat
			}
			arsip.TanggalRetensiAkhir = models.HitungRetensiBerakhir(baseDate, &kk)
		}
	}

	// Validasi file upload
	if msg, ok := validasiFileUpload(c); !ok {
		middleware.SetFlash(c, "error", "Upload gagal: "+msg)
		c.Redirect(http.StatusFound, "/arsip/"+id+"/edit")
		return
	}
	// Handle file upload
	if file, err := c.FormFile("file"); err == nil {
		uploadDir := filepath.Join(config.UploadDir(), "arsip")
		os.MkdirAll(uploadDir, 0755)
		ext := filepath.Ext(file.Filename)
		filename := fmt.Sprintf("%d_%s%s", time.Now().Unix(), arsip.ID[:8], ext)
		dst := filepath.Join(uploadDir, filename)
		if err := c.SaveUploadedFile(file, dst); err != nil {
			middleware.SetFlash(c, "error", "Gagal menyimpan file: "+err.Error())
			c.Redirect(http.StatusFound, "/arsip/"+id+"/edit")
			return
		}
		arsip.FilePath = dst
	}
	// Process file (image/PDF)
	fileSvc := &services.FileProcessingService{}
	if arsip.FilePath != "" {
		if fileSvc.IsImage(arsip.FilePath) {
			if meta, err := fileSvc.ProcessImage(arsip.FilePath); err == nil {
				if thumbPath, ok := meta["thumbnail_path"].(string); ok {
					arsip.FilePath = thumbPath
				}
			}
		} else if fileSvc.IsPDF(arsip.FilePath) {
			if meta, err := fileSvc.ProcessPDF(arsip.FilePath); err == nil {
				if text, ok := meta["text"].(string); ok && text != "" {
					arsip.OcrText = text
				}
			}
		}
	}

	if err := database.DB.Save(&arsip).Error; err != nil {
		middleware.SetFlash(c, "error", "Gagal memperbarui arsip: "+err.Error())
		c.Redirect(http.StatusFound, "/arsip/"+id+"/edit")
		return
	}

	logActivity(user.ID, "update", "Memperbarui arsip: "+arsip.NamaArsip, "arsip", arsip.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	msg := "Arsip berhasil diperbarui."
	if arsip.NomorArsip != "" {
		msg = "Arsip #" + arsip.NomorArsip + " berhasil diperbarui."
	}
	middleware.SetFlash(c, "success", msg)
	cache.InvalidatePrefix("dashboard:")
	c.Redirect(http.StatusFound, "/arsip/"+id)
}

func (h *ArsipHandler) Destroy(c *gin.Context) {
	id := c.Param("id")
	user := middleware.GetCurrentUser(c)

	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", id).Error; err != nil {
		middleware.SetFlash(c, "error", "Arsip tidak ditemukan.")
		c.Redirect(http.StatusFound, "/arsip")
		return
	}

	database.DB.Delete(&arsip)
	cache.InvalidatePrefix("dashboard:")
	logActivity(user.ID, "delete", "Menghapus arsip: "+arsip.NamaArsip, "arsip", arsip.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Arsip berhasil dihapus.")
	c.Redirect(http.StatusFound, "/arsip")
}

func (h *ArsipHandler) Download(c *gin.Context) {
	id := c.Param("id")
	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", id).Error; err != nil || arsip.FilePath == "" {
		c.String(http.StatusNotFound, "File tidak ditemukan")
		return
	}

	fullPath := arsip.FilePath
	if !filepath.IsAbs(fullPath) && !strings.HasPrefix(fullPath, "storage/") && !strings.HasPrefix(fullPath, "/tmp/") {
		fullPath = "storage/" + fullPath
	}
	file, err := os.Open(fullPath)
	if err != nil {
		c.String(http.StatusNotFound, "File tidak ditemukan di server")
		return
	}
	defer file.Close()

	c.Header("Content-Disposition", "attachment; filename="+filepath.Base(fullPath))
	io.Copy(c.Writer, file)
}

func logActivity(userID, action, desc, modelType, modelID, ipAddress, userAgent string) {
	log := models.ActivityLog{
		UserID:      &userID,
		Action:      action,
		Description: desc,
		ModelType:   modelType,
		ModelID:     modelID,
		CreatedAt:   time.Now(),
	}
	database.DB.Create(&log)

	// Record audit block on the blockchain
	_ = (&services.BlockchainAuditService{}).RecordAudit(modelType, modelID, action, desc, userID, ipAddress, userAgent)
}

func (h *ArsipHandler) View(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", c.Param("id")).Error; err != nil || arsip.FilePath == "" {
		c.String(http.StatusNotFound, "File tidak ditemukan")
		return
	}
	ext := strings.ToLower(filepath.Ext(arsip.FilePath))
	contentType := "application/octet-stream"
	switch ext {
	case ".pdf":
		contentType = "application/pdf"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	}
	fullPath := arsip.FilePath
	if !filepath.IsAbs(fullPath) && !strings.HasPrefix(fullPath, "storage/") && !strings.HasPrefix(fullPath, "/tmp/") {
		fullPath = "storage/" + fullPath
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "inline; filename="+filepath.Base(fullPath))
	c.File(fullPath)
}

func (h *ArsipHandler) CheckFile(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"exists": false})
		return
	}
	fullPath := arsip.FilePath
	if !filepath.IsAbs(fullPath) && !strings.HasPrefix(fullPath, "storage/") && !strings.HasPrefix(fullPath, "/tmp/") {
		fullPath = "storage/" + fullPath
	}
	_, err := os.Stat(fullPath)
	c.JSON(http.StatusOK, gin.H{"exists": err == nil, "path": fullPath})
}

func (h *ArsipHandler) ShowMoveLocationForm(c *gin.Context) {
	// JSON response untuk modal (dideteksi dari Accept header atau query params)
	if c.GetHeader("Accept") == "application/json" || len(c.QueryArray("ids[]")) > 0 || c.Query("ids") != "" {
		arsipIDs := c.QueryArray("ids[]")
		if len(arsipIDs) == 0 {
			if ids := c.Query("ids"); ids != "" {
				arsipIDs = strings.Split(ids, ",")
			}
		}

		var archives []gin.H
		if len(arsipIDs) > 0 {
			var arsipList []models.Arsip
			database.DB.Preload("LokasiArsip").Where("id IN ?", arsipIDs).Find(&arsipList)
			for _, a := range arsipList {
				loc := "-"
				if a.LokasiArsip != nil {
					loc = a.LokasiArsip.NamaLokasi
				}
				archives = append(archives, gin.H{
					"nomor_arsip":     a.NomorArsip,
					"nama_arsip":      a.NamaArsip,
					"current_location": loc,
				})
			}
		}

		var lokasiOpts []models.LokasiArsip
		database.DB.Where("is_active = 1").Order("nama_lokasi").Find(&lokasiOpts)
		var locations []gin.H
		for _, l := range lokasiOpts {
			locations = append(locations, gin.H{
				"id":          l.ID,
				"nama_lokasi": l.NamaLokasi,
				"deskripsi":   l.Deskripsi,
			})
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
			"archives":  archives,
			"locations": locations,
		}})
		return
	}

	var arsipList []models.Arsip
	var lokasiOpts []models.LokasiArsip
	database.DB.Preload("KodeKlasifikasi").Preload("LokasiArsip").Where("deleted_at IS NULL").Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) ASC, arsip.created_at DESC").Find(&arsipList)
	database.DB.Where("is_active = 1").Order("nama_lokasi").Find(&lokasiOpts)
	Render(c, 200, "arsip/index.html", gin.H{
		"title": "Pindah Lokasi Arsip", "pageTitle": "Pindah Lokasi Arsip",
		"arsipList": arsipList, "lokasiOpts": lokasiOpts, "moveMode": true,
	})
}

func (h *ArsipHandler) MoveLocation(c *gin.Context) {
	user := middleware.GetCurrentUser(c)

	arsipIDs := c.PostFormArray("ids[]")
	if len(arsipIDs) == 0 {
		arsipIDs = c.PostFormArray("arsip_ids[]")
	}
	lokasiID := c.PostForm("lokasi_arsip_id")
	catatan := c.PostForm("catatan")

	if lokasiID == "" || len(arsipIDs) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Pilih arsip dan lokasi tujuan."})
		return
	}

	var lokasi models.LokasiArsip
	if err := database.DB.First(&lokasi, "id = ?", lokasiID).Error; err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Lokasi tujuan tidak ditemukan."})
		return
	}

	var arsipList []models.Arsip
	database.DB.Preload("LokasiArsip").Where("id IN ?", arsipIDs).Find(&arsipList)

	tx := database.DB.Begin()
	tx.Model(&models.Arsip{}).Where("id IN ?", arsipIDs).Update("lokasi_arsip_id", lokasiID)

	now := time.Now()
	ipAddr := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	userID := user.ID

	var activityLogs []models.ActivityLog

	var prev models.BlockchainAudit
	tx.Order("block_number DESC").First(&prev)
	prevHash := prev.CurrentHash
	blockNumber := prev.BlockNumber

	for _, a := range arsipList {
		oldLokasi := "-"
		if a.LokasiArsip != nil {
			oldLokasi = a.LokasiArsip.NamaLokasi
		}
		desc := fmt.Sprintf("Pindah arsip %s (%s) dari %s ke %s", a.NomorArsip, a.NamaArsip, oldLokasi, lokasi.NamaLokasi)
		if catatan != "" {
			desc += " — " + catatan
		}
		activityLogs = append(activityLogs, models.ActivityLog{
			UserID: &userID, Action: "move_location", Description: desc,
			ModelType: "arsip", ModelID: a.ID, CreatedAt: now,
		})
	}

	if len(activityLogs) > 0 {
		tx.Create(&activityLogs)
	}

	blockNumber++
	ids := make([]string, len(arsipList))
	for i, a := range arsipList { ids[i] = a.ID + ":" + a.NomorArsip }
	detail := fmt.Sprintf("Pindah %d arsip ke %s. IDs: [%s]%s", len(arsipList), lokasi.NamaLokasi, strings.Join(ids, ","),
		map[bool]string{true: " — " + catatan, false: ""}[catatan != ""])
	// Truncate detail if too long for blockchain
	if len(detail) > 5000 {
		detail = detail[:5000] + "..."
	}
	blockData := fmt.Sprintf("%s:%s:%s:%s:%s", "arsip", "batch", "move_location", detail, prevHash)
	currentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(blockData)))

	audit := models.BlockchainAudit{
		UUID: uuid.New().String(), PreviousHash: prevHash, CurrentHash: currentHash,
		BlockNumber: blockNumber, Timestamp: now.Format(time.RFC3339),
		UserID: &userID, Action: "move_location", EntityType: "arsip", EntityID: "batch:" + strings.Join(arsipIDs, ","),
		Details: detail, IPAddress: ipAddr, UserAgent: userAgent, IsValid: true,
	}
	tx.Create(&audit)

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("%d arsip berhasil dipindahkan ke %s", len(arsipIDs), lokasi.NamaLokasi),
		"data": gin.H{"arsip_ids": arsipIDs, "lokasi_tujuan": lokasi.NamaLokasi},
	})
}

// ── PEMINDAHAN ARSIP ─────────────────────────────────────────────────────────

func (h *ArsipHandler) PemindahanIndex(c *gin.Context) {
	var lokasiOpts []models.LokasiArsip
	database.DB.Where("is_active = 1").Order("nama_lokasi").Find(&lokasiOpts)

	var totalMutasi int64
	database.DB.Model(&models.ActivityLog{}).Where("action = 'move_location'").Count(&totalMutasi)

	var mutasiBulanIni int64
	database.DB.Model(&models.ActivityLog{}).
		Where("action = 'move_location' AND created_at >= ?", time.Now().AddDate(0, -1, 0)).
		Count(&mutasiBulanIni)

	var mutasiLogs []models.ActivityLog
	var totalLogs int64
	logsDB := database.DB.Model(&models.ActivityLog{}).Where("action = 'move_location'")
	logsDB.Count(&totalLogs)
	perPage := 20
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (int(totalLogs) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	logsDB.Preload("User").Order("created_at DESC").Limit(perPage).Offset(offset).Find(&mutasiLogs)

	var unitOpts []models.UnitKerja
	database.DB.Where("deleted_at IS NULL").Order("nama_unit").Find(&unitOpts)

	var totalArsip int64
	database.DB.Model(&models.Arsip{}).Where("deleted_at IS NULL").Count(&totalArsip)

	Render(c, 200, "arsip/pemindahan.html", gin.H{
		"title":          "Pemindahan Arsip - SIMARC",
		"pageTitle":      "Pemindahan Arsip",
		"LokasiOpts":     lokasiOpts,
		"UnitOpts":       unitOpts,
		"TotalArsip":     totalArsip,
		"TotalLokasi":    len(lokasiOpts),
		"TotalMutasi":    totalMutasi,
		"MutasiBulanIni": mutasiBulanIni,
		"MutasiLogs":     mutasiLogs,
		"TempatDefault":  "Kantor",
		"Total":          totalLogs, "Page": page, "PerPage": perPage,
		"TotalPages":     totalPages, "StartIndex": offset + 1,
		"FirstItem":      offset + 1,
		"LastItem":       offset + len(mutasiLogs),
		"Pagination":     BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":       totalPages > 1,
	})
}

func (h *ArsipHandler) PemindahanSearchJSON(c *gin.Context) {
	q := c.Query("q")
	unit := c.Query("unit")
	jenis := c.Query("jenis")
	lokasi := c.Query("lokasi")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 200
	offset := (page - 1) * limit

	db := database.DB.Model(&models.Arsip{}).
		Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").
		Where("arsip.deleted_at IS NULL")

	if q != "" {
		like := "%" + q + "%"
		db = db.Where("(arsip.nama_arsip LIKE ? OR arsip.nomor_arsip LIKE ? OR arsip.uraian LIKE ?)", like, like, like)
	}
	if unit != "" {
		db = db.Where("arsip.unit_kerja_id IN (SELECT id FROM unit_kerja WHERE nama_unit = ?)", unit)
	}
	if jenis != "" {
		db = db.Where("arsip.jenis_arsip = ?", jenis)
	}
	if lokasi != "" {
		db = db.Where("arsip.lokasi_arsip_id IN (SELECT id FROM lokasi_arsips WHERE nama_lokasi = ?)", lokasi)
	}

	var total int64
	db.Count(&total)

	var arsipList []models.Arsip
	db.Order("(CAST(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '') AS UNSIGNED)) ASC, arsip.created_at DESC").
		Limit(limit).Offset(offset).Find(&arsipList)

	var items []gin.H
	for _, a := range arsipList {
		lokasiID := ""
		if a.LokasiArsipID != nil {
			lokasiID = *a.LokasiArsipID
		}
		items = append(items, gin.H{
			"id":        a.ID,
			"nomor":     a.NomorArsip,
			"nama":      a.NamaArsip,
			"jenis":     a.JenisArsip,
			"unit":      unitSafe(a.UnitKerja),
			"lokasi":    lokasiSafe(a.LokasiArsip),
			"lokasi_id": lokasiID,
			"kk":        kkSafe(a.KodeKlasifikasi),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"total":   total,
		"page":    page,
		"pages":   (total + int64(limit) - 1) / int64(limit),
		"limit":   200,
	})
}

func unitSafe(u *models.UnitKerja) string {
	if u != nil { return u.NamaUnit }; return ""
}
func lokasiSafe(l *models.LokasiArsip) string {
	if l != nil { return l.NamaLokasi }; return ""
}
func kkSafe(k *models.KodeKlasifikasi) string {
	if k != nil { return k.KodeKlasifikasi }; return ""
}

func (h *ArsipHandler) PemindahanStore(c *gin.Context) {
	user := middleware.GetCurrentUser(c)

	// Terima arsip_ids[] (form) atau arsip_data (JSON string)
	arsipIDs := c.PostFormArray("arsip_ids[]")
	if len(arsipIDs) == 0 {
		jsonStr := c.PostForm("arsip_data")
		if jsonStr != "" {
			if err := json.Unmarshal([]byte(jsonStr), &arsipIDs); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Data arsip tidak valid"})
				return
			}
		}
	}

	lokasiID := c.PostForm("lokasi_arsip_id")
	catatan := c.PostForm("catatan")

	if lokasiID == "" || len(arsipIDs) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": "Pilih arsip dan lokasi tujuan."})
		return
	}

	var lokasi models.LokasiArsip
	if err := database.DB.First(&lokasi, "id = ?", lokasiID).Error; err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": "Lokasi tujuan tidak ditemukan."})
		return
	}

	// Ambil data arsip yang akan dipindahkan (termasuk lokasi asal)
	var arsipList []models.Arsip
	database.DB.Preload("LokasiArsip").Where("id IN ? AND deleted_at IS NULL", arsipIDs).Find(&arsipList)

	if len(arsipList) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Tidak ada arsip ditemukan."})
		return
	}

	// Validasi: filter arsip yang SUDAH berada di lokasi tujuan
	var alreadyThere []string
	var toMove []string
	lokasiAsalID := ""
	lokasiAsalNama := ""
	for _, a := range arsipList {
		if a.LokasiArsipID != nil && *a.LokasiArsipID == lokasiID {
			alreadyThere = append(alreadyThere, a.ID)
		} else {
			toMove = append(toMove, a.ID)
			// Catat lokasi asal dari arsip pertama
			if lokasiAsalID == "" && a.LokasiArsipID != nil {
				lokasiAsalID = *a.LokasiArsipID
				if a.LokasiArsip != nil {
					lokasiAsalNama = a.LokasiArsip.NamaLokasi
				}
			}
		}
	}

	// Jika semua arsip sudah di lokasi tujuan
	if len(toMove) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Semua arsip terpilih sudah berada di lokasi tujuan."})
		return
	}

	// Jika ada yang sudah di lokasi tujuan, peringatkan
	if len(alreadyThere) > 0 {
		if len(toMove) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fmt.Sprintf("%d arsip sudah berada di lokasi tujuan.", len(alreadyThere))})
			return
		}
	}

	now := time.Now()
	ipAddr := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	userID := user.ID

	// Blockchain: handle genesis case
	var prev models.BlockchainAudit
	prevHash := ""
	var blockNumber uint64 = 1
	if err := database.DB.Order("block_number DESC").First(&prev).Error; err == nil {
		prevHash = prev.CurrentHash
		blockNumber = prev.BlockNumber + 1 // both uint64
	}

	tx := database.DB.Begin()
	rollback := true
	defer func() {
		if rollback {
			tx.Rollback()
		}
	}()

	// 1 query UPDATE semua arsip yang akan dipindahkan
	inPH := make([]string, len(toMove))
	inArgs := make([]interface{}, 0, len(toMove)+1)
	inArgs = append(inArgs, lokasiID)
	for i, id := range toMove {
		inPH[i] = fmt.Sprintf("$%d", i+2)
		inArgs = append(inArgs, id)
	}
	res := tx.Exec(fmt.Sprintf("UPDATE arsip SET lokasi_arsip_id = $1 WHERE id IN (%s) AND deleted_at IS NULL", strings.Join(inPH, ",")), inArgs...)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal memperbarui lokasi arsip: " + res.Error.Error()})
		return
	}
	moved := res.RowsAffected

	if moved == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Tidak ada arsip yang dipindahkan."})
		return
	}

	// 1 log untuk seluruh batch
	desc := fmt.Sprintf("Pindah %d arsip ke %s", moved, lokasi.NamaLokasi)
	if lokasiAsalNama != "" {
		desc = fmt.Sprintf("Pindah %d arsip dari %s ke %s", moved, lokasiAsalNama, lokasi.NamaLokasi)
	}
	if catatan != "" {
		desc += " — " + catatan
	}
	if err := tx.Exec("INSERT INTO activity_logs (user_id,action,description,model_type,model_id,created_at) VALUES ($1,$2,$3,$4,$5,$6)",
		userID, "move_location", desc, "arsip", "batch:"+strings.Join(toMove, ","), now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal mencatat aktivitas: " + err.Error()})
		return
	}

	// 1 blockchain untuk seluruh batch
	batchIDs := "batch:" + strings.Join(toMove, ",")
	if len(batchIDs) > 5000 {
		batchIDs = batchIDs[:5000] + "..."
	}
	blockData := fmt.Sprintf("%s:%s:%s:%s:%s", "arsip", batchIDs, "move_location", desc, prevHash)
	if err := tx.Exec("INSERT INTO blockchain_audits (uuid,previous_hash,current_hash,block_number,timestamp,user_id,action,entity_type,entity_id,details,ip_address,user_agent,is_valid) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,1)",
		uuid.New().String(), prevHash, fmt.Sprintf("%x", sha256.Sum256([]byte(blockData))),
		blockNumber, now.Format(time.RFC3339), userID, "move_location", "arsip",
		batchIDs, desc, ipAddr, userAgent).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal mencatat blockchain: " + err.Error()})
		return
	}

	rollback = false
	tx.Commit()

	// Siapkan response termasuk lokasi asal untuk BA
	responseData := gin.H{
		"arsip_ids":     toMove,
		"lokasi_tujuan": lokasi.NamaLokasi,
		"lokasi_tujuan_id": lokasiID,
	}
	if lokasiAsalID != "" {
		responseData["lokasi_asal_id"] = lokasiAsalID
		responseData["lokasi_asal"] = lokasiAsalNama
	}
	if len(alreadyThere) > 0 {
		responseData["skipped"] = len(alreadyThere)
	}

	msg := fmt.Sprintf("%d arsip berhasil dipindahkan ke %s", moved, lokasi.NamaLokasi)
	if len(alreadyThere) > 0 {
		msg += fmt.Sprintf(" (%d arsip sudah di lokasi tujuan, dilewati)", len(alreadyThere))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": msg,
		"data":    responseData,
	})
}

// ── END PEMINDAHAN ──// ── END PEMINDAHAN ───────────────────────────────────────────────────────────

func (h *ArsipHandler) History(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		Render404(c)
		return
	}
	var logs []models.ActivityLog
	database.DB.Preload("User").Where("model_type = 'arsip' AND model_id = ?", arsip.ID).Order("created_at DESC").Find(&logs)
	var versions []models.ArsipVersion
	database.DB.Where("arsip_id = ?", arsip.ID).Order("nomor_versi DESC").Find(&versions)
	Render(c, 200, "arsip/history.html", gin.H{
		"title": "Riwayat Arsip", "pageTitle": "Riwayat Arsip", "arsip": arsip, "logs": logs, "versions": versions,
	})
}

func (h *ArsipHandler) HistoryJSON(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Arsip tidak ditemukan"})
		return
	}

	var logs []models.ActivityLog
	database.DB.Preload("User").Where("model_type = 'arsip' AND model_id = ?", arsip.ID).Order("created_at DESC").Find(&logs)

	var versions []models.ArsipVersion
	database.DB.Where("arsip_id = ?", arsip.ID).Order("nomor_versi DESC").Find(&versions)

	var bcAudits []models.BlockchainAudit
	database.DB.Where("entity_type = 'arsip' AND entity_id = ?", arsip.ID).Order("id ASC").Find(&bcAudits)

	unitKerjaName := "-"
	if arsip.UnitKerja != nil {
		unitKerjaName = arsip.UnitKerja.NamaUnit
	}
	locationName := "-"
	if arsip.LokasiArsip != nil {
		locationName = arsip.LokasiArsip.NamaLokasi
	}
	jenisArsipName := arsip.JenisArsip
	if jenisArsipName == "" {
		jenisArsipName = "-"
	}

	arsipData := map[string]interface{}{
		"id":               arsip.ID,
		"nomor_arsip":      arsip.NomorArsip,
		"nama_arsip":       arsip.NamaArsip,
		"current_location": locationName,
		"unit_kerja":       unitKerjaName,
		"jenis_arsip":      jenisArsipName,
	}

	// Calculate statistics
	var totalVersions int64
	database.DB.Model(&models.ArsipVersion{}).Where("arsip_id = ?", arsip.ID).Count(&totalVersions)

	var totalActions int64
	database.DB.Model(&models.ActivityLog{}).Where("model_type = 'arsip' AND model_id = ?", arsip.ID).Count(&totalActions)

	var locationChanges int64
	database.DB.Model(&models.ActivityLog{}).Where("model_type = 'arsip' AND model_id = ? AND (action = 'move_location' OR action = 'pindah' OR description LIKE ? OR description LIKE ?)", arsip.ID, "%Pindah%", "%lokasi%").Count(&locationChanges)

	isBcValid := false
	if len(bcAudits) > 0 {
		isBcValid = true
		svc := &services.BlockchainAuditService{}
		verifyRes := svc.VerifyEntityChain("arsip", arsip.ID)
		if isValid, ok := verifyRes["is_valid"].(bool); ok && !isValid {
			isBcValid = false
		}
	}

	blockchainVerifiedText := "Belum Terverifikasi"
	if isBcValid {
		blockchainVerifiedText = fmt.Sprintf("Terverifikasi (%d block)", len(bcAudits))
	} else if len(bcAudits) > 0 {
		blockchainVerifiedText = "Chain Tidak Valid"
	}

	statisticsData := map[string]interface{}{
		"total_versions":      totalVersions,
		"total_actions":       totalActions,
		"location_changes":    locationChanges,
		"blockchain_verified": blockchainVerifiedText,
		"last_modified":       arsip.UpdatedAt.Format("02 Jan 2006 15:04:05"),
	}

	// Map timeline
	var timelineItems []map[string]interface{}
	for _, log := range logs {
		color := "primary"
		icon := "info-circle"
		title := log.Action

		switch log.Action {
		case "create":
			color = "success"
			icon = "plus-circle"
			title = "Arsip Dibuat"
		case "update":
			color = "warning"
			icon = "pencil-square"
			title = "Arsip Diperbarui"
		case "delete":
			color = "danger"
			icon = "trash"
			title = "Arsip Dihapus"
		case "move_location":
			color = "info"
			icon = "geo-alt"
			title = "Lokasi Dipindahkan"
		case "berkaskan":
			color = "primary"
			icon = "folder-plus"
			title = "Arsip Diberkaskan"
		case "bulk_berkaskan":
			color = "primary"
			icon = "folder-plus"
			title = "Arsip Diberkaskan (Bulk)"
		case "keluarkan_pemberkasan":
			color = "secondary"
			icon = "folder-minus"
			title = "Dikeluarkan dari Pemberkasan"
		}

		userName := "System"
		if log.User != nil {
			userName = log.User.Name
			if userName == "" {
				userName = log.User.Username
			}
		}

		bcVerified := false
		bcHash := ""
		for _, audit := range bcAudits {
			if audit.Action == log.Action && audit.CreatedAt.Sub(log.CreatedAt).Abs() < 5*time.Second {
				bcVerified = true
				bcHash = audit.CurrentHash
				break
			}
		}

		timelineItems = append(timelineItems, map[string]interface{}{
			"color":               color,
			"icon":                icon,
			"title":               title,
			"formatted_time":      log.CreatedAt.Format("02 Jan 2006 15:04:05"),
			"description":         log.Description,
			"user":                userName,
			"blockchain_verified": bcVerified,
			"bc_hash":             bcHash,
		})
	}

	if timelineItems == nil {
		timelineItems = make([]map[string]interface{}, 0)
	}

	// Map versions
	userIds := []string{}
	for _, v := range versions {
		if v.ChangedBy != "" {
			userIds = append(userIds, v.ChangedBy)
		}
	}

	userMap := make(map[string]string)
	if len(userIds) > 0 {
		var users []models.User
		database.DB.Where("id IN ?", userIds).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u.Name
			if u.Name == "" {
				userMap[u.ID] = u.Username
			}
		}
	}

	var mappedVersions []map[string]interface{}
	for _, v := range versions {
		editorName := userMap[v.ChangedBy]
		if editorName == "" {
			editorName = "System"
		}
		filename := filepath.Base(v.FilePath)

		mappedVersions = append(mappedVersions, map[string]interface{}{
			"id":                v.ID,
			"arsip_id":          v.ArsipID,
			"version":           v.Version,
			"nomor_versi":       v.Version,
			"file_path":         v.FilePath,
			"filename":          filename,
			"diubah_oleh":       editorName,
			"user":              editorName,
			"catatan_perubahan": v.ChangeNote,
			"notes":             v.ChangeNote,
			"created_at":        v.CreatedAt.Format("02 Jan 2006 15:04:05"),
			"date":              v.CreatedAt.Format("02 Jan 2006 15:04:05"),
		})
	}

	if mappedVersions == nil {
		mappedVersions = make([]map[string]interface{}, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"arsip":      arsipData,
			"statistics": statisticsData,
			"timeline":   timelineItems,
			"versions":   mappedVersions,
		},
	})
}

func (h *ArsipHandler) HistoryPDF(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		c.String(http.StatusNotFound, "Arsip tidak ditemukan")
		return
	}
	var logs []models.ActivityLog
	database.DB.Preload("User").Where("model_type = 'arsip' AND model_id = ?", arsip.ID).Order("created_at DESC").Find(&logs)
	Render(c, 200, "arsip/history-pdf.html", gin.H{
		"title": "Riwayat Arsip", "arsip": arsip, "logs": logs,
	})
}

func (h *ArsipHandler) Versions(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		Render404(c)
		return
	}
	var versions []models.ArsipVersion
	database.DB.Where("arsip_id = ?", arsip.ID).Order("nomor_versi DESC").Find(&versions)
	Render(c, 200, "arsip/versions.html", gin.H{
		"title": "Versi Arsip", "pageTitle": "Versi Dokumen", "arsip": arsip, "versions": versions,
	})
}

func (h *ArsipHandler) DownloadVersion(c *gin.Context) {
	var v models.ArsipVersion
	if err := database.DB.First(&v, "id = ?", c.Param("versionId")).Error; err != nil || v.FilePath == "" {
		c.String(http.StatusNotFound, "File versi tidak ditemukan")
		return
	}
	fullPath := v.FilePath
	if !filepath.IsAbs(fullPath) && !strings.HasPrefix(fullPath, "storage/") && !strings.HasPrefix(fullPath, "/tmp/") {
		fullPath = "storage/" + fullPath
	}
	c.Header("Content-Disposition", "attachment; filename="+filepath.Base(fullPath))
	c.File(fullPath)
}

func (h *ArsipHandler) CheckVersionFile(c *gin.Context) {
	var v models.ArsipVersion
	if err := database.DB.First(&v, "id = ?", c.Param("versionId")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"exists": false})
		return
	}
	fullPath := v.FilePath
	if !filepath.IsAbs(fullPath) && !strings.HasPrefix(fullPath, "storage/") && !strings.HasPrefix(fullPath, "/tmp/") {
		fullPath = "storage/" + fullPath
	}
	_, err := os.Stat(fullPath)
	c.JSON(http.StatusOK, gin.H{"exists": err == nil, "path": fullPath})
}

func (h *ArsipHandler) ShowBerkaskanForm(c *gin.Context) {
	var arsip models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").First(&arsip, "id = ?", c.Param("id"))
	var pemberkasanOpts []models.Pemberkasan
	database.DB.Where("status_berkas = 'aktif'").Order("nama_pemberkasan").Find(&pemberkasanOpts)
	Render(c, 200, "arsip/show.html", gin.H{
		"title": "Berkaskan Arsip", "pageTitle": "Berkaskan Arsip", "arsip": arsip,
		"pemberkasanOpts": pemberkasanOpts, "berkaskanMode": true,
	})
}

func (h *ArsipHandler) ShowBerkaskanFormJSON(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Arsip tidak ditemukan"})
		return
	}
	var pemberkasanOpts []models.Pemberkasan
	database.DB.Where("status_berkas = 'aktif'").Order("nama_pemberkasan").Find(&pemberkasanOpts)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"arsip": gin.H{
				"id": arsip.ID,
				"nomor_arsip": arsip.NomorArsip,
				"nama_arsip": arsip.NamaArsip,
			},
			"pemberkasanOptions": pemberkasanOpts,
		},
	})
}

func (h *ArsipHandler) Berkaskan(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	pemberkasanID := c.PostForm("pemberkasan_id")
	catatan := c.PostForm("catatan")
	arsipID := c.Param("id")

	acceptJSON := strings.Contains(c.GetHeader("Accept"), "application/json")

	if pemberkasanID == "" {
		if acceptJSON {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Pilih pemberkasan tujuan."})
			return
		}
		middleware.SetFlash(c, "error", "Pilih pemberkasan tujuan.")
		c.Redirect(http.StatusFound, "/arsip/"+arsipID+"/berkaskan")
		return
	}

	var pemberkasan models.Pemberkasan
	if err := database.DB.First(&pemberkasan, "id = ?", pemberkasanID).Error; err != nil {
		if acceptJSON {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Pemberkasan tidak ditemukan."})
			return
		}
		middleware.SetFlash(c, "error", "Pemberkasan tidak ditemukan.")
		c.Redirect(http.StatusFound, "/arsip/"+arsipID+"/berkaskan")
		return
	}

	database.DB.Model(&models.Arsip{}).Where("id = ?", arsipID).Updates(map[string]interface{}{
		"pemberkasan_id": pemberkasanID, "status_arsip": "diberkaskan",
	})

	desc := fmt.Sprintf("Arsip diberkaskan ke %s (%s)", pemberkasan.NamaPemberkasan, pemberkasan.KodeBerkas)
	if catatan != "" {
		desc += " — " + catatan
	}
	logActivity(user.ID, "berkaskan", desc, "arsip", arsipID, c.ClientIP(), c.GetHeader("User-Agent"))

	if acceptJSON {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Arsip berhasil diberkaskan ke " + pemberkasan.NamaPemberkasan + "."})
		return
	}
	middleware.SetFlash(c, "success", "Arsip berhasil diberkaskan.")
	c.Redirect(http.StatusFound, "/arsip/"+arsipID)
}

func (h *ArsipHandler) BulkBerkaskan(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	arsipIDs := c.PostFormArray("arsip_ids[]")
	pemberkasanID := c.PostForm("pemberkasan_id")
	catatan := c.PostForm("catatan")

	acceptJSON := strings.Contains(c.GetHeader("Accept"), "application/json")

	if pemberkasanID == "" || len(arsipIDs) == 0 {
		if acceptJSON {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Pilih arsip dan pemberkasan tujuan."})
			return
		}
		middleware.SetFlash(c, "error", "Pilih arsip dan pemberkasan tujuan.")
		c.Redirect(http.StatusFound, "/arsip")
		return
	}

	var pemberkasan models.Pemberkasan
	if err := database.DB.First(&pemberkasan, "id = ?", pemberkasanID).Error; err != nil {
		if acceptJSON {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Pemberkasan tidak ditemukan."})
			return
		}
		middleware.SetFlash(c, "error", "Pemberkasan tidak ditemukan.")
		c.Redirect(http.StatusFound, "/arsip")
		return
	}

	database.DB.Model(&models.Arsip{}).Where("id IN ?", arsipIDs).Updates(map[string]interface{}{
		"pemberkasan_id": pemberkasanID, "status_arsip": "diberkaskan",
	})

	desc := fmt.Sprintf("%d arsip diberkaskan ke %s (%s)", len(arsipIDs), pemberkasan.NamaPemberkasan, pemberkasan.KodeBerkas)
	if catatan != "" {
		desc += " — " + catatan
	}
	logActivity(user.ID, "bulk_berkaskan", desc, "arsip", "", c.ClientIP(), c.GetHeader("User-Agent"))

	if acceptJSON {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("%d arsip berhasil diberkaskan ke %s.", len(arsipIDs), pemberkasan.NamaPemberkasan)})
		return
	}
	middleware.SetFlash(c, "success", fmt.Sprintf("%d arsip berhasil diberkaskan.", len(arsipIDs)))
	c.Redirect(http.StatusFound, "/arsip")
}

func (h *ArsipHandler) KeluarkanDariPemberkasan(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	database.DB.Model(&models.Arsip{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"pemberkasan_id": nil, "status_arsip": "aktif",
	})
	logActivity(user.ID, "keluarkan_pemberkasan", "Arsip dikeluarkan dari pemberkasan", "arsip", c.Param("id"), c.ClientIP(), c.GetHeader("User-Agent"))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Arsip berhasil dikeluarkan dari pemberkasan."})
}

// autoClassifySPJ determines if an archive is SPJ or Non-SPJ based on uraian containing "SPM"
func autoClassifySPJ(uraian string) string {
	upper := strings.ToUpper(uraian)
	if strings.Contains(upper, "SPM") || strings.Contains(upper, "SPJ") {
		return "SPJ"
	}
	return "Non SPJ"
}

func autoClassifySPJFromName(nama, uraian, nomor string) string {
	combined := " " + strings.ToUpper(nama+" "+uraian+" "+nomor) + " "
	if strings.Contains(combined, " SPM") || strings.Contains(combined, " SPM-") ||
		strings.Contains(combined, " SPM ") || strings.Contains(combined, "SPM-LS") ||
		strings.Contains(combined, "SPM-GU") || strings.Contains(combined, "SPM-TU") ||
		strings.Contains(combined, " SPJ") || strings.Contains(combined, " SPJ-") ||
		strings.Contains(combined, " SPJ ") ||
		strings.Contains(combined, " SPP ") || strings.Contains(combined, " SPP-") ||
		strings.Contains(combined, "/GU/") || strings.Contains(combined, "/LS/") ||
		strings.Contains(combined, "/TU/") {
		return "SPJ"
	}
	return "Non SPJ"
}

func (h *ArsipHandler) Suggestions(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}
	var results []struct {
		ID         string `json:"id"`
		NomorArsip string `json:"nomor_arsip"`
		NamaArsip  string `json:"nama_arsip"`
	}
	database.DB.Model(&models.Arsip{}).
		Select("id, nomor_arsip, nama_arsip").
		Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'')) @@ plainto_tsquery('simple', ?))", q).
		Limit(10).Find(&results)
	c.JSON(http.StatusOK, gin.H{"data": results})
}

func (h *ArsipHandler) ExportSearch(c *gin.Context) {
	format := c.PostForm("export_format")
	if format == "" {
		format = "excel"
	}

	q := c.PostForm("q")
	var arsipList []models.Arsip
	query := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip")
	if q != "" {
		query = query.Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'')) @@ plainto_tsquery('simple', ?))", q)
	}
	query.Order("created_at DESC").Limit(5000).Find(&arsipList)

	headers := []string{"No", "Nomor Arsip", "Nama Arsip", "Uraian", "Kode Klasifikasi", "Unit Kerja", "Lokasi", "Status", "Jumlah", "Satuan", "Tgl Dibuat", "Retensi"}
	var rows [][]string
	for i, a := range arsipList {
		kk := ""
		if a.KodeKlasifikasi != nil {
			kk = a.KodeKlasifikasi.KodeKlasifikasi + " - " + a.KodeKlasifikasi.NamaKlasifikasi
		}
		uk := ""
		if a.UnitKerja != nil {
			uk = a.UnitKerja.NamaUnit
		}
		lokasi := ""
		if a.LokasiArsip != nil {
			lokasi = a.LokasiArsip.NamaLokasi
		}
		tgl := "-"
		if a.TanggalDibuat != nil {
			tgl = a.TanggalDibuat.Format("2006-01-02")
		}
		retensi := "-"
		if a.TanggalRetensiAkhir != nil {
			retensi = a.TanggalRetensiAkhir.Format("2006-01-02")
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), a.NomorArsip, a.NamaArsip, a.Uraian, kk, uk, lokasi, a.StatusArsip, fmt.Sprintf("%d", a.Jumlah), a.Satuan, tgl, retensi})
	}

	if format == "pdf" {
		exportPDF(c, "Export-Arsip-" + time.Now().Format("2006-01-02"), "Hasil Pencarian Arsip", headers, rows)
	} else {
		exportXLSX(c, "Export-Arsip-" + time.Now().Format("2006-01-02"), headers, rows)
	}
}

func (h *ArsipHandler) ExtractMetadataAI(c *gin.Context) {
	text := c.PostForm("text")
	metadata := map[string]string{}
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(line), "nomor") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata["nomor_arsip"] = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(strings.ToLower(line), "tanggal") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata["tanggal"] = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(strings.ToLower(line), "perihal") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata["nama_arsip"] = strings.TrimSpace(parts[1])
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "metadata": metadata})
}

func (h *ArsipHandler) SemanticSearchAI(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}
	var results []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'') || ' ' || COALESCE(uraian,'') || ' ' || COALESCE(ocr_text,'') || ' ' || COALESCE(tags,'')) @@ plainto_tsquery('simple', ?))", q).
		Limit(20).Find(&results)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": results, "count": len(results)})
}

func (h *ArsipHandler) PublicView(c *gin.Context) {
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").
		First(&arsip, "id = ?", c.Param("id")).Error; err != nil {
		Render404(c)
		return
	}
	Render(c, 200, "arsip/public-view.html", gin.H{"title": arsip.NamaArsip, "arsip": arsip, "publicView": true})
}

func (h *ArsipHandler) ShowImportForm(c *gin.Context) {
	var unitKerjaOptions []models.UnitKerja
	database.DB.Order("nama_unit ASC").Find(&unitKerjaOptions)
	Render(c, 200, "arsip/import.html", gin.H{
		"title": "Import Arsip", "pageTitle": "Import Arsip dari Excel",
		"unitKerjaOptions": unitKerjaOptions,
	})
}

func (h *ArsipHandler) ImportExcel(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		middleware.SetFlash(c, "error", "File wajib diunggah.")
		c.Redirect(http.StatusFound, "/arsip/import")
		return
	}
	uploadDir := filepath.Join(config.StorageDir(), "imports")
	os.MkdirAll(uploadDir, 0755)
	dst := filepath.Join(uploadDir, fmt.Sprintf("import_%d%s", time.Now().Unix(), filepath.Ext(file.Filename)))
	if err := c.SaveUploadedFile(file, dst); err != nil {
		middleware.SetFlash(c, "error", "Gagal menyimpan file.")
		c.Redirect(http.StatusFound, "/arsip/import")
		return
	}

	f, err := excelize.OpenFile(dst)
	if err != nil {
		middleware.SetFlash(c, "error", "Gagal membaca file Excel: "+err.Error())
		c.Redirect(http.StatusFound, "/arsip/import")
		return
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil || len(rows) < 2 {
		middleware.SetFlash(c, "error", "File Excel kosong atau tidak valid.")
		c.Redirect(http.StatusFound, "/arsip/import")
		return
	}

	imported := 0
	var importErrors []string
	userID := ""
	if u := middleware.GetCurrentUser(c); u != nil {
		userID = u.ID
	}

	for i, row := range rows[1:] {
		if len(row) < 2 {
			continue
		}
		nomorArsip := getCell(row, 0)
		namaArsip := getCell(row, 1)
		if nomorArsip == "" || namaArsip == "" {
			continue
		}

		// Check for duplicates
		var existing int64
		database.DB.Model(&models.Arsip{}).Where("nomor_arsip = ?", nomorArsip).Count(&existing)
		if existing > 0 {
			importErrors = append(importErrors, fmt.Sprintf("Baris %d: Nomor arsip '%s' sudah ada.", i+2, nomorArsip))
			continue
		}

		arsip := models.Arsip{
			ID:          uuid.New().String(),
			NomorArsip:  nomorArsip,
			NamaArsip:   namaArsip,
			Uraian:      getCell(row, 5),
			StatusArsip: "aktif",
		}
		
		// Kolom 3: Kode Klasifikasi
		if kkCode := getCell(row, 2); kkCode != "" {
			var kk models.KodeKlasifikasi
			if err := database.DB.Where("kode_klasifikasi = ? OR nama_klasifikasi LIKE ?", kkCode, "%"+kkCode+"%").First(&kk).Error; err == nil {
				arsip.KodeKlasifikasiID = kk.ID
			}
		}
		// Kolom 4: Unit Kerja
		if ukName := getCell(row, 3); ukName != "" {
			var uk models.UnitKerja
			if database.DB.Where("nama_unit = ?", ukName).First(&uk).Error == nil {
				arsip.UnitKerjaID = uk.ID
			}
		}
		// Kolom 5: Status Arsip
		if status := getCell(row, 4); status != "" {
			arsip.StatusArsip = status
		}
		// Kolom 6: Uraian
		if uraian := getCell(row, 5); uraian != "" {
			arsip.Uraian = uraian
		}
		// Kolom 7: Jumlah
		if v := getCell(row, 6); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				arsip.Jumlah = n
			}
		}
		// Kolom 8: Satuan
		if v := getCell(row, 7); v != "" {
			arsip.Satuan = v
		}

		if err := database.DB.Create(&arsip).Error; err != nil {
			importErrors = append(importErrors, fmt.Sprintf("Baris %d: Gagal menyimpan '%s': %s", i+2, namaArsip, err.Error()))
			continue
		}
		imported++

		// Record blockchain audit
		_ = (&services.BlockchainAuditService{}).RecordAudit("arsip", arsip.ID, "import", "Import arsip dari Excel: "+namaArsip, userID, c.ClientIP(), c.GetHeader("User-Agent"))
	}

	if len(importErrors) > 0 {
		session := middleware.GetSession(c)
		session.Values["importErrors"] = importErrors
		middleware.SaveSession(c, session)
	}

	if imported > 0 {
		middleware.SetFlash(c, "success", fmt.Sprintf("Berhasil mengimport %d arsip dari file Excel.", imported))
	} else {
		middleware.SetFlash(c, "error", "Tidak ada arsip yang berhasil diimport. Periksa kembali file Excel Anda.")
	}
	c.Redirect(http.StatusFound, "/arsip/import")
}

func (h *ArsipHandler) DownloadTemplate(c *gin.Context) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Template"
	idx, _ := f.NewSheet(sheet)
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	headers := []string{"nomor_arsip", "nama_arsip", "kode_klasifikasi", "unit_kerja", "status_arsip", "uraian", "jumlah", "satuan"}
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
	})
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, style)
	}

	f.SetCellValue(sheet, "A2", "1")
	f.SetCellValue(sheet, "B2", "Contoh Arsip")
	f.SetCellValue(sheet, "C2", "KP01")
	f.SetCellValue(sheet, "D2", "Bagian Umum")
	f.SetCellValue(sheet, "E2", "aktif")
	f.SetCellValue(sheet, "F2", "Contoh uraian arsip")
	f.SetCellValue(sheet, "G2", "5")
	f.SetCellValue(sheet, "H2", "Berkas")

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", "attachment; filename=template-arsip.xlsx")
	f.Write(c.Writer)
}

func getCell(row []string, idx int) string {
	if idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

// ── GOOGLE DRIVE SYNC ──────────────────────────────────────────────────────

// GDriveSync receives a Google Drive file ID from the client-side upload
// and saves it on the arsip record. The actual file upload happens in the
// browser via Google Identity Services OAuth — the server only stores the
// reference.
func (h *ArsipHandler) GDriveSync(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		GoogleDriveFileID string `json:"google_drive_file_id"`
		GoogleDriveURL    string `json:"google_drive_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Payload tidak valid"})
		return
	}
	if req.GoogleDriveFileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "google_drive_file_id wajib diisi"})
		return
	}

	var arsip models.Arsip
	if err := database.DB.First(&arsip, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Arsip tidak ditemukan"})
		return
	}

	updates := map[string]interface{}{
		"google_drive_file_id": req.GoogleDriveFileID,
	}
	if req.GoogleDriveURL != "" {
		updates["google_drive_url"] = req.GoogleDriveURL
	}
	if err := database.DB.Model(&arsip).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menyimpan: " + err.Error()})
		return
	}

	user := middleware.GetCurrentUser(c)
	if user != nil {
		logActivity(user.ID, "gdrive_sync", "File arsip di-sync ke Google Drive: "+arsip.NamaArsip, "arsip", arsip.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "File berhasil di-sync ke Google Drive"})
}
