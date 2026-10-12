# Backup Cloud (Offsite) — SIMARC

Dokumentasi fitur **Backup Cloud**: menyalin database dan file arsip lokal ke
layanan cloud gratis, sebagai cadangan jika server lokal rusak/hilang.

> **Prinsip:** Database lokal tetap menjadi **sumber utama** (source of truth).
> Aplikasi *tidak* membaca dari cloud. Cloud hanya menerima salinan (mirror)
> secara berkala, sehingga bisa dipakai untuk pemulihan (restore) bila perlu.

---

## 1. Ringkasan Alur

```
                 +---------------------------+
   server lokal  |  MariaDB (simarc)  <----  |  dipakai aplikasi (web + desktop)
                 |  storage/app/uploads      |
                 +-----------+---------------+
                             |
             setiap hari 02:00 (penjadwal di dalam server)
                             |
             +---------------+----------------+
             |                                |
    mysqldump -> impor ke cloud MySQL   arsip file -> unggah ke object storage
    (Aiven for MySQL, dst.)             (Cloudflare R2 / Backblaze B2)
```

- **Database:** `mysqldump` database lokal lalu diimpor ke database cloud.
  Secara default tabel lama/debug (Laravel Telescope, cache, tabel duplikat)
  dilewati agar dump kecil (~10 MB). Dengan opsi **Mirror 100% (1:1)**
  (`OFFSITE_DB_INCLUDE_LEGACY=true`) **semua** tabel disalin sehingga salinan
  cloud identik byte-per-byte dengan lokal (dump ~125 MB; masih dalam kuota
  gratis Aiven 1 GB).
- Dump memakai `--hex-blob` supaya kolom biner (BLOB) tercatat sebagai literal
  heksadesimal dan byte-nya **identik** setelah diimpor ke MySQL (tanpa ini,
  `backup_logs.file_content` bisa berubah 2 byte karena escaping teks).
- Tabel `backup_logs` dicerminkan **terpisah** sesaat setelah run mencatat
  dirinya sendiri, agar salinan cloud selalu memuat baris log terbaru (termasuk
  run yang baru saja selesai).
- **File:** isi `storage/app/uploads` dicerminkan ke bucket S3-compatible dengan
  `minio-go` (hanya file baru/berubah yang diunggah).
- Setiap eksekusi dicatat di tabel `backup_logs` (kolom `backup_type = 'cloud'`).
- Status terakhir disimpan di `storage/app/offsite/status.json`.

---

## 2. Mengatur Lewat Antarmuka

1. Masuk sebagai admin.
2. Buka **Pengaturan → tab "Backup Cloud"**.
3. Isi kolom **Database Cloud** dan/atau **Storage Object (S3)**.
4. Klik **Uji Database** / **Uji Storage** untuk memastikan koneksi berhasil.
5. Klik **Simpan**.
6. Klik **Backup Sekarang** untuk menjalankan sekali (tidak menunggu jadwal).

Setelah **Aktifkan backup cloud** dicentang dan minimal satu target terisi,
penjadwal otomatis berjalan **setiap hari pada jam yang dipilih** (default 02:00).

---

## 3. Contoh Penyedia Gratis

### 3a. Database Cloud — Aiven for MySQL (gratis)

1. Daftar di <https://aiven.io>, buat layanan **MySQL** (paket Free, 1 GB).
2. Tunggu status **Running**, lalu buka halaman **Connection information**.
3. Salin nilai berikut ke form **Database Cloud**:

   | Kolom form         | Contoh Aiven                                              |
   |--------------------|-----------------------------------------------------------|
   | Host               | `mysql-xxxxxxxx-xxxx.b.aivencloud.com`                    |
   | Port               | `19160`                                                   |
   | Nama database      | `defaultdb`                                               |
   | Username           | `avnadmin`                                                |
   | Password           | `AVNS_...` (ambil dari halaman Connection information)     |
   | TLS                | `required` (wajib untuk Aiven)                            |

   > Aiven mewajibkan koneksi TLS. Pilih `required` (tanpa verifikasi CA) atau
   > `verify` bila CA disimpan di server. Untuk `verify`, isi kolom CA file
   > (Aiven menyediakan `ca.pem`).

