package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/models"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ── OFFSITE (CLOUD) BACKUP ────────────────────────────────────────────────────
//
// The local database stays the source of truth. This service mirrors it to a
// cloud MySQL/MariaDB database and mirrors the archive files (storage/app/uploads)
// to an S3-compatible object-storage bucket. It is intended to run on the
// central server only (never in client mode or on Vercel).

// LegacyExcludeTables returns the legacy/debug tables that exist in the current
// database and should be skipped by backups. Only tables that actually exist are
// returned so mysqldump never fails on a missing one.
func LegacyExcludeTables() []string {
	if database.DB == nil {
		return nil
	}
	excluded := map[string]bool{
		// Laravel Telescope debug traces (~95MB on migrated installs)
		"telescope_entries": true, "telescope_entries_tags": true, "telescope_monitoring": true,
		// Caches — can be regenerated, not data
		"ai_classification_cache": true, "ai_classification_caches": true, "analytics_cache": true,
		// Legacy Laravel infrastructure / auth
		"failed_jobs": true, "jobs": true, "migrations": true, "tenants": true,
		// Duplicate Laravel schema (the Go app uses the singular forms)
		"arsips": true, "jadwal_retensis": true, "jadwal_retensi_arsips": true,
		"jenis_arsips": true, "kode_klasifikasis": true, "pemberkasans": true,
		"unit_kerjas": true, "pemusnahan_arsips": true,
	}
	var rows []struct {
		TableName string `gorm:"column:table_name"`
	}
	if err := database.DB.Raw(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()",
	).Scan(&rows).Error; err != nil {
		return nil
	}
	var out []string
	for _, r := range rows {
		if excluded[r.TableName] {
			out = append(out, r.TableName)
		}
	}
	sort.Strings(out)
	return out
}

// offsiteDir returns the directory used to persist offsite backup status.
func offsiteDir() string {
	if config.IsVercel() {
		return "/tmp/offsite"
	}
	return filepath.Join("storage", "app", "offsite")
}

// dumpLocalDatabase runs mysqldump against the local database and returns the
// SQL bytes. When called with no table names it dumps the whole database: the
// legacy/debug tables are skipped only when OffsiteConfig.IncludeLegacy is
// false (the dump stays small); with IncludeLegacy=true it is a full 1:1 copy.
func dumpLocalDatabase(tables ...string) ([]byte, error) {
	if !config.CanBackup() {
		return nil, fmt.Errorf("mysqldump tidak tersedia di sistem ini")
	}
	host := getEnv("DB_HOST", "127.0.0.1")
	user := getEnv("DB_USERNAME", "root")
	port := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_DATABASE", config.App.DBName)

	args := []string{
		"--host=" + host,
		"--port=" + port,
		"--user=" + user,
		"--no-tablespaces",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--add-drop-table",
		// --hex-blob is essential: without it binary columns (e.g. backup_logs
		// .file_content) are escaped as text and can be altered by charset
		// handling during dump → import, corrupting the offsite copy.
		"--hex-blob",
	}
	if len(tables) == 0 {
		// Full 1:1 mirror (include legacy tables) unless the operator opted out.
		if !config.LoadOffsite().IncludeLegacy {
			for _, t := range LegacyExcludeTables() {
				args = append(args, "--ignore-table="+dbName+"."+t)
			}
		}
		// backup_logs is mirrored separately (see syncBackupLogsToCloud) right
		// after the run records itself, so the cloud copy includes the newest
		// row. Skipping it here also avoids shipping its large LONGBLOB twice.
		args = append(args, "--ignore-table="+dbName+".backup_logs")
		args = append(args, dbName)
	} else {
		args = append(args, dbName)
		args = append(args, tables...)
	}

	cmd := exec.Command("mysqldump", args...)
	if pw := getEnv("DB_PASSWORD", ""); pw != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+pw)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		msg := err.Error()
		if s := strings.TrimSpace(errBuf.String()); s != "" {
			msg += " — " + s
		}
		return nil, fmt.Errorf("mysqldump gagal: %s", msg)
	}
	return normalizeDumpForCloud(out.Bytes()), nil
}

