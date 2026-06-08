# Wails regenerates build\windows\installer\project.nsi from its default template on
# every `wails build`, which would drop our customizations. Rather than fragile
# string surgery (which previously double-applied and corrupted the shortcut lines),
# this script deterministically writes a complete, known-good project.nsi after
# generation. It is fully idempotent: running it any number of times yields the
# same file. wails_tools.nsh (the generated defines: product name, version, etc.)
# is left untouched and included as usual.
#
# Customizations baked in here:
#   * Installer output is named "<ProjectName>-Setup.exe" (no -amd64 suffix).
#   * Start Menu and Desktop shortcuts carry the app icon.
#   * The Finish page offers to launch the app, started via Explorer so it runs
#     de-elevated (the installer itself runs as admin).

$ErrorActionPreference = 'Stop'

$Path = Join-Path $PSScriptRoot '..\build\windows\installer\project.nsi'
$installerDir = Split-Path $Path -Parent
$toolsPath = Join-Path $installerDir 'wails_tools.nsh'

if (!(Test-Path $toolsPath)) {
  Write-Error "wails_tools.nsh not found in $installerDir - run 'wails build -nsis' first so Wails generates the installer scaffolding."
  exit 1
}

$nsi = @'
Unicode true

# Keep the public installer compact without using executable packers such as UPX,
# which can hurt antivirus/SmartScreen trust for new apps.
SetCompressor /SOLID lzma
SetCompressorDictSize 64

####
## This file is written by scripts\patch-nsis-shortcuts.ps1 after `wails build`.
## Edit the script, not this file - Wails regenerates it and the script overwrites it.
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

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.

# Offer to launch the app from the finish page (checkbox, checked by default).
# LaunchAppDeElevated starts it through Explorer so the app runs with the user's
# normal medium integrity instead of the installer's elevated admin token - a
# clipboard / drag-and-drop tool must be de-elevated to see the user's Explorer.
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${INFO_PRODUCTNAME} now"
!define MUI_FINISHPAGE_RUN_FUNCTION "LaunchAppDeElevated"

!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

Name "${INFO_PRODUCTNAME}"
# Installer file name: <ProjectName>-Setup.exe (e.g. Ignore-Setup.exe), no arch suffix.
OutFile "..\..\bin\${INFO_PROJECTNAME}-Setup.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files
    File /oname=icon.ico "..\icon.ico"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}" "" "$INSTDIR\${PRODUCT_EXECUTABLE}" 0
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}" "" "$INSTDIR\${PRODUCT_EXECUTABLE}" 0

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

# Launch the installed app via Explorer so it inherits the logged-in user's medium
# integrity level rather than the installer's elevated (admin) token.
Function LaunchAppDeElevated
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
'@

$enc = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($Path, $nsi, $enc)
Write-Host "Wrote custom NSIS installer script: $Path"
