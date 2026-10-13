# SIMARC — Instalasi & Update Windows (Standalone, Tanpa Install Ulang)

Paket ini adalah **installer sekaligus updater** untuk Windows. Bisa dipakai
untuk komputer yang menjalankan SIMARC **offline-first** (server lokal +
database sendiri) maupun mode klien.

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

1. Ekstrak paket **versi baru** (zip baru) ke folder mana pun.
2. Klik dua kali **`Install-SIMARC.bat`** dari paket baru itu.
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
2. Build → `dist\SIMARC-Setup-<versi>.exe`.

Skrip `.iss` sudah diset agar instalasi/upgrade **in-place** (AppId tetap,
instal ke `%LocalAppData%\SIMARC`, `.env`/`storage` tidak pernah dipaket).

## 7) Membangun ulang paket zip (dari kode sumber, di Linux/GitHub Actions)

```bash
bash packaging/build-all.sh                 # biner + deb (sekali per rilis)
bash packaging/make-windows-package.sh      # -> dist/SIMARC-Windows-<versi>.zip
```