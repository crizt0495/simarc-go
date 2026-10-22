# SIMARC — Sinkronisasi Multi-Komputer (Offline-First)

Panduan agar **setiap komputer tetap bisa dipakai penuh walau tanpa jaringan
sama sekali**, lalu **menggabungkan perubahan antar komputer** saat jaringan
tersedia.

> Ringkas: *setiap komputer punya database sendiri (jalan offline), lalu
> menyatu lewat satu "hub" saat hub terjangkau.*

---

## 1. Konsep

| Peran | Apa yang dijalankan | Bisa offline? |
|-------|---------------------|:-------------:|
| **Komputer (node)** | Aplikasi SIMARC + **database lokal sendiri** (MySQL *atau* SQLite) | ✅ Ya, penuh |
| **Hub** (bisa salah satu komputer, atau cloud Aiven) | MySQL *atau* SQLite sebagai titik temu | ✅ (komputer lain tetap jalan, sekadar belum menyatu) |

Setiap komputer menulis ke database lokalnya sendiri. Saat hub **terjangkau**,
mesin sinkronisasi menukar perubahan dua arah:

- **Dorong (push)** — perubahan lokal dikirim ke hub.
- **Tarik (pull)** — perubahan dari komputer lain (yang sudah masuk hub) ditarik ke lokal.

Penggabungan memakai **waktu perubahan terakhir menang (last-write-wins,
LWW)**: baris dengan `updated_at` paling baru yang dipakai. Karena aplikasi
memakai **soft delete** (`deleted_at`) dan **UUID** sebagai primary key,
penghapusan dan penambahan antar komputer aman digabung.

> **Catatan jam:** LWW membandingkan waktu, jadi sebaiknya semua komputer
> mengaktifkan sinkronisasi waktu otomatis (NTP). Ada *jendela aman* (overlap)
> yang bisa diatur untuk toleransi selisih jam kecil.

---

## 2. Menyiapkan tiap komputer

> **Cara termudah (Windows, installer satu file).** Bila memakai
> `SIMARC-Setup.exe` mode **offline mandiri** (MariaDB sudah ikut di bundel,
> tak perlu pasang MySQL manual), cukup sediakan `simarc-install.txt` berisi
> `SYNC_HUB_DB_*` di samping installer. Installer otomatis menulis
> `SYNC_ENABLED=true`, `SYNC_NODE_ID` **unik per komputer** (dari nama
> komputer), dan `SYNC_INTERVAL_MINUTES=15`. Langkah manual di bawah hanya
> perlu bila memasang tanpa installer. Lihat
> `packaging/windows/README-WINDOWS.md` §2.2.

1. **Install MySQL/MariaDB** di komputer tersebut (bisa lokal).
2. Jalankan SIMARC dalam **mode server lokal** — artinya `SIMARC_SERVER_URL`
   **dikosongkan** (jangan mode klien), supaya komputer memakai DB-nya sendiri.
3. Isi `.env`:

```dotenv
# Database lokal komputer ini
DB_HOST=127.0.0.1
DB_DATABASE=simarc
DB_USERNAME=simarc
DB_PASSWORD=...

# Sinkronisasi
SYNC_ENABLED=true
SYNC_NODE_ID=komputer-front-office      # WAJIB berbeda tiap komputer
SYNC_INTERVAL_MINUTES=15

# Hub (titik temu) — sama di semua komputer
SYNC_HUB_DB_HOST=192.168.1.86           # atau host Aiven
SYNC_HUB_DB_NAME=simarc
SYNC_HUB_DB_USER=simarc
SYNC_HUB_DB_PASS=...
SYNC_HUB_DB_TLS=preferred               # LAN: preferred | cloud: required

# Berkas arsip (storage/app/uploads) — lewat aplikasi hub di port 8080
SYNC_FILES_ENABLED=true
SYNC_HUB_URL=http://192.168.1.86:8080   # alamat aplikasi hub
SYNC_HUB_TOKEN=<rahasia bersama>        # buat: openssl rand -hex 24
```

