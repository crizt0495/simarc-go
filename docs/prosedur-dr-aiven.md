# DR & Failover — SIMARC (MySQL lokal primer + Aiven cadangan)

> Dokumen prosedur untuk admin/operator. Arsitektur 2 database:
> **MySQL lokal** (primer; app menulis di sini) + **MySQL Aiven** (cadangan DR offsite).
>
> Dibuat: 2026-09-22 · Berlaku untuk pipeline `scripts/simarc-backup.sh`.

---

## 1. Arsitektur

| | MySQL Lokal | MySQL Aiven |
|---|---|---|
| Peran | **Primer** — aplikasi membaca/menulis | **Cadangan/DR** — salinan malam |
| Lokasi | Mesin lokal (127.0.0.1:3306) | Aiven Cloud (port 19160) |
| Mode | Writable | **Standby** (idealnya read-only; lihat §4) |
| Update data | Langsung dari app | Impor dump malam 02:15 (`cron`) |

Alur tiap malam (02:15):
1. `mysqldump` DB lokal → `storage/app/backups/database/backup_<ts>.sql.gz`
2. Catat ke tabel `backup_logs` (terlihat di UI `/backup`)
3. Rotasi: simpan 14 dump terbaru
4. **Push ke Aiven** (bila AIVEN_HOST & AIVEN_PASSWORD terisi):
   - Pra-cek `@@read_only` → lewati bila standby (hindari ERROR 1290 spam)
   - `DROP DATABASE` + `CREATE DATABASE` + impor dump → salinan selalu utuh & idempoten
     - Dump yang di-push **mengecualikan tabel debug internal Laravel** —
       `telescope_*` (`telescope_entries`, `telescope_entries_tags`, `telescope_monitoring`).
       Tabel itu berisi log debugging (bukan data arsip) & berukuran ±10 MB; mengecualikannya
       mencegah terminasi koneksi TLS Aiven saat impor (ERROR 2026 "unexpected eof").
   - Verifikasi jumlah baris `arsip` Aiven vs *snapshot lokal saat dump dibuat*
     (bukan count live — app boleh saja bertambah selama impor; selisih positif
     = arsip baru setelah dump, wajar, bukan kegagalan), catat di `backup.log`

Kredensial di `.env` (gitignored). Template: `.env.example` blok `AIVEN_*`.

---

## 2. Skenario & prosedur respons

### 2a. Aiven cadangan tidak terisi / error
- Cek log: `tail -n 40 storage/logs/backup.log`
- **`Aiven dilewati (read-only; matikan di konsol Aiven...)`** → layanan Aiven masih standby read-only.
  Backup lokal **tetap aman**. Untuk mengaktifkan impor: konsol Aiven → service MySQL → *Advanced configuration* →
  set **`read_only` = false** (atau Promote bila service adalah failsafe/replica) → Apply → cron malam berikutnya mengimpor.

### 2b. Failed over / "MySQL lokal tidak bisa diakses" (DR aktif)
> Tujuan: lanjutkan layanan dari Aiven selama lokal mati.

1. **Aktifkan tulis di Aiven** (konsol Aiven → Promote / `read_only=false` → Apply).
2. **Alihkan aplikasi** ke Aiven — edit `.env` dan restart:
   ```bash
   cp .env .env.lokal.bak
   sed -i \
     -e "s/^DB_HOST=.*/DB_HOST=$AIVEN_HOST/" \
     -e "s/^DB_PORT=.*/DB_PORT=$AIVEN_PORT/" \
     -e "s/^DB_USERNAME=.*/DB_USERNAME=$AIVEN_USERNAME/" \
     -e "s/^DB_PASSWORD=.*/DB_PASSWORD=$AIVEN_PASSWORD/" \
     -e "s/^DB_DATABASE=.*/DB_DATABASE=$AIVEN_DATABASE/" \
     .env
   pkill -f '^\./tmp/simarc-server'; sleep 1
   setsid ./tmp/simarc-server >/tmp/opencode/simarc-server.log 2>&1 < /dev/null & disown
   ```
3. **Verifikasi** aplikasi: buka UI, cek `/backup` & `/integrations` (pastikan tabel & baris utuh).
4. **Nonaktifkan writelock aplikasi** bila menulis: pastikan `DB_TLS=preferred` (sudah default) agar koneksi Aiven TLS aman.

> ⚠️ Aiven MySQL = **MySQL 8.4**; dump lokal berbasis **MariaDB**. Skrip sudah
> menormalisasi `DEFAULT uuid()` → `DEFAULT (uuid())` saat impor (baris `normalize_mysql8`).
> Tanpa normalisasi itu, impor ke Aiven gagal dengan `ERROR 1064` di baris `DEFAULT uuid()`.

### 2c. Rollback ke lokal (DR selesai, lokal sudah pulih)
1. Pastikan lokal pulih & berisi data terbaru (duplikat dari Aiven selama DR jika perlu).
2. Balik `.env` dari `.env.lokal.bak` (atau set DB_HOST kembali ke 127.0.0.1).
3. Restart server (langkah 2b.2). Verifikasi UI.
4. Aktifkan kembali cron malam (`crontab -e` pastikan baris `simarc-backup.sh` ada) — pipeline DR jalan normal.

### 2d. Perbaiki/migrasi Aiven penuh (reset salinan)
Jalankan manual sekali (idempoten — DROP+re-import):
```bash
cd /home/chris/Documents/aplikasi/simarc && bash scripts/simarc-backup.sh
```
Hanya impor ke Aiven bila layanan writable; bila read-only akan dilewati dengan pesan jelas.

---

## 3. Kredensial & keamanan
- Dump lokal: mode **600**; direktori backup mode **700**.
- `.env` berisi password DB (lokal & Aiven) — **JANGAN commit**. Sudah di gitignore.
- Koneksi Aiven memakai **`--ssl`** (TLS terenkripsi). Bila ingin verifikasi CA:
  simpan `ca.pem` Aiven lalu ubah opsi mysql di skrip dari `--ssl` → `--ssl-ca=... --ssl-verify-server-cert`.
- Ganti password admin default `admin/admin` bila belum (lihat §P3 keamanan).

---

## 4. Rekomendasi mode operasi (pilih 1)
- **A. Standby DR (default saat ini)** — Aiven read-only permanen; cron melewati impor;
  restore hanya via prosedur §2b. *Paling aman (salinan tak bisa rusak oleh impor salah).*
- **B. DR live (aktif false write)** — matikan `read_only` di konsol Aiven;
  cron tiap malam mengganti isi Aiven dengan dump lokal terbaru. *Salinan selalu segar,
  risiko: kesalahan konfigurasi bisa menimpa DR.*
- **C. Mirror dump** — nyalakan `BACKUP_MIRROR` ke lokasi cloud/USB; Aiven dipakai
  hanya saat DR riil.

Pilih sesuai SLA & keyakinan terhadap pipeline. Pipeline sudah siap untuk A & B
(guard read-only otomatis).
