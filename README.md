# SIMARC — Sistem Informasi Manajemen Arsip Record Center

Aplikasi manajemen arsip berbasis web dengan fitur pemindahan, pemusnahan, peminjaman, pemberkasan, retensi arsip, backup database, dan dukungan blockchain audit trail.

**Stack production:** Vercel (hosting) + Aiven MySQL (database utama / source of truth).

## ✨ Fitur Utama

- 📁 **Manajemen Arsip** — CRUD, upload file, QR code, OCR
- 📦 **Pemberkasan** — Mengelompokkan arsip ke dalam berkas
- 📍 **Pemindahan Arsip** — Pindah lokasi penyimpanan + Berita Acara PDF
- 🗑️ **Pemusnahan Arsip** — Jadwal retensi, pemusnahan terjadwal
- 📖 **Peminjaman** — Peminjaman arsip dengan tracking
- 🔗 **Blockchain Audit** — Catatan perubahan anti-manipulasi
- 💾 **Backup Database** — Backup database otomatis disimpan di storage lokal
- 📊 **Laporan** — Beragam laporan siap cetak/export

## 🚀 Prasyarat

| Komponen | Minimal |
|----------|---------|
| Go | 1.21+ |
| MySQL / MariaDB | 8.0+ / 10.5+ (Aiven direkomendasikan untuk production) |
| RAM | 512 MB |
| Storage | 100 MB (aplikasi) + data arsip |

## ⚡ Cara Instalasi (1 menit)

### 1. Setup Otomatis

```bash
chmod +x setup.sh
./setup.sh
```

Script akan:
- Memeriksa Go dan MySQL/MariaDB
- Membuat database jika belum ada
- Mengunduh dependensi Go
- Membangun binary
- Menawarkan untuk menjalankan server

### 2. Manual

```bash
# Clone atau masuk ke direktori project
cd simarc

# Edit konfigurasi database
nano .env

# Unduh dependensi
go mod tidy

# Bangun aplikasi
go build -o simarc-server ./cmd/server/main.go

# Jalankan
./simarc-server
```

## 🔧 File .env — SATU file untuk semua konfigurasi

Semua pengaturan (database **dan** aplikasi) disimpan dalam satu file `.env`:

```
# ── Aplikasi ──
APP_NAME="SIMARC-Arsip Record Center"
APP_URL=http://localhost:8080
APP_PORT=8080
APP_DEBUG=true
APP_TIMEZONE=Asia/Jakarta      # WIB / WITA / WIT

# ── Database (Aiven MySQL) ──
DB_HOST=mysql-xxxx-xxx.b.aivencloud.com
DB_PORT=19160
DB_DATABASE=defaultdb
DB_USERNAME=avnadmin
DB_PASSWORD=••••••

# Wajib di production — gunakan `openssl rand -hex 32`
SESSION_KEY=
```

> Nama aplikasi & zona waktu yang diubah lewat menu **Pengaturan → Umum** ikut
> disimpan ke `.env` yang sama — tidak ada file pengaturan terpisah.

## 🗄️ Ganti Database dari Web UI (tanpa edit file)

Tidak perlu membuka file `.env` untuk mengganti database:

1. Masuk sebagai **Admin** → menu **Administrasi → Pengaturan → tab Database**.
2. Ubah Host / Port / Nama Database / Username / Password.
3. Klik **Uji Koneksi** untuk memastikan kredensial benar (tanpa menyimpan).
4. Klik **Simpan & Terapkan Sekarang** — kredensial disimpan ke `.env` dan aplikasi
   langsung tersambung ke database baru **tanpa restart** (tabel dibuat otomatis
   jika database masih kosong).

> Password admin tidak pernah di-reset saat berpindah database.

### Mode Pemulihan (Database tidak bisa dihubungi)

Jika aplikasi tidak dapat terhubung ke database saat dinyalakan, aplikasi **tetap
berjalan** dan menampilkan halaman **Konfigurasi Database** (`/database-setup`)
alih-alih berhenti. Isi kredensial yang benar di sana, lalu aplikasi otomatis
menyambung dan mengarahkan Anda ke halaman masuk.