4. **Sinkronkan pertama kali** (mengisi hub / menarik data hub):

```bash
go run ./cmd/sync
```

Atau lewat aplikasi: **Pengaturan → Sinkronisasi → Sinkronkan Sekarang**.

---

## 2b. Mode SQLite (tanpa MySQL sama sekali)

Mode **SQLite** adalah.padanan MySQL untuk pemakaian desktop: tiap komputer
memakai **satu berkas database** sendiri, jadi tidak perlu memasang
MySQL/MariaDB dan tidak perlu menjalankan MariaDB portabel. Semua fitur
(aplikasi, input arsip, unggah berkas) tetap jalan penuh saat jaringan mati;
perubahan baru disinkronkan ke hub saat hub terjangkau.

```dotenv
# Tiap komputer: berkas database sendiri
DB_DRIVER=sqlite
DB_PATH=storage/simarc.db

# Sinkronisasi tetap sama seperti biasa
SYNC_ENABLED=true
SYNC_NODE_ID=komputer-front-office
SYNC_INTERVAL_MINUTES=15

# Hub: MySQL (Aiven/kantor) — sama untuk semua komputer
SYNC_HUB_DRIVER=mysql
SYNC_HUB_DB_HOST=...
SYNC_HUB_DB_NAME=simarc
SYNC_HUB_DB_USER=simarc
SYNC_HUB_DB_PASS=...
```

Yang perlu diketahui:

- **Driver SQLite murni Go** (`glebarez/sqlite`) — tanpa CGO, jadi bisa
  di-build untuk Windows dan Linux tanpa toolchain tambahan.
- **Mode SQLite otomatis melewati MariaDB portabel** yang biasa dipakai
  installer "offline mandiri"; tidak ada proses database tambahan yang jalan.
- **Skema tabel dibuat dari model GORM** — sama dengan MySQL, termasuk tabel
  yang belum ada di hub. Bila sebuah tabel belum ada di hub, node SQLite
  membuat ulang skemanya dari model sehingga kolomnya identik dengan
  instalasi MySQL (`id char(36)`, foreign key, indeks).
- **Waktu disimpan sebagai teks** berformat `2006-01-02 15:04:05.999999999-07:00`.
  Perbandingan "terbaru menang" memakai urutan leksikografis, jadi **semua
  komputer sebaiknya memakai zona waktu yang sama** (WIB) dan NTP aktif.
- **Hub boleh juga SQLite** bila instalasi tanpa server database:

  ```dotenv
  SYNC_HUB_DRIVER=sqlite
  SYNC_HUB_DB_PATH=D:/simarc/simarc-hub.db
  ```

  Handy untuk instalasi kecil (satu komputer jadi titik temu) dan untuk
  pengujian tanpa jaringan.

> Beralih dari MySQL ke SQLite **tidak** memindahkan data lama. Cara paling
> aman: pakai SQLite sebagai database baru, lalu biarkan sinkronisasi menarik
> data dari hub (jalankan "Sinkronkan Sekarang" sekali dengan hub terjangkau).

---

## 3. Memilih hub

- **Komputer server di kantor** (mis. `192.168.1.86`) — sederhana; hanya perlu
  komputer itu menyala saat komputer lain ingin menyatu. Untuk **baris
  database**, hub **tidak** perlu menjalankan aplikasi khusus: cukup MySQL-nya
  dapat diakses jaringan. Tetapi untuk **berkas arsip**, hub harus menjalankan
  **aplikasi SIMARC** (port `8080`) karena berkas disalin lewat aplikasi, dan
  hub perlu mengisi `SYNC_HUB_TOKEN` (rahasia bersama) agar bisa melayani.

  Di hub, isi `.env` hub-nya sendiri (DB lokal) dan **kosongkan** `SYNC_HUB_DB_*`
  bila ia tidak perlu menarik dari siapa pun. Komputer lain mengarah ke hub.
  Agar hub juga ikut menggabungkan perubahan komputer lain, hub cukup
  **dikunjungi tiap node** (node mendorong perubahannya ke hub; hub sendiri
  menarik saat ada node lain yang menjalankan sync). Untuk hub yang juga
  menjadi penulis aktif, arahkan hub ke salah satu node atau ke dirinya sendiri
  melalui kanal terpisah — paling mudah: **pakai cloud (Aiven) sebagai hub**.

