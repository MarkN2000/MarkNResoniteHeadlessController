# MarkN Resonite Headless Controller (MRHC)

ResoniteのヘッドレスサーバーをLAN内のブラウザ／スクリプトから操作・管理するツール。

> ⚠️ **現在 v2 を新規に作り直し中**です（Go + React、ランタイム不要の単一バイナリ、Windows / Linux 対応）。
> 旧 v1（Node.js + Svelte）は `dev` / `main` ブランチおよびタグ `v1.1.0` にあります。

## 対応プラットフォーム
Resonite ヘッドレスが動作する以下の環境に対応します（いずれもランタイム不要の単一バイナリ）。

| OS | アーキテクチャ | Go ターゲット | 備考 |
| --- | --- | --- | --- |
| Windows | x64 | `windows/amd64` | 文字コードは Shift_JIS(CP932) を自動判定 |
| Linux | x64 | `linux/amd64` | 文字コードは UTF-8 |
| Linux (ARM) | ARM64 | `linux/arm64` | Raspberry Pi 等。Resonite が ARM Linux で動く環境向け。文字コードは UTF-8 |

> **x64（Windows / Linux）**: Resonite 同梱の .NET ランタイムで起動するため、別途 .NET の導入は不要です。Resonite が Steam で導入済みなら、そのパスを指すだけで動きます。
>
> **ARM（Linux ARM64）** は x64 と少し事情が異なり、MRHC が初回セットアップで導入を補助します:
> - **.NET 10 ランタイムが別途必要**（Resonite 同梱の .NET は x64 のため ARM では使えません）。MRHC が未導入を検知したら導入手順を案内します。
> - **Resonite の入手 / 更新は DepotDownloader**（SteamCMD は ARM 非対応）。MRHC が DepotDownloader を自動取得し、ダウンロードと実行権限付与（`chmod +x`）まで行います。
> - ダウンロードには予備の Steam アカウント（Steam Guard オフ推奨）と、headless ベータコード（Resonite bot に `/headlessCode`）が必要です。

## ドキュメント
- **設計書**: [docs/DESIGN.md](docs/DESIGN.md)
- **Resoniteドメイン事実**（コンソールコマンド・出力書式・起動方法など）: [docs/resonite-domain-facts.md](docs/resonite-domain-facts.md)

## ステータス
v1.0「歩く骨格」を実装中（CLIセットアップ・ヘッドレス起動/停止・ライブログ(SSE)・コマンド送信・認証・React/Mantine製UI・両OS単一バイナリが動作）。以降、段階的に機能を肉付け。

## ビルド / 開発
前提: **Go 1.26+** と **Node 20+**。

```sh
# 1) フロントエンドをビルド（web/dist を生成 → Goが埋め込む）
cd web && npm install && npm run build && cd ..

# 2) バイナリをビルド
go build -o bin/mrhc ./cmd/mrhc                                      # 現OS向け
GOOS=windows GOARCH=amd64 go build -o bin/mrhc-windows-amd64.exe ./cmd/mrhc  # Windows (x64)
GOOS=linux   GOARCH=amd64 go build -o bin/mrhc-linux-amd64      ./cmd/mrhc   # Linux (x64)
GOOS=linux   GOARCH=arm64 go build -o bin/mrhc-linux-arm64      ./cmd/mrhc   # Linux (ARM64)
```
> いずれも **CGO 不要の純 Go**（依存は `golang.org/x/{crypto,sys,term,text}` のみ）なので、上記のように環境変数を変えるだけでクロスビルドできます。リリース用の全ターゲット一括ビルドは GitHub Actions（`.github/workflows/release.yml`）で行います。
> ⚠️ `web/dist` はビルド成果物のため **git管理外**。`go build`（embed.FSで同梱）の前に**必ずフロントをビルド**すること。

開発時:
- バックエンド: `go run ./cmd/mrhc -data ./bin/devdata`（初回は対話セットアップ）
- フロント: `cd web && npm run dev`（`/api` を `:8080` のバックエンドへプロキシ）

## ライセンス
MIT — [LICENSE](LICENSE)
