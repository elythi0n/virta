; Virta NSIS Windows installer script.
; Run: makensis packaging/virta.nsi (from the repo root, after `make app` produces the binary).
!define APP_NAME "Virta"
!define APP_VERSION "1.0.0"
!define INSTALLER_NAME "VirtaSetup-${APP_VERSION}.exe"
!define INSTALL_DIR "$PROGRAMFILES64\Virta"
!define UNINSTALLER "Uninstall.exe"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "dist\${INSTALLER_NAME}"
InstallDir "${INSTALL_DIR}"
RequestExecutionLevel admin

Section "Install"
  SetOutPath "${INSTALL_DIR}"
  File "frontends\desktop\build\bin\virta.exe"
  File "dist\virtad.exe"
  File "dist\virta-tui.exe"
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