- **Cloud (Aiven)** — hub selalu menyala, cocok bila kantor tidak punya
  komputer yang selalu hidup. Sinkronisasi butuh internet saat proses berjalan,
  tetapi aplikasi tetap jalan tanpa internet.

Sinkronisasi **tidak wajib** ke satu hub saja; model paling aman dan
dianjurkan adalah **satu hub untuk semua** (bintang), bukan saling-silang.

---

## 4. Tabel yang disinkronkan

Strategi per tabel ditentukan otomatis dari skema:

| Strategi | Syarat | Cara | Penghapusan |
|----------|--------|------|:-----------:|
| **soft** | punya `deleted_at` | delta `updated_at`/`deleted_at`, LWW | ✅ ikut (tombstone) |
| **append** | tabel log (daftar di kode) | delta `created_at`, insert-once | ❌ (riwayat tak diubah) |
| **keyed** | punya PK, tanpa `deleted_at` | delta `updated_at`/`created_at`, upsert by PK | ✅ ikut (tombstone) |

Sejak **v2**, PK dari empat tabel yang wajib menyatu antar komputer —
`jenis_arsip`, `berita_acara_items`, `pemusnahan_arsip_items`,
`blockchain_audits` — dikonversi dari `AUTO_INCREMENT` ke UUID `char(36)`
(baris lama diberi UUID deterministik, lihat §5.1). Keempat tabel itu kini
ikut disinkronkan ber-strategi **keyed**, termasuk penambahan/penghapusan
item berita acara dan pemusnahan lintas komputer.

Baris database di atas disinkronkan **lewat MySQL**. **Berkas arsip**
(`storage/app/uploads`) disinkronkan **terpisah** lewat aplikasi hub (lihat §7).

Yang **tidak** disinkronkan secara sengaja:

- **Cache & infrastruktur**: `*_cache`, `analytics_cache`, `telescope_*`,
  `jobs`, `failed_jobs`, `import_export_jobs`, `migrations`,
  `schema_migrations`, `ocr_temp_images`, dan tabel pembukuan lokal
  `sync_state`, `sync_runs`, `sync_keys` (`sync_tombstones` justru **ikut**
  disinkronkan agar penghapusan tersebar).
- **Tabel duplikat warisan (plural)**: `arsips`, `kode_klasifikasis`,
  `unit_kerjas`, `jenis_arsips`, `jadwal_retensis`, `jadwal_retensi_arsips`,
  `pemberkasans`, `pemusnahan_arsips`, `permission_roles`.
- **Tabel kunci UUID yang isinya lokal per komputer**: `api_tokens` (rahasia),
  `search_logs`, `webhooks`, `webhook_logs`, `workflows`,
  `workflow_executions`, `compliance_risks`, `compliance_scores`,
  `digital_signatures`, `leaderboard_stats` — jarang/bernilai hanya di
  komputer masing-masing, sengaja tidak digabung (lihat §5.1).

---

## 4b. Perbaikan penting: kolom `DEFAULT_GENERATED` (data klien = 0)

**Gejala:** komputer klien (SQLite) berhasil menarik tabel master (kode
klasifikasi, user, unit kerja, dil.) tetapi **tabel `arsip` tetap 0** — sehingga
terlihat "datanya 0".

**Sebab:** saat membaca struktur tabel hub (MySQL 8 / Aiven),
`information_schema.columns.EXTRA` untuk kolom PK `id char(36) DEFAULT (uuid())`
bernilai **`DEFAULT_GENERATED`**. Kode lama memakai uji
`strings.Contains(extra, "generated")` untuk membuang kolom *generated*, sehingga
kolom PK `id` **ikut terbuang**. Akibatnya pemeriksaan kecocokan primary key
(`restrictTablesToHub`) gagal dan **seluruh tabel `arsip` dilewati** dari
sinkronisasi.

