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

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
		"app":   "SIMARC",
		"db_ok": dbOK,
		"os":    runtime.GOOS,
		"arch":  runtime.GOARCH,
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

// GetDashboardStats returns KPI data matching the reference app's dashboard.
func (a *App) GetDashboardStats() map[string]interface{} {
	if database.DB == nil {
		return map[string]interface{}{"error": "database not connected"}
	}
	db := database.DB

	var stats struct {
		Total          int64 `gorm:"column:total"`
		TotalItems     int64 `gorm:"column:total_items"`
		Aktif          int64 `gorm:"column:aktif"`
		Inaktif        int64 `gorm:"column:inaktif"`
		SiapPenyusutan int64 `gorm:"column:siap_penyusutan"`
		Musnah         int64 `gorm:"column:musnah"`
		WithFile       int64 `gorm:"column:with_file"`
		WithoutFile    int64 `gorm:"column:without_file"`
		Spj            int64 `gorm:"column:spj"`
		NonSpj         int64 `gorm:"column:non_spj"`
		Unclassified   int64 `gorm:"column:unclassified"`
	}

	db.Raw(`
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(jumlah), 0) as total_items,
			SUM(CASE WHEN status_arsip IN ('aktif','diberkaskan') THEN 1 ELSE 0 END) as aktif,
			SUM(CASE WHEN status_arsip = 'inaktif' THEN 1 ELSE 0 END) as inaktif,
			SUM(CASE WHEN status_arsip = 'siap_penyusutan' THEN 1 ELSE 0 END) as siap_penyusutan,
			SUM(CASE WHEN status_arsip = 'musnah' THEN 1 ELSE 0 END) as musnah,
			SUM(CASE WHEN file_path IS NOT NULL AND file_path != '' THEN 1 ELSE 0 END) as with_file,
			SUM(CASE WHEN file_path IS NULL OR file_path = '' THEN 1 ELSE 0 END) as without_file,
			SUM(CASE WHEN jenis_arsip = 'SPJ' THEN 1 ELSE 0 END) as spj,
			SUM(CASE WHEN jenis_arsip = 'Non SPJ' THEN 1 ELSE 0 END) as non_spj,
			SUM(CASE WHEN jenis_arsip IS NULL OR jenis_arsip NOT IN ('SPJ','Non SPJ') THEN 1 ELSE 0 END) as unclassified
		FROM arsip
		WHERE deleted_at IS NULL
	`).Scan(&stats)

	var pemberkasanCount int64
	db.Model(&models.Pemberkasan{}).Count(&pemberkasanCount)

	return map[string]interface{}{
		"total":           stats.Total,
		"total_items":     stats.TotalItems,
		"aktif":           stats.Aktif,
		"inaktif":         stats.Inaktif,
		"siap_penyusutan": stats.SiapPenyusutan,
		"musnah":          stats.Musnah,
		"with_file":       stats.WithFile,
		"without_file":    stats.WithoutFile,
		"spj":             stats.Spj,
		"non_spj":         stats.NonSpj,
		"unclassified":    stats.Unclassified,
		"pemberkasan":     pemberkasanCount,
	}
}

// Wails runtime helper (unused but kept for future use)
var _ = wailsruntime.EventsEmit
