# ===========================================================================
#  SIMARC — Standalone Installer / Updater for Windows (PowerShell)
#
#  INSTALL :
#     1. Ekstrak paket SIMARC-Windows-<versi>.zip ke folder mana pun.
#     2. Klik dua kali Install-SIMARC.bat  (atau jalankan script ini).
#
#  UPDATE (tanpa uninstall / tanpa install ulang) :
#     Ekstrak paket versi BARU di atas folder lama (atau di folder apa pun),
#     lalu jalankan Install-SIMARC.bat lagi. Script ini hanya MENIMPA file
#     aplikasi di tempat yang sama. Berkas DATA — .env dan storage\ —
#     TIDAK pernah dihapus/ditimpa, sehingga revisi kode bisa diganti
#     tanpa kehilangan konfigurasi maupun arsip.
#
#  Beberapa hal yang dijamin:
#     * Idempoten — menjalankan berkali-kali aman (install == update).
#     * Tanpa admin (install ke %LocalAppData%\SIMARC), tanpa perlu Go.
#     * Data pada %LocalAppData%\SIMARC\.env dan ...\storage tetap utuh.
#     * Shortcut Start Menu + Desktop (opsional) + entry Uninstall HKCU.
# ===========================================================================
param(
  [switch]$Quiet,
  [switch]$NoDesktopShortcut,
  [ValidateSet("auto","offline","client","local")] [string]$Mode = "auto",
  [string]$Hub = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Info($msg) { if (-not $Quiet) { Write-Host "[INFO] $msg" -ForegroundColor Cyan } }
function Write-Ok($msg)   { if (-not $Quiet) { Write-Host "[OK]   $msg" -ForegroundColor Green } }
function Write-Warn($msg) { if (-not $Quiet) { Write-Host "[WARN] $msg" -ForegroundColor Yellow } }
function Write-Err($msg)  { Write-Host "[ERR]  $msg" -ForegroundColor Red }

# ── Lokasi ──────────────────────────────────────────────────────────────────
$Src  = Split-Path -Parent $MyInvocation.MyCommand.Path   # folder paket (payload)
if (-not $env:LOCALAPPDATA) {
  $env:LOCALAPPDATA = Join-Path "$env:USERPROFILE" "AppData\Local"
}
$Dest = Join-Path $env:LOCALAPPDATA "SIMARC"              # folder instalasi tetap

# ── Alamat hub (server pusat) + berkas konfigurasi instalasi ─────────────────
#  Bisa diatur lewat parameter -Hub / -Mode, atau berkas `simarc-install.txt`
#  di samping installer (baris KEY=VALUE), contoh:
#      mode=offline
#      hub=http://192.168.1.86:8080
#      SYNC_HUB_DB_HOST=192.168.1.86
$CfgFile = Join-Path $Src "simarc-install.txt"
$Cfg = @{}
if (Test-Path $CfgFile) {
  foreach ($line in Get-Content -LiteralPath $CfgFile) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
      $Cfg[$Matches[1].ToUpper()] = $Matches[2].Trim()
    }
  }
}
if (-not $Hub -and $Cfg.ContainsKey("HUB")) { $Hub = $Cfg["HUB"] }
$HubIP  = "192.168.1.86"                       # default kantor (bisa ditimpa)
$HubURL = if ($Hub) { $Hub.TrimEnd("/") } else { "http://$($HubIP):8080" }

# Set-EnvKey menulis/mengganti satu kunci pada berkas .env (tanpa BOM).
function Set-EnvKey([string]$Path, [string]$Key, [string]$Value) {
  $lines = @(Get-Content -LiteralPath $Path -ErrorAction SilentlyContinue)
  $found = $false
  $out = @(foreach ($line in $lines) {
    if ($line -match ("^\s*" + [regex]::Escape($Key) + "=")) { $found = $true; "$Key=$Value" } else { $line }
  })
  if (-not $found) { $out += "$Key=$Value" }
  $enc = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllLines($Path, [string[]]$out, $enc)
}