// uuidDefaultRe matches MariaDB's `DEFAULT uuid()` column default.
var uuidDefaultRe = regexp.MustCompile(`(?i)DEFAULT\s+uuid\(\)`)

// normalizeDumpForCloud rewrites MariaDB-specific syntax that MySQL rejects.
// MariaDB emits `DEFAULT uuid()`; MySQL requires the expression form
// `DEFAULT (uuid())`. Both MariaDB and MySQL accept the parenthesized form,
// so this rewrite is safe regardless of the target engine.
func normalizeDumpForCloud(sqlData []byte) []byte {
	if !bytes.Contains(sqlData, []byte("uuid()")) && !bytes.Contains(sqlData, []byte("UUID()")) {
		return sqlData
	}
	return uuidDefaultRe.ReplaceAll(sqlData, []byte("DEFAULT (uuid())"))
}

// cliSSLMode maps a friendly TLS mode to the mysql CLI --ssl-mode value.
func cliSSLMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "disabled", "false", "off", "none":
		return "DISABLED"
	case "preferred":
		return "PREFERRED"
	case "verify", "true", "verify-ca":
		return "VERIFY_CA"
	default: // required / skip-verify / empty
		return "REQUIRED"
	}
}

// dsSQLMode maps a friendly TLS mode to the go-sql-driver `tls=` value.
func dsSQLMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "disabled", "false", "off", "none":
		return "false"
	case "preferred":
		return "preferred"
	case "verify", "true", "verify-ca":
		return "true"
	default: // required / skip-verify / empty
		return "skip-verify"
	}
}

var (
	mysqlFlavorOnce sync.Once
	mysqlIsMariaDB  bool
)

// mysqlClientIsMariaDB reports whether the mysql CLI in PATH is the MariaDB
// client. MariaDB uses --ssl/--skip-ssl while MySQL 8 uses --ssl-mode; passing
// the wrong flag aborts the import with "unknown variable".
func mysqlClientIsMariaDB() bool {
	mysqlFlavorOnce.Do(func() {
		out, err := exec.Command("mysql", "--version").Output()
		if err == nil && strings.Contains(strings.ToLower(string(out)), "mariadb") {
			mysqlIsMariaDB = true
		}
	})
	return mysqlIsMariaDB
}

// mysqlTLSArgs returns the TLS flags appropriate for the detected mysql client.
func mysqlTLSArgs(cfg config.OffsiteConfig) []string {
	mode := strings.ToLower(strings.TrimSpace(cfg.DBTLS))
	if mysqlClientIsMariaDB() {
		switch mode {
		case "disabled", "false", "off", "none":
			return []string{"--skip-ssl"}
		case "verify", "true", "verify-ca":
			args := []string{"--ssl", "--ssl-verify-server-cert"}
			if cfg.DBCAFile != "" {
				args = append(args, "--ssl-ca="+cfg.DBCAFile)
			}
			return args
		default: // required / preferred / empty
			args := []string{"--ssl"}
			if cfg.DBCAFile != "" {
				args = append(args, "--ssl-ca="+cfg.DBCAFile)
			}
			return args
		}
	}
	// MySQL client.
	args := []string{"--ssl-mode=" + cliSSLMode(cfg.DBTLS)}
	if cfg.DBCAFile != "" {
		args = append(args, "--ssl-ca="+cfg.DBCAFile)
	}
	return args
}

