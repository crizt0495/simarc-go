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

REM -- Terapkan pembaruan otomatis (jika ada) -------------------------------
REM  Klien yang menerima pembaruan dari hub menyimpan binary baru sebagai
REM  tmp\simarc-server.exe.new. Tukar di sini sebelum server dijalankan.
if exist "tmp\simarc-server.exe.new" (
    if exist "tmp\simarc-server.exe" del /f /q "tmp\simarc-server.exe" >nul 2>&1
    move /y "tmp\simarc-server.exe.new" "tmp\simarc-server.exe" >nul 2>&1
    echo [OK]    Pembaruan otomatis diterapkan.
)

REM -- Baca konfigurasi dari .env --
set "SIMARC_SERVER_URL="
set "SIMARC_WINDOW="
if exist .env for /f "usebackq eol=# tokens=1,* delims==" %%A in (".env") do call :readenv "%%A" "%%B"

REM -- Bersihkan nilai (kutip & spasi) ---------------------------------------
REM  Pembersihan DIJAGA kondisi non-kosong. Substitusi %VAR:"=% terhadap nilai
REM  kosong terbukti merusak variabel pada sebagian implementasi cmd (nilai
REM  jadi "=\"), yang membuat server salah masuk mode klien. Dijaga begini,
REM  run.bat selalu menentukan mode dengan benar.
if not "%SIMARC_SERVER_URL%"=="" set "SIMARC_SERVER_URL=%SIMARC_SERVER_URL:"=%"
if not "%SIMARC_SERVER_URL%"=="" set "SIMARC_SERVER_URL=%SIMARC_SERVER_URL: =%"
if not "%SIMARC_WINDOW%"==""     set "SIMARC_WINDOW=%SIMARC_WINDOW:"=%"

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
if not exist "%EXE%" (
    echo [FAIL] Binary tidak ditemukan: %EXE%
    echo         Jalankan installer ulang atau sediakan %DISTEXE%.
    pause
    exit /b 1
)
if not exist ".env" (
    echo [FAIL] Berkas .env tidak ditemukan di %CD%
    echo         Pastikan instalasi lengkap. Jalankan installer ulang.
    pause
    exit /b 1
)
echo [OK]    Menjalankan server. Tutup jendela aplikasi untuk berhenti.
set SIMARC_APP_WINDOW=1
"%EXE%"
set "EXITCODE=%ERRORLEVEL%"

REM Hentikan database portabel (bila aplikasi tidak sempat menutupnya sendiri).
if exist "%CD%\mariadb\data\mysqld.pid" goto stopdb

if %EXITCODE% neq 0 (
    echo.
    echo [ERROR] Server keluar dengan kode %EXITCODE%.
    echo         Lihat pesan di atas. Jika port 8080 sudah dipakai, ubah APP_PORT di .env.
    echo         Tekan sembarang tombol untuk menutup...
    pause >nul
    exit /b %EXITCODE%
)
exit /b 0

:stopdb
set /p MDBPID=<"%CD%\mariadb\data\mysqld.pid"
if not "%MDBPID%"=="" taskkill /PID %MDBPID% /T /F >nul 2>&1
exit /b 0

REM ============ MODE KLIEN (server pusat) ============
:client
echo [INFO]  Mode KLIEN - server pusat: %SIMARC_SERVER_URL%

set "WINFLAG=--start-fullscreen"
if /i "%SIMARC_WINDOW%"=="normal"     set "WINFLAG="
if /i "%SIMARC_WINDOW%"=="maximized"  set "WINFLAG=--start-maximized"
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
