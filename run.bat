@echo off
REM ===========================================================================
REM  SIMARC — Run Script for Windows (Double-click to run)
REM  Build & run web server, auto-detect LAN IP
REM ===========================================================================
cd /d "%~dp0"

REM ── 0. MODE KLIEN (server pusat): tanpa server/database lokal ──
set "SIMARC_SERVER_URL="
if exist .env (
    for /f "usebackq tokens=1,* delims==" %%A in (`findstr /b /i "SIMARC_SERVER_URL=" .env`) do set "SIMARC_SERVER_URL=%%B"
)
set "SIMARC_SERVER_URL=%SIMARC_SERVER_URL:"=%"
if not "%SIMARC_SERVER_URL%"=="" goto :client

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
set SIMARC_APP_WINDOW=1
".\tmp\simarc-server.exe"
pause
exit /b 0

REM ===========================================================================
REM  MODE KLIEN — buka jendela aplikasi ke server pusat (tanpa Go / database)
REM ===========================================================================
:client
echo.
echo   Mode KLIEN - server pusat: %SIMARC_SERVER_URL%
echo.
set "SIMARC_CHROME="
for %%P in (
  "%ProgramFiles%\Google\Chrome\Application\chrome.exe"
  "%ProgramFiles(x86)%\Google\Chrome\Application\chrome.exe"
  "%LocalAppData%\Google\Chrome\Application\chrome.exe"
  "%ProgramFiles%\Microsoft\Edge\Application\msedge.exe"
  "%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe"
  "%LocalAppData%\Microsoft\Edge\Application\msedge.exe"
) do if exist %%P if not defined SIMARC_CHROME set "SIMARC_CHROME=%%~P"
if defined SIMARC_CHROME (
    start "" "%SIMARC_CHROME%" --app="%SIMARC_SERVER_URL%" --start-maximized
) else (
    start "" "%SIMARC_SERVER_URL%"
)
exit /b 0
