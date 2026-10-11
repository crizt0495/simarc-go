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

## 2) Windows (.exe Installer)

File: `dist/SIMARC-Setup-1.0.0.exe` (dibuat dengan Inno Setup Compiler di Windows)

Script Inno Setup: `simarc.iss`

Cara build di Windows:
1. Install [Inno Setup](https://jrsoftware.org/isinfo.php)
2. Buka `simarc.iss` dengan Inno Setup Compiler
3. Build → hasilnya `dist/SIMARC-Setup-1.0.0.exe`

Fitur installer:
- Membuat shortcut Start Menu: `SIMARC` & `Uninstall SIMARC`
- Opsi membuat shortcut Desktop
- Menambahkan entry Uninstall di Control Panel (HKEY_CURRENT_USER)
- Menjalankan `install-desktop.ps1` saat install untuk setup desktop integration
- Menjalankan `uninstall-desktop.ps1` saat uninstall

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
bash package-all.sh
```
