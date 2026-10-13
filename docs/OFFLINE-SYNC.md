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
| **Komputer (node)** | Aplikasi SIMARC + **MySQL lokal sendiri** | ✅ Ya, penuh |
| **Hub** (bisa salah satu komputer, atau cloud Aiven) | MySQL sebagai titik temu | ✅ (komputer lain tetap jalan, sekadar belum menyatu) |

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
```

4. **Sinkronkan pertama kali** (mengisi hub / menarik data hub):

```bash
go run ./cmd/sync
```

Atau lewat aplikasi: **Pengaturan → Sinkronisasi → Sinkronkan Sekarang**.

---

## 3. Memilih hub

- **Komputer server di kantor** (mis. `192.168.1.86`) — sederhana; hanya perlu
  komputer itu menyala saat komputer lain ingin menyatu. Hub **tidak** perlu
  menjalankan aplikasi khusus: cukup MySQL-nya dapat diakses jaringan.

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
| **keyed** | punya PK, tanpa `deleted_at` | upsert semua baris by PK | ⚠️ belum (lihat §5) |

Yang **tidak** disinkronkan secara sengaja:

- **Cache & infrastruktur**: `*_cache`, `analytics_cache`, `telescope_*`,
  `jobs`, `failed_jobs`, `import_export_jobs`, `migrations`,
  `schema_migrations`, `ocr_temp_images`, dan tabel `sync_*` (pembukuan lokal).
- **Tabel duplikat warisan (plural)**: `arsips`, `kode_klasifikasis`,
  `unit_kerjas`, `jenis_arsips`, `jadwal_retensis`, `jadwal_retensi_arsips`,
  `pemberkasans`, `pemusnahan_arsips`, `permission_roles`.
- **Tabel ber-PK auto-increment** — lihat batasan di §5.

---

## 5. Batasan (v1) — penting

1. **Tabel ber-PK `AUTO_INCREMENT` belum disinkronkan.** Beberapa node dapat
   membuat ID yang sama untuk baris yang berbeda, sehingga bisa saling
   menimpa. Yang terkena saat ini: `jenis_arsip`, `berita_acara_items`,
   `pemusnahan_arsip_items`, `blockchain_audits`, `search_logs`, `webhooks`,
   `webhook_logs`, `workflows`, `workflow_executions`, `api_tokens`,
   `compliance_risks`, `compliance_scores`, `digital_signatures`,
   `leaderboard_stats`. Solusi jangka panjang: ubah PK-nya ke UUID.
2. **Tabel tanpa `deleted_at` (strategi keyed)** belum menyebarkan
   **penghapusan** di v1 — penambahan/perubahan tetap tersinkron. Yang paling
   perlu diperhatikan: **`permission_role`** (pencabutan izin belum otomatis
   tersebar ke komputer lain; penambahan izin tersebar).
3. **Berkas arsip** (`storage/app/uploads`) belum ikut tersinkron. Saat ini
   berkas arsip masih **0**. Bila mulai dipakai, berkas perlu disinkronkan
   terpisah (mis. lewat rclone/offsite), sedangkan baris database-nya sudah
   aman karena hanya menyimpan *lokasi* berkas.

---

## 6. Cara memeriksa

- **Pengaturan → Sinkronisasi**: lihat node, hub, status, dan riwayat 10
  sinkronisasi terakhir (jumlah baris didorong/ditarik, tabel gagal).
- CLI: `go run ./cmd/sync` — mencetak ringkasan dan keluar dengan kode ≠ 0 bila
  ada tabel yang gagal.
- API (login): `GET /api/sync/status`, `POST /api/sync/run`.
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
- Pengujian: `SIMARC_TEST_DSN_BASE='user:pass@tcp(127.0.0.1:3306)/' go test ./internal/syncdb/`
  membuat dua database sementara dan memverifikasi konvergensi (termasuk soft
  delete, update, dan log append).

---

## 8. Hubungan dengan fitur lain

- **Backup Cloud (offsite)** tidak berubah: database lokal tetap sumber utama,
  cloud hanya salinan pemulihan. Bila memakai **Aiven** sebagai *hub sinkronisasi*,
  Anda boleh memakai instance yang sama maupun instance terpisah dari offsite.
- **Mode klien** (`SIMARC_SERVER_URL`) tetap ada untuk kantor yang memilih
  satu server pusat + klien tipis. Sinkronisasi ini adalah alternatifnya: setiap
  komputer mandiri (offline) lalu menyatu otomatis.

---

## 9. Hub yang sudah disiapkan (komputer server kantor)

Komputer ini sudah disiapkan sebagai **hub** LAN:

- **Host hub**: `192.168.1.86` (port `3306`)
- **Database hub**: `simarc`
- **User khusus sinkronisasi**: `simarc_sync` (hanya boleh dari jaringan
  `192.168.1.%`, hanya database `simarc`)
- **TLS**: `preferred`

Password user `simarc_sync` **tidak ditulis di dokumen ini** (agar tidak ikut
tersimpan di Git). Password disimpan di file khusus hanya-baca root di server,
dan dicetak saat penyiapan. Bila lupa, buat ulang dengan:

```bash
NEWPASS=$(openssl rand -hex 16)
mysql -u chris -p -e "ALTER USER 'simarc_sync'@'192.168.1.%' IDENTIFIED BY '${NEWPASS}';"
echo "Password baru: ${NEWPASS}"
```

### Isi ini di `.env` setiap komputer lain

```dotenv
SYNC_ENABLED=true
SYNC_NODE_ID=komputer-nama-unik        # WAJIB berbeda tiap komputer
SYNC_INTERVAL_MINUTES=15

SYNC_HUB_DB_HOST=192.168.1.86
SYNC_HUB_DB_PORT=3306
SYNC_HUB_DB_NAME=simarc
SYNC_HUB_DB_USER=simarc_sync
SYNC_HUB_DB_PASS=<password simarc_sync>
SYNC_HUB_DB_TLS=preferred
```

Setelah itu buka **Pengaturan → Sinkronisasi → Sinkronkan Sekarang**.

### Catatan pemeliharaan hub

- MariaDB di komputer ini kini mendengarkan di `0.0.0.0:3306`
  (`bind-address` di `/etc/mysql/mariadb.conf.d/50-server.cnf`). Salinan
  pengaturan lama: `50-server.cnf.simarc.bak`. Untuk menutup kembali ke LAN,
  kembalikan `bind-address = 127.0.0.1` lalu
  `sudo systemctl restart mariadb`.
- Karena alamat LAN bisa berubah (DHCP), sebaiknya **set IP statis** atau
  **reservasi DHCP** untuk komputer hub (`192.168.1.86`) agar alamat di `.env`
  komputer lain tetap benar.
- Hub **tidak perlu** menjalankan sinkronisasi (datanya sudah menjadi titik
  temu). Cukup MySQL-nya hidup dan terjangkau.
