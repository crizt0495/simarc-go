# Mode Offline-First (Desktop) — Sinkronisasi SIMARC

> Tujuan: setiap PC kantor memiliki database lokal sendiri. Pengarsipan **tetap
> berjalan walau server pusat / jaringan / WiFi mati**. Ketika koneksi pulih,
> seluruh perubahan otomatis dikirim ke database pusat (MySQL) dan data pusat
> ditarik ke lokal — tanpa aplikasi server perantara.

## 1. Arsitektur

```
┌─────────────────────────────┐         LAN / Internet          ┌──────────────────────┐
│  PC A (desktop Wails)       │   koneksi langsung klien→MySQL  │   Server Pusat (MySQL)│
│                             │ ─────────────────────────────►  │                      │
│  SQLite lokal (primer)      │   push  (outbox + rescan)       │  DB pusat yang sama  │
│  seluruh fitur berjalan     │ ◄─────────────────────────────  │  dibaca cmd/server / │
│  normal dari sini           │   pull  (watermark per tabel)   │  deploy Vercel       │
│  mesin desksync background  │                                 │                      │
└─────────────────────────────┘                                 └──────────────────────┘
┌─────────────────────────────┐
│  PC B (desktop Wails)       │   skema sama, ClientID berbeda
└─────────────────────────────┘
```

- **SQLite lokal menjadi database primer**: semua handler (arsip, pemberkasan,
  pemusnahan, peminjaman, retensi, pengaturan) membaca/menulis ke lokal.
- **Kredensial `DB_*` pada `.env`** dalam mode ini berarti *database pusat*.
- Sync berjalan **langsung klien → MySQL pusat** (driver MySQL di dalam
  aplikasi desktop), tidak tergantung `cmd/server` hidup atau tidak.
- Build `cmd/server` dan deployment Vercel **tidak berubah** (tetap MySQL saja,
  tanpa SQLite).

## 2. Konfigurasi `.env`

| Variabel | Default | Arti |
|---|---|---|
| `DB_MODE` | `mysql` | `local` (atau `sqlite`) = mode offline-first desktop; nilai lain = perilaku web biasa (MySQL primer) |
| `SQLITE_PATH` | otomatis | Lokasi file SQLite lokal. Kosong → `<data-dir>/simarc-local.db` (data dir aplikasi desktop) |
| `SYNC_ENABLED` | `1` | Mesin sinkronisasi aktif (hanya relevan saat `DB_MODE=local`) |
| `SYNC_INTERVAL_SECS` | `60` | Jeda antar siklus sync di belakang |
| `SYNC_CLIENT_ID` | (uuid) | Identitas node; diisi otomatis UUID sekali saat boot pertama dan disimpan balik ke `.env` — jangan dibagikan antar PC |
| `SYNC_PULL_FILE_BLOBS` | `1` | Menarik isi lampiran (BLOB) dari pusat ke lokal |
| `SYNC_FILE_MAX_BYTES` | `12582912` (12 MB) | Lampiran yang lebih besar tidak dikemas ke BLOB (metadata tetap ikut sync; file perlu dikopi manual per PC) |

Catatan: pada **desktop**, `.env` dibaca dari *data dir* aplikasi (lihat
`internal/desktop`). Saat mode non-local, nilai-nilai `SYNC_*` diabaikan.

## 3. Cara Kerja Siklus

Setiap `SYNC_INTERVAL_SECS` (atau diklik manual dari badge):

1. **Buka koneksi pusat** (ping 5 dtk; gagal = siklus dilewati, aplikasi lokal
   tidak terpengaruh apa pun).
2. **Push lokal → pusat** untuk setiap tabel syncable:
   - *Rescan* baris dengan `updated_at > watermark` tabel, **∪**
   - *Outbox*: antrean perubahan yang dicatat hook GORM
     (`AfterCreate/AfterUpdate/AfterDelete` → tabel `sync_outbox`) —
     inilah yang menangkap **soft delete** (yang tidak mengubah `updated_at`).
   - Baris yang gagal terkirim tetap menunggu di outbox untuk siklus berikutnya.
3. **Pull pusat → lokal** per tabel memakai watermark khusus pull:
   baris dengan `updated_at > watermark` **atau** `deleted_at > watermark`.
4. **Lampiran** (`arsip.file_content`) ikut dua arah dengan batas
   `SYNC_FILE_MAX_BYTES`.
5. **Ringkasan blockchain**: aktivitas audit yang terjadi selama offline
   digabung menjadi **satu blok ringkasan per siklus** — riwayat tetap
   terlacak tanpa membanjiri rantai.

### Cara tabel diidentifikasi

- Tabel ber-PK UUID (`arsip`, `users`, dst.) — baris disalin 1:1.
- Tabel ber-PK auto-increment (`jenis_arsip`,
  `pemusnahan_arsip_items`, `berita_acara_items`) — identitas memakai kolom
  **`uuid`** baru; peta `id_lokal ↔ id_pusat` disimpan di `sync_id_maps`, dan FK
  (`arsip.jenis_arsip_id`) diremap otomatis ke **dua arah**.
