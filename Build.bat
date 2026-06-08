@echo off
setlocal

cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
  echo Go is not installed or is not available in PATH.
  pause
  exit /b 1
)

set "GO_BIN_DIR="
for /f "delims=" %%G in ('go env GOBIN') do set "GO_BIN_DIR=%%G"
if "%GO_BIN_DIR%"=="" (
  for /f "delims=" %%G in ('go env GOPATH') do set "GOPATH_DIR=%%G"
  if not "%GOPATH_DIR%"=="" set "GO_BIN_DIR=%GOPATH_DIR%\bin"
)
if not "%GO_BIN_DIR%"=="" set "PATH=%GO_BIN_DIR%;%PATH%"

where npm >nul 2>nul
if errorlevel 1 (
  echo Node.js/npm is not installed or is not available in PATH.
  pause
  exit /b 1
)

where wails >nul 2>nul
if errorlevel 1 (
  echo Wails CLI is not installed. Installing Wails now...
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  if errorlevel 1 (
    echo Failed to install Wails CLI.
    pause
    exit /b 1
  )
  if not "%GO_BIN_DIR%"=="" set "PATH=%GO_BIN_DIR%;%PATH%"
  where wails >nul 2>nul
  if errorlevel 1 (
    if exist "%GO_BIN_DIR%\wails.exe" (
      set "WAILS_EXE=%GO_BIN_DIR%\wails.exe"
    ) else (
      echo Wails installed, but wails.exe could not be found.
      echo Expected location:
      echo %GO_BIN_DIR%\wails.exe
      pause
      exit /b 1
    )
  )
)

if "%WAILS_EXE%"=="" set "WAILS_EXE=wails"

set "PATH=%PATH%;C:\Program Files (x86)\NSIS;C:\Program Files\NSIS"
where makensis >nul 2>nul
if errorlevel 1 (
  echo NSIS is required to create the installer and was not found.
  echo Installing NSIS with winget...
  where winget >nul 2>nul
  if errorlevel 1 (
    echo winget is not available, so NSIS cannot be installed automatically.
    echo Install NSIS manually, then run Build.bat again:
    echo https://nsis.sourceforge.io/Download
    pause
    exit /b 1
  )
  winget install -e --id NSIS.NSIS --source winget --accept-source-agreements --accept-package-agreements
  if errorlevel 1 (
    echo Failed to install NSIS.
    pause
    exit /b 1
  )
  set "PATH=%PATH%;C:\Program Files (x86)\NSIS;C:\Program Files\NSIS"
  where makensis >nul 2>nul
  if errorlevel 1 (
    echo NSIS was installed, but makensis.exe is still not available in PATH.
    echo Close this window and run Build.bat again.
    pause
    exit /b 1
  )
)

where ffmpeg >nul 2>nul
if errorlevel 1 (
  echo ffmpeg is required to generate brand icons and was not found in PATH.
  pause
  exit /b 1
)

if not exist "assets\ignore_logo_transparent.png" (
  echo Missing brand source image:
  echo assets\ignore_logo_transparent.png
  pause
  exit /b 1
)

if not exist "ui\node_modules" (
  echo Installing frontend dependencies...
  npm install --prefix ui
  if errorlevel 1 (
    echo Failed to install frontend dependencies.
    pause
    exit /b 1
  )
)

if not exist "ui\node_modules\rcedit" (
  echo Installing icon patch dependency...
  npm install --save-dev rcedit --prefix ui
  if errorlevel 1 (
    echo Failed to install rcedit.
    pause
    exit /b 1
  )
)

echo Running Go tests...
go test ./...
if errorlevel 1 (
  echo Tests failed.
  pause
  exit /b 1
)

echo Preparing brand icons...
if not exist "build" mkdir "build"
if not exist "build\windows" mkdir "build\windows"
if not exist "ui\public\brand" mkdir "ui\public\brand"
ffmpeg -y -v error -i "assets\ignore_logo_transparent.png" -vf scale=1024:1024 -frames:v 1 -update 1 "build\appicon.png"
ffmpeg -y -v error -i "assets\ignore_logo_transparent.png" -filter_complex "[0:v]split=5[v16][v32][v48][v128][v256];[v16]scale=16:16[s16];[v32]scale=32:32[s32];[v48]scale=48:48[s48];[v128]scale=128:128[s128];[v256]scale=256:256[s256]" -map "[s16]" -map "[s32]" -map "[s48]" -map "[s128]" -map "[s256]" "build\windows\icon.ico"
ffmpeg -y -v error -i "assets\ignore_logo_transparent.png" -vf scale=32:32 -frames:v 1 -update 1 "ui\public\brand\ignore-logo-32.png"
ffmpeg -y -v error -i "assets\ignore_logo_transparent.png" -vf scale=64:64 -frames:v 1 -update 1 "ui\public\brand\ignore-logo-64.png"
ffmpeg -y -v error -i "assets\ignore_logo_transparent.png" -vf scale=128:128 -frames:v 1 -update 1 "ui\public\brand\ignore-logo-128.png"
copy /Y "build\windows\icon.ico" "ui\public\brand\favicon.ico" >nul