# Test-LocalIPContains memeriksa apakah komputer ini memiliki alamat $ip.
function Test-LocalIPContains([string]$ip) {
  try {
    $localIPv4 = @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
      Where-Object { $_.IPAddress -notmatch '^(127\.|169\.254\.)' } |
      Select-Object -ExpandProperty IPAddress)
    return ($localIPv4 -contains $ip)
  } catch { return $false }
}

# Install-MariaDB mengekstrak bundel database portabel ke folder instalasi.
function Install-MariaDB {
  $zip = Join-Path $Src "mariadb-winx64.zip"
  if (-not (Test-Path $zip)) { return $false }
  $dbDir = Join-Path $Dest "mariadb"
  if (-not (Test-Path (Join-Path $dbDir "bin\mariadbd.exe"))) {
    Write-Info "Mengekstrak database portabel ke $dbDir ..."
    try {
      Expand-Archive -LiteralPath $zip -DestinationPath $dbDir -Force
      Write-Ok "Database portabel terpasang."
    } catch {
      Write-Err "Gagal mengekstrak database portabel: $_"
      return $false
    }
  } else {
    Write-Info "Database portabel sudah ada - dilewati."
  }
  return (Test-Path (Join-Path $dbDir "bin\mariadbd.exe"))
}

# ── Persyaratan payload ─────────────────────────────────────────────────────
$BatRun   = Join-Path $Src "run.bat"
$EnvEx    = Join-Path $Src ".env.example"
$BinAmd   = Join-Path $Src "dist\simarc-server-windows-amd64.exe"
$BinArm   = Join-Path $Src "dist\simarc-server-windows-arm64.exe"
$Missing  = @()
if (-not (Test-Path $BatRun)) { $Missing += "run.bat" }
if (-not (Test-Path $EnvEx))  { $Missing += ".env.example" }
if (-not (Test-Path (Join-Path $Src "web\templates\layouts\app.html"))) { $Missing += "web\templates" }
if (-not ((Test-Path $BinAmd) -or (Test-Path $BinArm))) { $Missing += "dist\simarc-server-windows-*.exe" }
if ($Missing.Count -gt 0) {
  Write-Err "Paket tidak lengkap — hilang: $($Missing -join ', ')"
  Write-Err "Ekstrak ZIP penuh dan jalankan Install-SIMARC.bat dari dalam folder paket."
  exit 1
}

# ── Mode: INSTALL baru atau UPDATE in-place? ────────────────────────────────
$IsUpdate = Test-Path (Join-Path $Dest ".env") -or (Test-Path (Join-Path $Dest "web\templates\layouts\app.html"))
$RunMode = if ($IsUpdate) { "UPDATE IN-PLACE" } else { "INSTALL BARU" }
Write-Info "============================================================"
Write-Info "  SIMARC - Mode $RunMode"
Write-Info "  Sumber : $Src"
Write-Info "  Tujuan : $Dest"
Write-Info "============================================================"

# Peringatan bila port aplikasi sedang dipakai (server masih berjalan).
try {
  $listeners = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
  if ($listeners) {
    Write-Warn "Server SIMARC tampaknya sedang berjalan (port 8080)."
    Write-Warn "Tutup jendela aplikasi SIMARC dulu agar file bisa diganti dengan aman."
    if (-not $Quiet) {
      $ans = Read-Host "Tutup dulu aplikasi SIMARC, lalu ketik Y untuk lanjut"
      if ($ans -notmatch "^[Yy]$") { Write-Info "Dibatalkan."; exit 0 }
    }
  }
} catch { }

# ── Salin file aplikasi (overwrite in-place) ────────────────────────────────
New-Item -ItemType Directory -Path $Dest -Force | Out-Null

$Copied = 0
foreach ($item in @("run.bat", ".env.example", "simarc.ico", "web", "docs", "dist", "packaging")) {
  $s = Join-Path $Src $item
  if (-not (Test-Path $s)) { continue }
  Copy-Item -Path $s -Destination $Dest -Recurse -Force -ErrorAction SilentlyContinue
  $Copied++
}
Write-Ok "File aplikasi disalin/ditimpa di $Dest ($Copied komponen)."