Catatan: paket Free akan **tidur setelah idle** dan bangun lagi saat diakses.
Itu normal; backup harian tetap berjalan (koneksi pertama membangunkan server).

### 3b. Storage Object — Cloudflare R2 (gratis 10 GB) atau Backblaze B2

**Cloudflare R2**

1. Buka dasbor Cloudflare → **R2** → **Create bucket** (mis. `simarc-backup`).
2. Buka **Manage R2 API Tokens** → **Create API token** (izin *Object Read & Write*).
3. Catat **Access Key ID**, **Secret Access Key**, dan **endpoint**
   (bentuknya: `https://<account_id>.r2.cloudflarestorage.com`).
4. Isi form **Storage Object (S3)**:

   | Kolom form       | Nilai R2                                      |
   |------------------|-----------------------------------------------|
   | Endpoint         | `https://<account_id>.r2.cloudflarestorage.com` |
   | Region           | `auto`                                        |
   | Bucket           | `simarc-backup`                               |
   | Access Key       | Access Key ID                                  |
   | Secret Key       | Secret Access Key                              |

**Backblaze B2**

1. Buat bucket (mis. `simarc-backup`), catat **endpoint** (mis.
   `https://s3.<region>.backblazeb2.com`).
2. Buat **Application Key** (Access Key + Secret).
3. Region = region bucket (mis. `us-west-004`).

### 3c. Alternatif tanpa akun baru — rclone (Google Drive)

Jika di server sudah ada remote rclone yang terautentikasi (mis. Google Drive
yang diatur dengan `rclone config`), berkas arsip dapat dicadangkan ke sana
tanpa membuat akun baru:

1. Pastikan remote berfungsi: `rclone lsd gdrive:` (contoh nama remote `gdrive`).
2. Pada form **Storage Object (S3)**, pilih **Metode penyimpanan berkas** =
   *Otomatis* atau *rclone remote*, lalu isi:

   | Kolom form                    | Nilai                              |
   |-------------------------------|------------------------------------|
   | Metode penyimpanan berkas     | `Otomatis` atau `rclone remote`    |
   | rclone remote                 | `gdrive`                           |
   | Folder tujuan di remote       | `simarc-backup`                    |

3. Klik **Uji Storage** (menguji remote) lalu **Simpan**.

Perintah yang dijalankan: `rclone copy storage/app/uploads gdrive:simarc-backup`
(searah, tidak pernah menghapus apa pun di remote).

---

## 4. Kunci `.env` (untuk referensi / otomasi)

Semua pengaturan dapat juga diisi langsung di `.env` (file ini berisi sandi —
pastikan izin `chmod 600`):

```env
OFFSITE_ENABLED=true          # aktifkan penjadwal backup
OFFSITE_HOUR=2                # jam jalan (0-23, waktu lokal server)
OFFSITE_MINUTE=0              # menit jalan (0-59)
OFFSITE_BACKUP_DB=true        # salin database
OFFSITE_BACKUP_FILES=true     # salin file uploads

# Database cloud (MySQL/MariaDB)
OFFSITE_DB_HOST=mysql-xxxxxxxx.b.aivencloud.com
OFFSITE_DB_PORT=19160
OFFSITE_DB_NAME=defaultdb
OFFSITE_DB_USER=avnadmin
OFFSITE_DB_PASS=********
OFFSITE_DB_TLS=required       # disabled | preferred | required | verify
OFFSITE_DB_CA_FILE=           # path ca.pem (opsional, untuk TLS=verify)
OFFSITE_DB_INCLUDE_LEGACY=true # true = mirror 100% (semua tabel); false = lewati tabel legacy

# Object storage (S3-compatible)
OFFSITE_S3_ENDPOINT=https://<account_id>.r2.cloudflarestorage.com
OFFSITE_S3_REGION=auto
OFFSITE_S3_BUCKET=simarc-backup
OFFSITE_S3_ACCESS_KEY=********
OFFSITE_S3_SECRET_KEY=********
OFFSITE_S3_PREFIX=simarc      # awalan folder di dalam bucket
OFFSITE_S3_PATH_STYLE=false   # true untuk MinIO/B2, false/auto untuk R2

# Pemilihan backend berkas + rclone (mis. Google Drive)
OFFSITE_FILES_MODE=auto       # auto | s3 | rclone
OFFSITE_RCLONE_REMOTE=gdrive  # nama remote hasil `rclone config` (kosongkan bila pakai S3)
OFFSITE_RCLONE_PATH=simarc-backup
```