taskkill /F /IM "Ignore.exe" >nul 2>nul
if exist "build\bin" rmdir /s /q "build\bin"

echo Building optimized Ignore installer...
%WAILS_EXE% build -nsis -trimpath -ldflags "-s -w" -webview2 download
if errorlevel 1 (
  echo Build or installer creation failed.
  pause
  exit /b 1
)

if not exist "build\bin\Ignore.exe" (
  echo Build finished, but build\bin\Ignore.exe was not found.
  pause
  exit /b 1
)

if exist "build\windows\icon.ico" (
  copy /Y "build\windows\icon.ico" "build\bin\icon.ico" >nul
)

echo Re-applying brand icon to installer package...
ffmpeg -y -v error -i "assets\ignore_logo_transparent.png" -filter_complex "[0:v]split=5[v16][v32][v48][v128][v256];[v16]scale=16:16[s16];[v32]scale=32:32[s32];[v48]scale=48:48[s48];[v128]scale=128:128[s128];[v256]scale=256:256[s256]" -map "[s16]" -map "[s32]" -map "[s48]" -map "[s128]" -map "[s256]" "build\windows\icon.ico"
copy /Y "build\windows\icon.ico" "build\bin\icon.ico" >nul
echo Patching Ignore.exe icon resources...
node scripts\patch-icon.mjs
if errorlevel 1 (
  echo Failed to patch Ignore.exe icon.
  pause
  exit /b 1
)
echo Signing Ignore.exe (skipped if no certificate configured)...
powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\sign.ps1" -Path "build\bin\Ignore.exe"
if errorlevel 1 (
  echo Code signing of Ignore.exe failed.
  pause
  exit /b 1
)
if exist "build\windows\installer\project.nsi" (
  powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\patch-nsis-shortcuts.ps1"
  if errorlevel 1 (
    echo Failed to patch installer shortcuts.
    pause
    exit /b 1
  )
)
taskkill /F /IM "Ignore-Setup.exe" >nul 2>nul
taskkill /F /IM "Ignore-amd64-installer.exe" >nul 2>nul
del /q "build\bin\*installer*.exe" >nul 2>nul
del /q "build\bin\Ignore-Setup.exe" >nul 2>nul
if exist "build\windows\installer\project.nsi" (
  makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\Ignore.exe "build\windows\installer\project.nsi"
  if errorlevel 1 (
    echo Failed to rebuild branded installer.
    pause
    exit /b 1
  )
)

echo Signing Ignore-Setup.exe (skipped if no certificate configured)...
powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\sign.ps1" -Path "build\bin\Ignore-Setup.exe"
if errorlevel 1 (
  echo Code signing of the installer failed.
  pause
  exit /b 1
)

echo Verifying release artifacts...
powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\verify-release.ps1" -SkipDefenderScan
if errorlevel 1 (
  echo Release verification failed.
  pause
  exit /b 1
)
echo.
echo NOTE: If the verifier says the artifacts are unsigned, Windows SmartScreen
echo may block website downloads as an unrecognized app. Configure a trusted
echo code-signing certificate before public release.
echo.

echo Build sizes:
powershell -NoProfile -ExecutionPolicy Bypass -Command "$files=@('build\bin\Ignore.exe','build\bin\Ignore-Setup.exe'); foreach($f in $files){ if(Test-Path $f){ $i=Get-Item $f; '{0}: {1:N2} MB' -f $i.Name, ($i.Length/1MB) } }"
echo.

set "SETUP_EXE="
for /f "delims=" %%I in ('powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-ChildItem -Path 'build','dist','.' -Recurse -Filter '*.exe' -ErrorAction SilentlyContinue | Where-Object { $_.FullName -notmatch '\\\\ui\\\\node_modules\\\\' -and $_.FullName -notmatch '\\\\build\\\\bin\\\\Ignore.exe$' -and ($_.Name -match 'setup|installer|install|windows-amd64') } | Sort-Object LastWriteTime -Descending | Select-Object -First 1 -ExpandProperty FullName"') do set "SETUP_EXE=%%I"

if not "%SETUP_EXE%"=="" (
  echo Installer created:
  echo %SETUP_EXE%
  echo Refreshing Windows icon cache...
  ie4uinit.exe -show >nul 2>nul
  echo Opening installer...
  set "INSTALLER_TO_OPEN=%SETUP_EXE%"
  powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process -FilePath $env:INSTALLER_TO_OPEN -Verb RunAs"
  if errorlevel 1 (
    echo Could not open installer automatically. Open it manually:
    echo %SETUP_EXE%
    start "" "build\bin"
    pause
    exit /b 1
  )
  pause
  exit /b 0
)

echo.
echo The app exe was built, but no installer exe was found.
echo Expected Wails to create an NSIS installer from:
echo %WAILS_EXE% build -nsis
echo.
echo Check the build output above for NSIS packaging errors.
if exist "build\bin" start "" "build\bin"
pause
exit /b 1
