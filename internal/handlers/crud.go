package handlers

import (
	"net/http"
	"strconv"
	"time"

	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ── KODE KLASIFIKASI ──────────────────────────────────────────────────────────

type KodeKlasifikasiHandler struct{}

func (h *KodeKlasifikasiHandler) Index(c *gin.Context) {
	db := database.DB.Model(&models.KodeKlasifikasi{}).Preload("Parent")
	if q := c.Query("search"); q != "" {
		like := "%" + q + "%"
		db = db.Where("kode_klasifikasi LIKE ? OR nama_klasifikasi LIKE ?", like, like)
	}
	if v := c.Query("filter_penyusutan"); v != "" {
		db = db.Where("penyusutan_arsip = ?", v)
	}
	if v := c.Query("filter_keamanan"); v != "" {
		db = db.Where("klasifikasi_keamanan = ?", v)
	}
	if v := c.Query("filter_status"); v != "" {
		db = db.Where("is_active = ?", v)
	}
	var total int64
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
	var list []models.KodeKlasifikasi
	db.Order("kode_klasifikasi").Limit(perPage).Offset(offset).Find(&list)
	Render(c, 200, "kode-klasifikasi/index.html", gin.H{
		"title": "Kode Klasifikasi - SIMARC", "pageTitle": "Kode Klasifikasi",
		"List": list, "Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"Search":           c.Query("search"),
		"FilterPenyusutan": c.Query("filter_penyusutan"),
		"FilterKeamanan":   c.Query("filter_keamanan"),
		"FilterStatus":     c.Query("filter_status"),
		"FilterKategori":   c.Query("filter_kategori"),
		"RetensiMin":       c.Query("retensi_min"),
		"RetensiMax":       c.Query("retensi_max"),
		"FirstItem":        offset + 1,
		"LastItem":         offset + len(list),
		"Pagination":       BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"KategoriOptions":  []string{},
		"HasFilters":       c.Query("search") != "" || c.Query("filter_penyusutan") != "" || c.Query("filter_keamanan") != "" || c.Query("filter_status") != "" || c.Query("filter_kategori") != "" || c.Query("retensi_min") != "" || c.Query("retensi_max") != "",
		"HasPages":         totalPages > 1,
	})
}

func (h *KodeKlasifikasiHandler) Create(c *gin.Context) {
	var opts []models.KodeKlasifikasi
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&opts)
	Render(c, 200, "kode-klasifikasi/create.html", gin.H{"title": "Tambah Kode Klasifikasi", "pageTitle": "Tambah Kode Klasifikasi", "opts": opts})
}

func (h *KodeKlasifikasiHandler) Store(c *gin.Context) {
	isActive := c.PostForm("is_active") == "on" || c.PostForm("is_active") == "1"
	ra, _ := strconv.Atoi(c.PostForm("retensi_aktif"))
	ri, _ := strconv.Atoi(c.PostForm("retensi_inaktif"))
	m := models.KodeKlasifikasi{
		ID: uuid.New().String(), KodeKlasifikasi: c.PostForm("kode_klasifikasi"),
		NamaKlasifikasi: c.PostForm("nama_klasifikasi"), RetensiAktif: ra, RetensiInaktif: ri,
		PenyusutanArsip: c.PostForm("penyusutan_arsip"), KlasifikasiKeamanan: c.PostForm("klasifikasi_keamanan"),
		DasarPertimbangan: c.PostForm("dasar_pertimbangan"), IsActive: isActive,
	}
	if v := c.PostForm("parent_id"); v != "" {
		m.ParentID = &v
	}
	if err := database.DB.Create(&m).Error; err != nil {
		middleware.SetFlash(c, "error", "Gagal: "+err.Error())
		c.Redirect(http.StatusFound, "/kode-klasifikasi/create")
		return
	}
	middleware.SetFlash(c, "success", "Kode klasifikasi berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/kode-klasifikasi")
}

func (h *KodeKlasifikasiHandler) Edit(c *gin.Context) {
	var m models.KodeKlasifikasi
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/kode-klasifikasi")
		return
	}
	var opts []models.KodeKlasifikasi
	database.DB.Where("is_active = 1 AND id != ?", m.ID).Order("kode_klasifikasi").Find(&opts)
	Render(c, 200, "kode-klasifikasi/edit.html", gin.H{"title": "Edit Kode Klasifikasi", "pageTitle": "Edit Kode Klasifikasi", "m": m, "opts": opts})
}

