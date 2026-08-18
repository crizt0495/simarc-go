# ===========================================================================
#  SIMARC — Run Script for Windows (PowerShell)
#  Auto-detect LAN, build & run
#  Usage: .\run.ps1
# ===========================================================================

$APP_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $APP_DIR

Write-Host ""
Write-Host "  ============================================" -ForegroundColor Cyan
Write-Host "     SIMARC -- Arsip Record Center" -ForegroundColor Cyan
Write-Host "  ============================================" -ForegroundColor Cyan
Write-Host ""

# ── 1. Check Go ──
$GO_CMD = (Get-Command "go" -ErrorAction SilentlyContinue)
if (-not $GO_CMD) {
    Write-Host "[FAIL] Go belum terinstall." -ForegroundColor Red
    Write-Host "  Download: https://go.dev/dl/"
    exit 1
}
$GO_VER = go version
Write-Host "[OK]    $GO_VER" -ForegroundColor Green

# ── 2. Environment ──
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

$PORT = [Environment]::GetEnvironmentVariable("APP_PORT") -replace '"', ''
if (-not $PORT) { $PORT = "8080" }

# ── 3. Detect LAN IP ──
$LAN_IP = "127.0.0.1"
try {
    $ip = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -ne "127.0.0.1" -and $_.InterfaceOperationalStatus -eq "Up" }).IPAddress | Select-Object -First 1
    if ($ip) { $LAN_IP = $ip }
} catch {}

# ── 4. Display ──
Write-Host ""
Write-Host "  SIMARC SIAP DIGUNAKAN!" -ForegroundColor Green
Write-Host ""
Write-Host "  Lokal    http://localhost:$PORT" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Ctrl+C untuk berhenti" -ForegroundColor Gray
Write-Host ""

# ── 5. Run ──
Write-Host "Build aplikasi..." -ForegroundColor Cyan
go mod tidy
$env:CGO_ENABLED = "0"
go build -ldflags="-s -w" -o "tmp\simarc-server.exe" ".\cmd\server\main.go"
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] Build gagal!" -ForegroundColor Red
    exit 1
}
Write-Host "[OK]    Build selesai. Menjalankan server..." -ForegroundColor Green
$env:APP_DEBUG = "true"
& ".\tmp\simarc-server.exe"