// importToCloud pipes a SQL dump into the cloud MySQL database using the mysql
// CLI. mysqldump emits DROP TABLE + CREATE TABLE, so each run fully replaces the
// cloud copy (an idempotent offsite mirror).
func importToCloud(cfg config.OffsiteConfig, sqlData []byte) error {
	if !config.HasMysql() {
		return fmt.Errorf("klien mysql tidak tersedia di sistem ini")
	}
	if !cfg.DBReady() {
		return fmt.Errorf("pengaturan database cloud belum lengkap")
	}
	args := []string{
		"--host=" + cfg.DBHost,
		"--port=" + cfg.DBPort,
		"--user=" + cfg.DBUser,
		"--default-character-set=utf8mb4",
	}
	args = append(args, mysqlTLSArgs(cfg)...)
	args = append(args, cfg.DBName)

	// MySQL enforces sql_require_primary_key by default on some managed
	// services (e.g. Aiven). Relax it for this session so a table that lacks a
	// primary key cannot abort the whole import. MariaDB has no such variable.
	payload := sqlData
	if cloudNeedsPKWorkaround(cfg) {
		payload = append([]byte("SET SESSION sql_require_primary_key=0;\n"), sqlData...)
	}

	cmd := exec.Command("mysql", args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPass)

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := err.Error()
		if s := strings.TrimSpace(errBuf.String()); s != "" {
			msg += " — " + s
		}
		return fmt.Errorf("impor ke database cloud gagal: %s", msg)
	}
	return nil
}

// TestCloudDatabase verifies that the cloud MySQL settings are reachable.
func TestCloudDatabase(cfg config.OffsiteConfig) error {
	if !cfg.DBReady() {
		return fmt.Errorf("host, nama database, dan username cloud wajib diisi")
	}
	db, err := sql.Open("mysql", cloudDSN(cfg))
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err == nil {
		log.Printf("[offsite] database cloud OK (%s)", version)
	}
	return nil
}

// cloudDSN builds the go-sql-driver DSN for the cloud MySQL target.
func cloudDSN(cfg config.OffsiteConfig) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&timeout=10s&readTimeout=60s&writeTimeout=60s&tls=%s",
		cfg.DBUser, cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName, dsSQLMode(cfg.DBTLS))
}

// cloudVersion returns the target server's version string (empty on failure).
func cloudVersion(cfg config.OffsiteConfig) string {
	db, err := sql.Open("mysql", cloudDSN(cfg))
	if err != nil {
		return ""
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	var v string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err != nil {
		return ""
	}
	return v
}

// cloudNeedsPKWorkaround reports whether the target enforces
// sql_require_primary_key (MySQL) and therefore needs the session relaxation.
func cloudNeedsPKWorkaround(cfg config.OffsiteConfig) bool {
	v := strings.ToLower(cloudVersion(cfg))
	return v != "" && !strings.Contains(v, "mariadb")
}

// newS3Client builds a minio client for the configured S3-compatible endpoint.
func newS3Client(cfg config.OffsiteConfig) (*minio.Client, error) {
	if !cfg.S3Ready() {
		return nil, fmt.Errorf("endpoint, bucket, access key, dan secret key wajib diisi")
	}
	region := cfg.S3Region
	if region == "" {
		region = "auto"
	}
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3Scheme() == "https",
		Region: region,
	}
	if cfg.S3PathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	} else {
		opts.BucketLookup = minio.BucketLookupAuto
	}
	return minio.New(cfg.S3Host(), opts)
}

