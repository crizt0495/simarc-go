[Setup]
AppName=SIMARC
AppVersion=1.0.0
AppPublisher=Bakesbangpol Kota Probolinggo
AppPublisherURL=
AppSupportURL=
AppUpdatesURL=
DefaultDirName={autopf}\SIMARC
DefaultGroupName=SIMARC
AllowNoIcons=yes
; Output
OutputDir=dist
OutputBaseFilename=SIMARC-Setup-1.0.0
Compression=lzma
SolidCompression=yes
PrivilegesRequired=none
SetupIconFile=web\static\images\favicon.ico
UninstallDisplayIcon={app}\web\static\images\favicon.ico
UninstallDisplayName=SIMARC - Arsip Record Center

[Languages]
Name: "indonesian"; MessagesFile: "compiler:Languages\Indonesian.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "*"; DestDir: "{app}"; Flags: recursesubdirs createallsubdirs ignoreversion
Exclude: ".git\*"; Exclude: "tmp\*"; Exclude: "dist\*"; Exclude: "node_modules\*"; Exclude: "build-deb\*"; Exclude: "simarc.iss"; Exclude: "*.AppImage"

[Icons]
Name: "{group}\SIMARC"; Filename: "{app}\run.bat"; WorkingDir: "{app}"; IconFilename: "{app}\web\static\images\favicon.ico"
Name: "{group}\Uninstall SIMARC"; Filename: "{uninstallexe}"
Name: "{commondesktop}\SIMARC"; Filename: "{app}\run.bat"; WorkingDir: "{app}"; IconFilename: "{app}\web\static\images\favicon.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\install-desktop.ps1"; Parameters: "-Quiet"; Flags: runhidden nowait

[UninstallRun]
Filename: "{app}\uninstall-desktop.ps1"; Parameters: "-Quiet -Confirm"; Flags: runhidden