> ⚠️ Saat database tidak terhubung, halaman pemulihan dan endpoint simpan
> database dapat diakses tanpa login agar koneksi bisa diperbaiki. Pastikan
> aplikasi hanya dapat diakses dari jaringan yang tepercaya pada kondisi ini.

### Backup Database
```
# Backup disimpan di storage/app/backups/database/
```

### Backup ke Google Drive (opsional)

Backup database juga bisa diunggah otomatis ke Google Drive menggunakan
Google Drive REST API dari sisi browser (JavaScript / OAuth 2.0).

1. Buka **Menu → Backup & Restore → Google Drive Backup → Pengaturan**.
2. Isi **OAuth Client ID** (dan opsional **Folder ID**), lalu **Simpan**.
3. Klik **Backup & Upload ke Google Drive** — browser akan meminta izin
   akun Google (scope `drive.file`), lalu mengunggah file `.sql` hasil backup.

Persiapan di [Google Cloud Console](https://console.cloud.google.com/):
- Aktifkan **Google Drive API**.
- Buat **OAuth Client ID** tipe *Web application* dan tambahkan URL aplikasi
  (mis. `http://localhost:8080`) ke *Authorized JavaScript origins*.

Konfigurasi disimpan di `.env`:
```
GOOGLE_DRIVE_CLIENT_ID=1234567890-xxxx.apps.googleusercontent.com
GOOGLE_DRIVE_FOLDER_ID=1AbC...   # opsional
```

## 🏃 Cara Menjalankan

### Opsi 1 — Langsung (single run)
```bash
./simarc-server
```
Akses di: http://localhost:8080

### Opsi 2 — Dengan auto-reload (untuk development)
```bash
./run.sh
```
Server otomatis rebuild saat ada perubahan file Go/HTML.

### Opsi 3 — Control panel (GUI / Terminal)
```bash
./simarc-control.sh
```
Tampilkan menu untuk start/stop/buka browser.

## 💾 Backup Database

Backup disimpan di `storage/app/backups/database/`.

Via web: buka Menu → Backup & Restore → "Backup Database"

## 🖥️ Tampilan Aplikasi

Setelah server berjalan, buka browser:

| URL | Keterangan |
|-----|-----------|
| http://localhost:8080 | Halaman utama / login |
| http://localhost:8080/arsip | Daftar arsip |
| http://localhost:8080/arsip/pemindahan | Pemindahan arsip |
| http://localhost:8080/backup | Backup & Restore (lokal) |

## 📁 Struktur Direktori

```
simarc/
├── cmd/
│   ├── server/main.go       # Entry point server
├── internal/
│   ├── handlers/            # HTTP handlers
│   ├── models/              # Database models
│   ├── middleware/           # Auth, CSRF, session
│   ├── database/            # Koneksi & migrasi
│   ├── config/              # Konfigurasi aplikasi
│   ├── services/            # Business logic
├── web/templates/           # HTML templates
├── storage/                 # File uploads & backups
├── run.sh                   # Run dengan auto-reload
├── setup.sh                 # Setup otomatis
├── simarc-control.sh        # Control panel
```

## 🔒 Login Default

| Role | Username | Password |
|------|----------|----------|
| Admin | admin | (tersedia setelah seed) |

> Login pertama: buka http://localhost:8080, gunakan user yang telah di-seed.

## 🧰 Perintah Berguna

```bash
# Build ulang
go build -o simarc-server ./cmd/server/main.go

# Jalankan di port lain
APP_PORT=9090 ./simarc-server

# Mode debug
APP_DEBUG=true ./simarc-server

# Backup database (disimpan di storage/app/backups/database/)
# Backup otomatis via web: Backup & Restore → Backup Database
```

## 📝 Lisensi

© 2026 Bakesbangpol Kota Probolinggo. All rights reserved.
