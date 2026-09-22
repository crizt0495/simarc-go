package database

import (
	"log"

	"arsippro/internal/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Seed creates default data if tables are empty
func Seed() {
	seedRoles()
	seedAdmin()
	seedLokasiRakA()
	seedJenisArsip()
}

// SeedIfNeeded seeds default data only when the database is fresh (no users
// yet). Unlike Seed(), it never resets existing passwords — this is used when
// switching databases at runtime so the admin password is never overwritten.
func SeedIfNeeded() {
	var userCount int64
	DB.Model(&models.User{}).Count(&userCount)
	if userCount > 0 {
		// Database already in use — only ensure the default location exists
		seedLokasiRakA()
		seedJenisArsip()
		return
	}
	seedRoles()
	seedAdmin()
	seedLokasiRakA()
	seedJenisArsip()
}

func seedRoles() {
	var count int64
	DB.Model(&models.Role{}).Count(&count)
	if count > 0 {
		return
	}

	log.Println("Seeding default roles...")

	roles := []models.Role{
		{ID: uuid.New().String(), Name: "Admin", NamaRole: "Admin", Keterangan: "Administrator dengan akses penuh"},
		{ID: uuid.New().String(), Name: "Petugas", NamaRole: "Petugas", Keterangan: "Petugas pengarsipan"},
		{ID: uuid.New().String(), Name: "Arsiparis", NamaRole: "Arsiparis", Keterangan: "Arsiparis"},
		{ID: uuid.New().String(), Name: "Pimpinan", NamaRole: "Pimpinan", Keterangan: "Pimpinan"},
		{ID: uuid.New().String(), Name: "Staff", NamaRole: "Staff", Keterangan: "Staff pengarsipan"},
		{ID: uuid.New().String(), Name: "Viewer", NamaRole: "Viewer", Keterangan: "Hanya bisa melihat"},
		{ID: uuid.New().String(), Name: "User", NamaRole: "User", Keterangan: "Pengguna standar"},
	}
	for _, role := range roles {
		DB.Create(&role)
	}
}

func seedAdmin() {
	hashed, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		return
	}
	hashedStr := string(hashed)

	// Ensure admin user exists with known password
	var existing models.User
	result := DB.Where("username = ?", "admin").First(&existing)
	if result.Error != nil {
		// Find Admin role
		var adminRole models.Role
		if err := DB.Where("name = ?", "Admin").First(&adminRole).Error; err != nil {
			log.Println("Admin role not found, skipping user seed")
			return
		}

		log.Println("Creating default admin user...")
		user := models.User{
			ID:       uuid.New().String(),
			Username: "admin",
			Name:     "Administrator",
			Password: hashedStr,
			RoleID:   adminRole.ID,
			IsActive: true,
		}
		DB.Create(&user)
		log.Printf("Default user created: username=admin password=admin")
	} else {
		// Admin already exists — NEVER touch the stored password here.
		// Overwriting it on every boot used to reset production logins
		// back to "admin" silently (security + availability issue).
		log.Println("Admin user already exists, skipping password seed")
	}
}

// seedJenisArsip menambahkan kategori arsip standar ke tabel aktif (jenis_arsip)
// bila belum ada. Idempotent + aman-konkuren:
//   - dedupe baris kode_jenis ganda (pertahanan DB lama)
//   - index unik idx_jenis_arsip_kode membuat insert ganda gagal di level DB
func seedJenisArsip() {
	DB.Exec(`DELETE d1 FROM jenis_arsip d1 JOIN jenis_arsip d2
	         ON d1.kode_jenis = d2.kode_jenis AND d1.id > d2.id`)

	defs := []struct{ Kode, Nama, Ket string }{
		{"SPJ", "Surat Pertanggungjawaban (SPJ)", "Dokumen pertanggungjawaban keuangan/kegiatan"},
		{"NON_SPJ", "Non SPJ", "Dokumen di luar kategori pertanggungjawaban"},
		{"SK", "Surat Keputusan", "Surat keputusan pejabat berwenang"},
		{"Laporan", "Laporan", "Laporan kegiatan, keuangan, atau kinerja"},
		{"Surat", "Surat Umum", "Surat dinas umum / korespondensi"},
		{"Nota Dinas", "Nota Dinas", "Nota dinas internal"},
		{"Kontrak", "Kontrak / Perjanjian", "Kontrak, perjanjian, dan amandemennya"},
		{"Undangan", "Undangan", "Undangan rapat atau kegiatan"},
		{"Berita Acara", "Berita Acara", "Berita acara serah terima / kejadian"},
		{"Permohonan", "Permohonan", "Surat permohonan"},
	}
	for _, d := range defs {
		var existing models.JenisArsip
		err := DB.Where("kode_jenis = ?", d.Kode).First(&existing).Error
		if err != nil {
			created := DB.Create(&models.JenisArsip{KodeJenis: d.Kode, NamaJenis: d.Nama, Keterangan: d.Ket})
			if created.Error == nil {
				log.Printf("[SEED] Jenis arsip \"%s\" (%s) dibuat", d.Kode, d.Nama)
			}
		}
	}
}

func seedLokasiRakA() {
	// Create the main Record Center location if it doesn't exist
	upsertLokasi("Record Center", "Rak A")
}

func upsertLokasi(namaLokasi, deskripsi string) {
	var existing models.LokasiArsip
	result := DB.Where("nama_lokasi = ?", namaLokasi).First(&existing)
	if result.Error != nil {
		DB.Create(&models.LokasiArsip{
			ID:          uuid.New().String(),
			NamaLokasi:  namaLokasi,
			Deskripsi:   deskripsi,
			IsActive:    true,
		})
		log.Printf("[SEED] Lokasi arsip \"%s\" dibuat dengan deskripsi \"%s\"", namaLokasi, deskripsi)
	} else {
		DB.Model(&existing).Update("deskripsi", deskripsi)
		log.Printf("[SEED] Lokasi arsip \"%s\" diperbarui dengan deskripsi \"%s\"", namaLokasi, deskripsi)
	}
}
