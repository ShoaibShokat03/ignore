@echo off
setlocal

:: Repository URL
set REPO_URL=https://github.com/ShoaibShokat03/ignore-setup.git

:: Folder name (derived from repo name)
set REPO_DIR=ignore-setup

echo Cloning repository...
git clone %REPO_URL%

if errorlevel 1 (
    echo Failed to clone repository.
    pause
    exit /b 1
)

cd /d %REPO_DIR%

:: Delete .git folder
if exist ".git" (
    rmdir /s /q ".git"
    echo Deleted .git folder.
)

:: Delete README.md
if exist "README.md" (
    del /f /q "README.md"
    echo Deleted README.md.
)

echo Done.
pause