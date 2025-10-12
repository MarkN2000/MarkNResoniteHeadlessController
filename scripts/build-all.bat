@echo off
REM 全体ビルドスクリプト

echo ======================================
echo  MarkN Resonite Headless Controller
echo  本番環境用ビルド
echo ======================================
echo.

echo [1/4] 共通型定義をビルド中...
call npm run build --workspace shared
if %errorlevel% neq 0 (
    echo [エラー] 共通型定義のビルドに失敗しました。
    pause
    exit /b 1
)

echo.
echo [2/4] バックエンドをビルド中...
call npm run build --workspace backend
if %errorlevel% neq 0 (
    echo [エラー] バックエンドのビルドに失敗しました。
    pause
    exit /b 1
)

echo.
echo [3/4] フロントエンドをビルド中...
call npm run build --workspace frontend
if %errorlevel% neq 0 (
    echo [エラー] フロントエンドのビルドに失敗しました。
    pause
    exit /b 1
)

echo.
echo [4/4] ビルド後の確認...
if not exist "backend\dist" (
    echo [エラー] backend\dist ディレクトリが見つかりません。
    pause
    exit /b 1
)

if not exist "frontend\build" (
    echo [エラー] frontend\build ディレクトリが見つかりません。
    pause
    exit /b 1
)

echo.
echo ======================================
echo  ビルド完了！
echo ======================================
echo.
echo 次のステップ:
echo   1. .env ファイルで NODE_ENV=production を設定
echo   2. scripts\start-production.bat で起動
echo   3. http://localhost:8080 にアクセス
echo.
pause

