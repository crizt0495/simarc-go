@echo off
REM ===========================================================================
REM  SIMARC - Install / Update (double click me)
REM  Instal baru ATAU update in-place.Tanpa uninstall, tanpa admin.
REM ===========================================================================
cd /d "%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0SIMARC-Setup.ps1"
set "EXITCODE=%ERRORLEVEL%"
echo.
if "%EXITCODE%"=="0" (
  echo  [OK] Selesai. Aplikasi SIMARC siap dijalankan dari menu Start.
) else (
  echo  [GAGAL] Instalasi/update tidak selesai dengan sukses ^(kode %EXITCODE%^).
)
echo.
pause
exit /b %EXITCODE%