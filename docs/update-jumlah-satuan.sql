-- ============================================================
-- SIMARC: Update existing arsip records with jumlah=1, satuan='Berkas'
-- Jalankan script ini jika migrasi otomatis tidak berjalan
-- (MySQL 8.0+ — ADD COLUMN IF NOT EXISTS hanya ada di MariaDB)
-- ============================================================

-- Tambahkan kolom jika belum ada (cek manual karena MySQL tidak
-- mendukung ADD COLUMN IF NOT EXISTS)
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'arsip' AND column_name = 'jumlah');
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE arsip ADD COLUMN jumlah INT NOT NULL DEFAULT 1',
    'SELECT ''kolom jumlah sudah ada''');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'arsip' AND column_name = 'satuan');
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE arsip ADD COLUMN satuan VARCHAR(30) NOT NULL DEFAULT ''Berkas''',
    'SELECT ''kolom satuan sudah ada''');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Update semua data lama yang mungkin NULL
UPDATE arsip SET jumlah = 1 WHERE jumlah IS NULL OR jumlah < 1;
UPDATE arsip SET satuan = 'Berkas' WHERE satuan IS NULL OR satuan = '';

-- Verifikasi
SELECT COUNT(*) AS total_arsip FROM arsip WHERE deleted_at IS NULL;
SELECT COUNT(*) AS masih_null FROM arsip WHERE jumlah IS NULL OR satuan IS NULL;
