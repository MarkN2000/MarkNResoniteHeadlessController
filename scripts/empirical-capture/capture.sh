#!/usr/bin/env bash
# MRHC 構造化Driver 実機採取スクリプト
# Resoniteヘッドレスを test-multi-world.json で起動し、コマンド系列を順次送信して
# SSE(/events) を1ファイルに保存する。あとでログを解析する。
#
# 使い方（Linux機・bashで実行・fishからは `bash capture.sh`）:
#   1. mrhc-poc-linux, test-multi-world.json, このスクリプトを同じディレクトリに置く
#   2. chmod +x capture.sh && ./capture.sh
#   3. 完了後、capture.log を貼る
#
# 環境変数で上書き可:
#   DOTNET=...   同梱dotnetのパス
#   HEADLESS=... Headlessフォルダ
#   CONFIG=...   ヘッドレスconfigパス
#   PORT=8099    PoCのHTTPポート
#   OUT=...      保存先ログファイル

set -u

DOTNET="${DOTNET:-$HOME/.local/share/Steam/steamapps/common/Resonite/dotnet-runtime/dotnet}"
HEADLESS="${HEADLESS:-$HOME/.local/share/Steam/steamapps/common/Resonite/Headless}"
CONFIG="${CONFIG:-./test-multi-world.json}"
PORT="${PORT:-8099}"
OUT="${OUT:-./capture.log}"

for f in "$DOTNET" "$HEADLESS/Resonite.dll" "$CONFIG" "./mrhc-poc-linux"; do
  if [[ ! -e "$f" ]]; then echo "見つかりません: $f"; exit 1; fi
done

rm -f "$OUT"
echo "== 起動 =="
echo "  dotnet=$DOTNET"
echo "  headless=$HEADLESS"
echo "  config=$CONFIG"
echo "  port=$PORT  out=$OUT"

./mrhc-poc-linux -addr ":$PORT" -dir "$HEADLESS" -cmd "$DOTNET" -- Resonite.dll -HeadlessConfig "$CONFIG" >/dev/null 2>&1 &
POC_PID=$!
trap 'kill $POC_PID 2>/dev/null; pkill -f Resonite.dll 2>/dev/null' EXIT INT TERM

# SSE を長時間バックグラウンドで捕捉
( curl -s --max-time 600 "localhost:$PORT/events" > "$OUT" ) &
CURL_PID=$!

echo "== 起動待機（最大120秒で 'World running...' を待つ） =="
DEADLINE=$(( $(date +%s) + 120 ))
READY=0
while [[ $(date +%s) -lt $DEADLINE ]]; do
  if [[ -f "$OUT" ]] && grep -q "World running" "$OUT" 2>/dev/null; then READY=1; break; fi
  sleep 2
done
if [[ $READY -eq 0 ]]; then
  echo "!! 起動が確認できませんでした。capture.log の末尾を確認してください。続行を中止します。"
  kill $CURL_PID 2>/dev/null
  exit 2
fi
echo "ready! 少し待ってからコマンド送信開始"
sleep 5

send() {
  local cmd="$1"
  echo "send> $cmd"
  curl -s -X POST "localhost:$PORT/command" --data-urlencode "cmd=$cmd" >/dev/null
  sleep "${2:-3}"
}

echo "== コマンド系列 =="
send "worlds"
send "focus 0"
send "status"
send "users"
send "focus 1"
send "status"
send "users"
send 'name "テスト改名"'
send "worlds"
send "accesslevel LAN"
send "maxusers 8"
send "listbans"
send "friendrequests"
send "dummycommand"
send "shutdown" 12   # shutdown は終了処理に時間がかかる

echo "== 収束待ち =="
sleep 6
kill $CURL_PID 2>/dev/null
pkill -f Resonite.dll 2>/dev/null
echo ""
echo "== 完了 =="
echo "ログ: $OUT ($(wc -l < "$OUT" 2>/dev/null) 行)"
echo "貼り付け推奨範囲: 全文（長ければ末尾〜80行 or 'send>' 周辺）"
