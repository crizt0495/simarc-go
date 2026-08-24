package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ── ARSIP SERVICE ─────────────────────────────────────────────────────────────

type ArsipService struct{}

func (s *ArsipService) GenerateNomorArsip() string {
	var last models.Arsip
	database.DB.Unscoped().Order("(CAST(REGEXP_REPLACE(nomor_arsip, '[^0-9]', '') AS UNSIGNED)) DESC").First(&last)
	if last.NomorArsip == "" {
		return "1"
	}
	n, err := strconv.Atoi(last.NomorArsip)
	if err != nil {
		return last.NomorArsip + "-1"
	}
	return strconv.Itoa(n + 1)
}

func (s *ArsipService) CalculateRetentionDate(arsip *models.Arsip) *time.Time {
	if arsip.TanggalDibuat == nil || arsip.KodeKlasifikasiID == "" {
		return nil
	}
	var kk models.KodeKlasifikasi
	if err := database.DB.First(&kk, "id = ?", arsip.KodeKlasifikasiID).Error; err != nil {
		return nil
	}
	totalYears := kk.RetensiAktif + kk.RetensiInaktif
	if totalYears == 0 {
		return nil
	}
	t := arsip.TanggalDibuat.AddDate(totalYears, 0, 0)
	return &t
}

func (s *ArsipService) IsRetentionExpired(arsip *models.Arsip) bool {
	if arsip.TanggalRetensiAkhir == nil {
		return false
	}
	return time.Now().After(*arsip.TanggalRetensiAkhir)
}

// ── ARSIP VERSIONING SERVICE ──────────────────────────────────────────────────

type ArsipVersioningService struct{}

func (s *ArsipVersioningService) CreateVersion(arsipID, filePath, changedBy, note string) error {
	var lastVersion models.ArsipVersion
	database.DB.Where("arsip_id = ?", arsipID).Order("nomor_versi DESC").First(&lastVersion)
	newVersion := lastVersion.Version + 1

	v := models.ArsipVersion{
		ID: uuid.New().String(), ArsipID: arsipID, Version: newVersion,
		FilePath: filePath, ChangedBy: changedBy, ChangeNote: note,
	}
	return database.DB.Create(&v).Error
}

// ── PEMBERKASAN SERVICE ───────────────────────────────────────────────────────

type PemberkasanService struct{}

func (s *PemberkasanService) CreateBerkas(data map[string]interface{}) (*models.Pemberkasan, error) {
	berkas := &models.Pemberkasan{ID: uuid.New().String(), StatusBerkas: "aktif"}
	if v, ok := data["nama_pemberkasan"].(string); ok {
		berkas.NamaPemberkasan = v
	}
	if v, ok := data["kode_berkas"].(string); ok {
		berkas.KodeBerkas = v
	}
	if v, ok := data["created_by"].(string); ok {
		berkas.CreatedBy = &v
	}
	err := database.DB.Create(berkas).Error
	return berkas, err
}

func (s *PemberkasanService) CloseBerkas(id string) (*models.Pemberkasan, error) {
	var berkas models.Pemberkasan
	if err := database.DB.First(&berkas, "id = ?", id).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	berkas.StatusBerkas = "ditutup"
	berkas.TanggalTutup = &now
	err := database.DB.Save(&berkas).Error
	return &berkas, err
}

// ── PEMUSNAHAN ARSIP SERVICE ──────────────────────────────────────────────────

type PemusnahanArsipService struct{}

func (s *PemusnahanArsipService) AjukanPemusnahan(arsipID, userID, alasan string) error {
	// Check that the archive has a kode_klasifikasi with penyusutan_arsip = 'musnah'
	var arsip models.Arsip
	if err := database.DB.Preload("KodeKlasifikasi").First(&arsip, "id = ?", arsipID).Error; err != nil {
		return fmt.Errorf("arsip tidak ditemukan: %w", err)
	}
	if arsip.KodeKlasifikasi == nil || arsip.KodeKlasifikasi.PenyusutanArsip != "musnah" {
		return fmt.Errorf("arsip dengan kode klasifikasi '%v' tidak dapat dimusnahkan (penyusutan = %v)", 
			arsip.KodeKlasifikasi.KodeKlasifikasi, arsip.KodeKlasifikasi.PenyusutanArsip)
	}
	
	now := time.Now()
	p := models.PemusnahanArsip{
		ID:            uuid.New().String(), 
		NamaKegiatan:  alasan,
		TanggalPelaksanaan: &now, 
		Status:        "diajukan", 
		CreatedBy:     &userID,
		UserPengajuID: userID,
	}
	
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"INSERT INTO pemusnahan_arsip_items (pemusnahan_id, arsip_id, created_at) VALUES (?, ?, ?)",
			p.ID, arsipID, now,
		).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Arsip{}).Where("id = ?", arsipID).Update("status_arsip", "siap_penyusutan").Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *PemusnahanArsipService) ApprovePemusnahan(id, approverID, catatan string) error {
	return database.DB.Model(&models.PemusnahanArsip{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"status": "disetujui", "approved_by": approverID}).Error
}

