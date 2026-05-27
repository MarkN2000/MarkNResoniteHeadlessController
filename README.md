# MarkN Resonite Headless Controller (MRHC)

ResoniteのヘッドレスサーバーをLAN内のブラウザ／スクリプトから操作・管理するツール。

> ⚠️ **現在 v2 を新規に作り直し中**です（Go + React、ランタイム不要の単一バイナリ、Windows / Linux 対応）。
> 旧 v1（Node.js + Svelte）は `dev` / `main` ブランチおよびタグ `v1.1.0` にあります。

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
go build -o bin/mrhc ./cmd/mrhc                         # 現OS向け
GOOS=windows GOARCH=amd64 go build -o bin/mrhc.exe ./cmd/mrhc  # Windows向け
GOOS=linux   GOARCH=amd64 go build -o bin/mrhc    ./cmd/mrhc   # Linux向け
```
> ⚠️ `web/dist` はビルド成果物のため **git管理外**。`go build`（embed.FSで同梱）の前に**必ずフロントをビルド**すること。

開発時:
- バックエンド: `go run ./cmd/mrhc -data ./bin/devdata`（初回は対話セットアップ）
- フロント: `cd web && npm run dev`（`/api` を `:8080` のバックエンドへプロキシ）

## ライセンス
MIT — [LICENSE](LICENSE)
