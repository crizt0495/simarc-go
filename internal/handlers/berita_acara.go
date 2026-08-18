package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"strconv"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/middleware"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
)

func (h *ArsipHandler) BeritaAcaraList(c *gin.Context) {
	limit := 20

	var total int64
	database.DB.Model(&models.BeritaAcara{}).Count(&total)

	pages := (int(total) + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}

	// Validasi & clamp halaman agar tidak menghasilkan offset negatif / kosong
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	offset := (page - 1) * limit

	var list []models.BeritaAcara
	database.DB.Preload("Creator").Preload("LokasiAsal").Preload("LokasiTujuan").
		Order("created_at DESC").Limit(limit).Offset(offset).Find(&list)

	Render(c, 200, "arsip/berita_acara_list.html", gin.H{
		"title":     "Berita Acara Pemindahan - SIMARC",
		"pageTitle": "Berita Acara Pemindahan",
		"List":      list,
		"Total":     total,
		"Page":      page,
		"Pages":     pages,
		"Limit":     limit,
	})
}

func (h *ArsipHandler) BeritaAcaraDetail(c *gin.Context) {
	var ba models.BeritaAcara
	if err := database.DB.Preload("Items.Arsip.KodeKlasifikasi").
		Preload("Items.Arsip.LokasiArsip").
		Preload("Items.Arsip.UnitKerja").
		Preload("LokasiAsal").Preload("LokasiTujuan").
		Preload("Creator").
		First(&ba, "id = ?", c.Param("id")).Error; err != nil {
		Render404(c)
		return
	}
	Render(c, 200, "arsip/berita_acara_detail.html", gin.H{
		"title":   "Detail Berita Acara - SIMARC",
		"BA":      ba,
	})
}

