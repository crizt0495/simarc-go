# SIMARC — Instalasi & Update Windows (Standalone, Tanpa Install Ulang)

Paket ini adalah **installer sekaligus updater** untuk Windows. Bisa dipakai
untuk komputer yang menjalankan SIMARC **offline-first** (server lokal +
database sendiri) maupun mode klien.

---

## 0) Installer SATU FILE (.exe) — DIREKOMENDASIKAN

Cukup **satu berkas**: `SIMARC-Setup.exe` (biner amd64; varian `SIMARC-Setup-arm64.exe`
tersedia bila diinginkan). File ini **sudah memuat seluruh aplikasi** di dalamnya
(biner server, web/template, dokumentasi, `.env.example`, skrip integrasi) —
**tidak ada folder lain yang perlu disalin**.

Cara pakai di komputer lain:
1. Salin **`SIMARC-Setup.exe`** (satu file) ke komputer tujuan.
2. Klik dua kali file tersebut.
3. Selesai — aplikasi terpasang, shortcut Start Menu/Desktop dibuat.

> Installer ini juga **memuat database portabel (MariaDB)**. Pada mode
> **offline mandiri**, aplikasi langsung punya database sendiri — **tanpa perlu
> memasang MySQL/MariaDB** dan tanpa bergantung ke komputer server.

**UPDATE revisi kode / versi baru:** cukup salin exe versi baru → klik dua kali
lagi. File aplikasi ditimpa in-place; **`.env` dan `storage\` tidak pernah
disentuh**. Tidak perlu uninstall/reinstall.

> Alternatif paket ZIP (folder) tetap tersedia bila diinginkan; isinya setara
> dengan apa yang di-embed di dalam exe tunggal ini.

**Ikon aplikasi:** file `SIMARC-Setup.exe` dan biner aplikasi
(`simarc-server.exe`) sudah membawa **ikon SIMARC** (multi-resolusi 16–256 px,
cocok untuk layar biasa maupun high-DPI). Shortcut Start Menu/Desktop juga
memakai ikon ini. Ingin mengganti ikon? Jalankan
`bash packaging/make-windows-icon.sh` setelah menyiapkan `rsrc`
(`go install github.com/akavel/rsrc@latest`) dan `python3` + Pillow — skrip itu
membangkitkan `packaging/windows/simarc.ico` dari
`web/static/images/icon-256.png` beserta file resource `.syso`, lalu bangun
ulang installer.

---

## 1) Menjalankan aplikasi (setelah install)

Dari menu **Start → SIMARC** (atau shortcut Desktop). Aplikasi membuka
jendela mandiri (Chromium/Edge). Butuh browser Chrome/Edge sudah terpasang.

Pada install pertama, Anda memilih **mode instalasi** (installer menanyakan,
atau tetapkan lewat `-Mode` / `simarc-install.txt` — lihat §2):

* **Klien ke server hub** — data & berkas tersimpan di **server pusat**;
  komputer ini **tidak butuh MySQL**. Alamat hub default
  `http://192.168.1.86:8080`.
* **Offline mandiri** — komputer ini menjalankan **database-nya SENDIRI**
  (MariaDB portabel ikut di bundel installer, port `3307`) sehingga **tetap
  bisa dipakai walau server pusat mati**. Tak perlu memasang MySQL manual.
* **Server lokal** — memakai MySQL/MariaDB yang **sudah terpasang sendiri**.

Semua pengaturan ada di `%LocalAppData%\SIMARC\.env` (dibuat otomatis dari
`.env.example` pada install pertama).

> **Catatan mode offline mandiri:** tiap komputer punya database **sendiri**,
> jadi data baru menyatu antar-komputer lewat **sinkronisasi otomatis**
> (last-write-wins) saat hub terjangkau. Cukup isi `SYNC_HUB_DB_HOST` (dll.) di
> `simarc-install.txt` — installer mengaktifkan sinkronisasi **dan** memberi
> tiap komputer identitas simpul yang **unik** secara otomatis. Lihat §2.2 dan
> `docs/OFFLINE-SYNC.md`.

## 2) Cara INSTALL pertama

1. Ekstrak `SIMARC-Windows-<versi>.zip` ke folder mana pun (mis. `C:\SIMARC-paket`).
2. Klik dua kali **`Install-SIMARC.bat`** (atau `Install-SIMARC.ps1`).
3. Selesai — shortcut dibuat, aplikasi siap dipakai.

