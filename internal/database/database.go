package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"arsippro/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Connect opens the database connection using the current application config.
func Connect() error {
	db, err := openDB(config.App.DBHost, config.App.DBPort, config.App.DBName, config.App.DBUser, config.App.DBPass)
	if err != nil {
		return err
	}
	DB = db
	log.Println("Database connected successfully")
	return nil
}

// openDB opens a GORM PostgreSQL connection from raw parameters.
func openDB(host, port, name, user, pass string) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Jakarta",
		host, user, pass, name, port,
	)

	// Override with DATABASE_URL if set (Neon, PlanetScale, etc.)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		dsn = dbURL
	}

	logLevel := logger.Error

	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
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
	if err := DB.Raw("SELECT version()").Scan(&version).Error; err == nil {
		info.Version = version
	}
	var tables int64
	DB.Raw("SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'").Scan(&tables)
	info.Tables = tables
	return info
}