// TestS3 verifies the object-storage bucket is reachable.
func TestS3(cfg config.OffsiteConfig) error {
	cl, err := newS3Client(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exists, err := cl.BucketExists(ctx, cfg.S3Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("bucket %q tidak ditemukan / tidak dapat diakses", cfg.S3Bucket)
	}
	// A single list confirms the credentials can read the bucket.
	for obj := range cl.ListObjects(ctx, cfg.S3Bucket, minio.ListObjectsOptions{Prefix: strings.Trim(cfg.S3Prefix, "/"), MaxKeys: 1}) {
		if obj.Err != nil {
			return obj.Err
		}
		break
	}
	return nil
}

// uploadFilesToS3 mirrors the uploads directory to the bucket under
// <prefix>/uploads/<relative-path>.
func uploadFilesToS3(cfg config.OffsiteConfig) (int, int64, error) {
	cl, err := newS3Client(cfg)
	if err != nil {
		return 0, 0, err
	}
	ctx := context.Background()

	exists, err := cl.BucketExists(ctx, cfg.S3Bucket)
	if err != nil {
		return 0, 0, err
	}
	if !exists {
		return 0, 0, fmt.Errorf("bucket %q tidak ditemukan", cfg.S3Bucket)
	}

	root := config.UploadDir()
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 0, 0, nil
	}

	prefix := strings.Trim(cfg.S3Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}

	var count int
	var total int64
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		objectName := prefix + "uploads/" + filepath.ToSlash(rel)
		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if _, err := cl.FPutObject(ctx, cfg.S3Bucket, objectName, path, minio.PutObjectOptions{ContentType: contentType}); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		count++
		total += info.Size()
		return nil
	})
	return count, total, err
}

// dirStats counts files and total bytes under root (0,0 when root is absent).
func dirStats(root string) (int, int64, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return 0, 0, nil
	}
	var count int
	var total int64
	err = filepath.Walk(root, func(_ string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi.IsDir() {
			return nil
		}
		count++
		total += fi.Size()
		return nil
	})
	return count, total, err
}

// rcloneDest builds the "<remote>:<path>" destination for the file mirror.
func rcloneDest(cfg config.OffsiteConfig) string {
	remote := strings.TrimRight(strings.TrimSpace(cfg.RcloneRemote), ":")
	path := strings.Trim(cfg.RclonePath, "/")
	if path == "" {
		return remote + ":"
	}
	return remote + ":" + path
}

// TestRclone verifies that the configured rclone remote is reachable (i.e. its
// stored credentials still work).
func TestRclone(cfg config.OffsiteConfig) error {
	if !cfg.RcloneReady() {
		return fmt.Errorf("rclone remote wajib diisi")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rclone", "lsd", strings.TrimRight(strings.TrimSpace(cfg.RcloneRemote), ":")+":")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// uploadFilesViaRclone mirrors the uploads directory to an rclone remote, e.g.
// a Google Drive account already authenticated with `rclone config`. It uses
// `rclone copy` (one-way, never deletes) so the remote is only ever added to.
func uploadFilesViaRclone(cfg config.OffsiteConfig) (int, int64, error) {
	if !cfg.RcloneReady() {
		return 0, 0, fmt.Errorf("rclone remote wajib diisi")
	}
	root := config.UploadDir()
	count, total, err := dirStats(root)
	if err != nil {
		return 0, 0, err
	}
	if count == 0 {
		return 0, 0, nil
	}

	dest := rcloneDest(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rclone", "copy", root, dest,
		"--create-empty-src-dirs", "--stats-one-line", "--stats=30s")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if msg == "" {
			msg = err.Error()
		}
		return 0, 0, fmt.Errorf("rclone copy ke %s gagal: %s", dest, msg)
	}
	return count, total, nil
}

// syncBackupLogsToCloud re-mirrors just the backup_logs table. The main dump
// runs before the run records itself, so without this the newest backup_logs
// row (this very run) would be absent from the cloud copy. The table is small
// (a handful of rows), so the extra round trip is cheap.
func syncBackupLogsToCloud(cfg config.OffsiteConfig) error {
	data, err := dumpLocalDatabase("backup_logs")
	if err != nil {
		return err
	}
	return importToCloud(cfg, data)
}

// ── RESULT / STATUS ───────────────────────────────────────────────────────────

// OffsiteTargetResult describes the outcome for one mirror target.
type OffsiteTargetResult struct {
	Attempted bool   `json:"attempted"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail"`
	Bytes     int64  `json:"bytes"`
	Items     int    `json:"items"`
}

// OffsiteResult is the outcome of a single offsite backup run.
type OffsiteResult struct {
	StartedAt  time.Time           `json:"started_at"`
	FinishedAt time.Time           `json:"finished_at"`
	DurationMS int64               `json:"duration_ms"`
	Database   OffsiteTargetResult `json:"database"`
	Files      OffsiteTargetResult `json:"files"`
	Message    string              `json:"message"`
}

// OffsiteStatus returns the last recorded offsite backup result. It returns an
// empty result (not an error) when no run has happened yet.
func OffsiteStatus() OffsiteResult {
	var res OffsiteResult
	b, err := os.ReadFile(filepath.Join(offsiteDir(), "status.json"))
	if err != nil {
		return res
	}
	_ = json.Unmarshal(b, &res)
	return res
}

func writeOffsiteStatus(res OffsiteResult) {
	dir := offsiteDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	if b, err := json.MarshalIndent(res, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "status.json"), b, 0644)
	}
}