Tidak butuh hak admin, tidak butuh Go. Install ke `%LocalAppData%\SIMARC`.

> **Pilihan mode (otomatis / massal):** salin `simarc-install.txt.example`
> menjadi `simarc-install.txt` **di samping** `SIMARC-Setup.exe`, isi `mode=`
> dan `hub=`, lalu jalankan installer — tidak akan ditanya lagi. Contoh
> `mode=offline` (tiap komputer punya database sendiri) atau `mode=client`
> dengan `hub=http://192.168.1.86:8080`. Bisa juga lewat parameter CLI:
> `SIMARC-Setup.exe -mode offline`.
>
> Tanpa berkas/parameter itu, installer **menanyakan** mode saat dijalankan.
> Pada pemasangan senyap (`-quiet`) perilaku lama dipertahankan: komputer hub
> (`192.168.1.86`) = server lokal, komputer lain = klien.

## 2.1) Komputer BARU tanpa database — "Database Tidak Tersedia"

Bila SIMARC dibuka di komputer yang **belum punya database** `simarc` (misal
komputer hub yang belum disiapkan), aplikasi **tidak lagi menampilkan layar
buntu**: otomatis diarahkan ke **halaman Konfigurasi Database**
(`/database-setup`). Alur lengkapnya:

1. Pasang **MySQL/MariaDB** di komputer itu (mis. MariaDB 10.11) dan pastikan
   servisnya berjalan.
2. Buka SIMARC — Anda diarahkan ke halaman **Konfigurasi Database**.
3. Isi form:
   - **Host**: `127.0.0.1`
   - **Port**: `3306`
   - **Nama database**: `simarc`
   - **Username / Password**: kredensial MySQL yang berhak membuat database
     (mis. `root`).
4. Klik **Uji Koneksi**. Jika database belum ada, muncul pesan
   *"Server database terjangkau, tetapi database "simarc" belum ada. Simpan
   untuk membuatnya otomatis."*
5. Klik **Simpan**. Aplikasi otomatis:
   - **membuat database `simarc`**,
   - menjalankan seluruh migrasi (membuat semua tabel),
   - membuat akun admin awal **`admin` / `admin`**.
6. Login `admin` / `admin`, lalu **ganti password** di menu Pengaturan.

Keamanan: saat database bermasalah, perubahan konfigurasi database hanya
diizinkan **dari komputer itu sendiri** (loopback) — klien lain di jaringan
tidak bisa mengubahnya. Saat database normal, hanya **admin yang login** yang
bisa mengubahnya dari menu **Pengaturan**.

> Tidak punya hak `root`? Minta admin MySQL membuatkan sekali saja:
> `CREATE DATABASE simarc CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;`
> lalu di halaman Konfigurasi Database cukup isi user/password yang berhak.

## 2.2) Sinkronisasi otomatis antar-komputer (mode offline mandiri)

Setiap komputer offline punya database sendiri; agar perubahannya **menyatu
otomatis**, arahkan semuanya ke satu **hub** (komputer server kantor atau MySQL
cloud). Cukup satu berkas config untuk dipakai semua komputer:

1. Di komputer **hub**, pastikan MySQL/MariaDB-nya bisa diakses jaringan dan
   ada user khusus sinkronisasi (mis. `simarc_sync`). Lihat
   `docs/OFFLINE-SYNC.md` §9.
2. Salin `simarc-install.txt.example` → `simarc-install.txt` **di samping**
   `SIMARC-Setup.exe`, lalu isi:

   ```ini
   mode=offline
   hub=http://192.168.1.86:8080
   SYNC_HUB_DB_HOST=192.168.1.86
   SYNC_HUB_DB_PORT=3306
   SYNC_HUB_DB_NAME=simarc
   SYNC_HUB_DB_USER=simarc_sync
   SYNC_HUB_DB_PASS=<password simarc_sync>
   SYNC_HUB_URL=http://192.168.1.86:8080
   SYNC_HUB_TOKEN=<rahasia bersama>
   ```

3. Salin `SIMARC-Setup.exe` + `simarc-install.txt` ke **tiap** komputer klien
   dan jalankan. Installer otomatis menulis `SYNC_ENABLED=true`,
   `SYNC_NODE_ID=<nama-komputer>` (unik per komputer, jadi aman dipasang massal
   dari satu berkas yang sama), dan `SYNC_INTERVAL_MINUTES=15`.

