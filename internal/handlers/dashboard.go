package handlers

import (
	"time"

	"arsippro/internal/cache"
	"arsippro/internal/database"
	"arsippro/internal/models"
	"arsippro/internal/services"

	"github.com/gin-gonic/gin"
)


func Dashboard(c *gin.Context) {
	cacheKey := "dashboard:data"
	if cached, ok := cache.Default.Get(cacheKey); ok {
		Render(c, 200, "dashboard/index.html", cached.(gin.H))
		return
	}

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

	database.DB.Raw(`
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(jumlah), 0) as total_items,
			SUM(CASE WHEN status_arsip IN ('aktif','diberkaskan') THEN 1 ELSE 0 END) as aktif,
			SUM(CASE WHEN status_arsip = 'inaktif' THEN 1 ELSE 0 END) as inaktif,
			SUM(CASE WHEN status_arsip = 'siap_penyusutan' THEN 1 ELSE 0 END) as siap_penyusutan,
			SUM(CASE WHEN status_arsip = 'musnah' THEN 1 ELSE 0 END) as musnah,
			SUM(CASE WHEN file_path IS NOT NULL AND file_path != '' THEN 1 ELSE 0 END) as with_file,
			SUM(CASE WHEN file_path IS NULL OR file_path = '' THEN 1 ELSE 0 END) as without_file,
			SUM(CASE WHEN a.jenis_arsip = 'SPJ' THEN 1 ELSE 0 END) as spj,
			SUM(CASE WHEN a.jenis_arsip = 'Non SPJ' THEN 1 ELSE 0 END) as non_spj,
			SUM(CASE WHEN a.jenis_arsip IS NULL OR a.jenis_arsip NOT IN ('SPJ','Non SPJ') THEN 1 ELSE 0 END) as unclassified
		FROM arsip a
		WHERE a.deleted_at IS NULL
		  
	`).Scan(&stats)

	// Arsip per unit kerja
	type UnitStat struct {
		NamaUnit string `gorm:"column:nama_unit"`
		Count    int64  `gorm:"column:count"`
	}
	var arsipPerUnit []UnitStat
	database.DB.Raw(`
		SELECT uk.nama_unit, COUNT(a.id) as count
		FROM arsip a
		LEFT JOIN unit_kerja uk ON a.unit_kerja_id = uk.id
		WHERE a.deleted_at IS NULL
		  
		GROUP BY uk.nama_unit ORDER BY count DESC LIMIT 10
	`).Scan(&arsipPerUnit)

	// Lokasi distribution
	type LokasiStat struct {
		NamaLokasi     string `gorm:"column:nama_lokasi"`
		Count          int64  `gorm:"column:count"`
		IsRecordCenter bool   `gorm:"column:is_rc"`
	}
	var lokasiStats []LokasiStat
	database.DB.Raw(`
		SELECT la.nama_lokasi, COUNT(a.id) as count,
		       (la.nama_lokasi LIKE '%Record Center%') as is_rc
		FROM arsip a
		LEFT JOIN lokasi_arsips la ON a.lokasi_arsip_id = la.id
		WHERE a.deleted_at IS NULL
		  
		GROUP BY la.nama_lokasi ORDER BY count DESC
	`).Scan(&lokasiStats)

	var pemberkasanCount int64
	database.DB.Model(&models.Pemberkasan{}).Count(&pemberkasanCount)

	var blockchainCount int64
	database.DB.Table("blockchain_audits").Count(&blockchainCount)

	var recentActivities []struct {
		Action      string    `gorm:"column:action"`
		UserName    string    `gorm:"column:user_name"`
		CreatedAt   time.Time `gorm:"column:created_at"`
		Description string    `gorm:"column:description"`
	}
	database.DB.Raw(`
		SELECT al.action, u.name as user_name, al.created_at, al.description
		FROM activity_logs al
		LEFT JOIN users u ON al.user_id = u.id
		ORDER BY al.created_at DESC LIMIT 5
	`).Scan(&recentActivities)

	// Monthly stats for chart
	type MonthlyStat struct {
		Month string `gorm:"column:month"`
		Total int64  `gorm:"column:total"`
	}
	var monthlyStats []MonthlyStat
	database.DB.Raw(`
		SELECT DATE_FORMAT(created_at, '%Y-%m') as month, COUNT(*) as total
		FROM arsip
		WHERE deleted_at IS NULL
		  AND status_arsip != 'permanen'
		GROUP BY month ORDER BY month ASC LIMIT 12
	`).Scan(&monthlyStats)

	digitalPercent := 0.0
	if stats.Total > 0 {
		digitalPercent = float64(stats.WithFile) / float64(stats.Total) * 100
	}

	verifyResult := (&services.BlockchainAuditService{}).VerifyChain()
	blockchainIntegrity := gin.H{
		"IsValid":       verifyResult["is_valid"],
		"InvalidBlocks": verifyResult["invalid_blocks"],
	}

	digitalFileStats := gin.H{
		"WithDigitalFile":    stats.WithFile,
		"WithoutDigitalFile": stats.WithoutFile,
		"Percentage":         int64(digitalPercent),
	}

	forecastRaw := (&services.DataScienceService{}).ForecastGrowth(6)
	var forecastList []gin.H
	for _, f := range forecastRaw {
		month, _ := f["month"].(string)
		total, _ := f["total"].(int64)
		forecastList = append(forecastList, gin.H{
			"Month":          month,
			"PredictedCount": total,
		})
	}

	// ── BOX STATS (Inventaris Box RC) ─────────────────────────────────────────
	// Classify each Record Center box by SPJ/Non-SPJ composition
	type BoxClassification struct {
		NamaLokasi string `gorm:"column:nama_lokasi"`
		HasSpj     int    `gorm:"column:has_spj"`
		HasNonSpj  int    `gorm:"column:has_non_spj"`
	}
	var boxClassifications []BoxClassification
	database.DB.Raw(`
		SELECT la.nama_lokasi,
		       MAX(CASE WHEN a.jenis_arsip = 'SPJ' THEN 1 ELSE 0 END) as has_spj,
                       MAX(CASE WHEN a.jenis_arsip = 'Non SPJ' THEN 1 ELSE 0 END) as has_non_spj
		FROM arsip a
		INNER JOIN lokasi_arsips la ON a.lokasi_arsip_id = la.id
		WHERE a.deleted_at IS NULL
		  AND la.nama_lokasi LIKE 'Record Center%'
		GROUP BY la.nama_lokasi
		ORDER BY la.nama_lokasi ASC
	`).Scan(&boxClassifications)

	var listAll, listMurniSpj, listMurniNonSpj, listCampuran []string
	for _, bc := range boxClassifications {
		listAll = append(listAll, bc.NamaLokasi)
		if bc.HasSpj == 1 && bc.HasNonSpj == 1 {
			listCampuran = append(listCampuran, bc.NamaLokasi)
		} else if bc.HasSpj == 1 {
			listMurniSpj = append(listMurniSpj, bc.NamaLokasi)
		} else {
			listMurniNonSpj = append(listMurniNonSpj, bc.NamaLokasi)
		}
	}
	// Ensure non-nil slices for JSON/template rendering
	if listAll == nil {
		listAll = []string{}
	}
	if listMurniSpj == nil {
		listMurniSpj = []string{}
	}
	if listMurniNonSpj == nil {
		listMurniNonSpj = []string{}
	}
	if listCampuran == nil {
		listCampuran = []string{}
	}

	// Box-level digitalisasi: percentage of archives in RC boxes with digital files
	type RCDigitalStat struct {
		WithDigital    int64 `gorm:"column:with_digital"`
		WithoutDigital int64 `gorm:"column:without_digital"`
	}
	var rcDigitalStat RCDigitalStat
	database.DB.Raw(`
		SELECT
			SUM(CASE WHEN a.file_path IS NOT NULL AND a.file_path != '' THEN 1 ELSE 0 END) as with_digital,
			SUM(CASE WHEN a.file_path IS NULL OR a.file_path = '' THEN 1 ELSE 0 END) as without_digital
		FROM arsip a
		INNER JOIN lokasi_arsips la ON a.lokasi_arsip_id = la.id
		WHERE a.deleted_at IS NULL
		  AND la.nama_lokasi LIKE 'Record Center%'
	`).Scan(&rcDigitalStat)

	rcDigitalPercent := 0.0
	totalRC := rcDigitalStat.WithDigital + rcDigitalStat.WithoutDigital
	if totalRC > 0 {
		rcDigitalPercent = float64(rcDigitalStat.WithDigital) / float64(totalRC) * 100
	}

	data := gin.H{
		"title":               "Dashboard - SIMARC",
		"pageTitle":           "Dashboard",
		"Stats":               stats,
		"ArsipPerUnit":        arsipPerUnit,
		"PemberkasanCount":    pemberkasanCount,
		"LokasiStats":         lokasiStats,
		"BlockchainCount":     blockchainCount,
		"RecentActivities":    recentActivities,
		"MonthlyStats":        monthlyStats,
		"DigitalPercent":      digitalPercent,
		"BlockchainIntegrity": blockchainIntegrity,
		"DigitalFileStats":    digitalFileStats,
		"Intelligence": gin.H{
			"Forecast": forecastList,
		},
		"RefreshUrl": "/dashboard/refresh",
		"BoxStats": gin.H{
			"TotalRcBoxes":    len(listAll),
			"BoxMurniSpj":     len(listMurniSpj),
			"BoxMurniNonSpj":  len(listMurniNonSpj),
			"BoxCampuran":     len(listCampuran),
			"RcDigitalPercent": rcDigitalPercent,
			"RcWithDigital":   rcDigitalStat.WithDigital,
			"ListAll":         listAll,
			"ListMurniSpj":    listMurniSpj,
			"ListMurniNonSpj": listMurniNonSpj,
			"ListCampuran":    listCampuran,
		},
	}
	cache.Default.SetWithTTL(cacheKey, data, 30*time.Second)
	Render(c, 200, "dashboard/index.html", data)
}

