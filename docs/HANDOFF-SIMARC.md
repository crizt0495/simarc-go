# PENGINGAT PERCAKAPAN — SIMARC (serah-terima antar perangkat)

> Dokumen ini adalah **pengingat konteks** agar pekerjaan bisa dilanjutkan
> dari perangkat/komputer lain tanpa membaca ulang seluruh riwayat obrolan.
> Salin file ini bersama repo, atau ambil dari GitHub (`docs/HANDOFF-SIMARC.md`).

**Terakhir diperbarui:** 2026-10-23 07:40 WIB
**Status pekerjaan:** SELESAI & terverifikasi. Binary hub + installer sudah diperbarui
ke `1.0.0-66-ga427a3f`. Klien desktop akan memperbarui diri dari hub (maks. 30 menit)
dan data kembali 100% pada restart berikutnya.

---

## 1. Tujuan & Syarat

1. **Semua data harus 100% terbaca di komputer klien (SQLite) dan di aplikasi Android.**
2. Server = MariaDB/MySQL. Klien = **SQLite**, offline-first, sinkron otomatis, tanpa status "menunggu server".
3. APK Android harus bisa membaca data.
4. Installer harus mandiri (`SIMARC-Setup.exe`); pasang ulang **tidak menghapus** `.env` dan `storage/`.
5. Semua perubahan harus **ter-push ke GitHub** agar bisa dilanjutkan di perangkat lain.

---

## 2. Arsitektur yang dipakai (sudah final)

```
┌────────────────────┐        MySQL lokal (127.0.0.1:3306/simarc)   ← SERVER (authoritas data)
│  Komputer SERVER   │  ──────────────────────────────────────────▶  push
│  (Wine + port 8080)│
└─────────┬──────────┘
          │  aplikasi hub (port 8080) = sinkronisasi BERKAS (uploads/PDF)
          ▼
┌────────────────────┐
│  HUB MySQL Aiven   │  ◀── database cloud, titik temu semua node
│  simarc_hub        │
└─────────┬──────────┘
          │  pull (delta per tabel)
          ▼
┌────────────────────┐
│  Komputer KLIEN    │  SQLite lokal (storage/simarc.db) + sinkron otomatis
└────────────────────┘

Komputer ANDROID → APK (WebView) →(http://192.168.1.86:8080)→ SERVER
```

- **Hub database:** Aiven MySQL `simarc_hub` (bukan file LAN).
- **Hub berkas:** aplikasi di PC ini, `http://192.168.1.86:8080`.
- **APK Android** adalah *WebView client* ke server (lihat §6).

---

## 3. ✅ Akar masalah yang ditemukan (inti perbaikan)

**Gejala:** tabel `arsip` kosong di klien, hak akses (role↔permission) hilang,
`permission_role` kosong di hub, dan tombstone "muncul sendiri" terus-terusan.

**Akar masalah — perbedaan bentuk Primary Key `permission_role`:**

| Node | Bentuk PK `permission_role` (SEBELUM perbaikan) |
|---|---|
| Server MySQL & Aiven Hub | `id` (UUID) — tunggal |
| Klien SQLite | `(role_id, permission_id)` — **komposit** |

Rantainya:

1. `Role.Permissions []Permission` memakai tag `gorm:"many2many:permission_role;"`.
   Tanpa model join-table terdaftar, **GORM membuat tabel penghubung dengan PK komposit**.
2. Tombstone yang dibuat node ber-skema lama berisi **2 nilai**.
3. `applyTombstones(ctx, hub, ti)` memakai `ti` = bentuk PK **node itu sendiri**, bukan bentuk PK hub.
   → Tombstone 2-nilai **cocok** → **808 baris `permission_role` yang sah di hub dihapus**.
4. Tombstone itu disalin balik ke semua node (tabel `sync_tombstones` ikut disinkron) → **racun menyebar**.
5. Klien lama juga kehilangan barisnya sendiri karena bentuk kuncinya cocok.

**Pemicu tambahan yang ditemukan sesi ini:** ada **proses rogue** yang masih
menyinkronkan ke hub dengan skema SQLite lama:
`simarc-client` (PID 3621, service `simarc-client-test.service`) dan
`simarc-server` di `/tmp` (PID 41801/41803). Keduanya **sudah dimatikan**.
Selama keduanya hidup, hub akan terus "diracuni" ulang.

---

## 4. ✅ Perbaikan yang sudah diterapkan (di working tree → sudah di-commit & push)