Setelah itu tiap komputer menggabungkan perubahannya **otomatis tiap 15 menit**
(atau lewat **Pengaturan → Sinkronisasi → Sinkronkan Sekarang**), selama hub
terjangkau. Bila hub tidak terjangkau, aplikasi **tetap jalan normal**; data
menyatu begitu jaringan kembali.

> Berkas arsip (uploads) disalin lewat **aplikasi hub** (port `8080`), jadi hub
> harus menjalankan SIMARC dan memakai `SYNC_HUB_TOKEN` yang sama.

Untuk memverifikasi logika penulisan `.env` tanpa Windows:
`pwsh -NoProfile -File packaging/windows/test-installer-sync.ps1`.

## 3) Cara UPDATE (revisi kode / versi baru) — TANPA uninstall

Prosedur **satu langkah yang sama** untuk tiap revisi:

1. Dapatkan paket **versi baru** — entah `SIMARC-Setup.exe` (satu file)
   atau `SIMARC-Windows-<versi>.zip`.
2. Jalankan installer versi baru itu: klik dua kali **`SIMARC-Setup.exe`**,
   atau klik dua kali **`Install-SIMARC.bat`** dari zip baru.
3. Selesai. Aplikasi = versi baru. **Tidak ada uninstall, tidak ada install ulang.**

Yang dijamin oleh updater (`SIMARC-Setup.ps1`):

| Item | Di-update? |
|---|---|
| Biner server + web/ + template/ + docs/ | ✅ Ditimpa (overwrite in-place) |
| `.env` (konfigurasi komputer ini) | ❌ **Dipertahankan** — tidak pernah ditimpa |
| `storage\` (arsip yang di-upload, cache) | ❌ **Dipertahankan** — tidak pernah dihapus |
| Shortcut + registry uninstall | ✅ Dipastikan ada / diperbarui |

Karena `.env` dan `storage\` **tidak ikut dalam paket zip**, mengganti paket
tidak akan pernah menghapus konfigurasi atau arsip — aman dijalankan berulang
kali (idempoten).

> Jika server sedang berjalan saat update, tutup dulu jendela SIMARC agar
> file biner bisa diganti (skrip akan memperingatkan port 8080).

## 4) Uninstall

- `Uninstall-SIMARC.bat` — menghapus shortcut + registry, **data tetap ada**.
- Untuk menghapus total: hapus `%LocalAppData%\SIMARC` secara manual.

## 5) Catatan keamanan

- **Paket TIDAK berisi `.env`** (kredensial). Setiap komputer mengisi sendiri
  `.env`-nya (dibuat dari `.env.example`).
- Jangan pernah membagikan `.env` Anda (berisi kredensial DB/hub/token).
- Untuk sinkron antar-komputer offline-first, isi blok `SYNC_*` di `.env`
  sesuai dokumen `docs/OFFLINE-SYNC.md`.

## 6) (Opsional) Membuat Setup.exe klasik dengan Inno Setup

Windows + Inno Setup 6:
1. Buka `simarc.iss` dengan Inno Setup Compiler (`ISCC.exe`).
2. Build → `dist\SIMARC-Setup.exe`.

Skrip `.iss` sudah diset agar instalasi/upgrade **in-place** (AppId tetap,
instal ke `%LocalAppData%\SIMARC`, `.env`/`storage` tidak pernah dipaket).

## 7) Membangun ulang installer (dari kode sumber, di Linux/GitHub Actions)

**Cara cepat — hasilkan SATU file & rapikan `dist/` otomatis:**
```bash
bash packaging/build-windows-installer.sh
# -> dist/SIMARC-Setup.exe   (dan dist/ dibersihkan: hanya 1 file)

# varian ARM64 (opsional):
SIMARC_BUILD_ARM64=1 bash packaging/build-windows-installer.sh
# -> dist/SIMARC-Setup-arm64.exe
```

**Cara lengkap (semua OS + zip, untuk rilis besar):**
```bash
bash packaging/build-all.sh                 # biner Linux/Windows/macOS + deb
bash packaging/make-windows-package.sh      # -> dist/SIMARC-Windows-<versi>.zip
                                            #    dist/SIMARC-Setup.exe          (satu file, amd64)
```

Installer `.exe` dibuat dengan cross-compile Go (lihat
`packaging/windows/installer/`) — **tidak butuh Inno Setup**, bisa dibangun
dari Linux. Hasilnya satu file yang memuat seluruh aplikasi.
Tambahkan `SIMARC_KEEP_DIST=1` bila ingin skrip tidak membersihkan `dist/`.