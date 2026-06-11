; Virta Inno Setup Windows installer script.
; Run: iscc packaging/virta.iss (after `make app` / scripts/package-windows.sh produce the
; binaries). ISCC resolves relative paths against this script's directory, so the repo root
; is one level up regardless of where iscc is invoked from.
#define Root ".."
#define AppName "Virta"
; Overridable from the build: iscc /DAppVersion=v1.2.3 packaging/virta.iss
#ifndef AppVersion
  #define AppVersion "dev"
#endif

[Setup]
AppId={{9C4A7E62-5B1D-4F38-A0E9-D7361C2B8F54}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=Virta
AppPublisherURL=https://virta.lol
DefaultDirName={autopf}\{#AppName}
; Single start-menu shortcut, no program group folder — mirrors the previous installer.
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#Root}\dist
OutputBaseFilename=VirtaSetup-{#AppVersion}
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
UninstallDisplayName={#AppName}

[Tasks]
Name: desktopicon; Description: "{cm:CreateDesktopIcon}"

[Files]
; Go binaries carry no Win32 version resource, so force-replace on upgrade.
Source: "{#Root}\frontends\desktop\build\bin\virta.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Root}\dist\virtad.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Root}\dist\virta-tui.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\virta.exe"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\virta.exe"; Tasks: desktopicon

[Registry]
; Register virta:// as a URL protocol handler so the OS opens deep links like virta://install?url=...
Root: HKLM; Subkey: "Software\Classes\virta";                          ValueType: string; ValueName: "";             ValueData: "Virta"; Flags: uninsdeletekey
Root: HKLM; Subkey: "Software\Classes\virta";                          ValueType: string; ValueName: "URL Protocol"; ValueData: ""
Root: HKLM; Subkey: "Software\Classes\virta\DefaultIcon";              ValueType: string; ValueName: "";             ValueData: "{app}\virta.exe,0"
Root: HKLM; Subkey: "Software\Classes\virta\shell\open\command";       ValueType: string; ValueName: "";             ValueData: """{app}\virta.exe"" ""%1"""
