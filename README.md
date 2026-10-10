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
go build -o simarc-server ./cmd/server

# Jalankan
./simarc-server
```

## 🖥️ Aplikasi Desktop

Selain dibuka lewat browser, SIMARC dapat dijalankan sebagai **aplikasi desktop**
(jendela sendiri, tanpa tab / address bar).

| Platform | Cara menjalankan |
|----------|------------------|
| Linux | `./run-desktop.sh` |
| Windows | klik dua kali `run.bat` |
| macOS | klik dua kali `Start SIMARC.command` |

**Pasang ke menu aplikasi (Linux):**

```bash
./install-desktop.sh
```

Perintah di atas membuat entri menu **SIMARC**, shortcut di Desktop, dan perintah
`simarc` di terminal. Menutup jendela aplikasi otomatis mematikan server.

> Membutuhkan browser berbasis Chromium (Google Chrome / Chromium / Brave / Edge).
> Bila tidak tersedia, aplikasi otomatis memakai browser default.

### Pasang di banyak komputer (tanpa install Go)

Secara bawaan, launcher akan membangun aplikasi dari sumber saat pertama kali
dijalankan — ini memerlukan **Go**. Untuk komputer klien, cukup sediakan
**binary siap-pakai** agar tidak perlu Go sama sekali:

```bash
# di satu komputer (butuh Go) — hasilkan binary untuk semua OS
./scripts/build-release.sh
```

Perintah tersebut membuat folder `dist/` berisi binary Linux / Windows / macOS.
Salin folder SIMARC (termasuk `dist/`) ke komputer lain; launcher akan otomatis
memakai binary di `dist/` **tanpa** meng-install Go.

> Urutan pencarian binary oleh launcher: `tmp/` (hasil build terbaru) →
> `dist/` (binary rilis) → build dari sumber (butuh Go).

Menu yang sama juga berlaku untuk mode klien: cukup tambahkan
`SIMARC_SERVER_URL` di `.env`, dan komputer klien tidak menjalankan server lokal.

### Ukuran jendela (auto layar penuh)

Atur lewat `SIMARC_WINDOW` di `.env`:

| Nilai | Efek |
|-------|------|
| `normal` | Jendela ukuran biasa (default) |
| `maximized` | Terbuka otomatis memenuhi layar (tombol minimize/close tetap ada) |
| `fullscreen` | Layar penuh tanpa bingkai (keluar: `F11` atau `Alt+F4`) |
| `kiosk` | Mode kiosk terkunci penuh (untuk terminal / perangkat khusus) |

```bash
# contoh: buka otomatis memenuhi layar
SIMARC_WINDOW=maximized
```

### Satu database untuk banyak komputer (server pusat)

SIMARC menyimpan **berkas arsip di disk server** (database hanya mencatat
lokasinya). Karena itu, jika setiap komputer menjalankan server sendiri —
walaupun database-nya sama — berkas yang diunggah di satu komputer **tidak**
akan muncul di komputer lain.

Solusinya: jalankan **satu server pusat**, lalu komputer lain dijadikan **klien**.

**1) Server pusat** (satu komputer / VPS — pemegang database & semua berkas):

```bash
# di komputer server (tanpa membuka browser di server)
SIMARC_NO_BROWSER=1 ./tmp/simarc-server
```

Catat alamat jaringan yang tampil di banner (mis. `http://192.168.1.10:8080`)
dan pastikan port tersebut **dibuka di firewall** LAN.

**2) Komputer klien** — cukup satu baris di `.env`:

```bash
SIMARC_SERVER_URL=http://192.168.1.10:8080
```

Lalu jalankan seperti biasa (`./run-desktop.sh`, `run.bat`, atau ikon **SIMARC**).
Klien **tidak** menjalankan server/database lokal — hanya membuka jendela
aplikasi ke server pusat. Hasilnya: semua komputer memakai database **dan**
berkas yang sama, tanpa perlu menyalin data.

> Untuk beberapa komputer, "server pusat" dapat berupa salah satu komputer
> kantor; untuk akses dari luar kantor gunakan domain + reverse proxy (HTTPS)
> atau VPN. Bila `SIMARC_SERVER_URL` dibiarkan kosong, aplikasi kembali ke
> mode server lokal seperti biasa.

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
go build -o simarc-server ./cmd/server

# Jalankan di port lain
APP_PORT=9090 ./simarc-server

# Mode debug
APP_DEBUG=true ./simarc-server

# Backup database (disimpan di storage/app/backups/database/)
# Backup otomatis via web: Backup & Restore → Backup Database
```

## 📝 Lisensi

© 2026 Bakesbangpol Kota Probolinggo. All rights reserved.

