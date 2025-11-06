@echo off
setlocal

REM このスクリプトのあるディレクトリへ移動
cd /d "%~dp0"

echo Node.js のインストールを確認しています...
where node >nul 2>nul
if %errorlevel% neq 0 (
    echo -------------------------------------------------------------------
    echo [ERROR] Node.js が見つかりません。
    echo MarkN Resonite Headless Controller を実行するには Node.js 20 以降が必要です。
    echo https://nodejs.org/ からインストールして再度お試しください。
    echo -------------------------------------------------------------------
    pause
    exit /b 1
)

echo.
echo 起動準備を開始します...
node scripts\start.js

set EXIT_CODE=%errorlevel%
endlocal & exit /b %EXIT_CODE%

