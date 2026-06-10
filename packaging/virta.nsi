; Virta NSIS Windows installer script.
; Run: makensis packaging/virta.nsi (after `make app` / scripts/package-windows.sh produce the
; binaries). makensis resolves relative paths against this script's directory, so the repo root
; is one level up regardless of where makensis is invoked from.
!define ROOT ".."
!define APP_NAME "Virta"
; Overridable from the build: makensis -DAPP_VERSION=v1.2.3 packaging/virta.nsi
!ifndef APP_VERSION
  !define APP_VERSION "dev"
!endif
!define INSTALLER_NAME "VirtaSetup-${APP_VERSION}.exe"
!define INSTALL_DIR "$PROGRAMFILES64\Virta"
!define UNINSTALLER "Uninstall.exe"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "${ROOT}\dist\${INSTALLER_NAME}"
InstallDir "${INSTALL_DIR}"
RequestExecutionLevel admin

Section "Install"
  SetOutPath "${INSTALL_DIR}"
  File "${ROOT}\frontends\desktop\build\bin\virta.exe"
  File "${ROOT}\dist\virtad.exe"
  File "${ROOT}\dist\virta-tui.exe"
  WriteUninstaller "${INSTALL_DIR}\${UNINSTALLER}"
  CreateShortCut "$SMPROGRAMS\Virta.lnk" "${INSTALL_DIR}\virta.exe"
  CreateShortCut "$DESKTOP\Virta.lnk" "${INSTALL_DIR}\virta.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Virta" "DisplayName" "${APP_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Virta" "UninstallString" "${INSTALL_DIR}\${UNINSTALLER}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Virta" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Virta" "Publisher" "Virta"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Virta" "URLInfoAbout" "https://virta.lol"
SectionEnd

Section "Uninstall"
  Delete "${INSTALL_DIR}\virta.exe"
  Delete "${INSTALL_DIR}\virtad.exe"
  Delete "${INSTALL_DIR}\virta-tui.exe"
  Delete "${INSTALL_DIR}\${UNINSTALLER}"
  RMDir "${INSTALL_DIR}"
  Delete "$SMPROGRAMS\Virta.lnk"
  Delete "$DESKTOP\Virta.lnk"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Virta"
SectionEnd
