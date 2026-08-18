package handlers

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"arsippro/internal/config"
	"arsippro/internal/database"
	"arsippro/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"
)

func exportXLSX(c *gin.Context, filename string, headers []string, rows [][]string) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Sheet1"
	idx, _ := f.NewSheet(sheet)
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#000000", Style: 1},
			{Type: "top", Color: "#000000", Style: 1},
			{Type: "bottom", Color: "#000000", Style: 1},
			{Type: "right", Color: "#000000", Style: 1},
		},
	})
	cellStyle, _ := f.NewStyle(&excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Color: "#CCCCCC", Style: 1},
			{Type: "top", Color: "#CCCCCC", Style: 1},
			{Type: "bottom", Color: "#CCCCCC", Style: 1},
			{Type: "right", Color: "#CCCCCC", Style: 1},
		},
	})

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	for r, row := range rows {
		for i, val := range row {
			cell, _ := excelize.CoordinatesToCellName(i+1, r+2)
			f.SetCellValue(sheet, cell, val)
			f.SetCellStyle(sheet, cell, cell, cellStyle)
		}
	}

	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheet, col, col, 20)
	}

	var buf bytes.Buffer
	f.WriteTo(&buf)

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.xlsx", filename))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

func exportPDF(c *gin.Context, filename string, title string, headers []string, rows [][]string) {
	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(10, 8, 10)
	pdf.AddPage()

	colCount := len(headers)
	pageW, _ := pdf.GetPageSize()
	pageW -= 20

	// Header institusi
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 7, config.App.AppInstitution, "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.CellFormat(0, 5, config.App.AppInstitutionSub, "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "I", 7)
	pdf.CellFormat(0, 4, "Sistem Informasi Manajemen Kearsipan", "", 1, "C", false, 0, "")
	pdf.Ln(2)
	// Garis pembatas
	pdf.SetDrawColor(30, 41, 59)
	pdf.SetLineWidth(0.8)
	pdf.Line(10, pdf.GetY(), 10+pageW, pdf.GetY())
	pdf.Ln(1)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetLineWidth(0.3)
	pdf.Line(10, pdf.GetY(), 10+pageW, pdf.GetY())
	pdf.Ln(4)

	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 8, title, "", 1, "C", false, 0, "")
	pdf.Ln(4)

	colW := pageW / float64(colCount)
	lineH := 5.0
	leftMargin := 10.0

	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(30, 41, 59)
	pdf.SetTextColor(255, 255, 255)
	for _, h := range headers {
		pdf.CellFormat(colW, 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetTextColor(0, 0, 0)

	pdf.SetFont("Arial", "", 8)
	altFill := false
	for _, row := range rows {
		// Calculate max lines needed for this row
		maxLines := 1
		for _, val := range row {
			strWidth := pdf.GetStringWidth(val)
			lines := int(math.Ceil(strWidth / colW))
			if lines < 1 {
				lines = 1
			}
			if lines > maxLines {
				maxLines = lines
			}
		}
		rowH := float64(maxLines) * lineH

		// Page break check
		if pdf.GetY()+rowH > 190 {
			pdf.AddPage()
			altFill = false
			pdf.SetFont("Arial", "B", 8)
			pdf.SetFillColor(30, 41, 59)
			pdf.SetTextColor(255, 255, 255)
			for _, h := range headers {
				pdf.CellFormat(colW, 7, h, "1", 0, "C", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetTextColor(0, 0, 0)
			pdf.SetFont("Arial", "", 8)
		}

		yStart := pdf.GetY()
		xStart := leftMargin
		for i, val := range row {
			xPos := xStart + float64(i)*colW
			// Draw background and border rectangle for full cell height
			if altFill {
				pdf.SetFillColor(245, 247, 250)
			} else {
				pdf.SetFillColor(255, 255, 255)
			}
			pdf.SetDrawColor(220, 225, 232)
			pdf.Rect(xPos, yStart, colW, rowH, "DF")
			// Write text inside the cell (no border/fill, wraps within colW)
			pdf.SetXY(xPos+0.5, yStart+0.3)
			pdf.MultiCell(colW-1, lineH, val, "", "L", false)
		}
		pdf.SetY(yStart + rowH)
		pdf.SetX(leftMargin)
		altFill = !altFill
	}

	pdf.Ln(5)
	pdf.SetDrawColor(30, 41, 59)
	pdf.SetLineWidth(0.5)
	pdf.Line(10, pdf.GetY(), pageW+10, pdf.GetY())
	pdf.Ln(2)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(100, 116, 139)
	pdf.CellFormat(0, 5, fmt.Sprintf("Total: %d baris  |  Dicetak: %s", len(rows), time.Now().Format("02 Jan 2006 15:04")), "", 1, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	var buf bytes.Buffer
	pdf.Output(&buf)

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", filename))
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}

func exportCrudXLSX(c *gin.Context, filename string, headers []string, rows [][]string) {
	exportXLSX(c, filename, headers, rows)
}

func exportCrudPDF(c *gin.Context, filename string, title string, headers []string, rows [][]string) {
	exportPDF(c, filename, title, headers, rows)
}

func ExportKodeKlasifikasi(c *gin.Context) {
	var list []models.KodeKlasifikasi
	database.DB.Order("kode_klasifikasi ASC").Find(&list)
	headers := []string{"Kode", "Nama Klasifikasi", "Retensi Aktif", "Retensi Inaktif", "Penyusutan", "Keamanan", "Aktif"}
	rows := [][]string{}
	for _, v := range list {
		active := "Ya"
		if !v.IsActive { active = "Tidak" }
		rows = append(rows, []string{
			v.KodeKlasifikasi, v.NamaKlasifikasi,
			strconv.Itoa(v.RetensiAktif), strconv.Itoa(v.RetensiInaktif),
			v.PenyusutanArsip, v.KlasifikasiKeamanan, active,
		})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Kode-Klasifikasi", "Data Kode Klasifikasi", headers, rows)
	} else {
		exportCrudXLSX(c, "Kode-Klasifikasi", headers, rows)
	}
}

func ExportUnitKerja(c *gin.Context) {
	var list []models.UnitKerja
	database.DB.Order("nama_unit ASC").Find(&list)
	headers := []string{"Kode", "Nama Unit Kerja", "Deskripsi"}
	rows := [][]string{}
	for _, v := range list {
		rows = append(rows, []string{v.ID, v.NamaUnit, ""})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Unit-Kerja", "Data Unit Kerja", headers, rows)
	} else {
		exportCrudXLSX(c, "Unit-Kerja", headers, rows)
	}
}

func ExportLokasiArsip(c *gin.Context) {
	var list []models.LokasiArsip
	database.DB.Order("nama_lokasi ASC").Find(&list)
	headers := []string{"Kode", "Nama Lokasi", "Deskripsi"}
	rows := [][]string{}
	for _, v := range list {
		rows = append(rows, []string{v.ID, v.NamaLokasi, v.Deskripsi})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Lokasi-Arsip", "Data Lokasi Arsip", headers, rows)
	} else {
		exportCrudXLSX(c, "Lokasi-Arsip", headers, rows)
	}
}

func ExportJenisArsip(c *gin.Context) {
	var list []models.JenisArsip
	database.DB.Order("nama ASC").Find(&list)
	headers := []string{"Nama Jenis Arsip", "Deskripsi"}
	rows := [][]string{}
	for _, v := range list {
		rows = append(rows, []string{v.NamaJenis, v.Keterangan})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Jenis-Arsip", "Data Jenis Arsip", headers, rows)
	} else {
		exportCrudXLSX(c, "Jenis-Arsip", headers, rows)
	}
}

func ExportUsers(c *gin.Context) {
	var list []models.User
	database.DB.Preload("Role").Preload("UnitKerja").Order("name ASC").Find(&list)
	headers := []string{"Username", "Nama", "Email", "Role", "Unit Kerja", "Aktif", "Terakhir Login"}
	rows := [][]string{}
	for _, v := range list {
		active := "Ya"
		if !v.IsActive { active = "Tidak" }
		lastLogin := "-"
		if v.LastLoginAt != nil { lastLogin = v.LastLoginAt.Format("02/01/2006 15:04") }
		roleName := "-"
		if v.Role != nil { roleName = v.Role.Name }
		unitName := "-"
		if v.UnitKerja != nil { unitName = v.UnitKerja.NamaUnit }
		rows = append(rows, []string{v.Username, v.Name, v.Username, roleName, unitName, active, lastLogin})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Users", "Data Pengguna", headers, rows)
	} else {
		exportCrudXLSX(c, "Users", headers, rows)
	}
}

func ExportRoles(c *gin.Context) {
	var list []models.Role
	database.DB.Order("name ASC").Find(&list)
	headers := []string{"Nama Role", "Deskripsi"}
	rows := [][]string{}
	for _, v := range list {
		rows = append(rows, []string{v.Name, v.Keterangan})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Roles", "Data Role", headers, rows)
	} else {
		exportCrudXLSX(c, "Roles", headers, rows)
	}
}

func ExportPeminjaman(c *gin.Context) {
	var list []models.PeminjamanArsip
	database.DB.Preload("User").Preload("Arsip").Order("created_at DESC").Find(&list)
	headers := []string{"Peminjam", "Arsip", "Tanggal Pinjam", "Rencana Kembali", "Tanggal Kembali", "Status"}
	rows := [][]string{}
	for _, v := range list {
		peminjam := "-"
		if v.User != nil { peminjam = v.User.Name }
		arsipNama := "-"
		if v.Arsip != nil { arsipNama = v.Arsip.NamaArsip }
		tglPinjam := "-"
		if v.TanggalPinjam != nil { tglPinjam = v.TanggalPinjam.Format("02/01/2006") }
		tglRencana := "-"
		if v.TanggalKembaliRencana != nil { tglRencana = v.TanggalKembaliRencana.Format("02/01/2006") }
		tglKembali := "-"
		if v.TanggalKembaliAktual != nil { tglKembali = v.TanggalKembaliAktual.Format("02/01/2006") }
		rows = append(rows, []string{peminjam, arsipNama, tglPinjam, tglRencana, tglKembali, v.Status})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Peminjaman", "Data Peminjaman Arsip", headers, rows)
	} else {
		exportCrudXLSX(c, "Peminjaman", headers, rows)
	}
}

func ExportJadwalRetensi(c *gin.Context) {
	var list []models.JadwalRetensi
	database.DB.Preload("KodeKlasifikasi").Preload("UnitKerja").Order("created_at DESC").Find(&list)
	headers := []string{"Kode Klasifikasi", "Unit Kerja", "Tanggal Berakhir", "Status", "Keterangan"}
	rows := [][]string{}
	for _, v := range list {
		kkName := "-"
		if v.KodeKlasifikasi != nil { kkName = v.KodeKlasifikasi.KodeKlasifikasi + " - " + v.KodeKlasifikasi.NamaKlasifikasi }
		ukName := "-"
		if v.UnitKerja != nil { ukName = v.UnitKerja.NamaUnit }
		tglBerakhir := "-"
		if v.TanggalPelaksanaan != nil { tglBerakhir = v.TanggalPelaksanaan.Format("02/01/2006") }
		rows = append(rows, []string{kkName, ukName, tglBerakhir, v.Status, v.Keterangan})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Jadwal-Retensi", "Data Jadwal Retensi", headers, rows)
	} else {
		exportCrudXLSX(c, "Jadwal-Retensi", headers, rows)
	}
}

func ExportPemberkasan(c *gin.Context) {
	var list []models.Pemberkasan
	database.DB.Preload("UnitKerja").Preload("Arsip").Order("created_at DESC").Find(&list)
	headers := []string{"Kode Berkas", "Nama Berkas", "Unit Kerja", "Jumlah Arsip", "Status"}
	rows := [][]string{}
	for _, v := range list {
		ukName := "-"
		if v.UnitKerja != nil { ukName = v.UnitKerja.NamaUnit }
		jmlArsip := 0
		if v.Arsip != nil { jmlArsip = len(v.Arsip) }
		rows = append(rows, []string{v.KodeBerkas, v.NamaPemberkasan, ukName, strconv.Itoa(jmlArsip), v.StatusBerkas})
	}
	if c.Query("format") == "pdf" {
		exportCrudPDF(c, "Pemberkasan", "Data Pemberkasan", headers, rows)
	} else {
		exportCrudXLSX(c, "Pemberkasan", headers, rows)
	}
}