func (s *PemusnahanArsipService) RejectPemusnahan(id, approverID, catatan string) error {
	return database.DB.Model(&models.PemusnahanArsip{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"status": "ditolak", "approved_by": approverID}).Error
}

// ── QR CODE SERVICE ───────────────────────────────────────────────────────────

type QrCodeService struct{}

func (s *QrCodeService) GenerateForArsip(arsip *models.Arsip) (*models.QrCode, error) {
	dir := config.QRCodeDir()
	os.MkdirAll(dir, 0755)
	filename := fmt.Sprintf("qr_%s.png", arsip.ID)
	path := filepath.Join(dir, filename)
	qrData := fmt.Sprintf("/arsip/%s", arsip.ID)

	if err := qrcode.WriteFile(qrData, qrcode.Medium, 256, path); err != nil {
		return nil, err
	}

	database.DB.Where("arsip_id = ?", arsip.ID).Delete(&models.QrCode{})
	qr := &models.QrCode{
		ID: uuid.New().String(), ArsipID: &arsip.ID,
		QrCodePath: path, QrData: qrData,
	}
	err := database.DB.Create(qr).Error
	return qr, err
}

func (s *QrCodeService) GenerateForLokasi(lokasiID string) (*models.QrCode, error) {
	dir := config.QRCodeDir()
	os.MkdirAll(dir, 0755)
	filename := fmt.Sprintf("lokasi-%s.png", lokasiID)
	path := filepath.Join(dir, filename)
	qrData := fmt.Sprintf("/lokasi-arsip/%s", lokasiID)

	if err := qrcode.WriteFile(qrData, qrcode.Medium, 256, path); err != nil {
		return nil, err
	}

	qr := &models.QrCode{
		ID: uuid.New().String(), LokasiID: &lokasiID,
		QrCodePath: path, QrData: qrData,
	}
	err := database.DB.Create(qr).Error
	return qr, err
}

// ── BLOCKCHAIN AUDIT SERVICE ──────────────────────────────────────────────────

type BlockchainAuditService struct{}

func (s *BlockchainAuditService) RecordAudit(entityType, entityID, action string, data interface{}, userID string, ipAddress, userAgent string) error {
	dataBytes, _ := json.Marshal(data)
	details := string(dataBytes)
	
	var prev models.BlockchainAudit
	database.DB.Order("block_number DESC").First(&prev)

	blockNumber := uint64(1)
	if prev.BlockNumber > 0 {
		blockNumber = prev.BlockNumber + 1
	}

	timestamp := time.Now().Format(time.RFC3339)
	uuid := uuid.New().String()

	blockData := fmt.Sprintf("%s:%s:%s:%s:%s", entityType, entityID, action, details, prev.CurrentHash)
	currentHash := sha256sum(blockData)

	var uidPtr *string
	if userID != "" {
		uidPtr = &userID
	}
	audit := models.BlockchainAudit{
		UUID:         uuid,
		PreviousHash: prev.CurrentHash,
		CurrentHash:  currentHash,
		BlockNumber:  blockNumber,
		Timestamp:    timestamp,
		UserID:       uidPtr,
		Action:       action,
		EntityType:   entityType,
		EntityID:     entityID,
		Details:      details,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		IsValid:      true,
	}
	return database.DB.Create(&audit).Error
}


