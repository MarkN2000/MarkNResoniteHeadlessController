@echo off
REM Windows setup script

echo ======================================
echo  MarkN Resonite Headless Controller
echo  Initial Setup
echo ======================================
echo.

REM Verify Node.js installation
where node >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Node.js is not installed.
    echo Please install Node.js 20.x or newer.
    echo https://nodejs.org/
    pause
    exit /b 1
)

echo [1/3] Running setup script...
set SCRIPT_DIR=%~dp0
pushd "%SCRIPT_DIR%.."
node "%SCRIPT_DIR%setup.js"
if %errorlevel% neq 0 (
    echo [ERROR] Failed to execute setup script.
    pause
    popd
    exit /b 1
)

echo.
echo [2/3] Installing dependencies...
call npm install
if %errorlevel% neq 0 (
    echo [ERROR] Failed to install dependencies.
    pause
    popd
    exit /b 1
)

echo.
echo [3/3] Building shared workspace...
call npm run build --workspace shared
if %errorlevel% neq 0 (
    echo [ERROR] Build failed.
    pause
    popd
    exit /b 1
)

echo.
echo ======================================
echo  Setup Complete!
echo ======================================
echo.
echo Next steps:
echo   1. Update the .env file for your environment
echo   2. Update config\auth.json to set a secure password
echo   3. Start development servers with npm run dev
echo.
pause
popd

