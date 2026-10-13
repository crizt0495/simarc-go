@echo off
REM ===========================================================================
REM  SIMARC - Uninstall desktop integration (double click me)
REM  Menghapus shortcut + entry uninstall. MENYISAKAN data:
REM  %LocalAppData%\SIMARC\.env dan storage\ TIDAK dihapus.
REM ===========================================================================
cd /d "%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0Uninstall-SIMARC.ps1"
echo.
pause
exit /b %ERRORLEVEL%