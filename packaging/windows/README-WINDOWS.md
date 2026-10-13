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

**UPDATE revisi kode / versi baru:** cukup salin exe versi baru → klik dua kali
lagi. File aplikasi ditimpa in-place; **`.env` dan `storage\` tidak pernah
disentuh**. Tidak perlu uninstall/reinstall.

> Alternatif paket ZIP (folder) tetap tersedia bila diinginkan; isinya setara
> dengan apa yang di-embed di dalam exe tunggal ini.

---

## 1) Menjalankan aplikasi (setelah install)

Dari menu **Start → SIMARC** (atau shortcut Desktop). Aplikasi membuka
jendela mandiri (Chromium/Edge). Butuh browser Chrome/Edge sudah terpasang.

> Mode **server lokal** (default): komputer butuh **MySQL/MariaDB lokal** dan
> DB `simarc` dibuat. Semua pengaturan ada di `%LocalAppData%\SIMARC\.env`
> (dibuat otomatis dari `.env.example` pada install pertama).
>
> Mode **klien** (satu server pusat): isi `SIMARC_SERVER_URL=` di `.env`,
> lalu jalankan — tanpa MySQL.

## 2) Cara INSTALL pertama

1. Ekstrak `SIMARC-Windows-<versi>.zip` ke folder mana pun (mis. `C:\SIMARC-paket`).
2. Klik dua kali **`Install-SIMARC.bat`** (atau `Install-SIMARC.ps1`).
3. Selesai — shortcut dibuat, aplikasi siap dipakai.

Tidak butuh hak admin, tidak butuh Go. Install ke `%LocalAppData%\SIMARC`.

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

## 7) Membangun ulang paket (dari kode sumber, di Linux/GitHub Actions)

```bash
bash packaging/build-all.sh                 # biner + deb (sekali per rilis)
bash packaging/make-windows-package.sh      # -> dist/SIMARC-Windows-<versi>.zip
                                            #    dist/SIMARC-Setup.exe          (satu file, amd64)
                                            #    dist/SIMARC-Setup-arm64.exe    (opsional, ARM64)
```

Installer `.exe` dibuat dengan cross-compile Go (lihat
`packaging/windows/installer/`) — **tidak butuh Inno Setup**, bisa dibangun
dari Linux. Hasilnya satu file yang memuat seluruh aplikasi.