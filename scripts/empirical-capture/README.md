# 実機採取（構造化Driver 設計検証）

Resoniteヘッドレスを複数ワールドで起動し、各コマンドの実出力・プロンプト挙動・ambient混入・完了タイミングを採取する。採取結果を `docs/resonite-domain-facts.md` と突き合わせ、`docs/design/structured-driver.md` の妥当性を実証する。

## 同梱
- `test-multi-world.json` — 2ワールド・Private・非ログイン（必要なら login を追記）
- `capture.sh` — 起動 → 待機 → コマンド送信 → SSE 保存
- このREADME

## 前提
- Linux機（Resoniteヘッドレスがある）
- `mrhc-poc-linux`（手元にある PoC バイナリ）をこのフォルダに置く
- 本番ヘッドレスが**動いていない**こと（同じ Data path で重複起動はできない）

## 実行
```sh
# 1. このフォルダに mrhc-poc-linux / test-multi-world.json / capture.sh を揃える
chmod +x capture.sh

# 2. 実行（bash必須。fishからは bash capture.sh）
./capture.sh
```
所要時間: 起動～完了まで約 1〜2 分。完了後 `capture.log` が生成される。

## 環境変数で上書き
```sh
PORT=8100 OUT=cap2.log ./capture.sh
DOTNET=/path/to/dotnet HEADLESS=/path/to/Headless CONFIG=./other.json ./capture.sh
```

## 採取後
`capture.log` の全文（または `send>` 周辺を中心に）を共有してください。私が解析して
docs/設計の更新と、必要ならパーサ修正点を洗い出します。
