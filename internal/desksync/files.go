package desksync

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"arsippro/internal/models"

	qrcode "github.com/skip2/go-qrcode"
)

// ── FILE HANDLING: lampiran arsip lintas mesin ───────────────────────────────

// ResolveUploadPath menerjemahkan nilai kolom file_path/qrcode_path ke path
// os relatif working directory (aturan sama dengan handler download/view):
// path abs dipertahankan; path relatif tanpa prefix "storage/"/"/tmp/"
// diprefixkan "storage/".
func ResolveUploadPath(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "storage/") || strings.HasPrefix(p, "/tmp/") {
		return p
	}
	return "storage/" + p
}

// materializeArsipFile menulis isi lampiran (FileContent) yang masuk via sync
// ke disk lokal, sehingga View/Download berikutnya memakai file biasa.
func (e *Engine) materializeArsipFile(row *models.Arsip) {
	if len(row.FileContent) == 0 || row.FilePath == "" {
		return
	}
	abs := ResolveUploadPath(row.FilePath)
	if info, err := os.Stat(abs); err == nil && info.Size() == int64(len(row.FileContent)) {
		return // file sudah sama
	}
	if err := writeBytes(abs, row.FileContent); err != nil {
		log.Printf("[SYNC] Gagal materialisasi lampiran %s: %v", abs, err)
	}
}

// ensureQrFile membuat ulang gambar QR bila belum ada di disk — QR dibuat
// dari QrData yang ikut tersinkron, jadi tak perlu transfer PNG.
func (e *Engine) ensureQrFile(row *models.QrCode) {
	if row.QrCodePath == "" || row.QrData == "" {
		return
	}
	abs := ResolveUploadPath(row.QrCodePath)
	if _, err := os.Stat(abs); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		log.Printf("[SYNC] Gagal menyiapkan dir QR %s: %v", abs, err)
		return
	}
	if err := qrcode.WriteFile(row.QrData, qrcode.Medium, 256, abs); err != nil {
		log.Printf("[SYNC] Gagal membuat ulang QR %s: %v", abs, err)
	}
}

// writeBytes menulis file (dengan MkdirAll untuk parent).
func writeBytes(abs string, data []byte) error {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}