# ── Buat .env hanya bila belum ada (konfigurasi milik komputer ini) ─────────
$EnvDest = Join-Path $Dest ".env"
if (-not (Test-Path $EnvDest)) {
  Copy-Item -Path $EnvEx -Destination $EnvDest -Force

  # Tentukan mode: parameter -Mode > simarc-install.txt > tanya (interaktif) >
  # deteksi lama (komputer hub = server lokal, lainnya = klien).
  $chosen = $Mode
  if ($chosen -eq "auto" -and $Cfg.ContainsKey("MODE")) { $chosen = $Cfg["MODE"].ToLower() }
  if ($chosen -eq "auto" -and -not $Quiet) {
    Write-Host ""
    Write-Host "  Pilih mode instalasi SIMARC:" -ForegroundColor Cyan
    Write-Host "    [1] Klien ke server hub  (data di server; server harus menyala)"
    Write-Host "    [2] Offline mandiri      (database lokal otomatis; tetap jalan tanpa server)"
    $ans = Read-Host "  Pilihan [1/2] (Enter = 1)"
    if ($ans -eq "2") { $chosen = "offline" } else { $chosen = "client" }
  }
  if ($chosen -eq "auto") {
    if (Test-LocalIPContains $HubIP) { $chosen = "local" } else { $chosen = "client" }
  }

  if ($chosen -eq "offline") {
    if (Install-MariaDB) {
      $pw = -join ((48..57) + (97..122) | Get-Random -Count 24 | ForEach-Object { [char]$_ })
      Set-EnvKey $EnvDest "SIMARC_SERVER_URL" ""
      Set-EnvKey $EnvDest "DB_HOST"     "127.0.0.1"
      Set-EnvKey $EnvDest "DB_PORT"     "3307"   # port khusus bundel (hindari MySQL sistem)
      Set-EnvKey $EnvDest "DB_DATABASE" "simarc"
      Set-EnvKey $EnvDest "DB_USERNAME" "simarc"
      Set-EnvKey $EnvDest "DB_PASSWORD" $pw
      Set-EnvKey $EnvDest "SIMARC_EMBEDDED_DB" "1"
      # Salin kunci SYNC_* dari simarc-install.txt (bila ada) agar perubahan
      # antar-komputer menyatu lewat hub saat jaringan tersedia.
      foreach ($k in @($Cfg.Keys | Where-Object { $_ -like "SYNC_*" })) {
        Set-EnvKey $EnvDest $k $Cfg[$k]
      }
      Write-Ok "Mode OFFLINE MANDIRI: database lokal otomatis (port 3307)."
      Write-Info "Tanpa server pusat pun aplikasi tetap jalan."
    } else {
      Write-Warn "Bundel database portabel tidak ada - beralih ke mode klien."
      Set-EnvKey $EnvDest "SIMARC_SERVER_URL" $HubURL
      Write-Ok "Komputer ini dikonfigurasi sebagai KLIEN ke $HubURL."
    }
  }
  elseif ($chosen -eq "client") {
    Set-EnvKey $EnvDest "SIMARC_SERVER_URL" $HubURL
    Write-Ok "Komputer ini dikonfigurasi sebagai KLIEN ke $HubURL."
    Write-Info "Tidak perlu install MariaDB/MySQL di komputer ini."
  }
  else {
    Write-Ok "Mode SERVER LOKAL dipertahankan (butuh MySQL/MariaDB lokal)."
  }
} else {
  Write-Info ".env sudah ada - TIDAK ditimpa (dipertahankan)."
}

# ── Direktori runtime yang selalu boleh ditulis ─────────────────────────────
foreach ($d in @("tmp", "storage\app\uploads", "public\storage")) {
  New-Item -ItemType Directory -Path (Join-Path $Dest $d) -Force | Out-Null
}
# Pasang binary rilis sesuai arsitektur, SELALU menimpa versi lama (update).
$ExeDir = Join-Path $Dest "tmp"
$BinSrc = $BinAmd
foreach ($archEnv in @("PROCESSOR_ARCHITEW6432", "PROCESSOR_ARCHITECTURE")) {
  $arch = [Environment]::GetEnvironmentVariable($archEnv, "Process")
  if ($arch -eq "ARM64") { $BinSrc = $BinArm; break }
  if ($arch -eq "AMD64" -or $arch -eq "x86") { $BinSrc = $BinAmd; break }
}
if (-not (Test-Path $BinSrc)) { $BinSrc = $BinAmd }
if (Test-Path $BinSrc) {
  Copy-Item -Path $BinSrc -Destination (Join-Path $ExeDir "simarc-server.exe") -Force
  Write-Ok "Binary runtime diperbarui di tmp\simarc-server.exe"
}

