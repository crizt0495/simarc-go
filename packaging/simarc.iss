; ══════════════════════════════════════════════════════════════════
;  SIMARC — Inno Setup (Windows Setup.exe) — APLIKASI DESKTOP WAILS
;  Jalankan: ISCC.exe packaging\simarc.iss  (di komputer Windows
;  dengan hasil build wails di build\bin\SIMARC.exe).
;
;  • Per-user install ({localappdata}\SIMARC), tanpa admin.
;  • Shortcut Start Menu + Desktop menuju SIMARC.exe langsung
;    (tanpa command prompt, tanpa browser).
;  • .env dan storage/ milik pengguna tidak pernah ditimpa.
; ══════════════════════════════════════════════════════════════════
[Setup]
AppId={{D4A7F5C2-9B1E-4C6D-8F3A-2E5B77A91C08}}
AppName=SIMARC
AppVersion=1.0.0
AppVerName=SIMARC 1.0.0
AppPublisher=SIMARC
DefaultDirName={localappdata}\SIMARC
DefaultGroupName=SIMARC
DisableProgramGroupPage=yes
AllowNoIcons=yes
PrivilegesRequired=lowest
OutputDir=..\
OutputBaseFilename=SIMARC-Setup-1.0.0
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=windows\simarc.ico
UninstallDisplayIcon={app}\simarc.ico
UninstallDisplayName=SIMARC - Arsip Record Center
WizardStyle=modern

[Languages]
Name: "indonesian"; MessagesFile: "compiler:Languages\Indonesian.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: checkedonce

[Files]
; Executable desktop (frontend React sudah di-embed — tidak butuh Node/browser).
Source: "..\build\bin\SIMARC.exe";              DestDir: "{app}"; Flags: ignoreversion
Source: "windows\simarc.ico";                   DestDir: "{app}"; Flags: ignoreversion
; Contoh konfigurasi (file .env asli dibading di [Run] bila belum ada).
Source: "..\.env.example";                      DestDir: "{app}"; Flags: onlyifdoesntexist
; Dokumentasi singkat.
Source: "..\README.md";                         DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\SIMARC"; Filename: "{app}\SIMARC.exe"; IconFilename: "{app}\simarc.ico"; Comment: "SIMARC — Arsip Record Center"
Name: "{group}\Uninstall SIMARC"; Filename: "{uninstallexe}"
Name: "{autodesktop}\SIMARC"; Filename: "{app}\SIMARC.exe"; IconFilename: "{app}\simarc.ico"; Tasks: desktopicon

[Dirs]
; Folder data yang ditulisi aplikasi (backup SQL).
Name: "{app}\storage\backups"

[Run]
; Buat .env dari contoh bila belum ada (jangan pernah timpa milik pengguna).
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -Command ""if (-not (Test-Path '{app}\.env')) { Copy-Item '{app}\.env.example' '{app}\.env' }"""; Flags: runhidden
Filename: "{app}\SIMARC.exe"; Description: "Buka SIMARC sekarang"; Flags: postinstall nowait skipifsilent

; Catatan: update in-place otomatis — AppId tetap + ignoreversion;
; .env dan storage\ tidak dipaket sehingga tidak pernah ter-overwrite.