func (h *KodeKlasifikasiHandler) Update(c *gin.Context) {
	var m models.KodeKlasifikasi
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/kode-klasifikasi")
		return
	}
	ra, _ := strconv.Atoi(c.PostForm("retensi_aktif"))
	ri, _ := strconv.Atoi(c.PostForm("retensi_inaktif"))
	m.KodeKlasifikasi = c.PostForm("kode_klasifikasi")
	m.NamaKlasifikasi = c.PostForm("nama_klasifikasi")
	m.RetensiAktif = ra
	m.RetensiInaktif = ri
	m.PenyusutanArsip = c.PostForm("penyusutan_arsip")
	m.KlasifikasiKeamanan = c.PostForm("klasifikasi_keamanan")
	m.DasarPertimbangan = c.PostForm("dasar_pertimbangan")
	m.IsActive = c.PostForm("is_active") == "on" || c.PostForm("is_active") == "1"
	if v := c.PostForm("parent_id"); v != "" {
		m.ParentID = &v
	} else {
		m.ParentID = nil
	}
	database.DB.Save(&m)
	middleware.SetFlash(c, "success", "Kode klasifikasi berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/kode-klasifikasi")
}

func (h *KodeKlasifikasiHandler) Destroy(c *gin.Context) {
	database.DB.Delete(&models.KodeKlasifikasi{}, "id = ?", c.Param("id"))
	middleware.SetFlash(c, "success", "Berhasil dihapus.")
	c.Redirect(http.StatusFound, "/kode-klasifikasi")
}

// ── UNIT KERJA ────────────────────────────────────────────────────────────────

type UnitKerjaHandler struct{}

func (h *UnitKerjaHandler) Index(c *gin.Context) {
	var list []models.UnitKerja
	var total int64
	db := database.DB.Model(&models.UnitKerja{})
	if q := c.Query("search"); q != "" {
		db = db.Where("nama_unit LIKE ?", "%"+q+"%")
	}
	db.Count(&total)
	perPage := 10
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
	db.Order("nama_unit").Limit(perPage).Offset(offset).Find(&list)
	type UnitKerjaWithCount struct {
		models.UnitKerja
		ArsipCount int `json:"arsip_count"`
		UsersCount int `json:"users_count"`
	}
	var enrichedList []UnitKerjaWithCount
	for _, u := range list {
		var ac, uc int64
		database.DB.Model(&models.Arsip{}).Where("unit_kerja_id = ?", u.ID).Count(&ac)
		database.DB.Model(&models.User{}).Where("unit_kerja_id = ?", u.ID).Count(&uc)
		enrichedList = append(enrichedList, UnitKerjaWithCount{UnitKerja: u, ArsipCount: int(ac), UsersCount: int(uc)})
	}
	Render(c, 200, "unit-kerja/index.html", gin.H{
		"title": "Unit Kerja - SIMARC", "pageTitle": "Unit Kerja",
		"List": enrichedList, "Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(list),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
		"HasFilters": false,
	})
}

func (h *UnitKerjaHandler) Create(c *gin.Context) {
	Render(c, 200, "unit-kerja/create.html", gin.H{"title": "Tambah Unit Kerja", "pageTitle": "Tambah Unit Kerja"})
}

func (h *UnitKerjaHandler) Store(c *gin.Context) {
	m := models.UnitKerja{ID: uuid.New().String(), NamaUnit: c.PostForm("nama_unit")}
	if m.NamaUnit == "" {
		middleware.SetFlash(c, "error", "Nama unit wajib diisi.")
		c.Redirect(http.StatusFound, "/unit-kerja/create")
		return
	}
	database.DB.Create(&m)
	middleware.SetFlash(c, "success", "Unit kerja berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/unit-kerja")
}

func (h *UnitKerjaHandler) Edit(c *gin.Context) {
	var m models.UnitKerja
	database.DB.First(&m, "id = ?", c.Param("id"))
	Render(c, 200, "unit-kerja/edit.html", gin.H{"title": "Edit Unit Kerja", "pageTitle": "Edit Unit Kerja", "m": m})
}

func (h *UnitKerjaHandler) Update(c *gin.Context) {
	database.DB.Model(&models.UnitKerja{}).Where("id = ?", c.Param("id")).Update("nama_unit", c.PostForm("nama_unit"))
	middleware.SetFlash(c, "success", "Unit kerja berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/unit-kerja")
}

func (h *UnitKerjaHandler) Destroy(c *gin.Context) {
	database.DB.Delete(&models.UnitKerja{}, "id = ?", c.Param("id"))
	middleware.SetFlash(c, "success", "Unit kerja berhasil dihapus.")
	c.Redirect(http.StatusFound, "/unit-kerja")
}

// ── LOKASI ARSIP ──────────────────────────────────────────────────────────────

