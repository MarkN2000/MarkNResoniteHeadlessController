@echo off
REM Windows用セットアップスクリプト

echo ======================================
echo  MarkN Resonite Headless Controller
echo  初回セットアップ
echo ======================================
echo.

REM Node.jsがインストールされているか確認
where node >nul 2>&1
if %errorlevel% neq 0 (
    echo [エラー] Node.jsがインストールされていません。
    echo Node.js 20.x以上をインストールしてください。
    echo https://nodejs.org/
    pause
    exit /b 1
)

echo [1/3] セットアップスクリプトを実行中...
node scripts/setup.js
if %errorlevel% neq 0 (
    echo [エラー] セットアップスクリプトの実行に失敗しました。
    pause
    exit /b 1
)

echo.
echo [2/3] 依存関係をインストール中...
call npm install
if %errorlevel% neq 0 (
    echo [エラー] 依存関係のインストールに失敗しました。
    pause
    exit /b 1
)

echo.
echo [3/3] ビルド中...
call npm run build --workspace shared
if %errorlevel% neq 0 (
    echo [エラー] ビルドに失敗しました。
    pause
    exit /b 1
)

echo.
echo ======================================
echo  セットアップ完了！
echo ======================================
echo.
echo 次のステップ:
echo   1. .env ファイルを編集して設定を変更
echo   2. config\auth.json を編集してパスワードを変更
echo   3. npm run dev で開発サーバーを起動
echo.
pause

