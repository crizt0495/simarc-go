# ===========================================================================
#  Uji logika instalasi SIMARC-Setup.ps1 (tanpa perlu Windows).
#
#  Fungsi `Resolve-SyncEnv` dan `Set-EnvKey` diekstrak langsung dari berkas
#  SIMARC-Setup.ps1 (parse AST), lalu dijalankan apa adanya. Jadi yang diuji
#  benar-benar kode produksi, bukan salinannya.
#
#  Jalankan:  pwsh -NoProfile -File packaging/windows/test-installer-sync.ps1
# ===========================================================================
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$here = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
$installer = Join-Path $here "SIMARC-Setup.ps1"

$fail = 0
function Check([bool]$cond, [string]$msg) {
  if ($cond) { Write-Host "  OK   $msg" }
  else { Write-Host "  GAGAL $msg" -ForegroundColor Red; $script:fail++ }
}
function Val($pairs, [string]$key) {
  $m = @($pairs | Where-Object { $_.Key -eq $key })
  if ($m.Count -eq 0) { return $null }
  return [string]$m[0].Value
}
function KeyCount($pairs, [string]$key) {
  return @($pairs | Where-Object { $_.Key -eq $key }).Count
}

# ── Ekstrak fungsi dari berkas installer yang sebenarnya ────────────────────
$tokens = $null; $errors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($installer, [ref]$tokens, [ref]$errors)
if ($errors -and $errors.Count -gt 0) {
  Write-Host "Installer gagal di-parse:" -ForegroundColor Red
  $errors | ForEach-Object { Write-Host "  baris $($_.Extent.StartLineNumber): $($_.Message)" }
  exit 1
}
$wanted = @("Resolve-SyncEnv", "Set-EnvKey")
$funcs = $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true) |
  Where-Object { $wanted -contains $_.Name }
foreach ($name in $wanted) {
  $fn = $funcs | Where-Object { $_.Name -eq $name }
  if (-not $fn) { Write-Host "Fungsi $name tidak ditemukan di installer." -ForegroundColor Red; exit 1 }
  Invoke-Expression $fn.Extent.Text
}
Write-Host "Fungsi diekstrak: $(($wanted) -join ', ')"
Write-Host ""

# ── 1) Tanpa hub -> sinkronisasi tidak disetel ──────────────────────────────
Write-Host "1) tanpa hub"
Check (@(Resolve-SyncEnv @{} "PC-01").Count -eq 0) "config kosong -> 0 pasangan"
Check (@(Resolve-SyncEnv @{ MODE = "offline" } "PC-01").Count -eq 0) "tanpa SYNC_HUB_DB_HOST -> 0 pasangan"
Check (@(Resolve-SyncEnv @{ SYNC_HUB_DB_HOST = "  " } "PC-01").Count -eq 0) "host kosong/spasi -> 0 pasangan"

# ── 2) Hub saja -> auto-aktif + identitas unik + interval bawaan ────────────
Write-Host "2) hub dikonfigurasi saja"
$p = @(Resolve-SyncEnv @{ SYNC_HUB_DB_HOST = "192.168.1.86"; SYNC_HUB_DB_NAME = "simarc"; SYNC_HUB_DB_USER = "simarc_sync" } "PC-ARSP-01")
Check ((Val $p "SYNC_ENABLED") -eq "true") "SYNC_ENABLED otomatis true"
Check ((Val $p "SYNC_NODE_ID") -eq "pc-arsp-01") "SYNC_NODE_ID dari nama komputer (huruf kecil)"
Check ((Val $p "SYNC_INTERVAL_MINUTES") -eq "15") "SYNC_INTERVAL_MINUTES bawaan 15"
Check ((Val $p "SYNC_HUB_DB_HOST") -eq "192.168.1.86") "SYNC_HUB_DB_HOST disalin"
Check ($p.Count -eq 6) "jumlah pasangan = 6 (3 hub + 3 bawaan), dapat $($p.Count)"