type LokasiArsipHandler struct{}

func (h *LokasiArsipHandler) Index(c *gin.Context) {
	var list []models.LokasiArsip
	db := database.DB.Model(&models.LokasiArsip{})
	if q := c.Query("search"); q != "" {
		db = db.Where("nama_lokasi LIKE ? OR deskripsi LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	if fd := c.Query("filter_deskripsi"); fd != "" {
		db = db.Where("deskripsi LIKE ?", "%"+fd+"%")
	}
	var total int64
	db.Count(&total)
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	perPage := 10
	totalPages := (int(total) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	db.Order("nama_lokasi").Limit(perPage).Offset(offset).Find(&list)
	type LokasiWithCount struct {
		models.LokasiArsip
		ArsipCount int `json:"arsip_count"`
	}
	var enrichedList []LokasiWithCount
	for _, l := range list {
		var cnt int64
		database.DB.Model(&models.Arsip{}).Where("lokasi_arsip_id = ?", l.ID).Count(&cnt)
		enrichedList = append(enrichedList, LokasiWithCount{LokasiArsip: l, ArsipCount: int(cnt)})
	}
	Render(c, 200, "lokasi-arsip/index.html", gin.H{
		"title": "Lokasi Arsip - SIMARC", "pageTitle": "Lokasi Arsip",
		"List": enrichedList, "Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(list),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
		"HasFilters": c.Query("search") != "" || c.Query("filter_deskripsi") != "",
		"Search":     c.Query("search"),
		"FilterDeskripsi": c.Query("filter_deskripsi"),
	})
}

func (h *LokasiArsipHandler) Create(c *gin.Context) {
	Render(c, 200, "lokasi-arsip/create.html", gin.H{"title": "Tambah Lokasi Arsip", "pageTitle": "Tambah Lokasi Arsip"})
}

func (h *LokasiArsipHandler) Store(c *gin.Context) {
	isActive := c.PostForm("is_active") != "0"
	kap := c.PostForm("kapasitas")
	m := models.LokasiArsip{
		ID: uuid.New().String(), NamaLokasi: c.PostForm("nama_lokasi"),
		Deskripsi: c.PostForm("deskripsi"),
		IsActive: isActive,
	}
	if kap != "" {
		m.Kapasitas = &kap
	}
	database.DB.Create(&m)
	middleware.SetFlash(c, "success", "Lokasi arsip berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/lokasi-arsip")
}

func (h *LokasiArsipHandler) Edit(c *gin.Context) {
	var m models.LokasiArsip
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Lokasi arsip tidak ditemukan.")
		c.Redirect(http.StatusFound, "/lokasi-arsip")
		return
	}
	Render(c, 200, "lokasi-arsip/edit.html", gin.H{"title": "Edit Lokasi Arsip", "pageTitle": "Edit Lokasi Arsip", "Item": m})
}

func (h *LokasiArsipHandler) Update(c *gin.Context) {
	var m models.LokasiArsip
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Lokasi arsip tidak ditemukan.")
		c.Redirect(http.StatusFound, "/lokasi-arsip")
		return
	}
	m.NamaLokasi = c.PostForm("nama_lokasi")
	m.Deskripsi = c.PostForm("deskripsi")
	m.IsActive = c.PostForm("is_active") != "0"
	kap := c.PostForm("kapasitas")
	if kap != "" {
		m.Kapasitas = &kap
	} else {
		m.Kapasitas = nil
	}
	database.DB.Save(&m)
	middleware.SetFlash(c, "success", "Lokasi arsip berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/lokasi-arsip")
}

func (h *LokasiArsipHandler) Destroy(c *gin.Context) {
	database.DB.Model(&models.Arsip{}).Where("lokasi_arsip_id = ?", c.Param("id")).Update("lokasi_arsip_id", nil)
	database.DB.Delete(&models.LokasiArsip{}, "id = ?", c.Param("id"))
	middleware.SetFlash(c, "success", "Lokasi arsip berhasil dihapus.")
	c.Redirect(http.StatusFound, "/lokasi-arsip")
}

func (h *LokasiArsipHandler) Show(c *gin.Context) {
	var m models.LokasiArsip
	database.DB.First(&m, "id = ?", c.Param("id"))
	var arsipList []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("lokasi_arsip_id = ?", m.ID).Order("(REGEXP_REPLACE(arsip.nomor_arsip, '[^0-9]', '', 'g')::bigint) ASC, arsip.created_at DESC").Find(&arsipList)
	Render(c, 200, "lokasi-arsip/show.html", gin.H{
		"title": m.NamaLokasi, "pageTitle": "Detail Lokasi Arsip",
		"LokasiID": m.ID,
		"Item": gin.H{
			"NamaLokasi":           m.NamaLokasi,
			"Kapasitas":            m.Kapasitas,
			"IsActive":             m.IsActive,
			"Deskripsi":            m.Deskripsi,
			"ArsipCount":           len(arsipList),
			"ArsipList":            arsipList,
			"ID":                   m.ID,
		},
	})
}