func (h *ArsipHandler) BeritaAcaraDelete(c *gin.Context) {
	id := c.Param("id")
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Sesi habis, silakan login ulang"})
		return
	}

	// Cek apakah BA exists
	var ba models.BeritaAcara
	if err := database.DB.First(&ba, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Berita acara tidak ditemukan"})
		return
	}

	tx := database.DB.Begin()
	rollback := true
	defer func() {
		if rollback {
			tx.Rollback()
		}
	}()

	if err := tx.Where("berita_acara_id = ?", id).Delete(&models.BeritaAcaraItem{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menghapus item: " + err.Error()})
		return
	}
	if err := tx.Delete(&models.BeritaAcara{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menghapus BA: " + err.Error()})
		return
	}

	rollback = false
	tx.Commit()

	logActivity(user.ID, "delete_ba", "Menghapus Berita Acara "+ba.NomorBA, "berita_acara", id, c.ClientIP(), c.GetHeader("User-Agent"))

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Berita acara berhasil dihapus"})
}

func (h *ArsipHandler) GenerateBeritaAcara(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	arsipIDs := c.PostFormArray("arsip_ids[]")
	if len(arsipIDs) == 0 {
		jsonStr := c.PostForm("arsip_data")
		if jsonStr != "" {
			if err := json.Unmarshal([]byte(jsonStr), &arsipIDs); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Data arsip tidak valid"})
				return
			}
		}
	}
	lokasiAsalID := c.PostForm("lokasi_asal_id")
	lokasiTujuanID := c.PostForm("lokasi_tujuan_id")
	nomorBA := strings.TrimSpace(c.PostForm("nomor_ba"))
	tanggalStr := c.PostForm("tanggal")
	tempat := c.PostForm("tempat")
	pihak1Nama := c.PostForm("pihak1_nama")
	pihak1NIP := c.PostForm("pihak1_nip")
	pihak1Jabatan := c.PostForm("pihak1_jabatan")
	pihak1Unit := c.PostForm("pihak1_unit")
	pihak2Nama := c.PostForm("pihak2_nama")
	pihak2NIP := c.PostForm("pihak2_nip")
	pihak2Jabatan := c.PostForm("pihak2_jabatan")
	pihak2Unit := c.PostForm("pihak2_unit")
	catatan := c.PostForm("catatan")

	if len(arsipIDs) == 0 || pihak1Nama == "" || pihak2Nama == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Arsip, Pihak Pertama, dan Pihak Kedua harus diisi"})
		return
	}
	if tanggalStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Tanggal harus diisi"})
		return
	}
	if nomorBA == "" {
		nomorBA = fmt.Sprintf("BA/%s/%s", time.Now().Format("20060102"), uuid.New().String()[:8])
	}

	// Validasi lokasi asal & tujuan jika diisi
	if lokasiAsalID != "" {
		var asal models.LokasiArsip
		if err := database.DB.First(&asal, "id = ?", lokasiAsalID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Lokasi asal tidak ditemukan"})
			return
		}
	}
	if lokasiTujuanID != "" {
		var tujuan models.LokasiArsip
		if err := database.DB.First(&tujuan, "id = ?", lokasiTujuanID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Lokasi tujuan tidak ditemukan"})
			return
		}
	}

	var tanggal time.Time
	var err error
	if tanggalStr != "" {
		tanggal, err = time.Parse("2006-01-02", tanggalStr)
		if err != nil {
			tanggal = time.Now()
		}
	} else {
		tanggal = time.Now()
	}

	var arsipList []models.Arsip
	database.DB.Preload("LokasiArsip").Preload("KodeKlasifikasi").Preload("UnitKerja").
		Where("id IN ?", arsipIDs).Find(&arsipList)

	if len(arsipList) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Tidak ada arsip ditemukan untuk ID yang diberikan"})
		return
	}

	ba := models.BeritaAcara{
		ID:                uuid.New().String(),
		NomorBA:           nomorBA,
		Tanggal:           tanggal,
		Tempat:            tempat,
		PihakPertamaNama:  pihak1Nama,
		PihakPertamaNIP:   pihak1NIP,
		PihakPertamaJabatan: pihak1Jabatan,
		PihakPertamaUnit:  pihak1Unit,
		PihakKeduaNama:    pihak2Nama,
		PihakKeduaNIP:     pihak2NIP,
		PihakKeduaJabatan: pihak2Jabatan,
		PihakKeduaUnit:    pihak2Unit,
		LokasiAsalID:      &lokasiAsalID,
		LokasiTujuanID:    &lokasiTujuanID,
		Catatan:           catatan,
		CreatedBy:         user.ID,
	}

	if lokasiAsalID == "" {
		ba.LokasiAsalID = nil
	}
	if lokasiTujuanID == "" {
		ba.LokasiTujuanID = nil
	}

	tx := database.DB.Begin()
	if err := tx.Create(&ba).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menyimpan berita acara: " + err.Error()})
		return
	}

	for _, a := range arsipList {
		item := models.BeritaAcaraItem{
			BeritaAcaraID: ba.ID,
			ArsipID:       a.ID,
		}
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal menyimpan item berita acara"})
			return
		}
	}
	tx.Commit()

	logActivity(user.ID, "generate_ba",
		fmt.Sprintf("Membuat Berita Acara %s untuk %d arsip", nomorBA, len(arsipList)),
		"berita_acara", ba.ID, c.ClientIP(), c.GetHeader("User-Agent"))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Berita Acara %s berhasil dibuat", nomorBA),
		"data": gin.H{
			"id":       ba.ID,
			"nomor_ba": nomorBA,
		},
	})
}

