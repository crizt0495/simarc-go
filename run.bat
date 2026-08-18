@echo off
REM ===========================================================================
REM  SIMARC — Run Script for Windows (Double-click to run)
REM  Build & run web server, auto-detect LAN IP
REM ===========================================================================
cd /d "%~dp0"

echo.
echo   ============================================
echo     S I M A R C  —  Arsip Record Center
echo   ============================================
echo.

REM ── 1. Check Go ──
where go >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [FAIL] Go belum terinstall.
    echo   Download: https://go.dev/dl/
    pause
    exit /b 1
)
for /f "delims=" %%v in ('go version') do echo [OK]    %%v

REM ── 2. Environment ──
if not exist .env (
    if exist .env.example (
        copy .env.example .env >nul
        echo [OK]    .env dibuat dari .env.example
    ) else (
        echo [WARN] Membuat .env default...
        (
          echo APP_NAME="SIMARC-Arsip Record Center"
          echo APP_URL=http://localhost:8080
          echo APP_PORT=8080
          echo APP_DEBUG=true
          echo DB_HOST=127.0.0.1
          echo DB_PORT=3306
          echo DB_DATABASE=simarc_db
          echo DB_USERNAME=root
          echo DB_PASSWORD=
          echo SESSION_KEY=simarc-default-key
          echo # Backup disimpan di storage/app/backups/database/
        ) > .env
        echo [OK]    .env default dibuat
    )
)

REM ── 3. LAN IP ──
set PORT=8080
for /f "tokens=2 delims=:" %%a in ('ipconfig ^| findstr /i "IPv4"') do set LAN_IP=%%a
set LAN_IP=%LAN_IP: =%
if "%LAN_IP%"=="" set LAN_IP=127.0.0.1

REM ── 4. Display ──
echo.
echo   SIMARC SIAP DIGUNAKAN!
echo.
echo   Lokal    http://localhost:%PORT%
echo.
echo   Tekan Ctrl+C untuk berhenti
echo.

REM ── 5. Run ──
echo [INFO]  Build aplikasi...
call go mod tidy >nul 2>&1
set CGO_ENABLED=0
go build -buildvcs=false -ldflags="-s -w" -o "tmp\simarc-server.exe" ".\cmd\server\main.go"
if %ERRORLEVEL% neq 0 (
    echo [FAIL] Build gagal!
    pause
    exit /b 1
)
echo [OK]    Build selesai. Menjalankan server...
set APP_DEBUG=true
".\tmp\simarc-server.exe"
pause
