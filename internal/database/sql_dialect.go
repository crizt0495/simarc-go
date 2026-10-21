package database

import (
	"fmt"
	"regexp"
)

// Berkas ini berisi pembangun ekspresi SQL yang berbeda antara MySQL (server
// pusat / instalasi lama) dan SQLite (database lokal offline-first desktop).
// Semua pemanggil memakai helper ini agar kueri tetap portabel.
//
// AdaptSQL menerjemahkan potongan SQL gaya MySQL yang tidak dikenal SQLite.
// Pada MySQL fungsi ini mengembalikan kueri apa adanya (tanpa biaya berarti).

var (
	reDateAdd      = regexp.MustCompile(`(?i)DATE_ADD\(\s*([^,]+?)\s*,\s*INTERVAL\s*\(([^)]+)\)\s*YEAR\s*\)`)
	reCurDate      = regexp.MustCompile(`(?i)CURDATE\(\s*\)`)
	reDateFormat   = regexp.MustCompile(`(?i)DATE_FORMAT\(\s*([^,]+?)\s*,\s*'([^']*)'\s*\)`)
	reToChar       = regexp.MustCompile(`(?i)TO_CHAR\(\s*([^,]+?)\s*,\s*'YYYY-MM'\s*\)`)
	reExtractYear  = regexp.MustCompile(`(?i)EXTRACT\(\s*YEAR\s+FROM\s+([^)]+)\)`)
	reCastUnsigned = regexp.MustCompile(`(?i)\bAS\s+UNSIGNED\b`)
	reVersion      = regexp.MustCompile(`(?i)\bVERSION\(\s*\)`)
)

// AdaptSQL menyesuaikan kueri agar berjalan di mesin database aktif.
func AdaptSQL(q string) string {
	if !IsSQLite() {
		return q
	}
	q = reDateAdd.ReplaceAllString(q, `date($1, '+' || ($2) || ' years')`)
	q = reCurDate.ReplaceAllString(q, `date('now','localtime')`)
	q = reDateFormat.ReplaceAllString(q, `strftime('$2', $1)`)
	q = reToChar.ReplaceAllString(q, `strftime('%Y-%m', $1)`)
	q = reExtractYear.ReplaceAllString(q, `CAST(strftime('%Y', $1) AS INTEGER)`)
	q = reCastUnsigned.ReplaceAllString(q, `AS INTEGER`)
	q = reVersion.ReplaceAllString(q, `sqlite_version()`)
	return q
}

// SQLCurDate mengembalikan ekspresi tanggal hari ini (Y-m-d).
func SQLCurDate() string {
	if IsSQLite() {
		return "date('now','localtime')"
	}
	return "CURDATE()"
}

// SQLAddYears menjumlahkan kolom tanggal dengan sejumlah tahun.
func SQLAddYears(col, years string) string {
	if IsSQLite() {
		return fmt.Sprintf("date(%s, '+' || (%s) || ' years')", col, years)
	}
	return fmt.Sprintf("DATE_ADD(%s, INTERVAL (%s) YEAR)", col, years)
}

// SQLYearMonth menghasilkan teks "YYYY-MM" dari kolom waktu.
func SQLYearMonth(col string) string {
	if IsSQLite() {
		return fmt.Sprintf("strftime('%%Y-%%m', %s)", col)
	}
	return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m')", col)
}

// SQLYear menghasilkan tahun (integer) dari kolom tanggal.
func SQLYear(col string) string {
	if IsSQLite() {
		return fmt.Sprintf("CAST(strftime('%%Y', %s) AS INTEGER)", col)
	}
	return fmt.Sprintf("YEAR(%s)", col)
}

// SQLDateOf mengambil bagian tanggal dari kolom waktu.
func SQLDateOf(col string) string {
	if IsSQLite() {
		return fmt.Sprintf("date(%s)", col)
	}
	return fmt.Sprintf("DATE(%s)", col)
}

// SQLNumericSuffix mengubah nomor arsip menjadi angka untuk pengurutan alami
// (mis. "001/SPM/2025" -> 0012025). SQLite memakai fungsi REGEXP_REPLACE yang
// didaftarkan oleh driver di database.go.
func SQLNumericSuffix(col string) string {
	if IsSQLite() {
		return fmt.Sprintf("CAST(REGEXP_REPLACE(%s, '[^0-9]', '') AS INTEGER)", col)
	}
	return fmt.Sprintf("(CAST(REGEXP_REPLACE(%s, '[^0-9]', '') AS UNSIGNED))", col)
}
