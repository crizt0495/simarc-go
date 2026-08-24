package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"arsippro/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Connect opens the database connection using the current application config.
// Connection pool is tuned for Vercel serverless (short-lived workers).
func Connect() error {
	db, err := openDB(config.App.DBHost, config.App.DBPort, config.App.DBName, config.App.DBUser, config.App.DBPass)
	if err != nil {
		return err
	}
	DB = db
	log.Println("Database connected successfully")
	return nil
}

// openDB opens a GORM MySQL connection from raw parameters with sane pooling defaults.
// DSN format: user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Asia%2FJakarta
func openDB(host, port, name, user, pass string) (*gorm.DB, error) {
	// TLS mode for the MySQL connection (go-sql-driver values):
	//   preferred  – use TLS when the server supports it, fall back to plain
	//   skip-verify– always TLS, skip certificate verification (Aiven self-signed CA)
	//   true       – always TLS with full certificate verification
	//   false      – never TLS
	// "preferred" is the safe default: it works against cloud providers that
	// REQUIRE TLS (e.g. Aiven) as well as local MariaDB/MySQL without TLS.
	// Previously this was hardcoded to "true", which broke every connection
	// whose certificate was not signed by a system-trusted CA.
	tlsMode := config.App.DBTLS
	if tlsMode == "" {
		tlsMode = "preferred"
	}
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=10s&readTimeout=20s&writeTimeout=20s&multiStatements=true&interpolateParams=true&tls=%s",
		user, pass, host, port, name, tlsMode,
	)

	// Support DATABASE_URL if user prefers connection-string style.
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		// Accept both mysql:// scheme and raw DSN (mysql:// from PlanetScale, Aiven, etc.).
		cleanURL := dbURL
		// Strip mysql:// and postgres:// prefixes if any (legacy configs).
		cleanURL = strings.TrimPrefix(cleanURL, "mysql://")
		cleanURL = strings.TrimPrefix(cleanURL, "postgresql://")
		// mysql://user:pass@host:port/db?params → DSN is already in correct form after stripping scheme.
		dsn = cleanURL
		// Respect an explicit tls= parameter; otherwise apply the configured mode.
		if !strings.Contains(dsn, "tls=") {
			if strings.Contains(dsn, "?") {
				dsn += "&tls=" + tlsMode
			} else {
				dsn += "?tls=" + tlsMode
			}
		}
		log.Printf("Connecting to database with DATABASE_URL")
	}

	logLevel := logger.Error

	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, err
	}

	// Connection pooling — tuned for Vercel serverless:
	// Vercel functions are short-lived (a few seconds); we keep a modest pool
	// that opens fast, recycles frequently, and tolerates restarts.
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	if config.IsVercel() {
		// Serverless: most functions are cold; pool should be small but resilient.
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(2 * time.Minute)
		sqlDB.SetConnMaxIdleTime(1 * time.Minute)
	} else {
		// Local/long-running server.
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	}

	return gormDB, nil
}

// TestConnection tries to open and ping a database with the given parameters
// without changing the live connection. Returns nil on success.
func TestConnection(host, port, name, user, pass string) error {
	db, err := openDB(host, port, name, user, pass)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

// Reconnect swaps the live connection to the values currently stored in
// config.App, then re-runs migration + seed so the new database is ready.
func Reconnect() error {
	newDB, err := openDB(config.App.DBHost, config.App.DBPort, config.App.DBName, config.App.DBUser, config.App.DBPass)
	if err != nil {
		return err
	}
	if sqlDB, err := newDB.DB(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = sqlDB.PingContext(ctx)
		cancel()
		if err != nil {
			sqlDB.Close()
			return err
		}
	}

	if DB != nil {
		if oldSQL, err := DB.DB(); err == nil {
			oldSQL.Close()
		}
	}

	DB = newDB
	if err := Migrate(); err != nil {
		return err
	}
	SeedIfNeeded()
	return nil
}

// Connected reports whether the live database connection is reachable.
func Connected() bool {
	if DB == nil {
		return false
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx) == nil
}

// Info holds connection status information for display on the settings page.
type Info struct {
	Connected bool
	Version   string
	Tables    int64
}

// GetInfo returns current connection status (version, table count, ...).
func GetInfo() Info {
	info := Info{Connected: Connected()}
	if !info.Connected {
		return info
	}
	var version string
	if err := DB.Raw("SELECT VERSION()").Scan(&version).Error; err == nil {
		info.Version = version
	}
	// MySQL: DATABASE() returns the current database schema name.
	var tables int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'").Scan(&tables)
	info.Tables = tables
	return info
}
