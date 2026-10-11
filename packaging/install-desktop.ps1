# SIMARC Desktop Installer for Windows (PowerShell)
param(
  [switch]$Quiet,
  [switch]$NoDesktopShortcut
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Info($msg) { if (-not $Quiet) { Write-Host "[INFO] $msg" -ForegroundColor Cyan } }
function Write-Ok($msg) { if (-not $Quiet) { Write-Host "[OK]   $msg" -ForegroundColor Green } }
function Write-Warn($msg) { if (-not $Quiet) { Write-Host "[WARN] $msg" -ForegroundColor Yellow } }
function Write-Err($msg) { if (-not $Quiet) { Write-Host "[ERR]  $msg" -ForegroundColor Red } }

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$AppDir = $ScriptDir
$IconPath = Join-Path $AppDir "web\static\images\favicon.ico"
$BatPath = Join-Path $AppDir "run.bat"

if (-not (Test-Path $BatPath)) {
  Write-Err "run.bat tidak ditemukan di $AppDir"
  exit 1
}

$Shell = New-Object -ComObject WScript.Shell

# Start Menu
$StartMenuBase = [Environment]::GetFolderPath("StartMenu")
$StartMenuDir = Join-Path $StartMenuBase "SIMARC"
New-Item -ItemType Directory -Path $StartMenuDir -Force | Out-Null
$LnkStart = Join-Path $StartMenuDir "SIMARC.lnk"
$Shortcut = $Shell.CreateShortcut($LnkStart)
$Shortcut.TargetPath = $BatPath
$Shortcut.WorkingDirectory = $AppDir
if (Test-Path $IconPath) { $Shortcut.IconLocation = $IconPath }
$Shortcut.Description = "SIMARC - Arsip Record Center"
$Shortcut.Save()
Write-Ok "Shortcut Start Menu: $LnkStart"

# Desktop
if (-not $NoDesktopShortcut) {
  $DesktopDir = [Environment]::GetFolderPath("Desktop")
  $LnkDesktop = Join-Path $DesktopDir "SIMARC.lnk"
  $ShortcutD = $Shell.CreateShortcut($LnkDesktop)
  $ShortcutD.TargetPath = $BatPath
  $ShortcutD.WorkingDirectory = $AppDir
  if (Test-Path $IconPath) { $ShortcutD.IconLocation = $IconPath }
  $ShortcutD.Description = "SIMARC - Arsip Record Center"
  $ShortcutD.Save()
  Write-Ok "Shortcut Desktop: $LnkDesktop"
}

# Add uninstall registry entry (HKEY_CURRENT_USER)
try {
  $UninstallPath = "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\SIMARC"
  New-Item -Path $UninstallPath -Force | Out-Null
  Set-ItemProperty -Path $UninstallPath -Name "DisplayName" -Value "SIMARC - Arsip Record Center"
  Set-ItemProperty -Path $UninstallPath -Name "Publisher" -Value "Bakesbangpol Kota Probolinggo"
  Set-ItemProperty -Path $UninstallPath -Name "DisplayIcon" -Value "$IconPath"
  Set-ItemProperty -Path $UninstallPath -Name "InstallLocation" -Value $AppDir
  Set-ItemProperty -Path $UninstallPath -Name "UninstallString" -Value "powershell.exe -ExecutionPolicy Bypass -File `"$AppDir\uninstall-desktop.ps1`""
  Set-ItemProperty -Path $UninstallPath -Name "NoModify" -Value 1 -Type DWord
  Set-ItemProperty -Path $UninstallPath -Name "NoRepair" -Value 1 -Type DWord
  Write-Ok "Registrasi uninstall ditambahkan (HKCU)"
} catch {
  Write-Warn "Gagal menambahkan registry uninstall: $_"
}

Write-Ok "Instalasi desktop selesai."
Write-Info "Jalankan via: $LnkStart atau $LnkDesktop"
