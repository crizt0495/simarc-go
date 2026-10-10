@echo off
setlocal enableextensions
REM ===========================================================================
REM  SIMARC - Run Script for Windows (Desktop App)
REM
REM  - Mode SERVER LOKAL (default): menjalankan server Go dengan jendela
REM    aplikasi mandiri. Menutup jendela otomatis mematikan server.
REM  - Mode KLIEN: bila SIMARC_SERVER_URL diisi di .env, hanya membuka jendela
REM    ke server pusat (tanpa server/database lokal, tanpa perlu Go).
REM ===========================================================================
cd /d "%~dp0"

REM -- Baca konfigurasi dari .env --
set "SIMARC_SERVER_URL="
set "SIMARC_WINDOW="
if exist .env for /f "usebackq eol=# tokens=1,* delims==" %%A in (".env") do call :readenv "%%A" "%%B"
set "SIMARC_SERVER_URL=%SIMARC_SERVER_URL:"=%"
set "SIMARC_WINDOW=%SIMARC_WINDOW:"=%"
set "SIMARC_SERVER_URL=%SIMARC_SERVER_URL: =%"

echo.
echo   ============================================
echo     S I M A R C  -  Arsip Record Center
echo   ============================================
echo.

if not "%SIMARC_SERVER_URL%"=="" goto client

REM ============ MODE SERVER LOKAL ============
set "EXE=tmp\simarc-server.exe"

REM Pilih binary rilis sesuai arsitektur (bila tersedia).
set "DISTEXE=dist\simarc-server-windows-amd64.exe"
if /i "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "DISTEXE=dist\simarc-server-windows-arm64.exe"

if exist "%EXE%" goto runserver
if exist "%DISTEXE%" (
    if not exist tmp mkdir tmp
    copy /y "%DISTEXE%" "%EXE%" >nul
    echo [OK]    Memakai binary rilis.
    goto runserver
)

where go >nul 2>&1
if errorlevel 1 (
    echo [FAIL] Binary belum tersedia dan Go tidak terinstall.
    echo         Sediakan %DISTEXE% lalu jalankan lagi.
    pause
    exit /b 1
)
echo [INFO]  Build aplikasi (sekali)...
set CGO_ENABLED=0
if not exist tmp mkdir tmp
go build -buildvcs=false -ldflags="-s -w" -o "%EXE%" ".\cmd\server"
if errorlevel 1 (
    echo [FAIL] Build gagal.
    pause
    exit /b 1
)

:runserver
echo [OK]    Menjalankan server. Tutup jendela aplikasi untuk berhenti.
set SIMARC_APP_WINDOW=1
"%EXE%"
exit /b 0

REM ============ MODE KLIEN (server pusat) ============
:client
echo [INFO]  Mode KLIEN - server pusat: %SIMARC_SERVER_URL%

set "WINFLAG=--start-maximized"
if /i "%SIMARC_WINDOW%"=="normal"     set "WINFLAG="
if /i "%SIMARC_WINDOW%"=="fullscreen" set "WINFLAG=--start-fullscreen"
if /i "%SIMARC_WINDOW%"=="kiosk"      set "WINFLAG=--kiosk"

REM Cari browser berbasis Chromium (Chrome, lalu Edge).
set "CB="
if not defined CB if exist "%ProgramFiles%\Google\Chrome\Application\chrome.exe"        set "CB=%ProgramFiles%\Google\Chrome\Application\chrome.exe"
if not defined CB if exist "%ProgramFiles(x86)%\Google\Chrome\Application\chrome.exe" set "CB=%ProgramFiles(x86)%\Google\Chrome\Application\chrome.exe"
if not defined CB if exist "%LocalAppData%\Google\Chrome\Application\chrome.exe"      set "CB=%LocalAppData%\Google\Chrome\Application\chrome.exe"
if not defined CB if exist "%ProgramFiles%\Microsoft\Edge\Application\msedge.exe"     set "CB=%ProgramFiles%\Microsoft\Edge\Application\msedge.exe"
if not defined CB if exist "%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe" set "CB=%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe"
if not defined CB if exist "%LocalAppData%\Microsoft\Edge\Application\msedge.exe"     set "CB=%LocalAppData%\Microsoft\Edge\Application\msedge.exe"

if defined CB (
    start "" "%CB%" --app="%SIMARC_SERVER_URL%" %WINFLAG%
) else (
    start "" "%SIMARC_SERVER_URL%"
)
exit /b 0

REM ============ subrutin ============
:readenv
if /i "%~1"=="SIMARC_SERVER_URL" set "SIMARC_SERVER_URL=%~2"
if /i "%~1"=="SIMARC_WINDOW"     set "SIMARC_WINDOW=%~2"
exit /b 0
