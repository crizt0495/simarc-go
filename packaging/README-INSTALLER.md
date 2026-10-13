# SIMARC - Installer Desktop untuk Semua OS

## 1) Linux (.deb)

File: `dist/simarc_1.0.0_amd64.deb`

```bash
cd /home/chris/Documents/Aplikasi/simarc/dist
sudo dpkg -i simarc_1.0.0_amd64.deb
sudo apt-get install -f  # jika ada dependensi kurang
```

Setelah terinstall:
- Menu Aplikasi: `SIMARC` (Office/Database)
- Perintah terminal: `simarc`
- Shortcut Desktop (jika dibuat manual dari menu)

Uninstall:
```bash
sudo dpkg -r simarc
```

## 2) Windows — Installer SATU FILE (.exe) (DIREKOMENDASIKAN)

File: `dist/SIMARC-Setup-<versi>.exe` (+ `...-arm64.exe`)
(dibuat dengan `bash packaging/make-windows-package.sh`, dari Linux/CI)

Satu berkas ini **sudah memuat seluruh aplikasi** (biner server, web/, template,
dokumentasi, `.env.example`, skrip integrasi desktop). **Tidak perlu folder
lain** — cukup salin satu `.exe` ke komputer tujuan lalu klik dua kali.

**UPDATE revisi kode / versi baru — TANPA uninstall, TANPA install ulang:**
salin `SIMARC-Setup-<versi-baru>.exe` → klik dua kali. File aplikasi ditimpa
in-place; `.env` dan `storage\` **tidak pernah disentuh**.

Butuh admin? **Tidak.** Butuh Inno Setup? **Tidak** (dibangun dengan Go).
Install per-user ke `%LocalAppData%\SIMARC`.

## 2a) Windows — ZIP Standalone Installer/Updater (alternatif folder)

File: `dist/SIMARC-Windows-<versi>.zip`
(dibuat dengan `bash packaging/make-windows-package.sh`, bisa dari Linux/CI)

Isi paket: `Install-SIMARC.bat`, `SIMARC-Setup.ps1`, `run.bat`, `Uninstall-*.bat`,
`README-WINDOWS.md`, `web/`, `docs/`, `dist/simarc-server-windows-{amd64,arm64}.exe`,
`.env.example`, `VERSION`. **Tidak berisi `.env`/`storage`** (data pengguna).

Cara pakai di Windows:
1. Ekstrak ZIP → klik dua kali **`Install-SIMARC.bat`**.
2. Butuh hak admin? **Tidak.** Install per-user ke `%LocalAppData%\SIMARC`.
3. Butuh Go? **Tidak.** Biner rilis sudah tersedia.

**UPDATE revisi kode / versi baru — TANPA uninstall, TANPA install ulang:**
ekstrak ZIP versi baru → jalankan `Install-SIMARC.bat` lagi. Updater
menimpa file aplikasi in-place, sedangkan `.env` dan `storage\` (arsip)
**tidak pernah disentuh**. Idempoten (aman dijalankan berulang).

Detail lengkap: `packaging/windows/README-WINDOWS.md`.

## 2b) Windows (.exe Installer, opsional via Inno Setup)

File: `dist/SIMARC-Setup-1.0.0.exe` (dibuat dengan Inno Setup Compiler di Windows)

Script Inno Setup: `simarc.iss` (sudah diset agar **upgrade in-place**: AppId
tetap, install ke `%LocalAppData%\SIMARC`, `.env`/`storage` tidak dipaket).

Cara build di Windows:
1. Install [Inno Setup](https://jrsoftware.org/isinfo.php)
2. Jalankan `ISCC.exe packaging\simarc.iss`
3. Build → hasilnya `dist/SIMARC-Setup-1.0.0.exe`

Fitur installer:
- Membuat shortcut Start Menu: `SIMARC` & `Uninstall SIMARC`
- Opsi membuat shortcut Desktop
- Menambahkan entry Uninstall di Control Panel (HKEY_CURRENT_USER)
- Mengeksekusi `install-desktop.ps1` saat install / `uninstall-desktop.ps1` saat uninstall
- Menjalankan ulang Setup versi baru = **update in-place** (tanpa uninstall)

## 3) Script Installer (tanpa .exe/.deb)

### Windows (PowerShell)
- Install: `powershell -ExecutionPolicy Bypass -File .\install-desktop.ps1`
- Uninstall: `powershell -ExecutionPolicy Bypass -File .\uninstall-desktop.ps1`

### Linux (Bash)
- Install: `./install-desktop.sh`
- Uninstall: `./uninstall-desktop.sh`

## Binary Rilis (portable, cross-compile)

Sudah tersedia di `dist/`:
- `simarc-server-linux-amd64` / `simarc-server-linux-arm64`
- `simarc-server-windows-amd64.exe` / `simarc-server-windows-arm64.exe`
- `simarc-server-darwin-amd64` / `simarc-server-darwin-arm64`

## Build Semua

```bash
cd /home/chris/Documents/Aplikasi/simarc
bash packaging/build-all.sh
```
