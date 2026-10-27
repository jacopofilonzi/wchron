Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default "wchron"
## !define INFO_COMPANYNAME    "My Company" # Default "Jacopo Filonzi"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "wchron"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, My Company" # Default "(c) 2026, Jacopo Filonzi"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
## !define WAILS_INSTALL_SCOPE     "user"             # Default "machine" - set to "user" for per-user install ($LOCALAPPDATA) without UAC prompt
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}" # "Run wchron" checkbox on the last page
!insertmacro MUI_PAGE_FINISH # Finished installation page.

# Uninstall: confirm (says that all the data goes), progress, final page with the WhatsApp note.
!define MUI_UNCONFIRMPAGE_TEXT_TOP "${INFO_PRODUCTNAME} will be removed together with ALL its data: schedules, channels and their sessions, settings, logs and backups ($APPDATA\${INFO_PRODUCTNAME}).$\r$\n$\r$\nTo keep them, cancel and copy that folder first."
!insertmacro MUI_UNPAGE_CONFIRM
!define MUI_UNFINISHPAGE_NOAUTOCLOSE
!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page
!define MUI_FINISHPAGE_TITLE "${INFO_PRODUCTNAME} has been removed"
!define MUI_FINISHPAGE_TEXT "${INFO_PRODUCTNAME} and all its data have been removed from this computer.$\r$\n$\r$\nWhatsApp: the linked device $\"${INFO_PRODUCTNAME}$\" stays on your phone unless you deleted the channel in the app before uninstalling. Remove it from WhatsApp > Settings > Linked devices."
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
# Per-user only: no UAC, never machine-wide (see AGENTS.md).
!if "${WAILS_INSTALL_SCOPE}" != "user"
    !error "wchron installs per user only: build with -DWAILS_INSTALL_SCOPE=user"
!endif
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
ShowInstDetails show # This will always show the installation details.

# The running app locks its executable: before installing (or uninstalling) offer to close it.
# Only this user's instance is closed. In silent mode (/S) it is closed without asking.
# System tools by full path: with Git in the PATH, `find` may be the Unix one.
!macro closeRunningApp
    StrCpy $R1 0
    check:
        nsExec::ExecToStack '"$SYSDIR\cmd.exe" /c ""$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}" /FI "USERNAME eq %USERNAME%" /NH | "$SYSDIR\find.exe" /I "${PRODUCT_EXECUTABLE}""'
        Pop $R0 # exit code of find: 0 = running
        Pop $R2 # output, unused
        StrCmp $R0 0 0 done
        IntCmp $R1 0 ask
        IntCmp $R1 5 stuck kill stuck
    ask:
        MessageBox MB_OKCANCEL|MB_ICONINFORMATION "${INFO_PRODUCTNAME} is running.$\n$\nClick OK to close it and continue." /SD IDOK IDOK kill
        Abort
    kill:
        nsExec::Exec '"$SYSDIR\taskkill.exe" /F /IM "${PRODUCT_EXECUTABLE}" /FI "USERNAME eq %USERNAME%"'
        Pop $R0
        IntOp $R1 $R1 + 1
        Sleep 800
        Goto check
    stuck:
        MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "${INFO_PRODUCTNAME} could not be closed.$\n$\nQuit it from the tray icon, then click Retry." /SD IDCANCEL IDRETRY retry
        Abort
    retry:
        StrCpy $R1 1
        Goto check
    done:
!macroend

Function .onInit
   !insertmacro wails.checkArchitecture
   !insertmacro closeRunningApp
FunctionEnd

Function un.onInit
   !insertmacro closeRunningApp
FunctionEnd

# DATA SAFETY (AGENTS.md invariant "Upgrades never delete data"): the install section below never
# runs the uninstaller and never removes anything under the data dir; installing over an existing
# install (by hand or the silent self-update with /S) only overwrites the program files. The user's
# data is removed ONLY by the "uninstall" section (uninstall.exe, run explicitly). A Go test
# (nsis_test.go) checks that the data paths appear nowhere else.
Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # Start menu and desktop shortcuts (the desktop one was asked for by the user).
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

# Explicit uninstall: removes the program AND every trace of the app for this user.
Section "uninstall"
    !insertmacro wails.setShellContext

    # Program files: only ours, never the whole folder (the user may have picked a shared one).
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    # The login start entry (first run, or Settings > Start with Windows).
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${INFO_PROJECTNAME}"

    # Toast registration written by the Wails notifications service at every start: the
    # AppUserModelId key holds the per-install activator CLSID (a "{GUID}"), checked before use so
    # an empty or odd value can never delete Software\Classes\CLSID itself.
    ReadRegStr $0 HKCU "Software\Classes\AppUserModelId\${INFO_PROJECTNAME}" "CustomActivator"
    StrLen $1 $0
    StrCpy $2 $0 1
    ${If} $1 == 38
    ${AndIf} $2 == "{"
        DeleteRegKey HKCU "Software\Classes\CLSID\$0"
        Delete "$TEMP\${INFO_PROJECTNAME}$0.png" # the toast icon
    ${EndIf}
    DeleteRegKey HKCU "Software\Classes\AppUserModelId\${INFO_PROJECTNAME}"
    DeleteRegKey HKCU "Software\${INFO_PROJECTNAME}\NotificationCategories"
    DeleteRegKey /ifempty HKCU "Software\${INFO_PROJECTNAME}"

    # User data: config, state, channels (WhatsApp sessions), logs, backups; the WebView2 data;
    # the processors' temp copies.
    RMDir /r "$APPDATA\${INFO_PROJECTNAME}"
    RMDir /r "$APPDATA\${PRODUCT_EXECUTABLE}"
    RMDir /r "$TEMP\${INFO_PROJECTNAME}"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
    RMDir "$INSTDIR" # only if empty
SectionEnd