| File | Perubahan |
|---|---|
| `internal/models/models.go` | `PermissionRole.BeforeCreate` (isi UUID) + `TableName() = "permission_role"` |
| `internal/database/database.go` | `setupJoinTables()` → `SetupJoinTable(&models.Role{}, "Permissions", &models.PermissionRole{})` (SQLite & MySQL) |
| `internal/database/migrate_sqlite.go` | `repairPermissionRoleSQLite()` — rebuild tabel SQLite ke bentuk kanonis + buang `sync_keys`/`sync_tombstones`/`sync_state` lama |
| `internal/database/migrate.go` | Migrasi MySQL juga buang `sync_tombstones` untuk `permission_role` |
| `internal/syncdb/tombstones.go` | `detectDeletes` membuang snapshot dengan jumlah kunci ≠ PK; `purgeStaleTombstones()` baru; `copyTombstones(..., keyCount)` filter |
| `internal/syncdb/engine.go` | `Run()` bersihkan tombstone usang (lokal & hub, pakai skema masing-masing); `syncKeyedTable` pakai `hubTI` untuk `applyTombstones` + filter `copyTombstones` + log peringatan beda bentuk PK |
| `internal/syncdb/tables.go` | `loadTableFor()` — ambil bentuk 1 tabel dari DB tertentu |
| `internal/syncdb/store.go` | Kolom `sync_keys.pk_sig` (tanda bentuk PK) + handling ALTER SQLite/MySQL |
| `internal/syncdb/tombstones_test.go` | Test regresi perubahan bentuk PK |
| `docs/OFFLINE-SYNC.md` | Section baru **§4c** menjelaskan akar masalah + bentuk kanonis + proteksi berlapis |

**Bentuk kanonis `permission_role` (WAJIB sama di server/hub/klien):**

```sql
id            char(36) NOT NULL PRIMARY KEY,   -- UUID
permission_id char(36) NOT NULL,
role_id       char(36) NOT NULL,
created_at    datetime NULL,
updated_at    datetime NULL,
UNIQUE (permission_id, role_id)
```

---

## 5. ✅ Status data (sudah diverifikasi bersih)

```
server permission_role : 808      hub permission_role : 808
server sync_tombstones: 0        hub sync_tombstones: 0
tabel gagal sinkron    : 0        didorong/ditarik    : ~22123 baris
```

Tabel lain server↔hub sudah identik: arsip 4172, kode_klasifikasi 3919,
users 5, roles 29, permissions 76, unit_kerja 6, lokasi_arsips 231,
jenis_arsip 2, jadwal_retensi 4, jadwal_retensi_arsip 80, arsip_versions 185,
pemberkasan 1, peminjaman_arsips 6, notifications 1044, blockchain_audits 7791,
activity_logs 1378, audit_logs 445, login_logs 1695.

---

## 6b. 🔴 Penyebab "aplikasi desktop komputer klien menampilkan 0 data"

Perangkat lunak komputer klien **memperbarui dirinya dari HUB**, bukan dari GitHub:

```
klien (setiap 30 menit) → GET http://192.168.1.86:8080/api/update/version
                        → GET http://192.168.1.86:8080/api/update/binary
                        → simpan ke tmp\src\..\simarc-server.exe.new
                        → run.bat menukar binary saat aplikasi dibuka berikutnya
```

Hub di komputer ini ternyata masih menjalankan binary **lama**
(`1.0.0-62-g9bf06bb`, dipasang 22 Okt 10:17 — sebelum perbaikan
`permission_role`/tombstone/DEFAULT_GENERATED). Akibatnya setiap klien yang
auto-update justru menerima binary lama itu dan tetap menampilkan 0 data,
walaupun perbaikan sudah ada di GitHub.

**Perbaikan yang dilakukan:**

1. Binary Windows baru dibangun dari commit `HEAD` (a427a3f) di worktree bersih
   (tanpa mengganggu perubahan sesi lain di working tree).
2. Binary dipasang ke instalasi hub:
   `~/.wine/drive_c/users/chris/AppData/Local/SIMARC/tmp/simarc-server.exe`
   (+ `VERSION` di root dan `tmp/`) → **1.0.0-66-ga427a3f**.
3. `simarc-hub.service` dijalankan ulang. Verifikasi:
   - `GET /api/update/version` → `{"version":"1.0.0-66-ga427a3f","os":"windows"}`
   - `GET /api/update/binary` → SHA-256 `235dd7c89d7a9719d19f529f4a4f14379800c144fcdd97de4d5d19cc77f39a60`
   - `GET /login` → 200, `GET /viewer` → 200 (433 KB, berisi 4.172 arsip).
