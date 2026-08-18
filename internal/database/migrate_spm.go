package database

import (
	"log"
	"regexp"
	"strings"
)

// extractSPMNumber attempts to extract an SPM number from text.
// It looks for the "SPM" keyword and captures the following number/identifier.
func extractSPMNumber(text string) string {
	if text == "" {
		return ""
	}

	// Normalize: collapse whitespace
	normalized := strings.Join(strings.Fields(text), " ")

	// Pattern: SPM (possibly with dots like S.P.M.) followed by optional separators,
	// optional "No." prefix, then capture alphanumeric identifier
	re := regexp.MustCompile(`(?i)(?:S\.?P\.?M\.?)[\s:.-]*(?:No(?:mor)?\.?\s*:?\s*)?([A-Za-z]{0,3}[\s-]?\d[\d\-/,A-Za-z.]{0,})`)
	
	match := re.FindStringSubmatch(normalized)
	if len(match) < 2 {
		return ""
	}

	result := strings.TrimSpace(match[1])
	// Clean trailing separator characters
	result = strings.TrimRight(result, "-/.")

	// Must contain at least one digit
	hasDigit := false
	for _, c := range result {
		if c >= '0' && c <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return ""
	}

	// If the result is just letters (e.g., "LS" without numbers), skip
	// Minimum length of 2 chars
	if len(result) < 5 {
		return ""
	}

	return result
}

// migrateSPMFromUraian extracts SPM numbers from uraian/nama_arsip/nomor_arsip
// and stores them in the nomor_spm field
func migrateSPMFromUraian() {
	log.Println("[MIGRASI] Mengekstrak No. SPM dari uraian arsip...")

	// Find all records where nomor_spm is empty
	type ArsipRecord struct {
		ID         string
		Uraian     string
		NamaArsip  string
		NomorArsip string
		JenisArsip string
		NomorSPM   string
	}

	var records []ArsipRecord
	DB.Table("arsip").
		Select("id, uraian, nama_arsip, nomor_arsip, jenis_arsip, nomor_spm").
		Where("(nomor_spm IS NULL OR nomor_spm = '') AND (uraian LIKE '%SPM%' OR uraian LIKE '%spm%' OR nama_arsip LIKE '%SPM%' OR nama_arsip LIKE '%spm%')").
		Find(&records)

	if len(records) == 0 {
		log.Println("[MIGRASI] Tidak ada arsip yang perlu diekstrak No. SPM-nya.")
		return
	}

	updated := 0
	for _, r := range records {
		// Try uraian first, then nama_arsip, then nomor_arsip
		spmNumber := extractSPMNumber(r.Uraian)
		if spmNumber == "" {
			spmNumber = extractSPMNumber(r.NamaArsip)
		}
		if spmNumber == "" {
			spmNumber = extractSPMNumber(r.NomorArsip)
		}
		if spmNumber == "" {
			continue
		}

		// Update the record
		DB.Table("arsip").Where("id = ?", r.ID).Update("nomor_spm", spmNumber)
		updated++
	}

	log.Printf("[MIGRASI] Selesai: %d arsip diperbarui dengan No. SPM dari uraian.", updated)
}
