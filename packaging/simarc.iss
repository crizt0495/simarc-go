; ===========================================================================
;  SIMARC — Inno Setup script (Windows Setup.exe)
;
;  Dirancang untuk INSTALL dan UPGRADE IN-PLACE:
;    * AppId TETAP pada semua versi -> menjalankan Setup.exe versi baru
;      MENIMPA file di tempat yang sama, TIDAK perlu uninstall dulu.
;    * Instal per-user ke {localappdata}\SIMARC (tanpa admin, folder bisa
;      ditulis untuk storage\uploads).
;    * `.env` dan `storage\` TIDAK pernah dipaket -> data & konfigurasi
;      komputer pengguna selalu selamat (baik saat upgrade maupun uninstall).
;
;  Cara build di Windows:
;    1. Install Inno Setup 6 (https://jrsoftware.org/isinfo.php)
;    2. Jalankan: ISCC.exe packaging\simarc.iss
;    3. Hasil: dist\SIMARC-Setup-<AppVersion>.exe
;
;  Saat merilis versi baru: naikkan AppVersion dan OutputBaseFilename.
; ===========================================================================
[Setup]
AppId={{D4A7F5C2-9B1E-4C6D-8F3A-2E5B77A91C08}}
AppName=SIMARC
AppVersion=1.0.0
AppVerName=SIMARC 1.0.0
AppPublisher=Bakesbangpol Kota Probolinggo
AppPublisherURL=
AppSupportURL=
AppUpdatesURL=
DefaultDirName={localappdata}\SIMARC
DefaultGroupName=SIMARC
DisableProgramGroupPage=yes
AllowNoIcons=yes
PrivilegesRequired=lowest
OutputDir=dist
OutputBaseFilename=SIMARC-Setup-1.0.0
Compression=lzma
SolidCompression=yes
; Hanya arsitektur yang biner rilisnya tersedia.
ArchitecturesAllowed=x64compatible arm64
ArchitecturesInstallIn64BitMode=x64compatible arm64
SetupIconFile=web\static\images\favicon.ico
UninstallDisplayIcon={app}\web\static\images\favicon.ico
UninstallDisplayName=SIMARC - Arsip Record Center
WizardStyle=modern

[Languages]
Name: "indonesian"; MessagesFile: "compiler:Languages\Indonesian.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; Peluncur & konfigurasi contoh (hanya .example — .env milik pengguna dibuat saat install).
Source: "run.bat";                            DestDir: "{app}"; Flags: ignoreversion
Source: ".env.example";                       DestDir: "{app}"; Flags: onlyifdoesntexist
Source: "VERSION";                            DestDir: "{app}"; Flags: ignoreversion; Check: FileExists('{src}\VERSION')
; Aplikasi web (template, css/js, vendor).
Source: "web\*";                              DestDir: "{app}\web"; Flags: recursesubdirs createallsubdirs ignoreversion
Source: "docs\*";                             DestDir: "{app}\docs"; Flags: recursesubdirs createallsubdirs ignoreversion
; Biner server rilis (di-copy ke tmp\simarc-server.exe oleh run.bat sesuai arsitektur).
Source: "dist\simarc-server-windows-amd64.exe"; DestDir: "{app}\dist"; Flags: ignoreversion
Source: "dist\simarc-server-windows-arm64.exe"; DestDir: "{app}\dist"; Flags: ignoreversion
; Skrip integrasi desktop + uninstaller.
Source: "packaging\install-desktop.ps1";      DestDir: "{app}\packaging"; Flags: ignoreversion
Source: "packaging\uninstall-desktop.ps1";    DestDir: "{app}\packaging"; Flags: ignoreversion

[Icons]
Name: "{group}\SIMARC"; Filename: "{app}\run.bat"; WorkingDir: "{app}"; IconFilename: "{app}\web\static\images\favicon.ico"
Name: "{group}\Uninstall SIMARC"; Filename: "{uninstallexe}"
Name: "{commondesktop}\SIMARC"; Filename: "{app}\run.bat"; WorkingDir: "{app}"; IconFilename: "{app}\web\static\images\favicon.ico"; Tasks: desktopicon

[Run]
; Buat .env dari contoh bila belum ada (jangan pernah timpa config pengguna).
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -Command ""if (-not (Test-Path '{app}\.env')) {{ Copy-Item '{app}\.env.example' '{app}\.env' }}"""; Flags: runhidden nowait
; Direktori runtime yang bisa ditulis.
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -Command ""'{app}\tmp','{app}\storage\app\uploads','{app}\public\storage' | ForEach-Object {{ New-Item -ItemType Directory -Force -Path $_ | Out-Null }}"""; Flags: runhidden nowait
; Integrasi desktop (shortcut + registry uninstall; idempoten).
Filename: "{app}\install-desktop.ps1"; Parameters: "-Quiet"; Flags: runhidden nowait

[UninstallRun]
Filename: "{app}\uninstall-desktop.ps1"; Parameters: "-Quiet -Confirm"; Flags: runhidden

; Catatan: update in-place terjadi otomatis karena AppId tetap + ignoreversion.
; .env dan storage\ tidak dipaket, sehingga tidak pernah ter-overwrite
; (dan tidak ikut terhapus saat uninstall).