**Perbaikan:** uji dipersempit ke **`virtual generated` / `stored generated`**
saja (`isTrulyGeneratedColumn`). `DEFAULT_GENERATED` kini diperlakukan sebagai
kolom biasa, jadi kolom `id` (dan kolom ber-DEFAULT lain) tetap ikut disalin.
Setelah perbaikan, klien baru menarik **seluruh** arsip dari hub.

> Dampak: penginstal sebelum perbaikan ini akan membuat klien dengan arsip
> kosong. Pasang ulang dengan `SIMARC-Setup.exe` terbaru bila menemui gejala ini.

---

## 5. Batasan (v1) — penting

1. **Empat tabel dulu tidak ikut karena PK `AUTO_INCREMENT`.** Beberapa node
   dapat membuat ID yang sama untuk baris berbeda sehingga saling menimpa, maka
   `jenis_arsip`, `berita_acara_items`, `pemusnahan_arsip_items`, dan
   `blockchain_audits` tidak ikut sinkron. Sejak **v2** PK-nya dikonversi ke
   UUID `char(36)` otomatis saat aplikasi menyala (migrasi
   `migrateAutoIncToUUID`): baris lama diberi UUID deterministik
   `00000000-0000-0000-0000-<hex id lama>`, `arsip.jenis_arsip_id` ditulis
   ulang mengikuti, dan FK `fk_arsip_jenis` dibuat ulang. Migrasi idempotent —
   aman dijalankan ulang / lanjut setelah terputus. Sisa tabel ber-PK
   auto-increment **sengaja tidak digabung** karena isinya lokal per komputer:
   `search_logs`, `webhooks`, `webhook_logs`, `workflows`,
   `workflow_executions`, `api_tokens` (rahasia), `compliance_risks`,
   `compliance_scores`, `digital_signatures`, `leaderboard_stats`.
2. **Penghapusan pada tabel tanpa `deleted_at` (strategi keyed) sudah
   tersebar** lewat mekanisme *tombstone* (lihat §7). Ini mencakup
   **`permission_role`**, jadi **pencabutan izin sekarang ikut** ke komputer
   lain: setiap komputer menyimpan cuplikan primary key (`sync_keys`) dan
   menulis penanda hapus (`sync_tombstones`) yang ikut disinkronkan. Bila baris
   yang sama dibuat ulang setelah tanggal penghapusan, baris baru menang
   (tombstone lama tidak menghapusnya).
3. **Berkas arsip** (`storage/app/uploads`) **ikut disinkronkan** lewat aplikasi
   hub (lihat §7). Batasnya: berkas hanya tersalin saat **aplikasi hub**
   terjangkau (port `8080`), berkas yang lebih besar dari `SYNC_FILES_MAX_MB`
   dilewati, dan sub-folder pada `SYNC_FILES_EXCLUDE` (default
   `ocr-temp,imports,restore`) tidak pernah disalin. Baris database tetap aman
   walau berkas belum tersalin karena hanya menyimpan *lokasi* berkas.
4. **Mode SQLite** menyimpan waktu sebagai teks dan membandingkan secara
   leksikografis. Semua komputer sebaiknya memakai **zona waktu yang sama**
   (WIB) dan NTP aktif; komputer dengan zona waktu berbeda bisa memilih
   "pemenang" yang keliru bila selisih jamnya besar. MySQL tidak punya
   batasan ini karena waktu disimpan sebagai `DATETIME` asli.

---

## 6. Cara memeriksa

- **Pengaturan → Sinkronisasi**: lihat node, hub, status, dan riwayat 10
  sinkronisasi terakhir (jumlah baris didorong/ditarik, tabel gagal).
- CLI: `go run ./cmd/sync` — mencetak ringkasan **baris dan berkas** yang
  didorong/ditarik, lalu keluar dengan kode ≠ 0 bila ada tabel yang gagal.
