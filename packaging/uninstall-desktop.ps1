# SIMARC Desktop Uninstaller for Windows (PowerShell)
param(
  [switch]$Quiet,
  [switch]$Confirm
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "SilentlyContinue"

function Write-Info($msg) { if (-not $Quiet) { Write-Host "[INFO] $msg" -ForegroundColor Cyan } }
function Write-Ok($msg) { if (-not $Quiet) { Write-Host "[OK]   $msg" -ForegroundColor Green } }
function Write-Warn($msg) { if (-not $Quiet) { Write-Host "[WARN] $msg" -ForegroundColor Yellow } }

if (-not $Confirm -and -not $Quiet) {
  $answer = Read-Host "Hapus shortcut Start Menu, Desktop dan registry uninstall SIMARC? (Y/N)"
  if ($answer -notmatch "^[Yy]$") {
    Write-Info "Dibatalkan."
    exit 0
  }
}

$Shell = New-Object -ComObject WScript.Shell

# Start Menu
$StartMenuBase = [Environment]::GetFolderPath("StartMenu")
$StartMenuDir = Join-Path $StartMenuBase "SIMARC"
if (Test-Path $StartMenuDir) {
  Remove-Item -Recurse -Force $StartMenuDir 2>$null
  Write-Ok "Dihapus: $StartMenuDir"
}
$LnkStart = Join-Path $StartMenuDir "SIMARC.lnk"
if (Test-Path $LnkStart) { Remove-Item -Force $LnkStart 2>$null }

# Desktop
$DesktopDir = [Environment]::GetFolderPath("Desktop")
$LnkDesktop = Join-Path $DesktopDir "SIMARC.lnk"
if (Test-Path $LnkDesktop) {
  Remove-Item -Force $LnkDesktop 2>$null
  Write-Ok "Dihapus: $LnkDesktop"
}

# Registry
$UninstallPath = "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\SIMARC"
if (Test-Path $UninstallPath) {
  Remove-Item -Recurse -Force $UninstallPath 2>$null
  Write-Ok "Dihapus registry uninstall (HKCU)"
}

Write-Ok "Uninstall desktop integration selesai."
Write-Info "Folder aplikasi tetap ada. Jika ingin menghapus sepenuhnya, hapus folder instalasi SIMARC ini."