# ── Shortcut Start Menu + Desktop ───────────────────────────────────────────
$Shell = New-Object -ComObject WScript.Shell
# Prefer the high-quality multi-resolution app icon shipped with the package;
# fall back to the web favicon if it is absent.
$IconPath = Join-Path $Dest "simarc.ico"
if (-not (Test-Path $IconPath)) { $IconPath = Join-Path $Dest "web\static\images\favicon.ico" }
$RunBat   = Join-Path $Dest "run.bat"

$StartMenuBase = [Environment]::GetFolderPath("StartMenu")
$StartMenuDir  = Join-Path $StartMenuBase "SIMARC"
New-Item -ItemType Directory -Path $StartMenuDir -Force | Out-Null
$LnkStart = Join-Path $StartMenuDir "SIMARC.lnk"
$Shortcut = $Shell.CreateShortcut($LnkStart)
$Shortcut.TargetPath = $RunBat
$Shortcut.WorkingDirectory = $Dest
$Shortcut.Description = "SIMARC - Arsip Record Center"
if (Test-Path $IconPath) { $Shortcut.IconLocation = $IconPath }
$Shortcut.Save()
Write-Ok "Shortcut Start Menu : $LnkStart"

if (-not $NoDesktopShortcut) {
  $DesktopDir = [Environment]::GetFolderPath("Desktop")
  $LnkDesktop = Join-Path $DesktopDir "SIMARC.lnk"
  $ShortcutD = $Shell.CreateShortcut($LnkDesktop)
  $ShortcutD.TargetPath = $RunBat
  $ShortcutD.WorkingDirectory = $Dest
  $ShortcutD.Description = "SIMARC - Arsip Record Center"
  if (Test-Path $IconPath) { $ShortcutD.IconLocation = $IconPath }
  $ShortcutD.Save()
  Write-Ok "Shortcut Desktop    : $LnkDesktop"
}

# ── Registry uninstall (HKCU) + versi terpasang ─────────────────────────────
try {
  $Ver = "1.0.0"
  $VerFile = Join-Path $Src "VERSION"
  if (Test-Path $VerFile) { $Ver = (Get-Content $VerFile -Raw).Trim() }
  $UninstallPath = "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\SIMARC"
  New-Item -Path $UninstallPath -Force | Out-Null
  Set-ItemProperty -Path $UninstallPath -Name "DisplayName"    -Value "SIMARC - Arsip Record Center"
  Set-ItemProperty -Path $UninstallPath -Name "DisplayVersion" -Value $Ver
  Set-ItemProperty -Path $UninstallPath -Name "Publisher"      -Value "Bakesbangpol Kota Probolinggo"
  Set-ItemProperty -Path $UninstallPath -Name "DisplayIcon"    -Value $IconPath
  Set-ItemProperty -Path $UninstallPath -Name "InstallLocation"-Value $Dest
  Set-ItemProperty -Path $UninstallPath -Name "UninstallString"-Value "powershell.exe -NoProfile -ExecutionPolicy Bypass -File `"$Dest\packaging\uninstall-desktop.ps1`" -Quiet -Confirm"
  Set-ItemProperty -Path $UninstallPath -Name "NoModify" -Value 1 -Type DWord
  Set-ItemProperty -Path $UninstallPath -Name "NoRepair" -Value 1 -Type DWord
  Write-Ok "Registrasi uninstall diperbarui (HKCU)."
} catch {
  Write-Warn "Gagal memperbarui registry uninstall: $_"
}

Write-Ok "Selesai ($RunMode). Jalankan aplikasi dari shortcut SIMARC."
Write-Info "PENTING: .env dan storage\ dipertahankan utuh pada setiap update."