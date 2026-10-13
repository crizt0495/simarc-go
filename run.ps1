# ===========================================================================
#  SIMARC — Run Script for Windows (PowerShell)
#  Auto-detect LAN, build & run.
#  Mode KLIEN: bila SIMARC_SERVER_URL diisi di .env, server/database lokal
#  TIDAK dijalankan dan aplikasi hanya membuka jendela ke server pusat.
#  Usage: .\run.ps1
# ===========================================================================

$APP_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $APP_DIR

Write-Host ""
Write-Host "  ============================================" -ForegroundColor Cyan
Write-Host "     SIMARC -- Arsip Record Center" -ForegroundColor Cyan
Write-Host "  ============================================" -ForegroundColor Cyan
Write-Host ""

# ── 1. Environment (buat .env bila belum ada, lalu muat) ──
if (-not (Test-Path ".env")) {
    if (Test-Path ".env.example") {
        Copy-Item ".env.example" ".env"
        Write-Host "[OK]    .env dibuat dari .env.example" -ForegroundColor Green
    } else {
        Write-Host "[WARN] Membuat .env default..." -ForegroundColor Yellow
        @"
APP_NAME="SIMARC-Arsip Record Center"
APP_URL=http://localhost:8080
APP_PORT=8080
APP_DEBUG=true
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=simarc_db
DB_USERNAME=root
DB_PASSWORD=
SESSION_KEY=simarc-default-key
# Backup disimpan di storage/app/backups/database/
"@ | Out-File -FilePath ".env" -Encoding utf8
        Write-Host "[OK]    .env default dibuat" -ForegroundColor Green
    }
}

# Load env
Get-Content ".env" | ForEach-Object {
    if ($_ -match "^(.*?)=(.*)$") {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], "Process")
    }
}

# Helper: ambil nilai env tanpa tanda kutip / spasi berlebih.
function Get-EnvRaw($Name) {
    $v = [Environment]::GetEnvironmentVariable($Name)
    if ($null -eq $v) { return "" }
    return $v.Trim().Trim('"')
}

# ── 2. Mode KLIEN (server pusat): tanpa server/database lokal, tanpa Go ──
$SERVER_URL = (Get-EnvRaw "SIMARC_SERVER_URL").TrimEnd("/")
if ($SERVER_URL) {
    Write-Host "[INFO]  Mode KLIEN - server pusat: $SERVER_URL" -ForegroundColor Cyan

    $WINDOW_MODE = (Get-EnvRaw "SIMARC_WINDOW").ToLower()
    $WINFLAG = "--start-maximized"
    switch ($WINDOW_MODE) {
        "normal"     { $WINFLAG = $null }
        "fullscreen" { $WINFLAG = "--start-fullscreen" }
        "kiosk"      { $WINFLAG = "--kiosk" }
    }

    # Cari browser berbasis Chromium (Chrome, lalu Edge).
    $candidates = @(
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
        "$env:LocalAppData\Google\Chrome\Application\chrome.exe",
        "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe",
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
        "$env:LocalAppData\Microsoft\Edge\Application\msedge.exe"
    )
    $browser = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1

    if ($browser) {
        if ($WINFLAG) { Start-Process $browser -ArgumentList "--app=$SERVER_URL", $WINFLAG }
        else          { Start-Process $browser -ArgumentList "--app=$SERVER_URL" }
    } else {
        Start-Process $SERVER_URL
    }
    exit 0
}

# ── 3. Check Go (hanya untuk mode server lokal) ──
$GO_CMD = (Get-Command "go" -ErrorAction SilentlyContinue)
if (-not $GO_CMD) {
    Write-Host "[FAIL] Go belum terinstall." -ForegroundColor Red
    Write-Host "  Download: https://go.dev/dl/"
    exit 1
}
$GO_VER = go version
Write-Host "[OK]    $GO_VER" -ForegroundColor Green

$PORT = Get-EnvRaw "APP_PORT"
if (-not $PORT) { $PORT = "8080" }

# ── 4. Detect LAN IP ──
$LAN_IP = "127.0.0.1"
try {
    $ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -ne "127.0.0.1" -and $_.InterfaceOperationalStatus -eq "Up" }).IPAddress | Select-Object -First 1
    if ($ip) { $LAN_IP = $ip }
} catch {}

# ── 5. Display ──
Write-Host ""
Write-Host "  SIMARC SIAP DIGUNAKAN!" -ForegroundColor Green
Write-Host ""
Write-Host "  Lokal    http://localhost:$PORT" -ForegroundColor Cyan
Write-Host "  Lan      http://${LAN_IP}:$PORT" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Ctrl+C untuk berhenti" -ForegroundColor Gray
Write-Host ""

# ── 6. Run ──
Write-Host "Build aplikasi..." -ForegroundColor Cyan
go mod tidy
$env:CGO_ENABLED = "0"
if (-not (Test-Path "tmp")) { New-Item -ItemType Directory -Path "tmp" | Out-Null }
go build -ldflags="-s -w" -o "tmp\simarc-server.exe" ".\cmd\server"
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] Build gagal!" -ForegroundColor Red
    exit 1
}
Write-Host "[OK]    Build selesai. Menjalankan server..." -ForegroundColor Green
$env:APP_DEBUG = "true"
& ".\tmp\simarc-server.exe"