4. Installer `SIMARC-Setup.exe` dibangun ulang dari commit yang sama
   (SHA-256 `39d2673160a71cf3205d0de4564dbe0b9953d1f730220b6c464933fb02cf30b7`)
   dan di-push ke GitHub.

**Yang perlu dilakukan di komputer klien:** cukup **tutup lalu buka lagi**
aplikasi SIMARC. Bila update belum terunduh (cek 30 menit), jalankan
`SIMARC-Setup.exe` versi baru. Setelah binary baru berjalan, migrasi SQLite
otomatis memperbaiki `permission_role`, membuang tombstone racun, dan menarik
ulang seluruh data dari hub.

### Uji verifikasi yang sudah dilakukan

| Uji | Hasil |
|---|---|
| Klien SQLite **baru** (DB kosong) menarik dari hub | ✅ `arsip=4172`, `permission_role=808`, `users=5`, `kode_klasifikasi=3919`, `blockchain_audits=7791`, `login_logs=1701`, … **identik dengan server**; `tabel_error=0` |
| Klien SQLite **lama yang rusak** (PK komposit + 808 tombstone racun + `arsip` kosong) dinaikkan ke binary baru | ✅ log `[MIGRASI] permission_role diperbaiki … (808 baris lama dilepas)`; `arsip` ditarik 4.172 baris; `permission_role` ditarik 808; 808 tombstone racun dibuang; **hub tetap utuh** (808/4172, tombstone 0) |

---

## 6. Penting: APK Android

File: `../SIMARC-Android.apk` (3.013.134 bytes) — **berada di luar git repo**
(harus disalin manual ke perangkat lain, atau disimpan di release GitHub).

Hasil inspeksi (unzip + strings pada `classes.dex`):

- Package `com.simarc.app`, activity tunggal `MainActivity`.
- Berbasis **Android WebView** (memakai `androidx.webkit`), **bukan** app native
  dengan database SQLite sendiri.
- Menyimpan alamat server di `SharedPreferences` (`simarc_prefs`),
  dialog: *"Masukkan alamat server SIMARC (contoh: http://192.168.1.86:8080)"*.
- Default: `http://192.168.1.86:8080`.
- Tidak ada aset `assets/`, `www/`, atau `index.html` di dalam APK.

**Kesimpulan:** APK membaca data **melalui server pusat** (MySQL) lewat WebView.
Artinya "100% terbaca di Android" berarti server pusat harus serves data lengkap
— yang sudah diperbaiki di atas. APK **tidak** punya database SQLite terpisah,
jadi tidak bisa offline penuh tanpa server.
> Catatan: tidak ada Android SDK maupun source project Android di repo ini,
> sehingga APK tidak bisa dibangun ulang di sini. Kalau akan dibuatkan versi
> offline (SQLite di HP), perlu source Android + SDK + keputusan tersendiri.

---

## 7. ✅ Cara melanjutkan di perangkat lain

```bash
# 1) Ambil perubahan
cd /path/ke/simarc-go
git pull origin master
git log --oneline -3

# 2) Pastikan bersih
gofmt -l ./internal/ && go build ./... && go vet ./internal/... && go test ./internal/...
```

Kalau `git pull` konflik, simpan dulu kerjaan lokal:
`git stash && git pull --rebase && git stash pop`.

---

## 8. ✅ Daftar periksa sesi ini (sudah dikerjakan)

- [x] Akar masalah `permission_role` ditemukan & diperbaiki di kode
- [x] Proteksi tombstone shape-aware di mesin sinkron
- [x] Migrasi SQLite untuk klien yang sudah terpasang
- [x] Proses rogue (`simarc-client`, `simarc-server`) dimatikan
- [x] Data dibersihkan & disinkron ulang → 808/808, tombstone 0/0
- [x] `docs/OFFLINE-SYNC.md` §4c ditulis
- [x] Build/vet/test bersih
- [x] Commit + push ke `origin/master`
- [x] Installer Windows dibangun ulang + SHA baru di `docs/UJI-WINDOWS-ASLI.md`
- [x] `simarc-hub.service` dijalankan ulang dengan binary baru
- [x] Uji klien SQLite **bersih** (100% baca data dari hub)
- [x] Uji klien SQLite **lama/rusak** → binary baru memulihkan data otomatis
- [x] Uji server LAN `http://192.168.1.86:8080` (login + data)
- [x] Binary hub diperbarui ke `1.0.0-66-ga427a3f` (auto-update klien berfungsi)
- [x] Installer final `39d2673…` (identik dengan binary hub) di-push
- [x] Dokumen pengingat ini dibuat