// OffsiteRunning reports whether a run is currently in progress.
var (
	offsiteMu      sync.Mutex
	offsiteRunning bool
)

// OffsiteRunning reports whether an offsite backup is currently running.
func OffsiteRunning() bool {
	offsiteMu.Lock()
	defer offsiteMu.Unlock()
	return offsiteRunning
}

// RunOffsiteNow executes an offsite backup immediately: mirror the local
// database to the cloud MySQL database and the uploads directory to object
// storage. It is safe to call concurrently — a second call is rejected.
func RunOffsiteNow() (OffsiteResult, error) {
	cfg := config.LoadOffsite()

	offsiteMu.Lock()
	if offsiteRunning {
		offsiteMu.Unlock()
		return OffsiteResult{}, fmt.Errorf("backup offsite sedang berjalan")
	}
	offsiteRunning = true
	offsiteMu.Unlock()
	defer func() {
		offsiteMu.Lock()
		offsiteRunning = false
		offsiteMu.Unlock()
	}()

	if !cfg.HasWork() {
		return OffsiteResult{}, fmt.Errorf("target backup cloud belum dikonfigurasi (isi database cloud dan/atau object storage)")
	}

	res := OffsiteResult{StartedAt: time.Now()}
	var problems []string

	// ── Database mirror ──
	if cfg.BackupDB && cfg.DBReady() {
		res.Database.Attempted = true
		sqlData, err := dumpLocalDatabase()
		if err != nil {
			res.Database.Detail = err.Error()
			problems = append(problems, "database: "+err.Error())
		} else if err := importToCloud(cfg, sqlData); err != nil {
			res.Database.Detail = err.Error()
			problems = append(problems, "database: "+err.Error())
		} else {
			res.Database.OK = true
			res.Database.Bytes = int64(len(sqlData))
			res.Database.Detail = fmt.Sprintf("dump %s terkirim ke %s", humanBytes(int64(len(sqlData))), cfg.DBName)
		}
	}

	// ── File mirror ──
	if cfg.BackupFiles {
		switch cfg.FileBackend() {
		case "s3":
			res.Files.Attempted = true
			count, bytes, err := uploadFilesToS3(cfg)
			if err != nil {
				res.Files.Detail = err.Error()
				problems = append(problems, "file: "+err.Error())
			} else {
				res.Files.OK = true
				res.Files.Items = count
				res.Files.Bytes = bytes
				res.Files.Detail = fmt.Sprintf("%d file (%s) tersinkron ke bucket %s", count, humanBytes(bytes), cfg.S3Bucket)
			}
		case "rclone":
			res.Files.Attempted = true
			count, bytes, err := uploadFilesViaRclone(cfg)
			if err != nil {
				res.Files.Detail = err.Error()
				problems = append(problems, "file: "+err.Error())
			} else {
				res.Files.OK = true
				res.Files.Items = count
				res.Files.Bytes = bytes
				res.Files.Detail = fmt.Sprintf("%d file (%s) tersinkron ke %s", count, humanBytes(bytes), rcloneDest(cfg))
			}
		}
	}

	res.FinishedAt = time.Now()
	res.DurationMS = res.FinishedAt.Sub(res.StartedAt).Milliseconds()

	status := "success"
	if len(problems) > 0 {
		if res.Database.OK || res.Files.OK {
			status = "partial"
		} else {
			status = "failed"
		}
	}
	res.Message = fmt.Sprintf("status=%s", status)
	if len(problems) > 0 {
		res.Message += "; " + strings.Join(problems, " | ")
	}

	writeOffsiteStatus(res)
	recordOffsiteLog(cfg, res, status)

	// Make the cloud copy of backup_logs match exactly, including this run's row.
	if res.Database.OK {
		if err := syncBackupLogsToCloud(cfg); err != nil {
			log.Printf("[offsite] sinkronisasi backup_logs gagal: %v", err)
			res.Message += "; backup_logs sync: " + err.Error()
			writeOffsiteStatus(res)
		}
	}
	return res, nil
}