- API (login): `GET /api/sync/status`, `POST /api/sync/run`.
- Endpoint berkas (tanpa login, wajib header `X-Sync-Token`):
  `GET /api/sync/files/list`, `GET /api/sync/files/get?path=…`,
  `POST /api/sync/files/put?path=…&mtime=…`.
- Riwayat disimpan di tabel lokal `sync_runs`; posisi watermark di `sync_state`.

---

## 7. Teknis singkat

- Paket: `internal/syncdb` (mesin generik + registry tabel), `internal/config/sync.go`
  (konfigurasi), `cmd/sync` (CLI), `internal/handlers/sync.go` (API + simpan).
- Mesin menyalin skema tabel yang belum ada di hub (`CREATE TABLE` dari lokal)
  secara otomatis.
- Delta dibaca dengan `>=` *watermark* dikurangi *jendela aman*
  (`SYNC_OVERLAP_SECONDS`, default 30 detik) supaya baris di batas waktu tidak
  terlewat; pengiriman ulang aman karena LWW (idempoten).
- Penggabungan memakai `INSERT ... ON DUPLICATE KEY UPDATE` dengan kondisi
  `VALUES(updated_at) >= IFNULL(updated_at, ...)`.
- Delta dibaca dengan `COALESCE(updated_at, created_at) >= watermark`; baris
  warisan yang `updated_at`-nya `NULL` tetap terkirim (dibaca dari `created_at`,
  atau selalu dikirim bila kedua kolom waktu `NULL`) lalu di-upsert idempoten.
- **Penghapusan permanen (tombstone).** Untuk tabel tanpa `deleted_at`, setiap
  kali sinkron mesin membandingkan primary key saat ini dengan cuplikan
  sebelumnya (`sync_keys`, lokal saja). Key yang hilang = baris dihapus di
  komputer ini, lalu dicatat di `sync_tombstones` dan disinkronkan seperti tabel
  biasa. Penerima menghapus baris terkait kecuali baris dibuat ulang **setelah**
  `deleted_at` (perbandingan waktu `created_at`/`updated_at`).
- **Berkas arsip.** Setelah baris database, mesin menyalin berkas di
  `storage/app/uploads` lewat **aplikasi hub** (HTTP, header `X-Sync-Token`).
  Kesegaran memakai pasangan (ukuran, `mtime`) seperti pemeriksaan cepat rsync:
  berkas disalin bila belum ada di sisi lain atau jelas lebih baru; selisih
  waktu di bawah 2 detik dianggap sama (mencegah salin berulang karena beda
  jam). Berkas ditulis atomik (file sementara lalu di-`rename`) dan `mtime`
  dipertahankan. Endpoint hub: `internal/handlers/syncfiles.go`; mesin klien:
  `internal/syncdb/files.go`.
- Pengujian: `SIMARC_TEST_DSN_BASE='user:pass@tcp(127.0.0.1:3306)/' go test ./internal/syncdb/`
  membuat dua database sementara dan memverifikasi konvergensi (soft delete,
  update, log append, baris ber-`updated_at` NULL, dan pencabutan izin
  `permission_role` lewat tombstone). Sinkronisasi berkas diuji terpisah
  (`TestSyncFilesConverges`, `TestSyncFileEndpoints`).

---

## 8. Hubungan dengan fitur lain

- **Backup Cloud (offsite)** tidak berubah: database lokal tetap sumber utama,
  cloud hanya salinan pemulihan. Bila memakai **Aiven** sebagai *hub sinkronisasi*,
  Anda boleh memakai instance yang sama maupun instance terpisah dari offsite.
- **Mode klien** (`SIMARC_SERVER_URL`) tetap ada untuk kantor yang memilih
  satu server pusat + klien tipis. Sinkronisasi ini adalah alternatifnya: setiap
  komputer mandiri (offline) lalu menyatu otomatis.

---

## 9. Hub yang dipakai sekarang: database cloud (Aiven) + berkas LAN

Supaya tidak bergantung pada satu komputer kantor yang harus selalu menyala,
titik temu **baris database** dipindahkan ke **Aiven MySQL** (cloud), sedangkan
**berkas arsip** tetap lewat aplikasi hub LAN (port `8080`).