Tidak perlu restart setelah menekan **Simpan** — pengaturan dibaca ulang dari
`.env` pada tiap eksekusi dan tiap tick penjadwal.

---

## 5. Memulihkan (Restore) dari Cloud

**Database** (contoh, sesuaikan kredensial):

```bash
mysqldump --host=<HOST> --port=<PORT> --user=<USER> --ssl defaultdb \
  > simarc_restore.sql
mysql --user=chris --password simarc < simarc_restore.sql
```

**File:** unduh seluruh isi prefix dari bucket lalu salin ke
`storage/app/uploads` pada server.

---

## 6. Pemecahan Masalah

| Gejala | Penyebab umum | Solusi |
|--------|---------------|--------|
| "Koneksi database cloud gagal" | TLS salah / server tidur | Set TLS `required`; ulangi (Aiven bangun dari idle) |
| Impor gagal `DEFAULT uuid()` | MariaDB lokal → MySQL | Sudah ditangani otomatis (dump dinormalkan) |
| Impor gagal `sql_require_primary_key` | Aiven memaksa primary key | Sudah ditangani otomatis (relaksasi per sesi) |
| "Koneksi storage gagal" | Endpoint/region/bucket salah | Periksa endpoint & kunci; R2 region `auto` |
| "Koneksi rclone remote gagal" | Remote belum ada / token kedaluwarsa | Jalankan `rclone config` lalu `rclone lsd <remote>:` sebagai user server |
| Backup jalan tapi file 0 | `storage/app/uploads` kosong | Normal — belum ada file untuk dicadangkan |
| Backup tidak jalan otomatis | `OFFSITE_ENABLED=false` atau target kosong | Centang aktifkan & isi minimal satu target |

Log eksekusi dapat dilihat di:

```bash
journalctl -u simarc | grep -i offsite
cat storage/app/offsite/status.json
```

---

## 7. Catatan Teknis

- Klien `mysql`/`mysqldump` di server adalah **MariaDB**, sehingga argumen TLS
  disesuaikan otomatis (`--ssl` / `--skip-ssl`), bukan `--ssl-mode` milik MySQL.
- Tabel yang **dilewati** dari dump saat `OFFSITE_DB_INCLUDE_LEGACY=false`:
  `telescope_entries`, `telescope_entries_tags`, `telescope_monitoring`, cache
  AI/analytics, `failed_jobs`, `jobs`, `migrations`, `tenants`, dan tabel
  duplikat bentuk jamak (`arsips`, `jenis_arsips`, dll). Dengan
  `OFFSITE_DB_INCLUDE_LEGACY=true` **tidak ada** tabel yang dilewati (mirror 1:1).
- Agar salinan cloud benar-benar identik, opsi **Mirror 100% (1:1)** disarankan
  aktif: seluruh 71 tabel disalin dan data diverifikasi byte-per-byte.
- Penjadwal berjalan **di dalam proses server** dan tidak aktif pada mode klien
  atau saat dijalankan di Vercel.
- Untuk backend rclone, binary `rclone` harus tersedia di `PATH` user yang
  menjalankan server (unit systemd memakai `User=chris`), dan remote sudah
  dikonfigurasi lewat `rclone config` milik user tersebut.
- Karena `backup_logs.file_content` bisa besar, `max_allowed_packet` server
  cloud harus memadai; Aiven default 64 MB (cukup untuk blob 9 MB yang menjadi
  ~19 MB saat di-hex). Backup lokal di menu **Backup** sengaja **tidak** memakai
  `--hex-blob` agar tetap bisa di-restore ke MariaDB lokal yang batasnya 16 MB.
