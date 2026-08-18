-- ============================================================
-- SIMARC: Update existing arsip records with jumlah=1, satuan='Berkas'
-- Jalankan script ini jika migrasi otomatis tidak berjalan
-- ============================================================

-- Tambahkan kolom jika belum ada
ALTER TABLE arsip ADD COLUMN IF NOT EXISTS jumlah INT NOT NULL DEFAULT 1;
ALTER TABLE arsip ADD COLUMN IF NOT EXISTS satuan VARCHAR(30) NOT NULL DEFAULT 'Berkas';

-- Update semua data lama yang mungkin NULL
UPDATE arsip SET jumlah = 1 WHERE jumlah IS NULL OR jumlah < 1;
UPDATE arsip SET satuan = 'Berkas' WHERE satuan IS NULL OR satuan = '';

-- Verifikasi
SELECT COUNT(*) AS total_arsip FROM arsip WHERE deleted_at IS NULL;
SELECT COUNT(*) AS masih_null FROM arsip WHERE jumlah IS NULL OR satuan IS NULL;