### Database hub — Aiven MySQL

- **Host**: `mysql-1d4b6727-christwnto-b045.b.aivencloud.com` (port `19160`)
- **Database**: `simarc_hub` (dipisah dari `defaultdb` yang dipakai backup
  offsite agar tidak bentrok)
- **User**: `avnadmin`
- **TLS**: `required` (Aiven memakai CA sendiri, jadi verifikasi penuh `true`
  akan gagal; `required` = selalu TLS tanpa verifikasi sertifikat)

Skema/tabel TIDAK ditulis di repo: dibuat sekali dari dump komputer yang sudah
berisi data (`mysqldump --no-data`), lalu tiap komputer mengisi barisnya lewat
sinkronisasi. Aiven MySQL 8 menyalakan `sql_require_primary_key`, dan semua
tabel SIMARC memang sudah punya PRIMARY KEY.

### Berkas arsip — aplikasi hub LAN

Aiven hanya MySQL, tidak bisa melayani berkas. Berkas arsip (PDF/gambar di
`storage/app/uploads`) tetap dikirim lewat aplikasi SIMARC di komputer hub LAN
(`http://192.168.1.86:8080`), sehingga komputer harus berada di WiFi/LAN yang
sama agar berkas ikut tersinkron. Bila kelak berkas ingin ikut cloud penuh,
perlu server aplikasi (VPS) yang menunjuk ke database Aiven yang sama.

### Isi ini di `.env` setiap komputer lain

```dotenv
SYNC_ENABLED=true
SYNC_NODE_ID=komputer-nama-unik        # WAJIB berbeda tiap komputer
SYNC_INTERVAL_MINUTES=15

# Database hub: Aiven (cloud)
SYNC_HUB_DB_HOST=mysql-1d4b6727-christwnto-b045.b.aivencloud.com
SYNC_HUB_DB_PORT=19160
SYNC_HUB_DB_NAME=simarc_hub
SYNC_HUB_DB_USER=avnadmin
SYNC_HUB_DB_PASS=<password avnadmin>
SYNC_HUB_DB_TLS=required

# Berkas arsip (uploads) — aplikasi hub LAN di port 8080
SYNC_FILES_ENABLED=true
SYNC_HUB_URL=http://192.168.1.86:8080
SYNC_HUB_TOKEN=<token dari server>
```

### Seed otomatis aman untuk komputer baru

Saat aplikasi menyala pada database yang **masih kosong**, ia memeriksa hub:

- Hub **masih kosong** (komputer pertama) → aplikasi menyemai peran/admin/lokasi
  bawaan secara lokal; komputer itu menjadi sumber data.
- Hub **sudah berisi** (komputer ke-2 dan seterusnya) → aplikasi **tidak**
  menyemai; ia menarik peran, pengguna, izin, dan seluruh data dari hub. Ini
  mencegah peran/admin ganda yang tadinya bisa muncul di tiap komputer.

Karena itu, akun admin dan daftar peran di semua komputer mengikuti data hub —
jangan mengubah nama pengguna admin di satu komputer secara terpisah.

### Catatan pemeliharaan hub Aiven

- **Filter IP**: bila Aiven diberi batas IP, tambahkan alamat publik tiap kantor
  (atau matikan filter) agar semua komputer bisa menyambung.
- **Batas koneksi**: sinkronisasi memakai satu koneksi singkat tiap 15 menit,
  jadi aman untuk belasan komputer.
- `defaultdb` **tetap** dipakai fitur backup offsite (dump penuh). Jangan
  mengarahkan sinkronisasi ke `defaultdb`.
- Bila lupa password `avnadmin`, reset lewat konsol Aiven lalu perbarui
  `SYNC_HUB_DB_PASS` (dan `OFFSITE_DB_PASS`) di `.env` semua komputer.

### Bila ingin kembali ke hub LAN (alternatif tanpa cloud)

