package app

import (
	"net/http"

	"arsippro/internal/database"
	"arsippro/internal/handlers"
	"arsippro/internal/middleware"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
)

// registerRoutes mounts every application route on the given engine.
// Extracted from cmd/server/main.go so both the standalone server and
// Vercel serverless entry point share identical routing.
func registerRoutes(r *gin.Engine) {
	arsipH := &handlers.ArsipHandler{}
	kkH := &handlers.KodeKlasifikasiHandler{}
	ukH := &handlers.UnitKerjaHandler{}
	lokasiH := &handlers.LokasiArsipHandler{}
	jenisH := &handlers.JenisArsipHandler{}
	berkasH := &handlers.PemberkasanHandler{}
	pemusnahH := &handlers.PemusnahanHandler{}
	jadwalH := &handlers.JadwalRetensiHandler{}
	userH := &handlers.UserHandler{}
	roleH := &handlers.RoleHandler{}
	profilH := &handlers.ProfilHandler{}
	pengaturanH := &handlers.PengaturanHandler{}
	searchH := &handlers.SearchHandler{}
	peminjamanH := &handlers.PeminjamanHandler{}
	laporanH := &handlers.LaporanHandler{}
	qrH := &handlers.QrCodeHandler{}
	ocrH := &handlers.OcrHandler{}
	blockchainH := &handlers.BlockchainHandler{}
	backupH := &handlers.BackupHandler{}
	monitoringH := &handlers.MonitoringHandler{}

	// New handlers
	disposalH := &handlers.DisposalHandler{}
	advDashH := &handlers.AdvancedDashboardHandler{}
	integrationH := &handlers.IntegrationHandler{}
	importExportH := &handlers.ImportExportHandler{}
	laporanExpH := &handlers.LaporanExportHandler{}
	backupAdvH := &handlers.BackupAdvancedHandler{}

	blockchainAdvH := &handlers.BlockchainAdvancedHandler{}
	jadwalAdvH := &handlers.JadwalRetensiAdvancedHandler{}
	peminjamanAdvH := &handlers.PeminjamanAdvancedHandler{}
	settingsThemeH := &handlers.SettingsThemeHandler{}
	pengaturanAdvH := &handlers.PengaturanAdvancedHandler{}
	mobileAPIH := &handlers.MobileAPIHandler{}
	appsScriptH := &handlers.AppsScriptAPIHandler{}
	premiumAPIH := &handlers.PremiumAPIHandler{}
	ocrAdvH := &handlers.OcrAdvancedHandler{}
	supervisionH := &handlers.ArchivalSupervisionHandler{}

	// Root redirect
	r.GET("/", middleware.CSRF(), handlers.ShowLogin)

	// Health check
	r.GET("/health", handlers.HealthCheck)
	r.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// Database setup page (reachable without login when the DB is unreachable,
	// and used to repair the connection in recovery mode)
	r.GET("/database-setup", handlers.DatabaseSetupPage)

	// Public routes
	guest := r.Group("/")
	guest.Use(middleware.GuestOnly())
	guest.Use(middleware.CSRF())
	{
		guest.GET("/login", handlers.ShowLogin)
		guest.POST("/login", middleware.LoginRateLimit(), handlers.Login)
	}

	// Public arsip view (no auth)
	r.GET("/arsip/public/:id", arsipH.PublicView)

	// Auth routes
	auth := r.Group("/")
	auth.Use(middleware.Auth())
	{
		auth.POST("/logout", handlers.Logout)

		// ═══════════════════════════════════════════
		// DASHBOARD
		// ═══════════════════════════════════════════
		auth.GET("/dashboard", handlers.Dashboard)
		auth.POST("/dashboard/refresh", handlers.DashboardRefresh)
		auth.GET("/dashboard/api", handlers.DashboardAPI)
		auth.GET("/dashboard/premium", handlers.DashboardPremium)
		auth.GET("/dashboard/champion", handlers.DashboardChampion)

		// ═══════════════════════════════════════════
		// SEARCH
		// ═══════════════════════════════════════════
		auth.GET("/search", searchH.Results)
		auth.GET("/api/arsip/search/suggestions", arsipH.Suggestions)
		auth.POST("/arsip/export-search", arsipH.ExportSearch)

		// ═══════════════════════════════════════════
		// ARSIP (Manajemen Arsip)
		// ═══════════════════════════════════════════
		// Move location (before resource routes)
		auth.GET("/arsip/move-location", arsipH.ShowMoveLocationForm)
		auth.POST("/arsip/move-location", arsipH.MoveLocation)
		auth.GET("/arsip/pemindahan", arsipH.PemindahanIndex)
		auth.POST("/arsip/pemindahan", arsipH.PemindahanStore)
		auth.GET("/arsip/pemindahan/search", arsipH.PemindahanSearchJSON)
		auth.POST("/arsip/pemindahan/ba/generate", arsipH.GenerateBeritaAcara)
		auth.GET("/arsip/pemindahan/ba/:id/pdf", arsipH.BeritaAcaraPDF)
		auth.GET("/arsip/pemindahan/ba/:id", arsipH.BeritaAcaraDetail)
		auth.GET("/arsip/pemindahan/ba", arsipH.BeritaAcaraList)
		auth.DELETE("/arsip/pemindahan/ba/:id", arsipH.BeritaAcaraDelete)

		// AI routes (before resource routes)
		auth.POST("/arsip/ai/extract-metadata", arsipH.ExtractMetadataAI)
		auth.GET("/arsip/ai/semantic-search", arsipH.SemanticSearchAI)

		// Specific action routes (before resource routes)
		auth.GET("/arsip/:id/download", arsipH.Download)
		auth.GET("/arsip/:id/view", arsipH.View)
		auth.GET("/arsip/:id/check-file", arsipH.CheckFile)
		auth.GET("/arsip/:id/berkaskan", arsipH.ShowBerkaskanForm)
		auth.GET("/arsip/:id/berkaskan/json", arsipH.ShowBerkaskanFormJSON)
		auth.POST("/arsip/:id/berkaskan", arsipH.Berkaskan)
		auth.GET("/arsip/:id/versions", arsipH.Versions)
		auth.GET("/arsip/:id/qrcode", qrH.Generate)
		auth.GET("/arsip/:id/qrcode/download", qrH.DownloadByArsip)
		auth.GET("/arsip/:id/history", arsipH.History)
		auth.GET("/arsip/:id/history/json", arsipH.HistoryJSON)
		auth.GET("/arsip/:id/history/export-pdf", arsipH.HistoryPDF)
		auth.POST("/api/arsip/:id/keluarkan-dari-pemberkasan", arsipH.KeluarkanDariPemberkasan)

		// Version routes
		auth.GET("/arsip/version/:versionId/download", arsipH.DownloadVersion)
		auth.GET("/arsip/:id/versions/:versionId/download", arsipH.DownloadVersion)
		auth.GET("/arsip/version/:versionId/check-file", arsipH.CheckVersionFile)

		// Create supports POST for OCR prefill
		auth.Match([]string{"GET", "POST"}, "/arsip/create", arsipH.Create)

		// Bulk berkaskan (must be before :id routes)
		auth.POST("/arsip/bulk-berkaskan", arsipH.BulkBerkaskan)

		// CRUD routes
		auth.GET("/arsip", arsipH.Index)
		auth.POST("/arsip", arsipH.Store)
		auth.GET("/arsip/:id", arsipH.Show)
		auth.GET("/arsip/:id/edit", arsipH.Edit)
		auth.POST("/arsip/:id", arsipH.Update)
		auth.PUT("/arsip/:id", arsipH.Update)
		auth.DELETE("/arsip/:id", arsipH.Destroy)
		auth.POST("/arsip/:id/delete", arsipH.Destroy)

		// Import
		auth.GET("/arsip/import", arsipH.ShowImportForm)
		auth.POST("/arsip/import", arsipH.ImportExcel)
		auth.GET("/arsip/download-template", arsipH.DownloadTemplate)

		// ═══════════════════════════════════════════
		// KODE KLASIFIKASI (Master Data)
		// ═══════════════════════════════════════════
		auth.GET("/kode-klasifikasi", kkH.Index)
		auth.GET("/kode-klasifikasi/create", kkH.Create)
		auth.POST("/kode-klasifikasi", kkH.Store)
		auth.GET("/kode-klasifikasi/:id", kkH.Show)
		auth.GET("/kode-klasifikasi/:id/edit", kkH.Edit)
		auth.PUT("/kode-klasifikasi/:id", kkH.Update)
		auth.POST("/kode-klasifikasi/:id", kkH.Update)
		auth.DELETE("/kode-klasifikasi/:id", kkH.Destroy)
		auth.POST("/kode-klasifikasi/:id/delete", kkH.Destroy)
		auth.GET("/kode-klasifikasi/export", handlers.ExportKodeKlasifikasi)

		// ═══════════════════════════════════════════
		// JENIS ARSIP (Master Data)
		// ═══════════════════════════════════════════
		auth.GET("/jenis-arsip", jenisH.Index)
		auth.GET("/jenis-arsip/create", jenisH.Create)
		auth.POST("/jenis-arsip", jenisH.Store)
		auth.GET("/jenis-arsip/:id", jenisH.Show)
		auth.GET("/jenis-arsip/:id/edit", jenisH.Edit)
		auth.PUT("/jenis-arsip/:id", jenisH.Update)
		auth.POST("/jenis-arsip/:id", jenisH.Update)
		auth.DELETE("/jenis-arsip/:id", jenisH.Destroy)
		auth.POST("/jenis-arsip/:id/delete", jenisH.Destroy)
		auth.GET("/jenis-arsip/export", handlers.ExportJenisArsip)

		// ═══════════════════════════════════════════
		// PEMBERKASAN (Manajemen Arsip)
		// ═══════════════════════════════════════════
		auth.GET("/pemberkasan", berkasH.Index)
		auth.GET("/pemberkasan/create", berkasH.Create)
		auth.POST("/pemberkasan", berkasH.Store)
		auth.GET("/pemberkasan/:id", berkasH.Show)
		auth.GET("/pemberkasan/:id/edit", berkasH.Edit)
		auth.PUT("/pemberkasan/:id", berkasH.Update)
		auth.POST("/pemberkasan/:id", berkasH.Update)
		auth.DELETE("/pemberkasan/:id", berkasH.Destroy)
		auth.POST("/pemberkasan/:id/delete", berkasH.Destroy)
		auth.GET("/pemberkasan/export", handlers.ExportPemberkasan)
		auth.PUT("/pemberkasan/:id/close", berkasH.Close)
		auth.POST("/pemberkasan/:id/close", berkasH.Close)
		auth.GET("/pemberkasan/:id/isi", berkasH.ShowIsi)

		// ═══════════════════════════════════════════
		// PEMUSNAHAN ARSIP (Manajemen Arsip)
		// ═══════════════════════════════════════════
		auth.GET("/pemusnahan/export-excel", pemusnahH.ExportExcel)
		auth.GET("/pemusnahan/export-pdf", pemusnahH.ExportPDF)
		auth.GET("/pemusnahan/ajax/search-arsip", pemusnahH.SearchArsip)
		auth.GET("/pemusnahan/ajax/arsip/:id", pemusnahH.GetArsipDetail)
		auth.GET("/pemusnahan", pemusnahH.Index)
		auth.GET("/pemusnahan/create", pemusnahH.Create)
		auth.POST("/pemusnahan", pemusnahH.Store)
		auth.POST("/pemusnahan/auto-create", pemusnahH.AutoCreate)
		auth.GET("/pemusnahan/:id", pemusnahH.Show)
		auth.PUT("/pemusnahan/:id/approve", pemusnahH.Approve)
		auth.POST("/pemusnahan/:id/approve", pemusnahH.Approve)
		auth.PUT("/pemusnahan/:id/reject", pemusnahH.Reject)
		auth.POST("/pemusnahan/:id/reject", pemusnahH.Reject)

		// ═══════════════════════════════════════════
		// UNIT KERJA (Master Data)
		// ═══════════════════════════════════════════
		auth.GET("/unit-kerja", ukH.Index)
		auth.GET("/unit-kerja/create", ukH.Create)
		auth.POST("/unit-kerja", ukH.Store)
		auth.GET("/unit-kerja/:id", ukH.Show)
		auth.GET("/unit-kerja/:id/edit", ukH.Edit)
		auth.PUT("/unit-kerja/:id", ukH.Update)
		auth.POST("/unit-kerja/:id", ukH.Update)
		auth.DELETE("/unit-kerja/:id", ukH.Destroy)
		auth.POST("/unit-kerja/:id/delete", ukH.Destroy)

		// ═══════════════════════════════════════════
		// LOKASI ARSIP (Master Data)
		// ═══════════════════════════════════════════
		auth.GET("/lokasi-arsip", lokasiH.Index)
		auth.GET("/lokasi-arsip/create", lokasiH.Create)
		auth.POST("/lokasi-arsip", lokasiH.Store)
		auth.GET("/lokasi-arsip/:id", lokasiH.Show)
		auth.GET("/lokasi-arsip/:id/edit", lokasiH.Edit)
		auth.PUT("/lokasi-arsip/:id", lokasiH.Update)
		auth.POST("/lokasi-arsip/:id", lokasiH.Update)
		auth.DELETE("/lokasi-arsip/:id", lokasiH.Destroy)
		auth.POST("/lokasi-arsip/:id/delete", lokasiH.Destroy)

		// ═══════════════════════════════════════════
		// USERS (Master Data)
		// ═══════════════════════════════════════════
		auth.GET("/users", userH.Index)
		auth.GET("/users/create", userH.Create)
		auth.POST("/users", userH.Store)
		auth.GET("/users/:id", userH.Show)
		auth.GET("/users/:id/edit", userH.Edit)
		auth.PUT("/users/:id", userH.Update)
		auth.POST("/users/:id", userH.Update)
		auth.DELETE("/users/:id", userH.Destroy)
		auth.POST("/users/:id/delete", userH.Destroy)

		// ═══════════════════════════════════════════
		// PROFIL
		// ═══════════════════════════════════════════
		auth.GET("/profil", profilH.Index)
		auth.GET("/profil/edit", profilH.Index)
		auth.POST("/profil/update", profilH.Update)
		auth.PUT("/profil/update", profilH.Update)
		auth.GET("/profil/password", profilH.EditPassword)
		auth.PUT("/profil/password", profilH.UpdatePassword)
		auth.POST("/profil/password", profilH.UpdatePassword)

		// ═══════════════════════════════════════════
		// PENGATURAN
		// ═══════════════════════════════════════════
		auth.GET("/pengaturan", pengaturanH.Index)
		auth.POST("/pengaturan/update", pengaturanAdvH.Update)
		auth.POST("/pengaturan/clear-cache", pengaturanAdvH.ClearCache)
		auth.GET("/pengaturan/system", pengaturanAdvH.SystemInfo)

		// ═══════════════════════════════════════════
		// ROLES
		// ═══════════════════════════════════════════
		auth.GET("/roles", roleH.Index)
		auth.GET("/roles/create", roleH.Create)
		auth.POST("/roles", roleH.Store)
		auth.GET("/roles/:id", roleH.Show)
		auth.GET("/roles/:id/edit", roleH.Edit)
		auth.PUT("/roles/:id", roleH.Update)
		auth.POST("/roles/:id", roleH.Update)
		auth.DELETE("/roles/:id", roleH.Destroy)
		auth.POST("/roles/:id/delete", roleH.Destroy)
		auth.GET("/roles/:id/permissions", roleH.EditPermissions)
		auth.POST("/roles/:id/permissions", roleH.UpdatePermissions)
		auth.POST("/roles/:id/permissions/update", roleH.UpdatePermissions)

		// ═══════════════════════════════════════════
		// LAPORAN (Pencarian & Laporan)
		// ═══════════════════════════════════════════
		auth.GET("/laporan", laporanH.Index)
		auth.GET("/laporan/arsip", laporanH.Arsip)
		auth.GET("/laporan/arsip/export-pdf", laporanExpH.ArsipPDF)
		auth.GET("/laporan/arsip/export/pdf", laporanExpH.ArsipPDF)
		auth.GET("/laporan/arsip/export-excel", laporanExpH.ArsipExcel)
		auth.GET("/laporan/arsip/export/excel", laporanExpH.ArsipExcel)
		auth.GET("/laporan/digital", laporanExpH.Digital)
		auth.GET("/laporan/digital/export-pdf", laporanExpH.DigitalPDF)
		auth.GET("/laporan/digital/export/pdf", laporanExpH.DigitalPDF)
		auth.GET("/laporan/digital/export-excel", laporanExpH.DigitalExcel)
		auth.GET("/laporan/digital/export/excel", laporanExpH.DigitalExcel)
		auth.GET("/laporan/pemberkasan", laporanExpH.Pemberkasan)
		auth.GET("/laporan/pemberkasan/export-pdf", laporanExpH.PemberkasanPDF)
		auth.GET("/laporan/pemberkasan/export/pdf", laporanExpH.PemberkasanPDF)
		auth.GET("/laporan/pemberkasan/export-excel", laporanExpH.PemberkasanExcel)
		auth.GET("/laporan/pemberkasan/export/excel", laporanExpH.PemberkasanExcel)
		auth.GET("/laporan/retensi", laporanH.Retensi)
		auth.GET("/laporan/retensi/export-pdf", laporanExpH.RetensiPDF)
		auth.GET("/laporan/retensi/export/pdf", laporanExpH.RetensiPDF)
		auth.GET("/laporan/retensi/export-excel", laporanExpH.RetensiExcel)
		auth.GET("/laporan/retensi/export/excel", laporanExpH.RetensiExcel)
		auth.GET("/laporan/pemusnahan", laporanH.Pemusnahan)
		auth.GET("/laporan/pemusnahan/export-pdf", laporanExpH.PemusnahanPDF)
		auth.GET("/laporan/pemusnahan/export/pdf", laporanExpH.PemusnahanPDF)
		auth.GET("/laporan/pemusnahan/export-excel", laporanExpH.PemusnahanExcel)
		auth.GET("/laporan/pemusnahan/export/excel", laporanExpH.PemusnahanExcel)
		auth.GET("/laporan/aktivitas", func(c *gin.Context) {
			var list []models.ActivityLog
			database.DB.Preload("User").Order("created_at DESC").Limit(100).Find(&list)
			handlers.Render(c, 200, "laporan/aktivitas.html", gin.H{"title": "Laporan Aktivitas", "pageTitle": "Laporan Aktivitas", "list": list})
		})
		auth.GET("/laporan/statistik", laporanExpH.Statistik)
		auth.GET("/laporan/statistik/export-pdf", laporanExpH.StatistikPDF)
		auth.GET("/laporan/statistik/export-excel", laporanExpH.StatistikExcel)
		auth.GET("/laporan/klasifikasi", laporanExpH.KlasifikasiDetail)

		// Laporan per Lokasi
		auth.GET("/laporan/lokasi", laporanExpH.LokasiIndex)
		auth.GET("/laporan/lokasi/filter", laporanExpH.LokasiFilter)
		auth.GET("/laporan/lokasi/export-pdf", laporanExpH.LokasiPDF)
		auth.GET("/laporan/lokasi/export-excel", laporanExpH.LokasiExcel)
		auth.GET("/laporan/lokasi/statistik", laporanExpH.LokasiStatistik)

		// ═══════════════════════════════════════════
		// MONITORING
		// ═══════════════════════════════════════════
		auth.GET("/monitoring/retensi", monitoringH.Retensi)

		// ═══════════════════════════════════════════
		// JADWAL RETENSI (Auto-Schedule)
		// ═══════════════════════════════════════════
		auth.GET("/jadwal-retensi/calendar", jadwalAdvH.Calendar)
		auth.POST("/jadwal-retensi/auto-create", jadwalAdvH.AutoCreate)
		auth.GET("/jadwal-retensi/search-arsip", jadwalAdvH.SearchArsip)
		auth.POST("/jadwal-retensi/:id/schedule", jadwalAdvH.Schedule)
		auth.POST("/jadwal-retensi/:id/start", jadwalAdvH.StartExecution)
		auth.POST("/jadwal-retensi/:id/execute", jadwalAdvH.ExecuteDisposal)
		auth.POST("/jadwal-retensi/cancel/:id", jadwalAdvH.Cancel)
		auth.POST("/jadwal-retensi/process/:jadwalArsipId", jadwalAdvH.ProcessArchive)
		auth.GET("/jadwal-retensi/create", jadwalAdvH.Create)
		auth.GET("/jadwal-retensi/:id", jadwalAdvH.Show)
		auth.GET("/jadwal-retensi/:id/edit", jadwalAdvH.Edit)
		auth.GET("/jadwal-retensi", jadwalH.Index)
		auth.POST("/jadwal-retensi", jadwalH.Store)
		auth.PUT("/jadwal-retensi/:id", jadwalH.Update)
		auth.POST("/jadwal-retensi/:id", jadwalH.Update)
		auth.DELETE("/jadwal-retensi/:id", jadwalH.Destroy)
		auth.POST("/jadwal-retensi/:id/delete", jadwalH.Destroy)
		auth.GET("/jadwal-retensi/export", handlers.ExportJadwalRetensi)

		// ═══════════════════════════════════════════
		// SMART DISPOSAL
		// ═══════════════════════════════════════════
		auth.GET("/disposal", disposalH.Index)
		auth.GET("/disposal/classification/:kodeKlasifikasi", disposalH.ShowByClassification)
		auth.GET("/disposal/classification/:kodeKlasifikasi/archives", disposalH.ListArchivesForDisposal)
		auth.POST("/disposal/classification/:kodeKlasifikasi/schedule", disposalH.CreateSchedule)
		auth.GET("/disposal/schedules", disposalH.Schedules)
		auth.GET("/disposal/schedules/:id", disposalH.ShowSchedule)
		auth.POST("/disposal/auto-create-schedules", disposalH.AutoCreateSchedules)
		auth.GET("/disposal/preview", disposalH.PreviewRecommendations)

		// ═══════════════════════════════════════════
		// SETTINGS (Theme)
		// ═══════════════════════════════════════════
		auth.GET("/settings", settingsThemeH.Index)
		auth.PUT("/settings/theme", settingsThemeH.UpdateTheme)
		auth.POST("/settings/theme", settingsThemeH.UpdateTheme)
		auth.GET("/settings/theme/reset", settingsThemeH.ResetTheme)

		// ═══════════════════════════════════════════
		// BACKUP
		// ═══════════════════════════════════════════
		auth.GET("/backup", backupH.Index)
		auth.POST("/backup/create", backupH.Create)
		auth.POST("/backup/restore", backupAdvH.Restore)
		auth.POST("/backup/import-sql", backupAdvH.ImportSQL)
		auth.POST("/backup/cleanup", backupAdvH.Cleanup)

		auth.GET("/backup/download", backupH.Download)
		auth.GET("/backup/download/:filename", backupH.Download)
		auth.DELETE("/backup/:filename", backupH.Delete)
		auth.POST("/backup/:filename/delete", backupH.Delete)
		auth.POST("/backup/delete", backupH.Delete)

		// Google Drive backup (client-side upload via Drive REST API)
		auth.POST("/backup/gdrive/settings", backupH.SaveGDriveSettings)
		auth.POST("/backup/gdrive/attach", backupH.SaveGDriveFile)

		// ═══════════════════════════════════════════
		// BLOCKCHAIN
		// ═══════════════════════════════════════════
		auth.GET("/blockchain", blockchainH.Index)
		auth.GET("/blockchain/export", blockchainAdvH.Export)
		auth.GET("/blockchain/entity/:entityType/:entityId", blockchainAdvH.EntityAudit)
		auth.POST("/blockchain/verify", blockchainAdvH.Verify)
		auth.GET("/blockchain/block/search", blockchainAdvH.SearchByHash)
		auth.GET("/blockchain/:blockNumber", blockchainAdvH.Show)

		// ═══════════════════════════════════════════
		// PEMINJAMAN
		// ═══════════════════════════════════════════
		auth.GET("/peminjaman", peminjamanH.Index)
		auth.GET("/peminjaman/create", peminjamanAdvH.Create)
		auth.POST("/peminjaman/store", peminjamanH.Store)
		auth.POST("/peminjaman/:id/approve", peminjamanH.Approve)
		auth.POST("/peminjaman/:id/reject", peminjamanAdvH.Reject)
		auth.POST("/peminjaman/:id/return", peminjamanH.Return)

		// ═══════════════════════════════════════════
		// OCR
		// ═══════════════════════════════════════════
		auth.GET("/ocr", ocrH.Index)
		auth.POST("/ocr/process", ocrH.Process)
		auth.POST("/ocr/download", ocrAdvH.Download)
		auth.GET("/ocr/status", ocrAdvH.Status)

		// ═══════════════════════════════════════════
		// ARCHIVAL SUPERVISION
		// ═══════════════════════════════════════════
		auth.GET("/supervision/certificate/:id", supervisionH.DownloadCertificate)
	}

	// Database settings API — reachable without login so the app can be
	// repaired from /database-setup when the database is unreachable.
	dbSetup := r.Group("/pengaturan/database")
	dbSetup.Use(middleware.InjectUser())
	{
		dbSetup.POST("/test", pengaturanAdvH.DatabaseTest)
		dbSetup.POST("/save", pengaturanAdvH.DatabaseSave)
	}

	// ═══════════════════════════════════════════════
	// ADVANCED ROUTES (Tier A, B, C)
	// ═══════════════════════════════════════════════
	advanced := r.Group("/advanced")
	advanced.Use(middleware.Auth())
	{
		// Advanced Dashboard
		advanced.GET("/dashboard", advDashH.Index)
		advanced.GET("/dashboard/widget/:widgetKey", advDashH.GetWidgetData)
		advanced.POST("/dashboard/widgets", advDashH.SaveWidgetConfig)

		// Integration Hub
		advanced.GET("/integrations", integrationH.Index)
		advanced.GET("/integrations/create", integrationH.Create)
		advanced.POST("/integrations/store", integrationH.Store)
		advanced.GET("/integrations/:id", integrationH.Show)
		advanced.GET("/integrations/:id/edit", integrationH.Edit)
		advanced.PUT("/integrations/:id", integrationH.Update)
		advanced.POST("/integrations/:id", integrationH.Update)
		advanced.DELETE("/integrations/:id", integrationH.Destroy)
		advanced.POST("/integrations/:id/delete", integrationH.Destroy)
		advanced.POST("/integrations/:id/test", integrationH.Test)
		advanced.POST("/integrations/:id/sync", integrationH.Sync)
		advanced.GET("/integrations/log/:logId", integrationH.ShowLog)
		advanced.GET("/integrations/:id/status", integrationH.Status)

		// Import/Export
		advanced.GET("/import-export", importExportH.Index)
		advanced.GET("/import-export/import", importExportH.ShowImportForm)
		advanced.POST("/import-export/import", importExportH.ProcessImport)
		advanced.GET("/import-export/export", importExportH.ShowExportForm)
		advanced.POST("/import-export/export", importExportH.ProcessExport)
		advanced.GET("/import-export/template/:type", importExportH.DownloadTemplate)
		advanced.GET("/import-export/job/:jobId", importExportH.ShowJob)
		advanced.GET("/import-export/job/:jobId/download", importExportH.DownloadResult)
		advanced.GET("/import-export/job/:jobId/progress", importExportH.Progress)
		advanced.POST("/import-export/job/:jobId/retry", importExportH.Retry)

		// QR Code (advanced)
		advanced.GET("/qrcode", qrH.Index)
		advanced.GET("/qrcode/:qrCodeId", qrH.Show)
		advanced.GET("/qrcode/:qrCodeId/download", qrH.Download)
		advanced.POST("/qrcode/bulk-generate", qrH.BulkGenerate)
		advanced.POST("/qrcode/scan", qrH.ScanAPI)
		advanced.POST("/qrcode/:qrCodeId/deactivate", qrH.Deactivate)
		advanced.GET("/qrcode/arsip/:arsipId/location", qrH.CheckLocation)
		advanced.GET("/qrcode/location", qrH.GetByLocation)
		advanced.GET("/qrcode/scanner", qrH.Scanner)
		advanced.POST("/qrcode/generate/:arsipId", qrH.Generate)

		// Retention (advanced)
		advanced.GET("/retention", jadwalH.Index)
		advanced.GET("/retention/create", jadwalAdvH.Create)
		advanced.GET("/retention/:id", jadwalAdvH.Show)
		advanced.GET("/retention/:id/edit", jadwalAdvH.Edit)
		advanced.POST("/retention/store", jadwalAdvH.AdvancedStore)
		advanced.PUT("/retention/:id", jadwalAdvH.AdvancedUpdate)
		advanced.POST("/retention/:id/initiate-destruction", jadwalAdvH.InitiateDestruction)
		advanced.POST("/retention/:id/execute-destruction", jadwalAdvH.ExecuteDestruction)
		advanced.POST("/retention/:id/cancel", jadwalAdvH.CancelRetention)
		advanced.POST("/retention/approval/:approvalId", jadwalAdvH.ApproveRetention)
		advanced.POST("/retention/daily-check", jadwalAdvH.DailyRetentionCheck)
	}

	// Mobile public routes (no auth required for offline/share)
	r.GET("/mobile/offline", func(c *gin.Context) {
		handlers.Render(c, 200, "mobile/offline.html", gin.H{"title": "Offline"})
	})
	r.POST("/mobile/share", func(c *gin.Context) {
		title := c.PostForm("title")
		text := c.PostForm("text")
		url := c.PostForm("url")
		handlers.Render(c, 200, "mobile/share.html", gin.H{"title": title, "text": text, "url": url})
	})

	// ═══════════════════════════════════════════════
	// MOBILE ROUTES
	// ═══════════════════════════════════════════════
	mobile := r.Group("/mobile")
	mobile.Use(middleware.Auth())
	{
		mobile.GET("/", func(c *gin.Context) {
			var list []models.Arsip
			database.DB.Preload("KodeKlasifikasi").Order("created_at DESC").Limit(20).Find(&list)
			handlers.Render(c, 200, "mobile/index.html", gin.H{"arsipList": list})
		})
		mobile.GET("/archives", func(c *gin.Context) {
			var list []models.Arsip
			database.DB.Preload("KodeKlasifikasi").Order("created_at DESC").Limit(50).Find(&list)
			handlers.Render(c, 200, "mobile/archives.html", gin.H{"arsipList": list})
		})
		mobile.GET("/archives/:id", func(c *gin.Context) {
			var arsip models.Arsip
			database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").First(&arsip, "id = ?", c.Param("id"))
			handlers.Render(c, 200, "mobile/show.html", gin.H{"arsip": arsip})
		})
		mobile.GET("/create", mobileAPIH.Create)
		mobile.POST("/store", mobileAPIH.Store)
		mobile.GET("/search", func(c *gin.Context) {
			handlers.Render(c, 200, "mobile/search.html", gin.H{})
		})
		mobile.GET("/scan", func(c *gin.Context) {
			handlers.Render(c, 200, "mobile/scan.html", gin.H{})
		})
		mobile.GET("/settings", func(c *gin.Context) {
			handlers.Render(c, 200, "mobile/settings.html", gin.H{})
		})
		mobile.POST("/settings/update", mobileAPIH.UpdateSettings)
		// Mobile API
		mobile.POST("/api/scan", mobileAPIH.ScanQR)
		mobile.POST("/api/search", mobileAPIH.SearchAPI)
		mobile.GET("/api/search", mobileAPIH.SearchAPI)
		mobile.GET("/api/archive/:id", mobileAPIH.ArchiveAPI)
		mobile.GET("/api/offline", mobileAPIH.OfflineData)
	}

	// ═══════════════════════════════════════════════
	// GOOGLE APPS SCRIPT API (Public, read-only)
	// ═══════════════════════════════════════════════
	appsScript := r.Group("/api/apps-script")
	{
		appsScript.GET("/summary", appsScriptH.GetArchiveSummary)
		appsScript.GET("/classifications", appsScriptH.GetClassificationDistribution)
		appsScript.GET("/classifications/:kode/archives", appsScriptH.GetArchivesByClassification)
		appsScript.GET("/statistics/top", appsScriptH.GetTopStatistics)
		appsScript.GET("/search", appsScriptH.SearchArchives)
		appsScript.GET("/classifications/list", appsScriptH.GetAllClassificationCodes)
	}

	// ═══════════════════════════════════════════════
	// API ROUTES (Auth required)
	// ═══════════════════════════════════════════════
	api := r.Group("/api")
	api.Use(middleware.Auth())
	{
		api.GET("/arsip", arsipH.Index)
		api.POST("/arsip", arsipH.Store)
		api.GET("/arsip/:id", arsipH.Show)
		api.PUT("/arsip/:id", arsipH.Update)
		api.DELETE("/arsip/:id", arsipH.Destroy)
		api.GET("/dashboard/stats", handlers.DashboardAPI)
		api.GET("/search", searchH.Results)
		api.GET("/arsip/search", searchH.Results)
	}

	// ═══════════════════════════════════════════════
	// PREMIUM API V1 (Auth required)
	// ═══════════════════════════════════════════════
	apiv1 := r.Group("/api/v1")
	apiv1.Use(middleware.Auth())
	{
		apiv1.POST("/search", premiumAPIH.SmartSearch)
		apiv1.GET("/analytics", premiumAPIH.Analytics)
		apiv1.GET("/blockchain/verify", premiumAPIH.VerifyBlockchain)
		apiv1.GET("/blockchain/audit/:entityType/:entityId", premiumAPIH.AuditTrail)
	}

	// 404 handler
	r.NoRoute(func(c *gin.Context) {
		handlers.Render(c, http.StatusNotFound, "errors/404.html", gin.H{"title": "Halaman Tidak Ditemukan"})
	})
}
