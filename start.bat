@echo off
setlocal
cd /d "%~dp0"
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start-bootstrap.ps1"
set EXIT_CODE=%errorlevel%
if %EXIT_CODE% neq 0 (
  echo.
  echo An error occurred. Press any key to close this window...
  pause >nul
)
endlocal & exit /b %EXIT_CODE%