Arahkan `SYNC_HUB_DB_*` ke MySQL komputer kantor (mis. `192.168.1.86:3306`,
database `simarc`, user `simarc_sync`, TLS `preferred`) dan `SYNC_HUB_URL` ke
`http://192.168.1.86:8080`. Komputer hub tidak perlu ikut menyinkron, tetapi
mengaktifkan `SYNC_HUB_DB_*` di hub (seperti sekarang) juga tidak masalah.

---

## 10. Mode Offline-First untuk komputer client

Karena klien memakai **database kecil (SQLite) per PC** dan sinkronisasi berjalan
di latar belakang, tidak ada **sama sekali** kata "menunggu server" di klien:

- **Buka aplikasi → langsung bisa dipakai.** Aplikasi membuka jendela segera
  setelah database lokal siap (dengan seed awal: admin, lokasi, jenis arsip).
  Sinkronisasi ke hub berjalan di latar belakang tanpa menahan jendela.
- **Server/WiFi mati → tetap bisa input arsip.** Semua input baru (arsip,
  klasifikasi, unit kerja) ditulis ke database SQLite lokal. Statusnya tetap
  tersedia, browsing arsip lokal tetap cepat, dan unggah berkas disimpan secara
  lokal.
- **Server/WiFi kembali hidup → otomatis menyatu.** Setiap
  `SYNC_INTERVAL_MINUTES` (default 15 menit) aplikasi mencoba sinkronisasi ke
  hub: dorong perubahan lokal, tarik perubahan komputer lain, dan perbarui
  `updated_at`. Ini berjalan di latar belakang tanpa perlu klik tombol.
- **Sinkron manual kapan saja:** buka **Pengaturan → Sinkronisasi →
  Sinkronkan Sekarang** untuk memaksa penyatuan segera.

Contoh `.env` untuk komputer client (SQLite, tanpa MySQL):

```dotenv
# Database lokal komputer ini (per-PC)
DB_DRIVER=sqlite
DB_PATH=storage/simarc.db

# Aktifkan sinkronisasi otomatis ke hub
SYNC_ENABLED=true
SYNC_NODE_ID=komputer-front-office   # WAJIB berbeda tiap komputer
SYNC_INTERVAL_MINUTES=15

# Hub (sama untuk semua klien) — alamat aplikasi hub di LAN
SYNC_HUB_URL=http://192.168.1.86:8080
SYNC_HUB_TOKEN=<rahasia bersama>

# Hub database (Aiven atau MySQL kantor) — titik temu data
SYNC_HUB_DB_HOST=mysql-1d4b6727-christwnto-b045.b.aivencloud.com
SYNC_HUB_DB_PORT=19160
SYNC_HUB_DB_NAME=simarc_hub
SYNC_HUB_DB_USER=avnadmin
SYNC_HUB_DB_PASS=...
SYNC_HUB_DB_TLS=required
```

### Berkas arsip (PDF/gambar) di mode offline-first

Berkas disimpan di `storage/app/uploads` lokal terlebih dahulu, lalu disalin ke
aplikasi hub (port `8080`) saat sinkromatrik berkas aktif dan hub terjangkau.
Bila hub mati, berkas tetap tersimpan lokal dan tersalin saat hub kembali
hidup.

> Ringkas: komputer client = SQLite lokal + sinkron otomatis ke hub saat online.
> Tidak ada tombol "hubungkan", tidak ada layar "menunggu server", dan aplikasi
> selalu bisa dibuka.

### Catatan berkas hub LAN (komputer ini)

- Aplikasi SIMARC di komputer ini (port `8080`) menjadi sumber berkas arsip.
  Token bersama disimpan di `.env` sebagai `SYNC_HUB_TOKEN`.
- Bila firewall aktif, buka port `8080` untuk LAN:
  `sudo ufw allow from 192.168.1.0/24 to any port 8080`.
- Sebaiknya **set IP statis / reservasi DHCP** untuk `192.168.1.86` agar
  `SYNC_HUB_URL` di komputer lain tetap benar.
- Komputer ini juga ikut menyinkron ke hub Aiven (`SYNC_NODE_ID=server-hub`),
  sehingga data yang diubah di sini ikut naik ke cloud.
