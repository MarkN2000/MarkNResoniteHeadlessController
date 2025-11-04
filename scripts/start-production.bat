@echo off
REM 本番環境用起動スクリプト

echo ======================================
echo  MarkN Resonite Headless Controller
echo  Production Mode
echo ======================================
echo.

echo [INFO] Starting backend server...
echo Port: 8080
echo.

set SCRIPT_DIR=%~dp0
pushd "%SCRIPT_DIR%.."

set NODE_ENV=production
set APP_ROOT=%CD%
node backend\dist\backend\src\app.js

popd

pause

