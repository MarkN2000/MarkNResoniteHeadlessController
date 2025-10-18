@echo off
echo Building MarkNResoniteHeadlessController for distribution...

REM 依存関係のインストール
echo Installing dependencies...
call npm install

REM フロントエンドのビルド
echo Building frontend...
call npm run build --workspace=frontend

REM バックエンドのビルド
echo Building backend...
call npm run build --workspace=backend

REM EXEファイルの作成
echo Creating executable...
call npm run build:exe

REM 配布用ディレクトリの作成
echo Creating distribution directory...
if exist "dist\distribution" rmdir /s /q "dist\distribution"
mkdir "dist\distribution"

REM EXEファイルをコピー
echo Copying executable...
copy "backend\dist\MarkNResoniteHeadlessController.exe" "dist\distribution\"

REM フロントエンドのビルドファイルをコピー
echo Copying frontend files...
xcopy "frontend\build\*" "dist\distribution\frontend\" /E /I

REM サンプル設定ファイルをコピー
echo Copying sample configuration files...
mkdir "dist\distribution\config"
copy "config\*.example" "dist\distribution\config\"
copy "sample\*" "dist\distribution\config\"

REM READMEファイルをコピー
echo Copying documentation...
copy "DISTRIBUTION_README.md" "dist\distribution\README.md"
copy "DISTRIBUTION_REQUIREMENTS.md" "dist\distribution\"

echo.
echo Distribution build completed!
echo Files are available in: dist\distribution\
echo.
echo To run the application:
echo 1. Navigate to dist\distribution\
echo 2. Run MarkNResoniteHeadlessController.exe
echo 3. Open http://localhost:8080 in your browser
echo.
pause
