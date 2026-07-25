Unicode true

####
## Kaa Rada NSIS installer.
##
## Adapted from the Wails v2 default template. Local changes are marked with
## "PROJECT:" comments so a future Wails upgrade can be diffed against the
## stock template without guessing what was ours.
##
## Note: Go template replacements do NOT work in this file. Values not defined
## here are populated by "wails_tools.nsh", which Wails generates from
## wails.json at build time.
##
## To iterate on the installer without a full Wails build, run a Wails NSIS
## build once to generate wails_tools.nsh, then:
##   makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\kaa-rada.exe
####

## PROJECT: defined explicitly because wails.json sets no companyName. Left
## undefined, INSTALLDIR below expands to "C:\Program Files\\Kaa Rada"
## with an empty path component, and the uninstall registry key collides with
## any other product of the same name.
!define INFO_COMPANYNAME "Kaa Rada"

## PROJECT: pinned so the installer does not silently follow a rename of the
## Wails project name.
!define PRODUCT_EXECUTABLE "kaa-rada.exe"

## PROJECT: installs for the current user, into %LocalAppData%\Programs, and
## therefore asks for no elevation.
##
## Two reasons. The app is per-user already — settings, recordings and the
## Telegram token all live under %AppData% precisely so two people sharing a
## machine do not share one person's footage — so a machine-wide install buys
## nothing. And these binaries are unsigned: an elevation prompt on an unsigned
## installer is the dialog that says "Publisher: Unknown" in red, which is the
## most alarming thing a first-time user sees. Not requesting admin removes
## that prompt entirely. SmartScreen is separate and still applies.
!define REQUEST_EXECUTION_LEVEL "user"
!define WAILS_INSTALL_SCOPE "user"

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
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

## PROJECT: offer to launch straight from the last page. First run is where
## the user enters their Telegram bot token, so getting them into the app
## matters more than usual here.
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Start Kaa Rada"

## PROJECT: Start Menu folder selection. The stock template drops a single
## loose .lnk into the Programs root; a folder lets us also ship an uninstall
## entry, and the chosen name is remembered for the uninstaller.
!define MUI_STARTMENUPAGE_REGISTRY_ROOT "SHELL_CONTEXT"
!define MUI_STARTMENUPAGE_REGISTRY_KEY "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!define MUI_STARTMENUPAGE_REGISTRY_VALUENAME "StartMenuFolder"
!define MUI_STARTMENUPAGE_DEFAULTFOLDER "${INFO_PRODUCTNAME}"
Var StartMenuFolder

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_STARTMENU Application $StartMenuFolder ## PROJECT
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_CONFIRM ## PROJECT: confirm before removing anything
!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## Signing. Not enabled: these binaries ship unsigned, so Windows SmartScreen
## warns on first run. Uncomment once a code signing certificate exists.
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    # Wails apps render their UI in WebView2; this installs the runtime if the
    # machine does not already have it.
    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    ## PROJECT: the installer bundles ONLY what wails.files declares plus the
    ## two entries below. No .env or config.json is ever staged into build\bin,
    ## and CI fails the release if one appears there. Do not add File
    ## directives that glob a whole directory.

    ## OpenCV is built as shared libraries, so the app cannot start without
    ## libopencv_*460.dll and the MinGW runtime beside it. The pattern is
    ## scoped to an extension rather than to the directory: it can only ever
    ## match a DLL, never a .env or a config.json.
    File "..\..\bin\*.dll"

    ## The headless CLI ships with the app so a user can run the detector on a
    ## machine with no display, and it shares the DLLs installed above.
    File "..\..\bin\kaa-rada-cli.exe"

    ## PROJECT: Start Menu folder, plus an uninstall entry beside the app.
    !insertmacro MUI_STARTMENU_WRITE_BEGIN Application
        CreateDirectory "$SMPROGRAMS\$StartMenuFolder"
        CreateShortcut "$SMPROGRAMS\$StartMenuFolder\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
        CreateShortcut "$SMPROGRAMS\$StartMenuFolder\Uninstall ${INFO_PRODUCTNAME}.lnk" "$INSTDIR\uninstall.exe"
    !insertmacro MUI_STARTMENU_WRITE_END

    CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    ## PROJECT: deliberately NOT removing the per-user config directory. It
    ## holds the user's recordings and snapshots alongside config.json, and an
    ## uninstall must not silently delete footage. Reinstalling also keeps
    ## their Telegram settings, which is the behaviour people expect.

    ## PROJECT: remove the Start Menu folder created above.
    !insertmacro MUI_STARTMENU_GETFOLDER Application $StartMenuFolder
    Delete "$SMPROGRAMS\$StartMenuFolder\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\$StartMenuFolder\Uninstall ${INFO_PRODUCTNAME}.lnk"
    RMDir "$SMPROGRAMS\$StartMenuFolder" # only succeeds if now empty, which is intended
    DeleteRegKey SHELL_CONTEXT "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"

    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