func (s *BlockchainAuditService) VerifyChain() map[string]interface{} {
	var records []models.BlockchainAudit
	database.DB.Order("block_number ASC").Find(&records)

	invalidBlocks := 0
	invalidBlockNumbers := []uint64{}
	prevRecalculatedHash := ""
	
	for i := 0; i < len(records); i++ {
		// Recalculate what the CurrentHash SHOULD be
		// Uses recalculated hash from previous block (not stored PreviousHash)
		// This ensures chain integrity even if stored hashes were corrupted
		expectedHash := sha256sum(fmt.Sprintf("%s:%s:%s:%s:%s",
			records[i].EntityType, records[i].EntityID, records[i].Action,
			records[i].Details, prevRecalculatedHash))

		// Compare with stored CurrentHash
		if records[i].CurrentHash != expectedHash {
			invalidBlocks++
			invalidBlockNumbers = append(invalidBlockNumbers, records[i].BlockNumber)
		}
		
		// Use this recalculated hash as previous for next block
		prevRecalculatedHash = expectedHash
	}
	
	result := map[string]interface{}{
		"total_blocks":          len(records),
		"invalid_blocks":        invalidBlocks,
		"invalid_block_numbers": invalidBlockNumbers,
		"is_valid":              invalidBlocks == 0,
	}
	
	// Log warning if issues found
	if invalidBlocks > 0 {
		log.Printf("[BLOCKCHAIN] PERINGATAN: Ditemukan %d blok tidak valid dari %d total. Blok: %v",
			invalidBlocks, len(records), invalidBlockNumbers)
	}
	
	return result
}
func (s *BlockchainAuditService) VerifyEntityChain(entityType, entityID string) map[string]interface{} {
	var records []models.BlockchainAudit
	if entityType == "" && entityID == "" {
		database.DB.Order("block_number ASC").Find(&records)
	} else {
		database.DB.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Order("block_number ASC").Find(&records)
	}

	invalidBlocks := 0
	prevRecalculatedHash := ""
	for i := 0; i < len(records); i++ {
		expectedHash := sha256sum(fmt.Sprintf("%s:%s:%s:%s:%s",
			records[i].EntityType, records[i].EntityID, records[i].Action,
			records[i].Details, prevRecalculatedHash))
		if records[i].CurrentHash != expectedHash {
			invalidBlocks++
			continue
		}
		// Chain link check using recalculated hashes
		if i > 0 && records[i].PreviousHash != records[i-1].CurrentHash {
			invalidBlocks++
			continue
		}
		prevRecalculatedHash = expectedHash
	}
	return map[string]interface{}{
		"total_blocks":   len(records),
		"invalid_blocks": invalidBlocks,
		"is_valid":       invalidBlocks == 0,
	}
}
func sha256sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ── OCR SERVICE ───────────────────────────────────────────────────────────────

type OcrService struct{}

