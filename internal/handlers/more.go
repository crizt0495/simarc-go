package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ── USER ──────────────────────────────────────────────────────────────────────

type UserHandler struct{}

func (h *UserHandler) Index(c *gin.Context) {
	var users []models.User
	var total int64
	db := database.DB.Model(&models.User{}).Preload("Role").Preload("UnitKerja")
	if q := c.Query("search"); q != "" {
		like := "%" + q + "%"
		db = db.Where("username LIKE ? OR name LIKE ?", like, like)
	}
	db.Count(&total)
	perPage := 15
	page := 1
	if p, _ := strconv.Atoi(c.Query("page")); p > 0 {
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
	db.Order("name").Limit(perPage).Offset(offset).Find(&users)
	Render(c, 200, "users/index.html", gin.H{
		"title": "Pengguna - SIMARC", "pageTitle": "Manajemen Pengguna",
		"Users": users, "Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(users),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
		"HasFilters": false,
	})
}

func (h *UserHandler) Create(c *gin.Context) {
	var roles []models.Role
	var unitKerja []models.UnitKerja
	database.DB.Order("name").Find(&roles)
	database.DB.Order("nama_unit").Find(&unitKerja)
	Render(c, 200, "users/create.html", gin.H{"title": "Tambah Pengguna", "pageTitle": "Tambah Pengguna", "roles": roles, "unitKerja": unitKerja})
}

func (h *UserHandler) Store(c *gin.Context) {
	password := c.PostForm("password")
	if len(password) < 8 {
		middleware.SetFlash(c, "error", "Password minimal 8 karakter.")
		c.Redirect(http.StatusFound, "/users/create")
		return
	}
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	m := models.User{
		ID: uuid.New().String(), Username: c.PostForm("username"),
		Name: c.PostForm("name"), Password: string(hashed),
		RoleID: c.PostForm("role_id"), IsActive: c.PostForm("is_active") == "on",
	}
	if v := c.PostForm("unit_kerja_id"); v != "" {
		m.UnitKerjaID = &v
	}
	if err := database.DB.Create(&m).Error; err != nil {
		middleware.SetFlash(c, "error", "Gagal: "+err.Error())
		c.Redirect(http.StatusFound, "/users/create")
		return
	}
	currentUser := middleware.GetCurrentUser(c)
	logActivity(currentUser.ID, "user_create", "Menambah pengguna: "+m.Username, "users", m.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Pengguna berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/users")
}

func (h *UserHandler) Edit(c *gin.Context) {
	var user models.User
	if err := database.DB.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/users")
		return
	}
	var roles []models.Role
	var unitKerja []models.UnitKerja
	database.DB.Order("name").Find(&roles)
	database.DB.Order("nama_unit").Find(&unitKerja)
	Render(c, 200, "users/edit.html", gin.H{"title": "Edit Pengguna", "pageTitle": "Edit Pengguna", "User": user, "roles": roles, "unitKerja": unitKerja})
}

func (h *UserHandler) Update(c *gin.Context) {
	var user models.User
	if err := database.DB.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/users")
		return
	}
	user.Username = c.PostForm("username")
	user.Name = c.PostForm("name")
	user.RoleID = c.PostForm("role_id")
	user.IsActive = c.PostForm("is_active") == "on"
	if v := c.PostForm("unit_kerja_id"); v != "" {
		user.UnitKerjaID = &v
	} else {
		user.UnitKerjaID = nil
	}
	if pw := c.PostForm("password"); pw != "" {
		hashed, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		user.Password = string(hashed)
	}
	database.DB.Save(&user)
	currentUser := middleware.GetCurrentUser(c)
	logActivity(currentUser.ID, "user_update", "Memperbarui pengguna: "+user.Username, "users", user.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Pengguna berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/users")
}

func (h *UserHandler) Destroy(c *gin.Context) {
	database.DB.Delete(&models.User{}, "id = ?", c.Param("id"))
	currentUser := middleware.GetCurrentUser(c)
	logActivity(currentUser.ID, "user_delete", "Menghapus pengguna ID: "+c.Param("id"), "users", c.Param("id"), c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Pengguna berhasil dihapus.")
	c.Redirect(http.StatusFound, "/users")
}

// ── ROLE ──────────────────────────────────────────────────────────────────────

type RoleHandler struct{}

func (h *RoleHandler) Index(c *gin.Context) {
	var roles []models.Role
	var total int64
	db := database.DB.Model(&models.Role{})
	db.Count(&total)
	perPage := 15
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
	db.Preload("Permissions").Order("name").Limit(perPage).Offset(offset).Find(&roles)
	type RoleWithCount struct {
		models.Role
		UsersCount       int `json:"users_count"`
		PermissionsCount int `json:"permissions_count"`
	}
	var enriched []RoleWithCount
	var totalRoles, totalPermissions int64

	if len(roles) > 0 {
		roleIDs := make([]string, len(roles))
		for i, r := range roles {
			roleIDs[i] = r.ID
		}
		type userCountRow struct {
			RoleID string
			Count  int
		}
		var userCounts []userCountRow
		database.DB.Model(&models.User{}).
			Select("role_id, COUNT(*) as count").
			Where("role_id IN ?", roleIDs).
			Group("role_id").
			Scan(&userCounts)
		userMap := make(map[string]int)
		for _, r := range userCounts {
			userMap[r.RoleID] = r.Count
		}

		for _, r := range roles {
			enriched = append(enriched, RoleWithCount{
				Role:             r,
				UsersCount:       userMap[r.ID],
				PermissionsCount: len(r.Permissions),
			})
			totalRoles++
			totalPermissions += int64(len(r.Permissions))
		}
	}
	Render(c, 200, "role/index.html", gin.H{
		"title": "Role - SIMARC", "pageTitle": "Manajemen Role",
		"Roles": enriched, "TotalRoles": totalRoles, "TotalPermissions": totalPermissions,
		"Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(roles),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}

func (h *RoleHandler) Show(c *gin.Context) {
	var role models.Role
	if err := database.DB.Preload("Permissions").First(&role, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/roles")
		return
	}
	var users []models.User
	database.DB.Where("role_id = ?", role.ID).Preload("UnitKerja").Find(&users)
	var uc int64
	database.DB.Model(&models.User{}).Where("role_id = ?", role.ID).Count(&uc)
	type RoleShowData struct {
		models.Role
		UsersCount       int `json:"users_count"`
		PermissionsCount int `json:"permissions_count"`
	}
	permsByModule := map[string][]models.Permission{}
	for _, p := range role.Permissions {
		permsByModule[p.Module] = append(permsByModule[p.Module], p)
	}
	Render(c, 200, "role/show.html", gin.H{
		"title": "Detail Role", "pageTitle": "Detail Role",
		"Role": RoleShowData{
			Role:             role,
			UsersCount:       int(uc),
			PermissionsCount: len(role.Permissions),
		},
		"PermissionsByModule": permsByModule,
		"Users":               users,
	})
}

func (h *RoleHandler) Create(c *gin.Context) {
	Render(c, 200, "role/create.html", gin.H{"title": "Tambah Role", "pageTitle": "Tambah Role"})
}

func (h *RoleHandler) Store(c *gin.Context) {
	m := models.Role{ID: uuid.New().String(), Name: c.PostForm("name"), NamaRole: c.PostForm("nama_role"), Keterangan: c.PostForm("keterangan")}
	if err := database.DB.Create(&m).Error; err != nil {
		middleware.SetFlash(c, "error", "Gagal: "+err.Error())
		c.Redirect(http.StatusFound, "/roles/create")
		return
	}
	currentUser := middleware.GetCurrentUser(c)
	logActivity(currentUser.ID, "role_create", "Menambah role: "+m.NamaRole, "roles", m.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Role berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/roles")
}

func (h *RoleHandler) Edit(c *gin.Context) {
	var role models.Role
	if err := database.DB.First(&role, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Role tidak ditemukan.")
		c.Redirect(http.StatusFound, "/roles")
		return
	}
	Render(c, 200, "role/edit.html", gin.H{"title": "Edit Role", "pageTitle": "Edit Role", "Role": role})
}

func (h *RoleHandler) Update(c *gin.Context) {
	systemRoles := map[string]bool{"Admin": true, "Pimpinan": true, "Petugas": true, "Viewer": true}
	var role models.Role
	if err := database.DB.First(&role, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Role tidak ditemukan.")
		c.Redirect(http.StatusFound, "/roles")
		return
	}
	if !systemRoles[role.Name] {
		role.Name = c.PostForm("name")
	}
	role.NamaRole = c.PostForm("nama_role")
	role.Keterangan = c.PostForm("keterangan")
	database.DB.Save(&role)
	currentUser := middleware.GetCurrentUser(c)
	logActivity(currentUser.ID, "role_update", "Memperbarui role: "+role.NamaRole, "roles", role.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Role berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/roles")
}
func (h *RoleHandler) Destroy(c *gin.Context) {
	var role models.Role
	if err := database.DB.First(&role, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Role tidak ditemukan.")
		c.Redirect(http.StatusFound, "/roles")
		return
	}
	systemRoles := map[string]bool{"Admin": true, "Pimpinan": true, "Petugas": true, "Viewer": true}
	if systemRoles[role.Name] {
		middleware.SetFlash(c, "error", "Tidak dapat menghapus role sistem.")
		c.Redirect(http.StatusFound, "/roles")
		return
	}
	database.DB.Delete(&role)
	currentUser := middleware.GetCurrentUser(c)
	logActivity(currentUser.ID, "role_delete", "Menghapus role: "+role.NamaRole, "roles", role.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Role berhasil dihapus.")
	c.Redirect(http.StatusFound, "/roles")
}

func (h *RoleHandler) EditPermissions(c *gin.Context) {
	var role models.Role
	if err := database.DB.Preload("Permissions").First(&role, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/roles")
		return
	}
	var permissions []models.Permission
	database.DB.Where("is_active = 1").Order("module, name").Find(&permissions)
	// Group by module
	permsByModule := map[string][]models.Permission{}
	for _, p := range permissions {
		permsByModule[p.Module] = append(permsByModule[p.Module], p)
	}
	rolePermIDs := map[string]bool{}
	var rolePermissions []string
	for _, p := range role.Permissions {
		rolePermIDs[p.ID] = true
		rolePermissions = append(rolePermissions, p.ID)
	}
	Render(c, 200, "role/permissions.html", gin.H{
		"title": "Kelola Permission", "pageTitle": "Kelola Permission",
		"Role":             role,
		"role":             role, // fallback
		"Permissions":      permsByModule,
		"permsByModule":    permsByModule, // fallback
		"RolePermissions":  rolePermissions,
		"rolePermIDs":      rolePermIDs, // fallback
		"TotalPermissions": len(permissions),
	})
}

func (h *RoleHandler) UpdatePermissions(c *gin.Context) {
	var role models.Role
	if err := database.DB.First(&role, "id = ?", c.Param("id")).Error; err != nil {
		c.Redirect(http.StatusFound, "/roles")
		return
	}
	if role.Name == "Admin" {
		middleware.SetFlash(c, "error", "Tidak dapat mengubah permission role Admin.")
		c.Redirect(http.StatusFound, "/roles/"+role.ID+"/permissions")
		return
	}

	permIDs := c.PostFormArray("permissions[]")
	var permissions []models.Permission
	if len(permIDs) > 0 {
		database.DB.Where("id IN ?", permIDs).Find(&permissions)
	}

	if err := database.DB.Model(&role).Association("Permissions").Replace(&permissions); err != nil {
		middleware.SetFlash(c, "error", "Gagal memperbarui hak akses: "+err.Error())
	} else {
		// Log the security configuration change
		currentUser := middleware.GetCurrentUser(c)
		logActivity(currentUser.ID, "role_permissions_update", "Memperbarui hak akses role: "+role.NamaRole, "roles", role.ID, c.ClientIP(), c.GetHeader("User-Agent"))
		middleware.SetFlash(c, "success", "Hak akses berhasil diperbarui.")
	}

	c.Redirect(http.StatusFound, "/roles/"+role.ID+"/permissions")
}

// ── PEMUSNAHAN ARSIP ──────────────────────────────────────────────────────────

type PemusnahanHandler struct{}

// getExpiredArsipForPemusnahan returns archives whose retention has expired,
// matched with kode_klasifikasi where penyusutan_arsip = 'musnah',
// and not already in any active (diajukan/disetujui) pemusnahan.
func getExpiredArsipForPemusnahan() []models.Arsip {
	var expiredArsip []models.Arsip
	now := time.Now().Format("2006-01-02")
	database.DB.
		Preload("KodeKlasifikasi").
		Preload("UnitKerja").
		Joins("INNER JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id").
		Where("arsip.status_arsip NOT IN ('musnah', 'siap_penyusutan', 'permanen') AND arsip.tanggal_retensi_berakhir IS NOT NULL AND arsip.tanggal_retensi_berakhir < ?", now).
		Where("kode_klasifikasi.penyusutan_arsip = ?", "musnah").
		// Exclude arsip already in pemusnahan_arsip_items (new Go structure)
		Where("arsip.id NOT IN (SELECT pi.arsip_id FROM pemusnahan_arsip_items pi INNER JOIN pemusnahan_arsip pa ON pa.id = pi.pemusnahan_id WHERE pa.status IN ('diajukan','disetujui') AND pa.deleted_at IS NULL)").
		// Exclude arsip already in pemusnahan_arsip.arsip_id (legacy Laravel structure, backward compat)
		Where("arsip.id NOT IN (SELECT pa2.arsip_id FROM pemusnahan_arsip pa2 WHERE pa2.arsip_id IS NOT NULL AND pa2.arsip_id != '' AND pa2.status IN ('diajukan','disetujui') AND pa2.deleted_at IS NULL)").
		Where("arsip.deleted_at IS NULL").
		Order("arsip.tanggal_retensi_berakhir ASC").
		Limit(100).
		Find(&expiredArsip)
	return expiredArsip
}

func (h *PemusnahanHandler) Index(c *gin.Context) {
	var list []models.PemusnahanArsip
	var total int64
	db := database.DB.Model(&models.PemusnahanArsip{}).Preload("Creator").Preload("Items").Preload("Items.Arsip")
	if v := c.Query("status"); v != "" {
		db = db.Where("status = ?", v)
	}
	db.Count(&total)
	page := 1
	if p, _ := strconv.Atoi(c.Query("page")); p > 0 {
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

	var stats struct {
		Total     int64 `gorm:"column:total"`
		Diajukan  int64 `gorm:"column:diajukan"`
		Disetujui int64 `gorm:"column:disetujui"`
		Ditolak   int64 `gorm:"column:ditolak"`
		Auto      int64 `gorm:"column:auto"`
		Manual    int64 `gorm:"column:manual"`
	}
	database.DB.Raw(`SELECT COUNT(*) as total,
		SUM(CASE WHEN status='diajukan' THEN 1 ELSE 0 END) as diajukan,
		SUM(CASE WHEN status='disetujui' THEN 1 ELSE 0 END) as disetujui,
		SUM(CASE WHEN status='ditolak' THEN 1 ELSE 0 END) as ditolak,
		SUM(CASE WHEN is_auto=1 THEN 1 ELSE 0 END) as auto,
		SUM(CASE WHEN is_auto=0 OR is_auto IS NULL THEN 1 ELSE 0 END) as manual
		FROM pemusnahan_arsip WHERE deleted_at IS NULL`).Scan(&stats)

	// Hitung total arsip dengan status 'musnah' dari tabel arsip (sama dengan dashboard)
	var totalArsipMusnah int64
	database.DB.Model(&models.Arsip{}).Where("status_arsip = ? AND deleted_at IS NULL", "musnah").Count(&totalArsipMusnah)

	// Auto-detect: arsip yang masa retensinya habis dan siap dimusnahkan
	expiredArsip := getExpiredArsipForPemusnahan()

	Render(c, 200, "pemusnahan/index.html", gin.H{
		"title": "Pemusnahan Arsip - SIMARC", "pageTitle": "Pemusnahan Arsip",
		"List": list, "Total": total, "Stats": stats, "Page": page,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":    offset + 1,
		"HasPages":     totalPages > 1,
		"HasFilters":   false,
		"ExpiredArsip": expiredArsip,
		"TotalExpired": len(expiredArsip),
		"HasExpired":   len(expiredArsip) > 0,
		"TotalArsipMusnah": totalArsipMusnah,
	})
}

func (h *PemusnahanHandler) Create(c *gin.Context) {
	var arsipList []models.Arsip
	now := time.Now().Format("2006-01-02")
	db := database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Joins("INNER JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id").
		Where("arsip.status_arsip NOT IN ('musnah', 'siap_penyusutan', 'permanen')").
		Where("arsip.tanggal_retensi_berakhir IS NOT NULL AND arsip.tanggal_retensi_berakhir < ?", now).
		Where("kode_klasifikasi.penyusutan_arsip = ?", "musnah").
		Where("arsip.deleted_at IS NULL").
		Where("arsip.id NOT IN (SELECT pi.arsip_id FROM pemusnahan_arsip_items pi INNER JOIN pemusnahan_arsip pa ON pa.id = pi.pemusnahan_id WHERE pa.status IN ('diajukan','disetujui') AND pa.deleted_at IS NULL)").
		Where("arsip.id NOT IN (SELECT pa2.arsip_id FROM pemusnahan_arsip pa2 WHERE pa2.arsip_id IS NOT NULL AND pa2.arsip_id != '' AND pa2.status IN ('diajukan','disetujui') AND pa2.deleted_at IS NULL)").
		Order("arsip.tanggal_retensi_berakhir ASC")
	if q := c.Query("search"); q != "" {
		db = db.Where("(arsip.nama_arsip LIKE ? OR arsip.nomor_arsip LIKE ?)", "+"+q+"*")
	}
	if v := c.Query("kode_klasifikasi_id"); v != "" {
		db = db.Where("arsip.kode_klasifikasi_id = ?", v)
	}
	db.Limit(100).Find(&arsipList)

	// Get kode klasifikasi options for filter
	var kodeKlasifikasiOpts []models.KodeKlasifikasi
	database.DB.Where("penyusutan_arsip = ? AND is_active = 1", "musnah").Order("kode_klasifikasi").Find(&kodeKlasifikasiOpts)

	Render(c, 200, "pemusnahan/create.html", gin.H{
		"title": "Ajukan Pemusnahan", "pageTitle": "Ajukan Pemusnahan",
		"List": arsipList, "Search": c.Query("search"),
		"KodeKlasifikasiOptions": kodeKlasifikasiOpts,
		"FilterKodeKlasifikasi":  c.Query("kode_klasifikasi_id"),
	})
}

func (h *PemusnahanHandler) Store(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	arsipIDs := c.PostFormArray("arsip_ids[]")
	if len(arsipIDs) == 0 {
		middleware.SetFlash(c, "error", "Pilih minimal satu arsip.")
		c.Redirect(http.StatusFound, "/pemusnahan/create")
		return
	}
	now := time.Now()
	m := models.PemusnahanArsip{
		ID: uuid.New().String(), NamaKegiatan: c.PostForm("alasan_pengajuan"),
		TanggalPelaksanaan: &now, TanggalPengajuan: &now, Status: "diajukan", CreatedBy: &user.ID,
		UserPengajuID: user.ID,
		IsAuto: false,
	}
err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		for _, aid := range arsipIDs {
			if err := tx.Exec("INSERT INTO pemusnahan_arsip_items (pemusnahan_id, arsip_id, created_at) VALUES ($1, $2, $3)", m.ID, aid, now).Error; err != nil {
				return err
			}
		}
		// Update status arsip to siap_penyusutan
		if err := tx.Model(&models.Arsip{}).Where("id IN ?", arsipIDs).Update("status_arsip", "siap_penyusutan").Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		middleware.SetFlash(c, "error", "Gagal mengajukan pemusnahan: "+err.Error())
		c.Redirect(http.StatusFound, "/pemusnahan/create")
		return
	}
	middleware.SetFlash(c, "success", fmt.Sprintf("Pemusnahan berhasil diajukan untuk %d arsip.", len(arsipIDs)))
	c.Redirect(http.StatusFound, "/pemusnahan")
}

// AutoCreate automatically creates a pemusnahan record for all expired archives
// whose kode klasifikasi has penyusutan_arsip = 'musnah'.
func (h *PemusnahanHandler) AutoCreate(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	expiredArsip := getExpiredArsipForPemusnahan()
	if len(expiredArsip) == 0 {
		middleware.SetFlash(c, "info", "Tidak ada arsip yang masa retensinya habis.")
		c.Redirect(http.StatusFound, "/pemusnahan")
		return
	}
	now := time.Now()
	m := models.PemusnahanArsip{
		ID:                 uuid.New().String(),
		NamaKegiatan:       fmt.Sprintf("Pemusnahan Otomatis - Retensi Habis (%s)", now.Format("02 Jan 2006")),
		TanggalPelaksanaan: &now,
		TanggalPengajuan:   &now,
		Status:             "diajukan",
		CreatedBy:          &user.ID,
		UserPengajuID:      user.ID,
		IsAuto:             true,
		AlasanPengajuan:    "Otomatis oleh sistem: masa retensi arsip telah habis berdasarkan kode klasifikasi (penyusutan = musnah).",
	}
	arsipIDs := make([]string, 0, len(expiredArsip))
	for _, a := range expiredArsip {
		arsipIDs = append(arsipIDs, a.ID)
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		for _, a := range expiredArsip {
			if err := tx.Exec("INSERT INTO pemusnahan_arsip_items (pemusnahan_id, arsip_id, created_at) VALUES ($1, $2, $3)", m.ID, a.ID, now).Error; err != nil {
				return err
			}
		}
		// Update status arsip to siap_penyusutan
		if err := tx.Model(&models.Arsip{}).Where("id IN ?", arsipIDs).Update("status_arsip", "siap_penyusutan").Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		middleware.SetFlash(c, "error", "Gagal membuat pemusnahan otomatis: "+err.Error())
		c.Redirect(http.StatusFound, "/pemusnahan")
		return
	}
	middleware.SetFlash(c, "success", fmt.Sprintf("Pemusnahan otomatis berhasil dibuat untuk %d arsip yang masa retensinya habis.", len(expiredArsip)))
	c.Redirect(http.StatusFound, "/pemusnahan")
}

func (h *PemusnahanHandler) Approve(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	var m models.PemusnahanArsip
	if err := database.DB.Preload("Arsip").First(&m, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Pemusnahan tidak ditemukan.")
		c.Redirect(http.StatusFound, "/pemusnahan")
		return
	}
	database.DB.Model(&models.PemusnahanArsip{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "disetujui", "approved_by": user.ID, "tanggal_approve": time.Now(),
	})
	var arsipIDs []string
	database.DB.Model(&models.PemusnahanItem{}).
		Where("pemusnahan_id = ?", c.Param("id")).
		Pluck("arsip_id", &arsipIDs)
	if len(arsipIDs) > 0 {
		database.DB.Model(&models.Arsip{}).Where("id IN ?", arsipIDs).Update("status_arsip", "musnah")
	}
	middleware.SetFlash(c, "success", "Pemusnahan disetujui. Status arsip diubah menjadi musnah.")
	c.Redirect(http.StatusFound, "/pemusnahan")
}

func (h *PemusnahanHandler) Reject(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	database.DB.Model(&models.PemusnahanArsip{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "ditolak", "approved_by": user.ID,
	})
	var arsipIDs []string
	database.DB.Model(&models.PemusnahanItem{}).
		Where("pemusnahan_id = ?", c.Param("id")).
		Pluck("arsip_id", &arsipIDs)
	if len(arsipIDs) > 0 {
		database.DB.Model(&models.Arsip{}).Where("id IN ? AND status_arsip = ?", arsipIDs, "siap_penyusutan").Update("status_arsip", "inaktif")
	}
	middleware.SetFlash(c, "success", "Pemusnahan ditolak.")
	c.Redirect(http.StatusFound, "/pemusnahan")
}

// ── JADWAL RETENSI ────────────────────────────────────────────────────────────

type JadwalRetensiHandler struct{}

func (h *JadwalRetensiHandler) Index(c *gin.Context) {
	var list []models.JadwalRetensi
	var total int64
	db := database.DB.Model(&models.JadwalRetensi{})
	if q := c.Query("search"); q != "" {
		db = db.Where("nama_jadwal LIKE ?", "%"+q+"%")
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
	db.Preload("KodeKlasifikasi").Preload("UnitKerja").Order("nama_jadwal").Limit(perPage).Offset(offset).Find(&list)
	type JadwalRetensiWithMeta struct {
		models.JadwalRetensi
		IsOverdue          bool   `json:"is_overdue"`
		StatusBadgeClass   string `json:"status_badge_class"`
		ProgressPercentage int    `json:"progress_percentage"`
	}
	var enrichedList []JadwalRetensiWithMeta
	for _, j := range list {
		isOverdue := j.Status == "scheduled" && j.TanggalJadwal != nil && j.TanggalJadwal.Before(time.Now())
		badgeClass := "bg-secondary"
		switch j.Status {
		case "draft":
			badgeClass = "bg-secondary"
		case "scheduled":
			badgeClass = "bg-info"
		case "in_progress":
			badgeClass = "bg-warning"
		case "completed":
			badgeClass = "bg-success"
		case "cancelled":
			badgeClass = "bg-dark"
		case "overdue":
			badgeClass = "bg-danger"
		}
		progressPct := 0
		if j.TotalArsip > 0 {
			progressPct = j.ArsipDiproses * 100 / j.TotalArsip
		}
		enrichedList = append(enrichedList, JadwalRetensiWithMeta{
			JadwalRetensi:      j,
			IsOverdue:          isOverdue,
			StatusBadgeClass:   badgeClass,
			ProgressPercentage: progressPct,
		})
	}
	var totalSchedules, draft, scheduled, inProgress, completed, overdue int64
	database.DB.Model(&models.JadwalRetensi{}).Count(&totalSchedules)
	database.DB.Model(&models.JadwalRetensi{}).Where("status = 'draft'").Count(&draft)
	database.DB.Model(&models.JadwalRetensi{}).Where("status = 'scheduled'").Count(&scheduled)
	database.DB.Model(&models.JadwalRetensi{}).Where("status = 'in_progress'").Count(&inProgress)
	database.DB.Model(&models.JadwalRetensi{}).Where("status = 'completed'").Count(&completed)
	database.DB.Model(&models.JadwalRetensi{}).Where("status = 'overdue'").Count(&overdue)
	Render(c, 200, "jadwal-retensi/index.html", gin.H{
		"title": "Jadwal Retensi - SIMARC", "pageTitle": "Jadwal Retensi Arsip (JRA)",
		"List": enrichedList, "Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(enrichedList),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
		"Stats": gin.H{
			"TotalSchedules": totalSchedules,
			"Draft":          draft,
			"Scheduled":      scheduled,
			"InProgress":     inProgress,
			"Completed":      completed,
			"Overdue":        overdue,
		},
	})
}

func (h *JadwalRetensiHandler) Store(c *gin.Context) {
	ra, _ := strconv.Atoi(c.PostForm("retensi_aktif"))
	ri, _ := strconv.Atoi(c.PostForm("retensi_inaktif"))
	m := models.JadwalRetensi{
		ID: uuid.New().String(), NamaJadwal: c.PostForm("nama_jadwal"),
		RetensiAktif: ra, RetensiInaktif: ri, Nasib: c.PostForm("nasib"),
		Keterangan:  c.PostForm("keterangan"),
		Deskripsi:   c.PostForm("deskripsi"),
		JenisJadwal: c.PostForm("jenis_jadwal"),
		Status:      "draft",
		Catatan:     c.PostForm("catatan"),
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
	if v := c.PostForm("tanggal_pelaksanaan"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			m.TanggalPelaksanaan = &t
		}
	}
	if v := c.PostForm("tanggal_selesai"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			m.TanggalSelesai = &t
		}
	}
	m.TotalArsip, _ = strconv.Atoi(c.PostForm("total_arsip"))
	m.ArsipDiproses, _ = strconv.Atoi(c.PostForm("arsip_diproses"))
	user := middleware.GetCurrentUser(c)
	if user != nil {
		m.CreatedBy = &user.ID
	}
	database.DB.Create(&m)
	middleware.SetFlash(c, "success", "Jadwal retensi berhasil ditambahkan.")
	c.Redirect(http.StatusFound, "/jadwal-retensi")
}

func (h *JadwalRetensiHandler) Update(c *gin.Context) {
	var m models.JadwalRetensi
	if err := database.DB.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		middleware.SetFlash(c, "error", "Jadwal retensi tidak ditemukan.")
		c.Redirect(http.StatusFound, "/jadwal-retensi")
		return
	}
	ra, _ := strconv.Atoi(c.PostForm("retensi_aktif"))
	ri, _ := strconv.Atoi(c.PostForm("retensi_inaktif"))
	m.NamaJadwal = c.PostForm("nama_jadwal")
	m.RetensiAktif = ra
	m.RetensiInaktif = ri
	m.Nasib = c.PostForm("nasib")
	m.Keterangan = c.PostForm("keterangan")
	m.Deskripsi = c.PostForm("deskripsi")
	m.JenisJadwal = c.PostForm("jenis_jadwal")
	m.Status = c.PostForm("status")
	m.Catatan = c.PostForm("catatan")
	if v := c.PostForm("tanggal_jadwal"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			m.TanggalJadwal = &t
		}
	} else {
		m.TanggalJadwal = nil
	}
	if v := c.PostForm("tanggal_pelaksanaan"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			m.TanggalPelaksanaan = &t
		}
	} else {
		m.TanggalPelaksanaan = nil
	}
	if v := c.PostForm("tanggal_selesai"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			m.TanggalSelesai = &t
		}
	} else {
		m.TanggalSelesai = nil
	}
	m.TotalArsip, _ = strconv.Atoi(c.PostForm("total_arsip"))
	m.ArsipDiproses, _ = strconv.Atoi(c.PostForm("arsip_diproses"))
	database.DB.Save(&m)
	middleware.SetFlash(c, "success", "Jadwal retensi berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/jadwal-retensi")
}

func (h *JadwalRetensiHandler) Destroy(c *gin.Context) {
	database.DB.Delete(&models.JadwalRetensi{}, "id = ?", c.Param("id"))
	middleware.SetFlash(c, "success", "Jadwal retensi berhasil dihapus.")
	c.Redirect(http.StatusFound, "/jadwal-retensi")
}

// ── PROFIL ────────────────────────────────────────────────────────────────────

type ProfilHandler struct{}

func (h *ProfilHandler) Index(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	var loginLogs []models.LoginLog
	database.DB.Where("user_id = ?", user.ID).Order("login_time DESC").Limit(10).Find(&loginLogs)
	Render(c, 200, "profil/index.html", gin.H{"title": "Profil - SIMARC", "pageTitle": "Profil Saya", "loginLogs": loginLogs})
}

func (h *ProfilHandler) Update(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	updates := map[string]interface{}{"name": c.PostForm("name")}
	if pw := c.PostForm("password"); pw != "" {
		if len(pw) < 8 {
			middleware.SetFlash(c, "error", "Password minimal 8 karakter.")
			c.Redirect(http.StatusFound, "/profil")
			return
		}
		hashed, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		updates["password"] = string(hashed)
	}
	database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates)
	middleware.SetFlash(c, "success", "Profil berhasil diperbarui.")
	c.Redirect(http.StatusFound, "/profil")
}

// ── PENGATURAN ────────────────────────────────────────────────────────────────

type PengaturanHandler struct{}

func (h *PengaturanHandler) Index(c *gin.Context) {
	var totalUsers, totalArsip, totalUnitKerja int64
	database.DB.Model(&models.User{}).Count(&totalUsers)
	database.DB.Model(&models.Arsip{}).Count(&totalArsip)
	database.DB.Model(&models.UnitKerja{}).Count(&totalUnitKerja)
	settings := loadAppSettings()

	user := middleware.GetCurrentUser(c)
	isAdmin := user != nil && user.IsAdmin()

	// Database connection info (only loaded for admins)
	dbInfo := gin.H{
		"Host": config.App.DBHost,
		"Port": config.App.DBPort,
		"Name": config.App.DBName,
		"User": config.App.DBUser,
	}
	if isAdmin {
		info := database.GetInfo()
		dbInfo["Connected"] = info.Connected
		dbInfo["Version"] = info.Version
		dbInfo["Tables"] = info.Tables
	}

	Render(c, 200, "pengaturan/index.html", gin.H{
		"title": "Pengaturan - SIMARC", "pageTitle": "Pengaturan Sistem",
		"Stats": gin.H{
			"TotalUsers":     totalUsers,
			"TotalArsip":     totalArsip,
			"TotalUnitKerja": totalUnitKerja,
		},
		"Settings":          settings,
		"DBInfo":            dbInfo,
		"CanManageDatabase": isAdmin,
		"GDriveClientID":    config.App.GoogleDriveClientID,
		"GDriveFolderID":    config.App.GoogleDriveFolderID,
	})
}

// ── SEARCH ────────────────────────────────────────────────────────────────────

type SearchHandler struct{}

func (h *SearchHandler) Results(c *gin.Context) {
	q := c.Query("q")
	var results []models.Arsip
	var total int64
	perPage := 25
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := 1
	offset := 0
	if q != "" {
		tsQuery := "(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'') || ' ' || COALESCE(uraian,'') || ' ' || COALESCE(ocr_text,'') || ' ' || COALESCE(tags,'')) @@ plainto_tsquery('simple', ?))"
		db := database.DB.Model(&models.Arsip{}).Where(tsQuery, q)
		db.Count(&total)
		totalPages = (int(total) + perPage - 1) / perPage
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}
		offset = (page - 1) * perPage
		database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Preload("LokasiArsip").
			Where(tsQuery, q).
			Limit(perPage).Offset(offset).Find(&results)
	}
	Render(c, 200, "search/results.html", gin.H{
		"title": "Hasil Pencarian - SIMARC", "pageTitle": "Pencarian Arsip",
		"query": q, "results": results, "count": len(results),
		"Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(results),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}


// ── PEMINJAMAN ────────────────────────────────────────────────────────────────

type PeminjamanHandler struct{}

func (h *PeminjamanHandler) Index(c *gin.Context) {
	var list []models.PeminjamanArsip
	var total int64
	db := database.DB.Model(&models.PeminjamanArsip{})
	db.Count(&total)
	perPage := 15
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
	db.Preload("Arsip").Preload("User").Order("created_at DESC").Limit(perPage).Offset(offset).Find(&list)
	Render(c, 200, "peminjaman/index.html", gin.H{
		"title": "Peminjaman Arsip - SIMARC", "pageTitle": "Peminjaman Arsip", "List": list,
		"Total": total, "Page": page, "PerPage": perPage,
		"TotalPages": totalPages, "StartIndex": offset + 1,
		"FirstItem":  offset + 1,
		"LastItem":   offset + len(list),
		"Pagination": BuildPagination(page, totalPages, removePageParam(c.Request.URL.RawQuery)),
		"HasPages":   totalPages > 1,
	})
}

func (h *PeminjamanHandler) Store(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	dueDate, _ := time.Parse("2006-01-02", c.PostForm("tanggal_due_date"))
	now := time.Now()
	m := models.PeminjamanArsip{
		ID: uuid.New().String(), ArsipID: c.PostForm("arsip_id"),
		UserID: user.ID, TanggalPinjam: &now, TanggalKembaliRencana: &dueDate,
		Keperluan: c.PostForm("keperluan"), Status: "pending",
	}
	database.DB.Create(&m)
	logActivity(user.ID, "peminjaman_create", "Mengajukan peminjaman arsip ID: "+m.ArsipID, "peminjaman", m.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Peminjaman berhasil diajukan.")
	c.Redirect(http.StatusFound, "/peminjaman")
}

func (h *PeminjamanHandler) Approve(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	database.DB.Model(&models.PeminjamanArsip{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "disetujui", "approved_by": user.ID,
	})
	logActivity(user.ID, "peminjaman_approve", "Menyetujui peminjaman ID: "+c.Param("id"), "peminjaman", c.Param("id"), c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Peminjaman disetujui.")
	c.Redirect(http.StatusFound, "/peminjaman")
}

func (h *PeminjamanHandler) Return(c *gin.Context) {
	now := time.Now()
	database.DB.Model(&models.PeminjamanArsip{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
		"status": "dikembalikan", "tanggal_kembali": now,
	})
	user := middleware.GetCurrentUser(c)
	logActivity(user.ID, "peminjaman_return", "Mengembalikan arsip untuk peminjaman ID: "+c.Param("id"), "peminjaman", c.Param("id"), c.ClientIP(), c.GetHeader("User-Agent"))
	middleware.SetFlash(c, "success", "Arsip berhasil dikembalikan.")
	c.Redirect(http.StatusFound, "/peminjaman")
}

// ── APP SETTINGS ────────────────────────────────────────────────────────────
//
// Semua pengaturan (database + aplikasi) disimpan dalam SATU file konfigurasi
// .env. Fungsi-fungsi ini membaca/menulis .env melalui package config.

func loadAppSettings() gin.H {
	return gin.H{
		"app_name":     config.App.AppName,
		"app_timezone": config.App.AppTimezone,
	}
}

func saveAppSettings(appName, timezone string) error {
	oldName, oldTimezone := config.App.AppName, config.App.AppTimezone
	if appName != "" {
		config.App.AppName = appName
	}
	if timezone != "" {
		config.App.AppTimezone = timezone
	}
	// Simpan ke .env (satu-satunya file konfigurasi) + proses env
	if err := config.UpdateEnv(map[string]string{
		"APP_NAME":     config.App.AppName,
		"APP_TIMEZONE": config.App.AppTimezone,
	}); err != nil {
		// Kembalikan nilai memori agar konsisten dengan file yang gagal ditulis
		config.App.AppName = oldName
		config.App.AppTimezone = oldTimezone
		return err
	}
	return nil
}
