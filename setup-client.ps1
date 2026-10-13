# ===========================================================================
#  SIMARC — Setup Komputer KLIEN (Windows)
#
#  Mengatur komputer ini sebagai KLIEN yang memakai SERVER PUSAT: menulis
#  SIMARC_SERVER_URL ke .env sehingga aplikasi hanya membuka jendela ke
#  server pusat — tanpa menjalankan server dan tanpa database lokal.
#
#  Pemakaian:
#     powershell -ExecutionPolicy Bypass -File .\setup-client.ps1
#     powershell -ExecutionPolicy Bypass -File .\setup-client.ps1 -ServerUrl http://192.168.1.10:8080
# ===========================================================================
param(
    [string]$ServerUrl = ""
)

$ErrorActionPreference = "Stop"
$APP_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $APP_DIR

Write-Host ""
Write-Host "  ============================================" -ForegroundColor Cyan
Write-Host "     SIMARC -- Setup Komputer Klien" -ForegroundColor Cyan
Write-Host "  ============================================" -ForegroundColor Cyan
Write-Host ""

# -- 1. Minta / validasi alamat server pusat --
if (-not $ServerUrl) {
    $ServerUrl = Read-Host "  Alamat server pusat (contoh http://192.168.1.10:8080)"
}
$ServerUrl = $ServerUrl.Trim().TrimEnd("/")

if ($ServerUrl -notmatch "^https?://") {
    Write-Host "[FAIL] Alamat tidak valid: '$ServerUrl' (harus dimulai http:// atau https://)." -ForegroundColor Red
    exit 1
}

# -- 2. Buat .env dari template klien bila belum ada --
if (-not (Test-Path ".env")) {
    if (Test-Path ".env.client.example") {
        Copy-Item ".env.client.example" ".env"
        Write-Host "[OK]    .env dibuat dari .env.client.example" -ForegroundColor Green
    } else {
        New-Item -ItemType File -Path ".env" -Force | Out-Null
        Write-Host "[WARN]  .env baru dibuat (kosong)." -ForegroundColor Yellow
    }
}

# -- 3. Tulis / perbarui SIMARC_SERVER_URL di .env --
$lines = @(Get-Content ".env")
$found = $false
$out = foreach ($line in $lines) {
    if ($line -match "^\s*SIMARC_SERVER_URL=") {
        $found = $true
        "SIMARC_SERVER_URL=$ServerUrl"
    } else {
        $line
    }
}
if (-not $found) {
    $out = @($out) + "", "# Mode klien (multi-komputer)", "SIMARC_SERVER_URL=$ServerUrl"
}
Set-Content -Path ".env" -Value $out -Encoding UTF8
if ($found) {
    Write-Host "[OK]    SIMARC_SERVER_URL diperbarui di .env" -ForegroundColor Green
} else {
    Write-Host "[OK]    SIMARC_SERVER_URL ditambahkan ke .env" -ForegroundColor Green
}

# -- 4. Uji koneksi ke server pusat (tidak menggagalkan setup) --
Write-Host "[INFO]  Menguji koneksi ke $ServerUrl ..." -ForegroundColor Cyan
try {
    $resp = Invoke-WebRequest -Uri "$ServerUrl/ping" -TimeoutSec 8 -UseBasicParsing
    if ($resp.StatusCode -eq 200) {
        Write-Host "[OK]    Server pusat dapat dihubungi" -ForegroundColor Green
    } else {
        Write-Host "[WARN]  Respons tidak terduga: $($resp.StatusCode)" -ForegroundColor Yellow
    }
} catch {
    Write-Host "[WARN]  Belum bisa menghubungi $ServerUrl/ping" -ForegroundColor Yellow
    Write-Host "        - Pastikan server pusat menyala & port-nya terbuka di firewall" -ForegroundColor Yellow
    Write-Host "        - Pastikan alamat benar & sejaringan (LAN), atau melalui VPN/HTTPS" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "[OK]    Selesai -- komputer ini kini berjalan sebagai KLIEN." -ForegroundColor Green
Write-Host "        Jalankan aplikasi: klik dua kali run.bat (atau ikon SIMARC)"
Write-Host ""