# ── 3) Identitas unik per komputer dari config yang SAMA ────────────────────
Write-Host "3) unik per komputer"
$shared = @{ SYNC_HUB_DB_HOST = "192.168.1.86" }
$a = Val (Resolve-SyncEnv $shared "PC-FRONT-OFFICE") "SYNC_NODE_ID"
$b = Val (Resolve-SyncEnv $shared "PC-BACK-OFFICE") "SYNC_NODE_ID"
Check ($a -ne $b) "dua komputer beda -> node id beda ($a != $b)"
Check ($a -eq "pc-front-office") "nama disanitasi/lower -> $a"

# ── 4) Nilai eksplisit tidak ditimpa ────────────────────────────────────────
Write-Host "4) nilai eksplisit menang"
$p = @(Resolve-SyncEnv @{ SYNC_HUB_DB_HOST = "hub"; SYNC_ENABLED = "false"; SYNC_NODE_ID = "meja-depan"; SYNC_INTERVAL_MINUTES = "60" } "PC-99")
Check ((Val $p "SYNC_ENABLED") -eq "false") "SYNC_ENABLED eksplisit dipertahankan"
Check ((Val $p "SYNC_NODE_ID") -eq "meja-depan") "SYNC_NODE_ID eksplisit dipertahankan"
Check ((Val $p "SYNC_INTERVAL_MINUTES") -eq "60") "interval eksplisit dipertahankan"
Check ((KeyCount $p "SYNC_ENABLED") -eq 1) "SYNC_ENABLED tidak ganda"
Check ((KeyCount $p "SYNC_NODE_ID") -eq 1) "SYNC_NODE_ID tidak ganda"

# ── 5) Sanitasi nama komputer aneh ──────────────────────────────────────────
Write-Host "5) sanitasi nama"
Check ((Val (Resolve-SyncEnv @{ SYNC_HUB_DB_HOST = "hub" } "PC ARSP/01!") "SYNC_NODE_ID") -eq "pc-arsp-01") "spasi & simbol -> tanda hubung, dipangkas"
Check ((Val (Resolve-SyncEnv @{ SYNC_HUB_DB_HOST = "hub" } "") "SYNC_NODE_ID") -eq "node-unknown") "nama kosong -> node-unknown"

# ── 6) Terapkan ke .env nyata (Set-EnvKey) tanpa duplikat ───────────────────
Write-Host "6) tulis .env nyata"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("simarc-env-test-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
$envf = Join-Path $tmp ".env"
@("# awal", "DB_HOST=127.0.0.1", "SYNC_ENABLED=false", "") | Set-Content -LiteralPath $envf

$pairs = @(Resolve-SyncEnv @{ SYNC_HUB_DB_HOST = "192.168.1.86"; SYNC_HUB_DB_PASS = "rahasia" } "PC-UJI")
foreach ($kv in $pairs) { Set-EnvKey $envf $kv.Key $kv.Value }
# Terapkan dua kali: harus tetap idempoten (tidak menumpuk).
foreach ($kv in $pairs) { Set-EnvKey $envf $kv.Key $kv.Value }

$lines = Get-Content -LiteralPath $envf
Check (@($lines | Where-Object { $_ -match '^SYNC_ENABLED=' }).Count -eq 1) "SYNC_ENABLED tepat 1 baris"
Check ((@($lines | Where-Object { $_ -match '^SYNC_ENABLED=true$' }).Count) -eq 1) "nilai SYNC_ENABLED yang ada diganti jadi true"
Check (@($lines | Where-Object { $_ -match '^SYNC_HUB_DB_HOST=' }).Count -eq 1) "SYNC_HUB_DB_HOST tepat 1 baris"
Check (@($lines | Where-Object { $_ -match '^SYNC_NODE_ID=pc-uji$' }).Count -eq 1) "SYNC_NODE_ID ditulis dari nama komputer"
Check (@($lines | Where-Object { $_ -match '^DB_HOST=127\.0\.0\.1$' }).Count -eq 1) "kunci lama tetap utuh"
# Tanpa BOM (penting agar run.bat/godotenv membacanya dengan benar).
$bytes = [System.IO.File]::ReadAllBytes($envf)
Check (-not ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)) "berkas .env tanpa BOM"
Remove-Item -Recurse -Force $tmp

Write-Host ""
if ($fail -gt 0) { Write-Host "$fail pemeriksaan GAGAL" -ForegroundColor Red; exit 1 }
Write-Host "Semua pemeriksaan LULUS" -ForegroundColor Green
