@echo off
REM 本番環境用起動スクリプト

echo ======================================
echo  MarkN Resonite Headless Controller
echo  本番環境モード
echo ======================================
echo.

echo [起動] バックエンドサーバーを起動中...
echo ポート: 8080
echo.

REM バックエンドを起動（cross-envで環境変数設定）
call npm start

pause