func DashboardAPI(c *gin.Context) {
	cacheKey := "dashboard:api"
	if cached, ok := cache.Default.Get(cacheKey); ok {
		c.JSON(200, gin.H{"success": true, "data": cached})
		return
	}

	var stats struct {
		Total          int64 `json:"total"`
		TotalItems     int64 `json:"total_items"`
		Aktif          int64 `json:"aktif"`
		Inaktif        int64 `json:"inaktif"`
		SiapPenyusutan int64 `json:"siap_penyusutan"`
		Musnah         int64 `json:"musnah"`
		WithFile       int64 `json:"with_file"`
		WithoutFile    int64 `json:"without_file"`
		Spj            int64 `json:"spj"`
		NonSpj         int64 `json:"non_spj"`
		Unclassified   int64 `json:"unclassified"`
	}

	database.DB.Raw(`
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(jumlah), 0) as total_items,
			SUM(CASE WHEN status_arsip IN ('aktif','diberkaskan') THEN 1 ELSE 0 END) as aktif,
			SUM(CASE WHEN status_arsip = 'inaktif' THEN 1 ELSE 0 END) as inaktif,
			SUM(CASE WHEN status_arsip = 'siap_penyusutan' THEN 1 ELSE 0 END) as siap_penyusutan,
			SUM(CASE WHEN status_arsip = 'musnah' THEN 1 ELSE 0 END) as musnah,
			SUM(CASE WHEN file_path IS NOT NULL AND file_path != '' THEN 1 ELSE 0 END) as with_file,
			SUM(CASE WHEN file_path IS NULL OR file_path = '' THEN 1 ELSE 0 END) as without_file,
			SUM(CASE WHEN a.jenis_arsip = 'SPJ' THEN 1 ELSE 0 END) as spj,
			SUM(CASE WHEN a.jenis_arsip = 'Non SPJ' THEN 1 ELSE 0 END) as non_spj,
			SUM(CASE WHEN a.jenis_arsip IS NULL OR a.jenis_arsip NOT IN ('SPJ','Non SPJ') THEN 1 ELSE 0 END) as unclassified
		FROM arsip a
		WHERE a.deleted_at IS NULL
		  
	`).Scan(&stats)

	digitalPercent := 0.0
	if stats.Total > 0 {
		digitalPercent = float64(stats.WithFile) / float64(stats.Total) * 100
	}

	// Arsip per unit kerja
	type UnitStat struct {
		NamaUnit string `json:"nama_unit"`
		Count    int64  `json:"count"`
	}
	var arsipPerUnit []UnitStat
	database.DB.Raw(`
		SELECT uk.nama_unit, COUNT(a.id) as count
		FROM arsip a
		LEFT JOIN unit_kerja uk ON a.unit_kerja_id = uk.id
		WHERE a.deleted_at IS NULL
		  
		GROUP BY uk.nama_unit ORDER BY count DESC LIMIT 10
	`).Scan(&arsipPerUnit)

	var pemberkasanCount int64
	database.DB.Model(&models.Pemberkasan{}).Count(&pemberkasanCount)

	var blockchainCount int64
	database.DB.Table("blockchain_audits").Count(&blockchainCount)

	// Recent activities
	var recentActivities []struct {
		Action      string    `json:"action"`
		UserName    string    `json:"user_name"`
		CreatedAt   time.Time `json:"created_at"`
		Description string    `json:"description"`
	}
	database.DB.Raw(`
		SELECT al.action, u.name as user_name, al.created_at, al.description
		FROM activity_logs al
		LEFT JOIN users u ON al.user_id = u.id
		ORDER BY al.created_at DESC LIMIT 5
	`).Scan(&recentActivities)

	// Monthly stats for chart
	type MonthlyStat struct {
		Month string `json:"month"`
		Total int64  `json:"total"`
	}
	var monthlyStats []MonthlyStat
	database.DB.Raw(`
		SELECT DATE_FORMAT(created_at, '%Y-%m') as month, COUNT(*) as total
		FROM arsip
		WHERE deleted_at IS NULL
		  AND status_arsip != 'permanen'
		GROUP BY month ORDER BY month ASC LIMIT 12
	`).Scan(&monthlyStats)

	// Lokasi distribution
	type LokasiStat struct {
		NamaLokasi string `json:"nama_lokasi"`
		Count      int64  `json:"count"`
	}
	var lokasiStats []LokasiStat
	database.DB.Raw(`
		SELECT la.nama_lokasi, COUNT(a.id) as count
		FROM arsip a
		LEFT JOIN lokasi_arsips la ON a.lokasi_arsip_id = la.id
		WHERE a.deleted_at IS NULL
		  
		GROUP BY la.nama_lokasi ORDER BY count DESC
	`).Scan(&lokasiStats)

	data := gin.H{
		"stats":            stats,
		"arsipPerUnit":     arsipPerUnit,
		"pemberkasanCount": pemberkasanCount,
		"blockchainCount":  blockchainCount,
		"recentActivities": recentActivities,
		"monthlyStats":     monthlyStats,
		"lokasiStats":      lokasiStats,
		"digitalPercent":   int64(digitalPercent),
	}
	cache.Default.SetWithTTL(cacheKey, data, 30*time.Second)
	c.JSON(200, gin.H{"success": true, "data": data})
}