// ── JENIS ARSIP ───────────────────────────────────────────────────────────────

type JenisArsipHandler struct{}

func (h *JenisArsipHandler) Index(c *gin.Context) {
	var list []models.JenisArsip
	db := database.DB.Model(&models.JenisArsip{})
	if q := c.Query("search"); q != "" {
		db = db.Where("nama_jenis LIKE ? OR kode_jenis LIKE ? OR keterangan LIKE ?", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}
	db.Order("nama_jenis").Find(&list)
	Render(c, 200, "jenis-arsip/index.html", gin.H{
		"title": "Jenis Arsip - SIMARC", "pageTitle": "Jenis Arsip",
		"List": list, "Search": c.Query("search"),
		"FirstItem":  1,
		"LastItem":   len(list),
		"HasPages":   false,
		"Pagination": "",
	})
}

func (h *JenisArsipHandler) Store(c *gin.Context) {
	m := models.JenisArsip{NamaJenis: c.PostForm("nama_jenis"), KodeJenis: c.PostForm("kode_jenis"), Keterangan: c.PostForm("keterangan")}
	database.DB.Create(&m)
	middleware.SetFlash(c, "success", "Jenis arsip berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/jenis-arsip")
}

func (h *JenisArsipHandler) Update(c *gin.Context) {
	database.DB.Model(&models.JenisArsip{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"nama_jenis": c.PostForm("nama_jenis"), "kode_jenis": c.PostForm("kode_jenis"), "keterangan": c.PostForm("keterangan"),
	})
	middleware.SetFlash(c, "success", "Jenis arsip berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/jenis-arsip")
}

func (h *JenisArsipHandler) Create(c *gin.Context) {
	Render(c, 200, "jenis-arsip/create.html", gin.H{"title": "Tambah Jenis Arsip", "pageTitle": "Tambah Jenis Arsip"})
}

func (h *JenisArsipHandler) Show(c *gin.Context) {
	var m models.JenisArsip
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/jenis-arsip")
		return
	}
	Render(c, 200, "jenis-arsip/show.html", gin.H{
		"title": m.NamaJenis, "pageTitle": "Detail Jenis Arsip", "m": m,
	})
}

func (h *JenisArsipHandler) Edit(c *gin.Context) {
	var m models.JenisArsip
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/jenis-arsip")
		return
	}
	Render(c, 200, "jenis-arsip/edit.html", gin.H{
		"title": "Edit Jenis Arsip", "pageTitle": "Edit Jenis Arsip", "m": m,
	})
}

func (h *JenisArsipHandler) Destroy(c *gin.Context) {
	database.DB.Delete(&models.JenisArsip{}, "id = ?", c.Param("id"))
	middleware.SetFlash(c, "success", "Jenis arsip berhasil dihapus.")
	c.Redirect(http.StatusFound, "/jenis-arsip")
}

// ── PEMBERKASAN ───────────────────────────────────────────────────────────────

type PemberkasanHandler struct{}

func (h *PemberkasanHandler) Index(c *gin.Context) {
	var list []models.Pemberkasan
	var total int64
	db := database.DB.Model(&models.Pemberkasan{}).Preload("Creator").Preload("UnitKerja")
	db.Count(&total)
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	perPage := 10
	totalPages := (int(total) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage
	db.Order("created_at DESC").Limit(perPage).Offset(offset).Find(&list)
	// Count arsip per pemberkasan
	type PemberkasanWithCount struct {
		models.Pemberkasan
		ArsipCount int `json:"arsip_count"`
	}
	var enrichedList []PemberkasanWithCount
	for _, p := range list {
		var cnt int64
		database.DB.Model(&models.Arsip{}).Where("pemberkasan_id = ?", p.ID).Count(&cnt)
		enrichedList = append(enrichedList, PemberkasanWithCount{Pemberkasan: p, ArsipCount: int(cnt)})
	}
	Render(c, 200, "pemberkasan/index.html", gin.H{
		"title": "Pemberkasan - SIMARC", "pageTitle": "Pemberkasan",
		"List": enrichedList, "Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(list),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
		"HasFilters": false,
	})
}

func (h *PemberkasanHandler) Create(c *gin.Context) {
	var unitKerjaOpts []models.UnitKerja
	var kodeKlasifikasiOpts []models.KodeKlasifikasi
	database.DB.Order("nama_unit").Find(&unitKerjaOpts)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOpts)
	Render(c, 200, "pemberkasan/create.html", gin.H{
		"title": "Tambah Pemberkasan", "pageTitle": "Tambah Pemberkasan",
		"unitKerjaOpts": unitKerjaOpts, "kodeKlasifikasiOpts": kodeKlasifikasiOpts,
	})
}

