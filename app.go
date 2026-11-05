package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/models"
)

// App holds the application state.
type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

// startup initializes config, database (GORM AutoMigrate), and services.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Load .env (godotenv)
	config.Load()

	// Connect to database (GORM)
	if err := database.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "[simarc] DB gagal: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[simarc] DB ok\n")

	// Run migration (AutoMigrate all models + schema repairs)
	if err := database.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "[simarc] Migrasi gagal: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[simarc] Migrasi selesai\n")

	// Seed default data
	database.SeedIfNeeded()
}

func loadEnvConfig() {
	// .env is loaded by config.Load() via godotenv.
}

// shutdown closes the database connection.
func (a *App) shutdown(ctx context.Context) {
	if database.DB != nil {
		if sqlDB, err := database.DB.DB(); err == nil {
			sqlDB.Close()
		}
	}
}

// GetStatus returns application status.
func (a *App) GetStatus() map[string]interface{} {
	dbOK := database.Connected()
	return map[string]interface{}{
		"app":    "SIMARC",
		"db_ok":  dbOK,
		"os":     runtime.GOOS,
		"arch":   runtime.GOARCH,
	}
}

// PingBackend confirms bindings work.
func (a *App) PingBackend() string {
	return fmt.Sprintf("pong @ %s", time.Now().Format("15:04:05"))
}

// ListArsip returns recent arsip records.
func (a *App) ListArsip() []map[string]interface{} {
	if database.DB == nil {
		return []map[string]interface{}{}
	}
	var arsip []models.Arsip
	database.DB.Order("created_at desc").Limit(200).Find(&arsip)
	result := make([]map[string]interface{}, len(arsip))
	for i, ar := range arsip {
		result[i] = map[string]interface{}{
			"id":      ar.ID,
			"nomor":   ar.NomorArsip,
			"nama":    ar.NamaArsip,
			"status":  ar.StatusArsip,
			"created": ar.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated": ar.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	return result
}