- `permission_role` (many2many) — diganti penuh per siklus pull.

### Penyelesaian konflik (LWW + tombstone)

- Konflik isi → **last-write-wins** berdasarkan `updated_at`;
  timestamp setara = **pusat menang saat pull / dilewati saat push**.
- Konflik hapus → **tombstone menang berdasarkan `deleted_at`**:
  baris yang dihapus lebih baru akan menghapus salinan di sisi lain, meski
  `updated_at` tidak berubah (soft delete tidak memutakhirkan `updated_at`).
- Selama mesin menulis (penerapan pull, peta id, ringkasan chain) hook outbox
  dipause agar tidak mencatat perubahan milik mesin itu sendiri.

### Tabel yang tidak ikut sync

`login_logs`, `activity_logs`, `audit_logs`, tabel blockchain, tabel antrean
`backup_logs`/`jobs`, dan tiga tabel meta sync — riwayat teknis ini tetap
per-PC. (Blockchain audit tetap terwakili lewat ringkasan per siklus.)

## 4. Antarmuka Pengguna

Pada shell desktop ada **badge status sinkronisasi** di kanan-atas
(`web/static/js/sync-badge.js`, polling 15 detik):

- 🟢 **Online** — tersambung pusat; menampilkan jam sync terakhir.
- ⚪ **Offline** — pusat tidak terjangkau; aplikasi tetap normal, badge
  menampilkan jumlah perubahan yang menunggu.
- 🟠 **Sinkron…** — siklus sedang berjalan.
- 🔴 — terjadi error sync (detail di tooltip).
- **Klik badge** = sinkronkan sekarang.

Badge otomatis disembunyikan pada mode web biasa (`DB_MODE=mysql`).

API pendukung (butuh login):
- `GET  /api/sync/status` — ikhtisar engine (JSON untuk badge)
- `POST /api/sync/run`    — picu satu siklus segera

## 5. Persyaratan di Sisi Pusat

Database pusat harus menjalankan migrasi terbaru
(`SchemaVersion = 2026.10.22-01`) **satu kali** — mis. boot aplikasi server
baru, atau jalankan `cmd/server` sekali. Migrasi menambahkan kolom aditif:
`arsip.file_content` (LONGBLOB), kolom `uuid` pada
`jenis_arsip`/`pemusnahan_arsip_items`/`berita_acara_items` (dengan backfill
UUID), dan index `arsip(updated_at)`. Kredensial klien yang dipakai cukup
akun MySQL yang dapat SELECT/INSERT/UPDATE/DELETE pada database pusat.

> Jika pusat belum termigrasi, baris yang memerlukan kolom baru akan dilewati
> dengan log `[SYNC] ... dilewati` sampai migrasi dijalankan — tidak merusak.

## 6. Keterbatasan & Catatan

- **Roles/lokasi/unit kerja**: pada pull pertama baris pusat diterima "apa
  adanya" — bila lokal sudah memiliki baris dgn nama sama dari seed, dapat
  muncul duplikat bernama sama; gunakan salah satu (data tidak hilang).
- **Akun `admin` hasil seed lokal** pada PC yang pernah online: bila pusat
  juga punya `admin`, baris seed lokal dihapus otomatis pengambilan pertama
  (dicatat `sync_state` `__meta:local_admin_id`) agar tidak dobel login.
- Kolom password user ikut sync; **akun user bersifat pull-only** — perubahan
  akun dikelola di pusat supaya pusat menjadi acuan tunggal kebijakan akun.
- Lampiran > `SYNC_FILE_MAX_BYTES` tidak terbawa BLOB (baris ikut, file
  tidak). Untuk PC baru: kopi folder `storage/arsip` manual satu kali.
- Konflik yang terjadi **di menit yang sama** pada dua PC untuk baris yang
  sama diselesaikan LWW — data yang kalah masih tersimpan di sisi yang
  menulisnya (tidak dihapus), selain kasus tombstone.
- Tombstone (`deleted_at`) dipertahankan permanen; tidak ada vacuum otomatis.
- Perubahan pengguna yang terjadi **tepat bersamaan** dengan siklus berjalan
  tetap tertangkap rescan watermark siklus berikutnya.

## 7. Troubleshooting Singkat

| Gejala | Penyebab umum | Tindakan |
|---|---|---|
| Badge merah + tooltip error | kredensial `DB_*` salah / pusat down | cek `.env`, uji koneksi dari menu Pengaturan |
| `[SYNC] ... dilewati: ... unsupported/unknown column` | pusat belum termigrasi | jalankan migrasi versi terbaru di pusat |
| Data tidak muncul di PC lain | siklush interval belum jalan & offline | klik badge / tunggu `SYNC_INTERVAL_SECS` |
| Lampiran 404 di PC lain meski baris ada | file > batas BLOB atau `SYNC_PULL_FILE_BLOBS=0` | kopi file manual / naikkan batas |