func (h *ArsipHandler) BeritaAcaraPDF(c *gin.Context) {
	var ba models.BeritaAcara
	if err := database.DB.
		Preload("Items.Arsip.KodeKlasifikasi").
		Preload("Items.Arsip.LokasiArsip").
		Preload("Items.Arsip.UnitKerja").
		Preload("LokasiAsal").
		Preload("LokasiTujuan").
		First(&ba, "id = ?", c.Param("id")).Error; err != nil {
		Render404(c)
		return
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 10, 20)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	margin := 20.0
	contentW := pageW - 2*margin

	// Kop surat - Instansi
	pdf.SetFont("Times", "B", 14)
	pdf.CellFormat(contentW, 7, config.App.AppInstitution, "", 1, "C", false, 0, "")
	pdf.SetFont("Times", "B", 12)
	pdf.CellFormat(contentW, 6, config.App.AppInstitutionSub, "", 1, "C", false, 0, "")
	pdf.SetFont("Times", "", 8)
	infoLine := "Jalan " + config.App.AppAddress
	if config.App.AppPhone != "" {
		infoLine += "  \u2502  Telp. " + config.App.AppPhone
	}
	if config.App.AppFax != "" {
		infoLine += "  \u2502  Fax. " + config.App.AppFax
	}
	pdf.CellFormat(contentW, 4, infoLine, "", 1, "C", false, 0, "")
	webLine := ""
	if config.App.AppWeb != "" {
		webLine = "Website: " + config.App.AppWeb
	}
	if config.App.AppEmail != "" {
		if webLine != "" {
			webLine += "  \u2502  "
		}
		webLine += "Email: " + config.App.AppEmail
	}
	if webLine != "" {
		pdf.CellFormat(contentW, 4, webLine, "", 1, "C", false, 0, "")
	}
	// Garis kop
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.8)
	y1 := pdf.GetY()
	pdf.Line(margin, y1, pageW-margin, y1)
	pdf.SetLineWidth(0.3)
	y2 := y1 + 1
	pdf.Line(margin, y2, pageW-margin, y2)
	pdf.Ln(5)

	// Judul
	pdf.SetFont("Times", "BU", 13)
	pdf.CellFormat(contentW, 8, "BERITA ACARA PEMINDAHAN ARSIP", "", 1, "C", false, 0, "")
	pdf.Ln(2)

	// Nomor
	pdf.SetFont("Times", "", 11)
	pdf.CellFormat(contentW, 7, "Nomor : "+ba.NomorBA, "", 1, "C", false, 0, "")
	pdf.Ln(5)

	// Pembukaan
	pdf.SetFont("Times", "", 11)
	hari := ba.Tanggal.Weekday().String()
	hariIndo := map[string]string{
		"Monday": "Senin", "Tuesday": "Selasa", "Wednesday": "Rabu",
		"Thursday": "Kamis", "Friday": "Jumat", "Saturday": "Sabtu", "Sunday": "Minggu",
	}
	tglStr := fmt.Sprintf("%s, %d %s %d",
		hariIndo[hari],
		ba.Tanggal.Day(),
		[]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}[ba.Tanggal.Month()-1],
		ba.Tanggal.Year(),
	)

	teksPembuka := fmt.Sprintf("Pada hari %s, bertempat di %s, kami yang bertanda tangan di bawah ini:", tglStr, ba.Tempat)
	pdf.MultiCell(contentW, 6, teksPembuka, "", "J", false)
	pdf.Ln(2)

	// Pihak Pertama
	pdf.SetFont("Times", "", 11)
	pdf.CellFormat(contentW, 7, "1.", "", 1, "L", false, 0, "")
	pdf.CellFormat(contentW, 6, "   NAMA           : "+ba.PihakPertamaNama, "", 1, "L", false, 0, "")
	if ba.PihakPertamaNIP != "" {
		pdf.CellFormat(contentW, 6, "   NIP            : "+ba.PihakPertamaNIP, "", 1, "L", false, 0, "")
	}
	jabatan1 := ba.PihakPertamaJabatan
	if ba.PihakPertamaUnit != "" {
		jabatan1 = ba.PihakPertamaJabatan + " " + ba.PihakPertamaUnit
	}
	pdf.MultiCell(contentW, 6, "   JABATAN        : "+jabatan1, "", "L", false)
	selaku1 := "Selaku Pihak Pertama Unit Pengolah."
	pdf.SetFont("Times", "I", 11)
	pdf.CellFormat(contentW, 6, "   "+selaku1, "", 1, "L", false, 0, "")
	pdf.Ln(2)

	// Pihak Kedua
	pdf.SetFont("Times", "", 11)
	pdf.CellFormat(contentW, 7, "2.", "", 1, "L", false, 0, "")
	pdf.CellFormat(contentW, 6, "   NAMA           : "+ba.PihakKeduaNama, "", 1, "L", false, 0, "")
	if ba.PihakKeduaNIP != "" {
		pdf.CellFormat(contentW, 6, "   NIP            : "+ba.PihakKeduaNIP, "", 1, "L", false, 0, "")
	}
	jabatan2 := ba.PihakKeduaJabatan
	if ba.PihakKeduaUnit != "" {
		jabatan2 = ba.PihakKeduaJabatan + " " + ba.PihakKeduaUnit
	}
	pdf.MultiCell(contentW, 6, "   JABATAN        : "+jabatan2, "", "L", false)
	selaku2 := "Selaku Pihak Kedua Unit Kearsipan."
	pdf.SetFont("Times", "I", 11)
	pdf.CellFormat(contentW, 6, "   "+selaku2, "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Isi
	pdf.SetFont("Times", "", 11)
	isi := "Menerangkan bahwa Pihak Pertama telah menyerahkan arsip kepada Pihak Kedua untuk dilakukan pemindahan lokasi penyimpanan, sesuai dengan ketentuan yang berlaku."
	pdf.MultiCell(contentW, 6, isi, "", "J", false)
	pdf.Ln(3)

	// Lokasi
	lokasiAsal := "-"
	if ba.LokasiAsal != nil {
		lokasiAsal = ba.LokasiAsal.NamaLokasi
	}
	lokasiTujuan := "-"
	if ba.LokasiTujuan != nil {
		lokasiTujuan = ba.LokasiTujuan.NamaLokasi
	}
	pdf.CellFormat(contentW, 6, "Lokasi asal   : "+lokasiAsal, "", 1, "L", false, 0, "")
	pdf.CellFormat(contentW, 6, "Lokasi tujuan : "+lokasiTujuan, "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Catatan
	if ba.Catatan != "" {
		pdf.SetFont("Times", "I", 10)
		pdf.MultiCell(contentW, 5, "Catatan: "+ba.Catatan, "", "L", false)
		pdf.Ln(3)
	}

	// Tutup
	pdf.SetFont("Times", "", 11)
	penutup := "Demikian Berita Acara ini dibuat dengan sebenarnya untuk dipergunakan sebagaimana mestinya."
	pdf.MultiCell(contentW, 6, penutup, "", "J", false)
	pdf.Ln(8)

	// Tanda tangan - style PDF template
	halfW := contentW/2 - 10
	pdf.SetFont("Times", "B", 11)
	pdf.CellFormat(halfW, 6, "PIHAK PERTAMA", "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 6, "PIHAK KEDUA", "", 0, "C", false, 0, "")
	pdf.Ln(6)

	// Jabatan both sides (use max line count)
	pdf.SetFont("Times", "", 9)
	p1Lines := pdf.SplitText(jabatan1, halfW)
	p2Lines := pdf.SplitText(jabatan2, halfW)
	maxLines := len(p1Lines)
	if len(p2Lines) > maxLines {
		maxLines = len(p2Lines)
	}
	for i := 0; i < maxLines; i++ {
		left := ""
		if i < len(p1Lines) {
			left = p1Lines[i]
		}
		right := ""
		if i < len(p2Lines) {
			right = p2Lines[i]
		}
		pdf.CellFormat(halfW, 4, left, "", 0, "C", false, 0, "")
		pdf.CellFormat(halfW, 4, right, "", 0, "C", false, 0, "")
		pdf.Ln(4)
	}

	pdf.Ln(4)

	// Garis tanda tangan
	pdf.SetFont("Times", "", 14)
	pdf.CellFormat(halfW, 8, "/", "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 8, "/", "", 0, "C", false, 0, "")
	pdf.Ln(14)

	// Nama
	pdf.SetFont("Times", "BU", 11)
	pdf.CellFormat(halfW, 6, ba.PihakPertamaNama, "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 6, ba.PihakKeduaNama, "", 0, "C", false, 0, "")
	pdf.Ln(6)

	// NIP
	pdf.SetFont("Times", "", 9)
	nip1 := ""
	if ba.PihakPertamaNIP != "" {
		nip1 = "NIP. " + ba.PihakPertamaNIP
	}
	nip2 := ""
	if ba.PihakKeduaNIP != "" {
		nip2 = "NIP. " + ba.PihakKeduaNIP
	}
	pdf.CellFormat(halfW, 5, nip1, "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 5, nip2, "", 0, "C", false, 0, "")
	pdf.Ln(2)

	// ── HALAMAN BARU: DAFTAR ARSIP ──
	pdf.AddPage()

	// Header tabel
	pdf.SetFont("Times", "B", 11)
	pdf.CellFormat(contentW, 8, "LAMPIRAN : DAFTAR ARSIP", "", 1, "C", false, 0, "")
	pdf.Ln(3)

	colW := []float64{8, 38, 48, 30, 30, 0}
	colW[5] = contentW - colW[0] - colW[1] - colW[2] - colW[3] - colW[4]
	headers := []string{"No", "Nomor Arsip", "Nama Arsip", "Klasifikasi", "Unit Kerja", "Lokasi Asal"}

	pdf.SetFont("Times", "B", 9)
	pdf.SetFillColor(200, 200, 200)
	for i, h := range headers {
		pdf.CellFormat(colW[i], 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Times", "", 8)
	pdf.SetFillColor(255, 255, 255)
	altFill := false
	for i, item := range ba.Items {
		if pdf.GetY() > 242 {
			pdf.AddPage()
			pdf.SetFont("Times", "B", 9)
			pdf.SetFillColor(200, 200, 200)
			for j, h := range headers {
				pdf.CellFormat(colW[j], 7, h, "1", 0, "C", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Times", "", 8)
			pdf.SetFillColor(255, 255, 255)
			altFill = false
		}

		arsip := item.Arsip
		no := fmt.Sprintf("%d", i+1)
		nomor := ""
		if arsip != nil {
			nomor = arsip.NomorArsip
		}
		nama := ""
		if arsip != nil {
			nama = arsip.NamaArsip
		}
		klasifikasi := ""
		if arsip != nil && arsip.KodeKlasifikasi != nil {
			klasifikasi = arsip.KodeKlasifikasi.KodeKlasifikasi
		}
		unit := ""
		if arsip != nil && arsip.UnitKerja != nil {
			unit = arsip.UnitKerja.NamaUnit
		}
		lokasi := ""
		if arsip != nil && arsip.LokasiArsip != nil {
			lokasi = arsip.LokasiArsip.NamaLokasi
		}

		rowData := []string{no, nomor, nama, klasifikasi, unit, lokasi}
		if altFill {
			pdf.SetFillColor(245, 245, 245)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		for j, val := range rowData {
			maxChars := int(colW[j] / 1.7)
			if len(val) > maxChars {
				val = val[:maxChars-3] + "..."
			}
			pdf.CellFormat(colW[j], 5, val, "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
		altFill = !altFill
	}

	// Tanda tangan di halaman terakhir (setelah tabel)
	if pdf.GetY() > 230 {
		pdf.AddPage()
	}
	pdf.Ln(10)

	pdf.SetFont("Times", "B", 11)
	pdf.CellFormat(halfW, 6, "PIHAK PERTAMA", "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 6, "PIHAK KEDUA", "", 0, "C", false, 0, "")
	pdf.Ln(6)

	pdf.SetFont("Times", "", 9)
	maxLines2 := len(p1Lines)
	if len(p2Lines) > maxLines2 {
		maxLines2 = len(p2Lines)
	}
	for i := 0; i < maxLines2; i++ {
		left := ""
		if i < len(p1Lines) {
			left = p1Lines[i]
		}
		right := ""
		if i < len(p2Lines) {
			right = p2Lines[i]
		}
		pdf.CellFormat(halfW, 4, left, "", 0, "C", false, 0, "")
		pdf.CellFormat(halfW, 4, right, "", 0, "C", false, 0, "")
		pdf.Ln(4)
	}
	pdf.Ln(4)
	pdf.SetFont("Times", "", 14)
	pdf.CellFormat(halfW, 8, "/", "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 8, "/", "", 0, "C", false, 0, "")
	pdf.Ln(14)
	pdf.SetFont("Times", "BU", 11)
	pdf.CellFormat(halfW, 6, ba.PihakPertamaNama, "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 6, ba.PihakKeduaNama, "", 0, "C", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Times", "", 9)
	pdf.CellFormat(halfW, 5, nip1, "", 0, "C", false, 0, "")
	pdf.CellFormat(halfW, 5, nip2, "", 0, "C", false, 0, "")
	pdf.Ln(2)

	var buf bytes.Buffer
	pdf.Output(&buf)

	filename := fmt.Sprintf("BA-Pemindahan-%s", ba.NomorBA)
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", filename))
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}