---

## 9. ✅ Perintah & kredensial (untuk lanjutan)

```bash
cd /home/chris/Documents/aplikasi/simarc/simarc-go
PW="$(grep '^DB_PASSWORD=' .env | cut -d= -f2-)"

# Server (lokal MySQL)
mysql -u chris -p"$PW" -h 127.0.0.1 simarc

# Hub (Aiven) — butuh --ssl (bukan --ssl-mode=REQUIRED)
mysql --ssl -h mysql-1d4b6727-christwnto-b045.b.aivencloud.com -P 19160 \
      -u avnadmin -p'AVNS_pi7_rHU5bZAqSVmiJyl' simarc_hub

# Sinkronisasi manual satu siklus
go run ./cmd/sync

# Build installer Windows satu-file
bash packaging/build-windows-installer.sh

# Service hub (Wine)
systemctl --user status simarc-hub.service --no-pager
systemctl --user restart simarc-hub.service
```

**Kredensial & alamat:**

| Item | Nilai |
|---|---|
| Repo | `git@github.com:crizt0495/simarc-go.git` (branch `master`) |
| MySQL lokal | `127.0.0.1:3306/simarc`, user `chris` (password di `.env`) |
| Aiven hub | `mysql-1d4b6727-christwnto-b045.b.aivencloud.com:19160/simarc_hub`, user `avnadmin`, TLS required (password di `.env`) |
| LAN server/hub HTTP | `http://192.168.1.86:8080` |
| `SYNC_HUB_TOKEN` | di `.env` (`SYNC_HUB_TOKEN=...`) |
| Service hub | `~/.config/systemd/user/simarc-hub.service` + `~/.local/bin/simarc-hub-run` |
| Installer | `../SIMARC-Setup.exe` dan `simarc-go/dist/SIMARC-Setup.exe` |

**Cara baca `.env` (server MySQL mode):** `~/.wine/drive_c/users/chris/AppData/Local/SIMARC/.env`

---

## 10. Jebakan yang harus diingat (penting!)

1. **Jangan** menjalankan proses simulasi klien lama (`/tmp/opencode/simarc-client`,
   service `simarc-client-test.service`) bersamaan dengan sinkronisasi hub.
   Skema SQLite lamanya meracuni hub. Kalau perlu uji klien, **build ulang**
   dari kode terbaru.
2. **Jangan** `pkill -f 'simarc-<nama>'` dari baris perintah yang mengandung
   nama itu — shell-nya ikut mati. Gunakan `kill <PID>`.
3. `mariadb` CLI: pakai `--ssl`; kolom `keys` itu *reserved* (jangan alias
   `AS keys`); `SELECT ROW_COUNT()` dipakai untuk cek hasil DELETE.
4. Proses Wine/Chrome yang dijalankan dari shell agent dapat kena SIGTERM —
   jalankan via `systemd-run --user` atau `setsid`.
5. **Jangan** mengubah `permission_role` kembali ke PK komposit.
6. Tabel ber-PK auto-increment **sengaja** tidak ikut sinkron (dokumentasi di
   `docs/OFFLINE-SYNC.md` §5.1) — itu bukan bug.
7. **Klien memperbarui binary dari HUB**, bukan dari GitHub
   (`/api/update/version` + `/api/update/binary`, lalu `run.bat` menukar
   `tmp/simarc-server.exe.new`). Setiap kali ada perbaikan kode, binary hub di
   Wine **wajib** ikut diganti (lihat §6b), kalau tidak klien akan terus
   memakai versi lama. Versi hub dicek di `GET /api/update/version`.

---

## 11. Berkas penting

| Berkas | Isi |
|---|---|
| `docs/OFFLINE-SYNC.md` | Dokumentasi sinkronisasi lengkap (baru: §4c perbaikannya) |
| `docs/UJI-WINDOWS-ASLI.md` | Checklist uji Windows + SHA installer terbaru |
| `docs/MULTI-KOMPUTER.md` | Panduan setup multi-komputer |
| `docs/HANDOFF-SIMARC.md` | Dokumen ini |
| `internal/syncdb/` | Mesin sinkron (engine, tables, tombstones, store) |
| `internal/database/migrate_sqlite.go` | Perbaikan skema SQLite untuk klien |
| `packaging/build-windows-installer.sh` | Build installer `SIMARC-Setup.exe` |
| `../SIMARC-Setup.exe` | Installer siap salin (di luar repo) |
| `../SIMARC-Android.apk` | APK Android (di luar repo) |
