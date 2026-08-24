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
		return
	}
	seedRoles()
	seedAdmin()
	seedLokasiRakA()
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
