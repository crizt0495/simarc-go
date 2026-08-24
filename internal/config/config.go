package config

import (
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppName     string
	AppURL      string
	AppPort     string
	AppTimezone string
	DBHost      string
	DBPort      string
	DBName      string
	DBUser      string
	DBPass      string
	SessionKey  string
	AppDebug    string
	// Institution header for PDF documents
	AppInstitution    string
	AppInstitutionSub string
	AppAddress        string
	AppPhone          string
	AppFax            string
	AppEmail          string
	AppWeb            string

	// Google Drive backup (client-side OAuth via Google Identity Services)
	GoogleDriveClientID string
	GoogleDriveFolderID string
}

var App Config

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// MySQL defaults — Aiven MySQL is the primary database.
	dbHost := getEnv("DB_HOST", "127.0.0.1")
	dbPort := getEnv("DB_PORT", "3306")
	dbName := getEnv("DB_DATABASE", "defaultdb")
	dbUser := getEnv("DB_USERNAME", "avnadmin")
	dbPass := getEnv("DB_PASSWORD", "")

	// Support DATABASE_URL for cloud convenience (mysql://user:pass@host:port/db).
	if dbURL := getEnv("DATABASE_URL", ""); dbURL != "" {
		// Strip scheme prefix; the rest is a valid MySQL DSN.
		cleanURL := dbURL
		cleanURL = strings.TrimPrefix(cleanURL, "mysql://")
		cleanURL = strings.TrimPrefix(cleanURL, "postgresql://")
		// We pass through to the connector; if the user supplies a full URL we use it as-is.
		log.Printf("DATABASE_URL detected")
		// Keep parser for backward-compat (could extract host/port) but defaults above stand.
		_ = cleanURL
		if parsed, err := url.Parse(dbURL); err == nil {
			if h := parsed.Hostname(); h != "" {
				dbHost = h
			}
			if p := parsed.Port(); p != "" {
				dbPort = p
			}
			if u := parsed.User.Username(); u != "" {
				dbUser = u
			}
			if pw, ok := parsed.User.Password(); ok {
				dbPass = pw
			}
			dbName = strings.TrimPrefix(parsed.Path, "/")
			if idx := strings.Index(dbName, "?"); idx != -1 {
				dbName = dbName[:idx]
			}
			log.Printf("DATABASE_URL parsed — host=%s port=%s db=%s", dbHost, dbPort, dbName)
		}
	}

	App = Config{
		AppName:     getEnv("APP_NAME", "SIMARC-Sistem Informasi Manajemen Arsip Record Center"),
		AppURL:      getEnv("APP_URL", "http://localhost:8080"),
		AppPort:     getEnv("APP_PORT", "8080"),
		AppTimezone: getEnv("APP_TIMEZONE", "Asia/Jakarta"),
		DBHost:      dbHost,
		DBPort:     dbPort,
		DBName:     dbName,
		DBUser:     dbUser,
		DBPass:     dbPass,

		SessionKey: getEnv("SESSION_KEY", ""),
		AppDebug:   getEnv("APP_DEBUG", "false"),
		AppInstitution:    getEnv("APP_INSTITUTION", "PEMERINTAH KOTA PROBOLINGGO"),
		AppInstitutionSub: getEnv("APP_INSTITUTION_SUB", "BADAN KESATUAN BANGSA DAN POLITIK"),
		AppAddress:        getEnv("APP_ADDRESS", "Jalan Mawar No. 39A, Kota Probolinggo, Jawa Timur 67219"),
		AppPhone:          getEnv("APP_PHONE", "(0335) 426436"),
		AppFax:            getEnv("APP_FAX", "(0335) 426437"),
		AppEmail:          getEnv("APP_EMAIL", "bakesbangpol@probolinggokota.go.id"),
		AppWeb:            getEnv("APP_WEB", "bakesbangpol.probolinggokota.go.id"),

		GoogleDriveClientID: getEnv("GOOGLE_DRIVE_CLIENT_ID", ""),
		GoogleDriveFolderID: getEnv("GOOGLE_DRIVE_FOLDER_ID", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvFilePath returns the path to the .env file (relative to project root,
// matching where godotenv.Load() looks for it).
func EnvFilePath() string {
	return ".env"
}

// UpdateEnv updates the given keys in the .env file (creating it if missing)
// and also sets them in the process environment so os.Getenv reflects the
// change immediately (needed by services like mysqldump backup that read
// credentials from the environment).
func UpdateEnv(updates map[string]string) error {
	path := EnvFilePath()
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")

	// Find keys that need updating
	keys := make(map[string]bool, len(updates))
	for k := range updates {
		keys[k] = true
	}

	found := make(map[string]bool, len(updates))
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Keep comments, blanks, and unrelated lines untouched
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			out = append(out, line)
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		if !keys[key] {
			out = append(out, line)
			continue
		}
		found[key] = true
		out = append(out, key+"="+quoteEnvValue(updates[key]))
	}

	// Append keys that were not present in the file
	for k, v := range updates {
		if !found[k] {
			out = append(out, k+"="+quoteEnvValue(v))
		}
	}

	// 0600: the file contains database secrets
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0600); err != nil {
		return err
	}

	// Reflect in process environment immediately
	for k, v := range updates {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// quoteEnvValue quotes a value when it contains characters that could break
// .env parsing (spaces, quotes, hashes, semicolons — some dotenv parsers treat
// a leading ; as a comment marker).
func quoteEnvValue(v string) string {
	if strings.ContainsAny(v, " \t#;\"'") {
		return "\"" + strings.ReplaceAll(v, "\"", "\\\"") + "\""
	}
	return v
}

// IsVercel returns true when running on Vercel's serverless platform.
func IsVercel() bool {
	return os.Getenv("VERCEL") == "1"
}

// StorageDir returns the base directory for file storage.
// On Vercel, uses /tmp (ephemeral); locally uses the project storage dir.
func StorageDir() string {
	if IsVercel() {
		return "/tmp"
	}
	return "."
}

// BackupDir returns the directory for database backups.
func BackupDir() string {
	if IsVercel() {
		return "/tmp/backups"
	}
	return "storage/app/backups/database"
}

// QRCodeDir returns the directory for QR code images.
func QRCodeDir() string {
	if IsVercel() {
		return "/tmp/qr-codes"
	}
	return "public/storage/qr-codes"
}

// UploadDir returns the directory for file uploads.
func UploadDir() string {
	if IsVercel() {
		return "/tmp/uploads"
	}
	return "storage/app/uploads"
}