// recordOffsiteLog persists the run into backup_logs so it shows up in the
// existing Backup & Restore history.
func recordOffsiteLog(cfg config.OffsiteConfig, res OffsiteResult, status string) {
	if database.DB == nil {
		return
	}
	now := time.Now()
	name := fmt.Sprintf("cloud_%s", now.Format("2006-01-02_150405"))
	var parts []string
	if cfg.BackupDB && cfg.DBReady() {
		parts = append(parts, "mysql")
	}
	cloudPath := strings.Trim(cfg.S3Prefix, "/")
	switch cfg.FileBackend() {
	case "s3":
		parts = append(parts, "s3")
	case "rclone":
		parts = append(parts, "rclone")
		cloudPath = rcloneDest(cfg)
	}
	provider := strings.Join(parts, "+")
	if provider == "" {
		provider = "offsite"
	}
	logRow := models.BackupLog{
		ID:            uuid.New().String(),
		FileName:      name,
		FilePath:      "",
		FileSize:      res.Database.Bytes,
		BackupType:    "cloud",
		Status:        status,
		CloudProvider: provider,
		CloudPath:     cloudPath,
		Notes:         res.Message,
		CompletedAt:   &now,
	}
	database.DB.Create(&logRow)
}

// StartOffsiteScheduler launches the daily offsite backup goroutine. It is a
// no-op when the feature is disabled, on Vercel, or when no target is
// configured yet. Safe to call repeatedly (e.g. after saving settings).
func StartOffsiteScheduler() {
	if config.IsVercel() {
		return
	}
	offsiteSchedMu.Lock()
	defer offsiteSchedMu.Unlock()
	if offsiteSchedActive {
		return
	}
	cfg := config.LoadOffsite()
	if !cfg.Enabled {
		return
	}
	if !cfg.HasWork() {
		log.Printf("[offsite] diaktifkan tetapi kredensial cloud belum lengkap — penjadwal belum berjalan")
		return
	}
	offsiteSchedActive = true
	go offsiteSchedulerLoop()
}

var (
	offsiteSchedMu     sync.Mutex
	offsiteSchedActive bool
)

func offsiteSchedulerLoop() {
	first := config.LoadOffsite()
	log.Printf("[offsite] penjadwal aktif — setiap hari %02d:%02d (lokal)", first.Hour, first.Minute)
	for {
		next := nextOffsiteRun(time.Now(), config.LoadOffsite())
		d := time.Until(next)
		if d < 0 {
			d = time.Minute
		}
		time.Sleep(d)

		// Re-read settings: enabling/disabling takes effect without a restart.
		cfg := config.LoadOffsite()
		if !cfg.Enabled || !cfg.HasWork() {
			continue
		}
		if _, err := RunOffsiteNow(); err != nil {
			log.Printf("[offsite] backup gagal: %v", err)
		}
		// Avoid firing twice within the same minute.
		time.Sleep(time.Minute)
	}
}

// nextOffsiteRun returns the next occurrence of the configured daily time.
func nextOffsiteRun(now time.Time, cfg config.OffsiteConfig) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), cfg.Hour, cfg.Minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
