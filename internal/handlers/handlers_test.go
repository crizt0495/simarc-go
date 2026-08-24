package handlers

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Global test data
var (
	testAdminUser *models.User
	testUnitKerja *models.UnitKerja
	testKK        *models.KodeKlasifikasi
	testLokasi    *models.LokasiArsip
	testRoleAdmin *models.Role
)

// getTestProjectRoot finds the project root by looking for web/templates directory.
func getTestProjectRoot() string {
	wd, _ := os.Getwd()
	// Walk up from current directory until we find web/templates
	dir := wd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "web", "templates")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return wd
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	// Change to project root so template paths work
	rootDir := getTestProjectRoot()
	if err := os.Chdir(rootDir); err != nil {
		log.Fatalf("Failed to chdir to project root %s: %v", rootDir, err)
	}
	log.Printf("[TEST] Project root: %s", rootDir)

	config.Load()

	// Use test database
	config.App.DBName = getTestDBName()
	ensureTestDB()

	if err := database.Connect(); err != nil {
		log.Fatalf("[TEST] Failed to connect to database: %v", err)
	}
	if err := database.Migrate(); err != nil {
		log.Fatalf("[TEST] Failed to migrate: %v", err)
	}

	seedTestData()
	middleware.InitSession()

	// Initialize templates for Render() to work
	TemplateSets = make(map[string]*template.Template)
	initTemplates()

	code := m.Run()

	cleanupTestData()
	os.Exit(code)
}

func getTestDBName() string {
	if name := os.Getenv("TEST_DB_DATABASE"); name != "" {
		return name
	}
	return "arsippro_test"
}

func ensureTestDB() {
	host := config.App.DBHost
	port := config.App.DBPort
	user := config.App.DBUser
	pass := config.App.DBPass

	// Connect to MySQL server with no specific database to create the test DB if missing.
	serverDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=True&loc=Local",
		user, pass, host, port)
	tmpDB, err := gorm.Open(mysql.Open(serverDSN), &gorm.Config{})
	if err != nil {
		log.Printf("[TEST] Cannot connect to create test DB: %v (assuming it exists)", err)
		return
	}
	tmpDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", getTestDBName()))
	sqlDB, _ := tmpDB.DB()
	sqlDB.Close()
}

func seedTestData() {
	// Admin role
	testRoleAdmin = &models.Role{}
	if err := database.DB.Where("name = ?", "Admin").First(testRoleAdmin).Error; err != nil {
		testRoleAdmin = &models.Role{ID: uuid.New().String(), Name: "Admin", NamaRole: "Admin"}
		database.DB.Create(testRoleAdmin)
	}

	// Unit kerja
	testUnitKerja = &models.UnitKerja{}
	if err := database.DB.Where("nama_unit = ?", "Test Unit").First(testUnitKerja).Error; err != nil {
		testUnitKerja = &models.UnitKerja{ID: uuid.New().String(), NamaUnit: "Test Unit"}
		database.DB.Create(testUnitKerja)
	}

	// Kode klasifikasi
	testKK = &models.KodeKlasifikasi{}
	if err := database.DB.Where("kode_klasifikasi = ?", "TEST-001").First(testKK).Error; err != nil {
		testKK = &models.KodeKlasifikasi{
			ID:              uuid.New().String(),
			KodeKlasifikasi: "TEST-001",
			NamaKlasifikasi: "Test Klasifikasi",
			IsActive:        true,
		}
		database.DB.Create(testKK)
	}

	// Lokasi arsip
	testLokasi = &models.LokasiArsip{}
	if err := database.DB.Where("nama_lokasi = ?", "Test Lokasi").First(testLokasi).Error; err != nil {
		testLokasi = &models.LokasiArsip{
			ID:         uuid.New().String(),
			NamaLokasi: "Test Lokasi",
			Deskripsi:  "Test lokasi",
			IsActive:   true,
		}
		database.DB.Create(testLokasi)
	}

	// Admin user
	hashed, _ := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
	testAdminUser = &models.User{}
	if err := database.DB.Where("username = ?", "testadmin").First(testAdminUser).Error; err != nil {
		testAdminUser = &models.User{
			ID:       uuid.New().String(),
			Username: "testadmin",
			Name:     "Test Admin",
			Password: string(hashed),
			RoleID:   testRoleAdmin.ID,
			IsActive: true,
		}
		database.DB.Create(testAdminUser)
	}

	log.Println("[TEST] Test data seeded")
}

func cleanupTestData() {
	database.DB.Exec("DELETE FROM arsip WHERE nama_arsip LIKE '[TEST]%'")
	database.DB.Exec("DELETE FROM users WHERE username = 'testadmin'")
	database.DB.Exec("DELETE FROM lokasi_arsips WHERE nama_lokasi = 'Test Lokasi'")
	database.DB.Exec("DELETE FROM kode_klasifikasi WHERE kode_klasifikasi = 'TEST-001'")
	database.DB.Exec("DELETE FROM unit_kerja WHERE nama_unit = 'Test Unit'")
	database.DB.Exec("DELETE FROM roles WHERE name = 'Admin'")
}

// initTemplates initializes the TemplateSets with real template files needed for tests.
func initTemplates() {
	// Load all shared files (layouts + components)
	layoutFiles, _ := filepath.Glob("web/templates/layouts/*.html")
	compFiles, _ := filepath.Glob("web/templates/components/*.html")
	sharedFiles := append(layoutFiles, compFiles...)

	// Parse the arsip/create page with all shared files
	pageFiles := []string{
		"web/templates/arsip/create.html",
	}

	for _, pf := range pageFiles {
		allFiles := append([]string{}, sharedFiles...)
		allFiles = append(allFiles, pf)
		ts := template.Must(template.New("").Funcs(TemplateFuncs()).ParseFiles(allFiles...))
		for _, t := range ts.Templates() {
			n := t.Name()
			if !stringContains(n, "/") {
				continue
			}
			if stringHasPrefix(n, "layouts/") || stringHasPrefix(n, "components/") {
				continue
			}
			TemplateSets[n] = ts
		}
	}
	log.Println("[TEST] Templates initialized")
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && stringContainsStr(s, substr)
}

func stringContainsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func stringHasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

// newTestContext creates a Gin test context with an authenticated admin user.
func newTestContext(method, path string, formData url.Values) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var body *bytes.Buffer
	if formData != nil {
		body = bytes.NewBufferString(formData.Encode())
	} else {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, body)
	if formData != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	c.Request = req
	c.Set("user", testAdminUser)
	c.Set("user_id", testAdminUser.ID)

	return c, w
}

// newTestEngine creates a Gin engine with routes for testing.
// The engine includes an auth middleware that sets the test user.
func newTestEngine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	// Auth middleware: sets test user in context
	r.Use(func(c *gin.Context) {
		c.Set("user", testAdminUser)
		c.Set("user_id", testAdminUser.ID)
		c.Next()
	})
	// Register routes
	h := &ArsipHandler{}
	r.POST("/arsip", h.Store)
	return r
}

// performRequest performs an HTTP request against the test engine and returns the response.
func performRequest(r *gin.Engine, method, path string, formData url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	body := bytes.NewBufferString(formData.Encode())
	req := httptest.NewRequest(method, path, body)
	if formData != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.ServeHTTP(w, req)
	return w
}