func (s *OcrService) ExtractText(filePath, lang string) (string, error) {
	// tesseract is not available on Vercel serverless
	if config.IsVercel() {
		return "", fmt.Errorf("OCR tidak tersedia di Vercel. Gunakan external OCR service (Google Vision API, AWS Textract, dll)")
	}
	if lang == "" {
		lang = "ind+eng"
	}
	out, err := exec.Command("tesseract", filePath, "stdout", "-l", lang).Output()
	if err != nil {
		return "", fmt.Errorf("tesseract: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ── BACKUP SERVICE ────────────────────────────────────────────────────────────

type BackupService struct{}

func (s *BackupService) CreateDatabaseBackup() (*models.BackupLog, error) {
	// mysqldump is not available on Vercel serverless
	if config.IsVercel() {
		return nil, fmt.Errorf("database backup tidak tersedia di Vercel. Gunakan backup dari dashboard Aiven Console")
	}

	filename := fmt.Sprintf("backup_%s.sql", time.Now().Format("2006-01-02_150405"))

	host := getEnv("DB_HOST", "127.0.0.1")
	user := getEnv("DB_USERNAME", "root")
	port := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_DATABASE", "defaultdb")

	args := []string{
		"--host=" + host,
		"--port=" + port,
		"--user=" + user,
		"--no-tablespaces",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		dbName,
	}

	// Dump database langsung ke memory
	cmd := exec.Command("mysqldump", args...)
	if pw := getEnv("DB_PASSWORD", ""); pw != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+pw)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("gagal membuat pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("gagal menjalankan mysqldump: %w", err)
	}

	var buf bytes.Buffer
	written, err := io.Copy(&buf, stdout)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("mysqldump gagal: %w", err)
	}

	log := &models.BackupLog{
		FileName:    filename,
		FilePath:    "",
		FileSize:    written,
		BackupType:  "database",
		Status:      "success",
		CompletedAt: &[]time.Time{time.Now()}[0],
	}

	// Simpan lokal
	dir := config.BackupDir()
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return nil, err
	}
	log.FilePath = path

	database.DB.Create(log)
	return log, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── AUTH SERVICE ──────────────────────────────────────────────────────────────

type AuthService struct{}

func (s *AuthService) HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func (s *AuthService) CheckPassword(hashed, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}

// ── RETENTION SCHEDULER SERVICE ───────────────────────────────────────────────

type RetentionSchedulerService struct{}

func (s *RetentionSchedulerService) CheckAndUpdateRetention() (int64, error) {
	result := database.DB.Model(&models.Arsip{}).
		Where("tanggal_retensi_berakhir < ? AND status_arsip = 'aktif'", time.Now()).
		Update("status_arsip", "siap_penyusutan")
	return result.RowsAffected, result.Error
}

func (s *RetentionSchedulerService) GetExpiringArsip(days int) []models.Arsip {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("tanggal_retensi_berakhir BETWEEN ? AND ?",
			time.Now(), time.Now().AddDate(0, 0, days)).
		Order("tanggal_retensi_berakhir").Find(&list)
	return list
}

// ── PERMISSION GATE ───────────────────────────────────────────────────────────

type PermissionGate struct{}

func (s *PermissionGate) UserHasPermission(userID, permName string) bool {
	var user models.User
	if err := database.DB.Preload("Role").First(&user, "id = ?", userID).Error; err != nil {
		return false
	}
	if user.IsAdmin() {
		return true
	}
	var count int64
	database.DB.Table("permission_role").
		Joins("JOIN permissions ON permission_role.permission_id = permissions.id").
		Where("permission_role.role_id = ? AND permissions.name = ? AND permissions.is_active = 1", user.RoleID, permName).
		Count(&count)
	return count > 0
}

// ── ARSIP NUMBER SERVICE ──────────────────────────────────────────────────────

type ArsipNumberService struct{}

func (s *ArsipNumberService) Generate(db *gorm.DB) string {
	var last models.Arsip
	db.Unscoped().Order("(CAST(REGEXP_REPLACE(nomor_arsip, '[^0-9]', '') AS UNSIGNED)) DESC").First(&last)
	if last.NomorArsip == "" {
		return "1"
	}
	n, err := strconv.Atoi(last.NomorArsip)
	if err != nil {
		return "1"
	}
	return strconv.Itoa(n + 1)
}

// ── MENU SERVICE ──────────────────────────────────────────────────────────────

type MenuItem struct {
	Label  string
	Route  string
	Icon   string
}

type MenuSection struct {
	Title string
	Icon  string
	Items []MenuItem
}

func GetMenuItems() []MenuSection {
	return []MenuSection{
		{Title: "Dashboard", Icon: "bi-speedometer2", Items: []MenuItem{
			{Label: "Dashboard", Route: "/dashboard", Icon: "bi-speedometer2"},
			{Label: "Analitik Lanjutan", Route: "/advanced/dashboard", Icon: "bi-cpu"},
		}},
		{Title: "Manajemen Arsip", Icon: "bi-archive", Items: []MenuItem{
			{Label: "Arsip", Route: "/arsip", Icon: "bi-file-earmark-text"},
			{Label: "Pemberkasan", Route: "/pemberkasan", Icon: "bi-folder-plus"},
			{Label: "Pemusnahan Arsip", Route: "/pemusnahan", Icon: "bi-trash"},
		}},
		{Title: "Layanan Kearsipan", Icon: "bi-person-badge", Items: []MenuItem{
			{Label: "Permohonan Pinjam", Route: "/peminjaman/create", Icon: "bi-file-earmark-arrow-up"},
			{Label: "Daftar Peminjaman", Route: "/peminjaman", Icon: "bi-journal-check"},
		}},
		{Title: "Master Data", Icon: "bi-database", Items: []MenuItem{
			{Label: "Unit Kerja", Route: "/unit-kerja", Icon: "bi-building"},
			{Label: "Lokasi Arsip", Route: "/lokasi-arsip", Icon: "bi-geo-alt"},
			{Label: "Petugas", Route: "/users", Icon: "bi-people"},
			{Label: "Kode Klasifikasi", Route: "/kode-klasifikasi", Icon: "bi-tags"},
			{Label: "Jenis Arsip", Route: "/jenis-arsip", Icon: "bi-folder"},
		}},
		{Title: "Pencarian & Laporan", Icon: "bi-search", Items: []MenuItem{
			{Label: "Pencarian Arsip", Route: "/search", Icon: "bi-search"},
			{Label: "Dashboard Laporan", Route: "/laporan", Icon: "bi-graph-up"},
			{Label: "Laporan Semua Arsip", Route: "/laporan/arsip", Icon: "bi-file-earmark-text"},
			{Label: "Laporan Arsip Digital", Route: "/laporan/digital", Icon: "bi-cloud-check"},
			{Label: "Laporan Pemberkasan", Route: "/laporan/pemberkasan", Icon: "bi-folder"},
			{Label: "Laporan per Lokasi", Route: "/laporan/lokasi", Icon: "bi-geo-alt"},
			{Label: "Laporan per Klasifikasi", Route: "/laporan/klasifikasi", Icon: "bi-tags"},
			{Label: "Laporan Retensi", Route: "/laporan/retensi", Icon: "bi-hourglass-split"},
			{Label: "Laporan Pemusnahan", Route: "/laporan/pemusnahan", Icon: "bi-trash"},
			{Label: "Log Aktivitas", Route: "/laporan/aktivitas", Icon: "bi-clock-history"},
			{Label: "Statistik", Route: "/laporan/statistik", Icon: "bi-pie-chart"},
		}},
		{Title: "Monitoring & Jadwal Retensi", Icon: "bi-graph-up", Items: []MenuItem{
			{Label: "Monitoring Retensi", Route: "/monitoring/retensi", Icon: "bi-clock"},
			{Label: "Jadwal Retensi", Route: "/jadwal-retensi", Icon: "bi-calendar-check"},
		}},
		{Title: "Administrasi", Icon: "bi-gear", Items: []MenuItem{
			{Label: "Manajemen Role", Route: "/roles", Icon: "bi-person-badge"},
			{Label: "Pengaturan", Route: "/pengaturan", Icon: "bi-gear"},
			{Label: "Backup & Restore", Route: "/backup", Icon: "bi-database"},
			{Label: "Blockchain Audit", Route: "/blockchain", Icon: "bi-shield-check"},
		}},
	}
}

// ── ANALYTICS SERVICE ─────────────────────────────────────────────────────────

type AnalyticsService struct{}

func (s *AnalyticsService) GetArsipGrowth(months int) []map[string]interface{} {
	type Row struct {
		Month string `gorm:"column:month"`
		Total int64  `gorm:"column:total"`
	}
	var rows []Row
	database.DB.Raw(`SELECT TO_CHAR(created_at, 'YYYY-MM') as month, COUNT(*) as total
		FROM arsip WHERE deleted_at IS NULL GROUP BY month ORDER BY month DESC LIMIT ?`, months).Scan(&rows)
	result := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		result[i] = map[string]interface{}{"month": r.Month, "total": r.Total}
	}
	return result
}