func (h *PemberkasanHandler) Store(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	tahun, _ := strconv.Atoi(c.PostForm("tahun"))
	m := models.Pemberkasan{
		ID: uuid.New().String(), KodeBerkas: c.PostForm("kode_berkas"),
		NamaPemberkasan: c.PostForm("nama_pemberkasan"), Tahun: tahun,
		StatusBerkas: "aktif", CreatedBy: &user.ID,
	}
	if v := c.PostForm("unit_kerja_id"); v != "" {
		m.UnitKerjaID = &v
	}
	if v := c.PostForm("kode_klasifikasi_id"); v != "" {
		m.KodeKlasifikasiID = &v
	}
	database.DB.Create(&m)
	middleware.SetFlash(c, "success", "Pemberkasan berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/pemberkasan")
}

func (h *PemberkasanHandler) Show(c *gin.Context) {
	var m models.Pemberkasan
	if err := database.DB.Preload("Creator").Preload("UnitKerja").Preload("KodeKlasifikasi").
		First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/pemberkasan")
		return
	}
	// Load arsip separately to ensure all related archives are fetched
	var arsipList []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("pemberkasan_id = ?", m.ID).
		Order("created_at DESC").
		Find(&arsipList)

	type PemberkasanShow struct {
		models.Pemberkasan
		ArsipCount int            `json:"arsip_count"`
		ArsipList  []models.Arsip `json:"arsip_list"`
	}
	Render(c, 200, "pemberkasan/show.html", gin.H{
		"title": m.NamaPemberkasan, "pageTitle": "Detail Pemberkasan",
		"Item": PemberkasanShow{
			Pemberkasan: m,
			ArsipCount:  len(arsipList),
			ArsipList:   arsipList,
		},
	})
}

func (h *PemberkasanHandler) Edit(c *gin.Context) {
	var m models.Pemberkasan
	database.DB.First(&m, "id = ?", c.Param("id"))
	var unitKerjaOpts []models.UnitKerja
	var kodeKlasifikasiOpts []models.KodeKlasifikasi
	database.DB.Order("nama_unit").Find(&unitKerjaOpts)
	database.DB.Where("is_active = 1").Order("kode_klasifikasi").Find(&kodeKlasifikasiOpts)
	Render(c, 200, "pemberkasan/edit.html", gin.H{
		"title": "Edit Pemberkasan", "pageTitle": "Edit Pemberkasan", "m": m,
		"unitKerjaOpts": unitKerjaOpts, "kodeKlasifikasiOpts": kodeKlasifikasiOpts,
	})
}
func (h *PemberkasanHandler) Update(c *gin.Context) {
	var m models.Pemberkasan
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Pemberkasan tidak ditemukan.")
		c.Redirect(http.StatusFound, "/pemberkasan")
		return
	}
	tahun, _ := strconv.Atoi(c.PostForm("tahun"))
	m.KodeBerkas = c.PostForm("kode_berkas")
	m.NamaPemberkasan = c.PostForm("nama_pemberkasan")
	m.Tahun = tahun
	m.StatusBerkas = c.PostForm("status_berkas")
	if v := c.PostForm("unit_kerja_id"); v != "" {
		m.UnitKerjaID = &v
	}
	database.DB.Save(&m)
	middleware.SetFlash(c, "success", "Pemberkasan berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/pemberkasan")
}

func (h *PemberkasanHandler) Destroy(c *gin.Context) {
	database.DB.Delete(&models.Pemberkasan{}, "id = ?", c.Param("id"))
	middleware.SetFlash(c, "success", "Pemberkasan berhasil dihapus.")
	c.Redirect(http.StatusFound, "/pemberkasan")
}

func (h *PemberkasanHandler) Close(c *gin.Context) {
	now := time.Now()
	database.DB.Model(&models.Pemberkasan{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status_berkas": "ditutup", "tanggal_tutup": now,
	})
	middleware.SetFlash(c, "success", "Pemberkasan berhasil ditutup.")
	c.Redirect(http.StatusFound, "/pemberkasan")
}