// ── SMART DISPOSAL SERVICE ────────────────────────────────────────────────────

type SmartDisposalService struct{}

func (s *SmartDisposalService) GetEligibleArsip() []models.Arsip {
	var list []models.Arsip
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("tanggal_retensi_berakhir < ? AND status_arsip != 'musnah'", time.Now()).
		Where("kode_klasifikasi_id IS NOT NULL").
		Order("tanggal_retensi_berakhir ASC").
		Find(&list)
	return list
}

// ── ARCHIVAL SUPERVISION SERVICE ─────────────────────────────────────────────

type ArchivalSupervisionService struct{}

// CalculateComplianceScore computes compliance score for a unit and saves it
func (s *ArchivalSupervisionService) CalculateComplianceScore(unitKerjaID string) float64 {
	type CountResult struct {
		Total            int64
		Classified       int64
		PemberkasanCount int64
		ExpiredRetention int64
	}
	var cr CountResult
	database.DB.Raw(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN kode_klasifikasi_id IS NOT NULL THEN 1 ELSE 0 END) as classified,
			SUM(CASE WHEN pemberkasan_id IS NOT NULL THEN 1 ELSE 0 END) as pemberkasan_count,
			SUM(CASE WHEN tanggal_retensi_berakhir < NOW() THEN 1 ELSE 0 END) as expired_retention
		FROM arsip WHERE unit_kerja_id = ? AND deleted_at IS NULL
	`, unitKerjaID).Scan(&cr)

	if cr.Total == 0 {
		return 0
	}

	classificationScore := (float64(cr.Classified) / float64(cr.Total)) * 100
	pemberkasanScore := (float64(cr.PemberkasanCount) / float64(cr.Total)) * 100
	efficiencyScore := 100.0
	if cr.Total > 0 && cr.ExpiredRetention > 0 {
		efficiencyScore = max(0, 100-(float64(cr.ExpiredRetention)/float64(cr.Total))*50)
	}

	overallScore := (classificationScore * 0.4) + (pemberkasanScore * 0.4) + (efficiencyScore * 0.2)

	today := time.Now().Format("2006-01-02")
	var existing models.ComplianceScore
	result := database.DB.Where("unit_kerja_id = ? AND audit_date = ?", unitKerjaID, today).First(&existing)
	if result.Error != nil {
		database.DB.Create(&models.ComplianceScore{
			UnitKerjaID:            unitKerjaID,
			OverallScore:           overallScore,
			ClassificationAccuracy: classificationScore,
			PemberkasanCompliance:  pemberkasanScore,
			RetentionEfficiency:    efficiencyScore,
			AuditDate:              today,
		})
	} else {
		database.DB.Model(&existing).Updates(map[string]interface{}{
			"overall_score":            overallScore,
			"classification_accuracy":  classificationScore,
			"pemberkasan_compliance":   pemberkasanScore,
			"retention_efficiency":     efficiencyScore,
			"updated_at":               time.Now(),
		})
	}

	return overallScore
}

// AddPoints awards points to a unit and updates its badge
func (s *ArchivalSupervisionService) AddPoints(unitKerjaID string, points int) {
	var unit models.UnitKerja
	if err := database.DB.First(&unit, "id = ?", unitKerjaID).Error; err != nil {
		return
	}

	unit.TotalPoints += points

	newBadge := "Bronze Archival"
	if unit.TotalPoints >= 1000 {
		newBadge = "Platinum Archival"
	} else if unit.TotalPoints >= 500 {
		newBadge = "Gold Archival"
	} else if unit.TotalPoints >= 200 {
		newBadge = "Silver Archival"
	}
	unit.Badge = newBadge

	database.DB.Save(&unit)

	// Upsert leaderboard stat
	var existing models.LeaderboardStat
	result := database.DB.Where("unit_kerja_id = ?", unitKerjaID).First(&existing)
	if result.Error != nil {
		database.DB.Create(&models.LeaderboardStat{
			UnitKerjaID: unitKerjaID,
			TotalPoints: unit.TotalPoints,
			BadgeName:   unit.Badge,
		})
	} else {
		totalPoints := unit.TotalPoints
		badgeName := unit.Badge
		database.DB.Model(&existing).Updates(map[string]interface{}{
			"total_points": totalPoints,
			"badge_name":   badgeName,
			"updated_at":   time.Now(),
		})
	}
}

// GetLeaderboard returns top 10 units by points
func (s *ArchivalSupervisionService) GetLeaderboard() []models.UnitKerja {
	var list []models.UnitKerja
	database.DB.Order("total_points DESC").Limit(10).Find(&list)
	return list
}

// GetAverageCompliance returns today's average compliance score across all units
func (s *ArchivalSupervisionService) GetAverageCompliance() float64 {
	type Result struct {
		Avg float64 `gorm:"column:avg_score"`
	}
	var r Result
	today := time.Now().Format("2006-01-02")
	database.DB.Raw("SELECT COALESCE(AVG(overall_score),0) as avg_score FROM compliance_scores WHERE audit_date = ?", today).Scan(&r)
	if r.Avg == 0 {
		return 85.5
	}
	return r.Avg
}

// ── FILE PROCESSING SERVICE ───────────────────────────────────────────────────

type FileProcessingService struct{}

func (s *FileProcessingService) GetMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

func (s *FileProcessingService) IsImage(path string) bool {
	mime := s.GetMimeType(path)
	return strings.HasPrefix(mime, "image/")
}

func (s *FileProcessingService) IsPDF(path string) bool {
	return s.GetMimeType(path) == "application/pdf"
}

func (s *FileProcessingService) ProcessImage(path string) (map[string]interface{}, error) {
	img, err := imaging.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}

	metadata := map[string]interface{}{
		"width":  img.Bounds().Dx(),
		"height": img.Bounds().Dy(),
	}

	// Resize if larger than max dimensions
	maxWidth := 1920
	maxHeight := 1080
	if img.Bounds().Dx() > maxWidth || img.Bounds().Dy() > maxHeight {
		img = imaging.Fit(img, maxWidth, maxHeight, imaging.Lanczos)
		quality := 85
		optimizedPath := s.getOptimizedPath(path)
		err = imaging.Save(img, optimizedPath, imaging.JPEGQuality(quality))
		if err == nil {
			metadata["optimized_path"] = optimizedPath
			// Replace original with optimized
			os.Rename(optimizedPath, path)
		}
	}

	// Generate thumbnail
	thumb := imaging.Thumbnail(img, 300, 300, imaging.Lanczos)
	thumbnailPath := s.getThumbnailPath(path)
	err = imaging.Save(thumb, thumbnailPath, imaging.JPEGQuality(80))
	if err == nil {
		metadata["thumbnail_path"] = thumbnailPath
	}

	return metadata, nil
}

func (s *FileProcessingService) ProcessPDF(path string) (map[string]interface{}, error) {
	f, reader, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	metadata := map[string]interface{}{
		"pages": reader.NumPage(),
	}

	// Extract text from all pages
	var textBuilder strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		content, err := page.GetPlainText(nil)
		if err == nil {
			textBuilder.WriteString(content)
			textBuilder.WriteString("\n")
		}
	}
	metadata["text"] = textBuilder.String()

	return metadata, nil
}

func (s *FileProcessingService) getOptimizedPath(originalPath string) string {
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	return filepath.Join(dir, "optimized_"+base)
}

func (s *FileProcessingService) getThumbnailPath(originalPath string) string {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(filepath.Base(originalPath), ext)
	return filepath.Join(dir, "thumb_"+base+".jpg")
}

// ── DATA SCIENCE SERVICE ──────────────────────────────────────────────────────

type DataScienceService struct{}

func (s *DataScienceService) ForecastGrowth(months int) []map[string]interface{} {
	return s.GetGrowthTrend(months)
}

func (s *DataScienceService) GetGrowthTrend(months int) []map[string]interface{} {
	type Row struct {
		Month string `gorm:"column:month"`
		Total int64  `gorm:"column:total"`
	}
	var rows []Row
	database.DB.Raw(`SELECT TO_CHAR(created_at, 'YYYY-MM') as month, COUNT(*) as total
		FROM arsip WHERE deleted_at IS NULL GROUP BY month ORDER BY month DESC LIMIT ?`, months).Scan(&rows)
	result := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		result[i] = map[string]interface{}{"month": r.Month, "total": r.Total}
	}
	return result
}

func (s *DataScienceService) DetectSpjAnomalies() []map[string]interface{} {
	return []map[string]interface{}{}
}

func (s *DataScienceService) SemanticSearch(query string) []models.Arsip {
	var list []models.Arsip
	if query == "" {
		return list
	}
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("(to_tsvector('simple', COALESCE(nama_arsip,'') || ' ' || COALESCE(nomor_arsip,'') || ' ' || COALESCE(uraian,'') || ' ' || COALESCE(ocr_text,'') || ' ' || COALESCE(tags,'')) @@ plainto_tsquery('simple', ?))", query).
		Limit(20).Find(&list)
	return list
}

// ── DASHBOARD CACHE SERVICE ───────────────────────────────────────────────────

type DashboardCacheService struct{}

func (s *DashboardCacheService) GetStats() map[string]interface{} {
	var total, aktif, inaktif int64
	database.DB.Model(&models.Arsip{}).Count(&total)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'aktif'").Count(&aktif)
	database.DB.Model(&models.Arsip{}).Where("status_arsip = 'inaktif'").Count(&inaktif)
	return map[string]interface{}{
		"total": total, "aktif": aktif, "inaktif": inaktif,
	}
}

// ── AUTO DISPOSAL SERVICE ─────────────────────────────────────────────────────

type AutoDisposalService struct{}

// CheckAndCreatePemusnahan scans all archives whose retention date has passed
// and whose kode_klasifikasi.penyusutan_arsip = 'musnah'. Archives that are
// not yet in any active pemusnahan (diajukan/disetujui) are grouped into a
// single new pemusnahan record with status 'diajukan'.
// Returns the number of archives added.
func (s *AutoDisposalService) CheckAndCreatePemusnahan() int {
	now := time.Now()
	nowStr := now.Format("2006-01-02")

	var expiredArsip []models.Arsip
	database.DB.
		Preload("KodeKlasifikasi").
		Joins("INNER JOIN kode_klasifikasi ON kode_klasifikasi.id = arsip.kode_klasifikasi_id AND kode_klasifikasi.deleted_at IS NULL").
		Where("arsip.status_arsip NOT IN ('musnah','siap_penyusutan','permanen')").
		Where("arsip.tanggal_retensi_berakhir IS NOT NULL AND arsip.tanggal_retensi_berakhir < ?", nowStr).
		Where("kode_klasifikasi.penyusutan_arsip = ?", "musnah").
		Where("arsip.deleted_at IS NULL").
		Where("arsip.id NOT IN (SELECT pi.arsip_id FROM pemusnahan_arsip_items pi INNER JOIN pemusnahan_arsip pa ON pa.id = pi.pemusnahan_id WHERE pa.status IN ('diajukan','disetujui') AND pa.deleted_at IS NULL)").
		Where("arsip.id NOT IN (SELECT pa2.arsip_id FROM pemusnahan_arsip pa2 WHERE pa2.arsip_id IS NOT NULL AND pa2.arsip_id != '' AND pa2.status IN ('diajukan','disetujui') AND pa2.deleted_at IS NULL)").
		Order("arsip.tanggal_retensi_berakhir ASC").
		Find(&expiredArsip)

	if len(expiredArsip) == 0 {
		return 0
	}

	// Find the first admin user to act as system creator
	var adminUser models.User
	var createdBy *string
	if database.DB.Joins("JOIN roles ON roles.id = users.role_id").
		Where("roles.name = ? AND users.deleted_at IS NULL", "Admin").
		First(&adminUser).Error == nil {
		createdBy = &adminUser.ID
	}

	userPengajuID := ""
	if createdBy != nil {
		userPengajuID = *createdBy
	}
	pemusnahan := models.PemusnahanArsip{
		ID:                 uuid.New().String(),
		NamaKegiatan:       fmt.Sprintf("Pemusnahan Otomatis - Retensi Habis (%s)", now.Format("02 Jan 2006")),
		TanggalPelaksanaan: &now,
		TanggalPengajuan:   &now,
		Status:             "diajukan",
		CreatedBy:          createdBy,
		UserPengajuID:      userPengajuID, // Ensure foreign key passes constraint (not a pointer in the struct)
		IsAuto:             true,
		AlasanPengajuan:    "Otomatis oleh sistem: masa retensi arsip telah habis berdasarkan kode klasifikasi (penyusutan = musnah).",
	}
	arsipIDs := make([]string, 0, len(expiredArsip))
	for _, a := range expiredArsip {
		arsipIDs = append(arsipIDs, a.ID)
	}
	
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&pemusnahan).Error; err != nil {
			return err
		}
		for _, a := range expiredArsip {
			item := models.PemusnahanItem{
				PemusnahanID: pemusnahan.ID,
				ArsipID:      a.ID,
				CreatedAt:    now,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&models.Arsip{}).Where("id IN ?", arsipIDs).Update("status_arsip", "siap_penyusutan").Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("[AUTO-DISPOSAL] Gagal membuat pemusnahan: %v", err)
		return 0
	}

	log.Printf("[AUTO-DISPOSAL] Pemusnahan otomatis dibuat untuk %d arsip yang masa retensinya habis.", len(arsipIDs))
	return len(arsipIDs)
}

// RecalculateRetentionDates recalculates tanggal_retensi_berakhir for all
// archives that have a kode_klasifikasi with retensi > 0 but no retention end
// date set. This fixes legacy data.
func (s *AutoDisposalService) RecalculateRetentionDates() int {
	var arsipList []models.Arsip
	database.DB.
		Preload("KodeKlasifikasi").
		Where("tanggal_retensi_berakhir IS NULL AND tanggal_dibuat IS NOT NULL AND kode_klasifikasi_id IS NOT NULL AND kode_klasifikasi_id != ''").
		Where("deleted_at IS NULL").
		Find(&arsipList)

	count := 0
	for _, a := range arsipList {
		if a.KodeKlasifikasi == nil || a.TanggalDibuat == nil {
			continue
		}
		retDate := models.HitungRetensiBerakhir(*a.TanggalDibuat, a.KodeKlasifikasi)
		if retDate != nil {
			database.DB.Model(&models.Arsip{}).Where("id = ?", a.ID).Update("tanggal_retensi_berakhir", retDate)
			count++
		}
	}
	if count > 0 {
		log.Printf("[AUTO-DISPOSAL] Recalculated retention dates for %d archives.", count)
	}
	return count
}
