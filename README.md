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

> **.NET ランタイムは自動で用意されます（全 OS / ARM 共通）**: Resonite のダウンロード品に .NET ランタイムは含まれませんが、MRHC が Resonite のダウンロード後（および起動時に不足を検知したとき）に公式ビルドから自動設置します（管理者権限不要・設置先は Resonite フォルダ内）。システムに .NET 10 が導入済みの場合はそれをそのまま使い、何も変更しません。
>
> **Resonite の入手 / 更新は DepotDownloader**（SteamCMD は ARM 非対応のため全 OS で統一）:
> - MRHC が DepotDownloader を自動取得し、ダウンロードと実行権限付与（`chmod +x`）まで行います。
> - ダウンロードには予備の Steam アカウント（Steam Guard オフ推奨）と、headless ベータコード（Resonite bot に `/headlessCode`）が必要です。
> - 既に Steam で Resonite を導入済みの場合は、設定でそのインストール先を指定して流用できます。

## インストール

> ⚠️ 以下のダウンロードリンクは **v2 の初回リリース公開後に有効**になります。

### Linux

置きたい場所（例: `~/servers`）で次の 1 行を実行します:

```sh
curl -fsSL https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download/install.sh | sh
```

実行した場所に `mrhc-linux-amd64/`（ARM では `mrhc-linux-arm64/`）フォルダが作られるので、その中で起動します:

```sh
cd mrhc-linux-amd64 && ./mrhc
```

初回はセットアップウィザード（日本語/英語）が起動し、管理パスワード・ポート・Resonite 本体のダウンロード（Steam アカウントが必要・スキップ可）まで対話で完結して、そのままサーバーが立ち上がります。

手動で導入する場合は[最新リリース](https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest)から `mrhc-linux-amd64.tar.gz` / `mrhc-linux-arm64.tar.gz` を取得し、好きな場所に展開してください。**tar.gz は実行権を保持しているため `chmod +x` は不要です。**

### Windows

[mrhc-windows-amd64.zip](https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download/mrhc-windows-amd64.zip) をダウンロード → 展開 → フォルダ内の `mrhc.exe` を実行（初回は Linux と同じセットアップウィザードが起動します）。

> 未署名のため、初回実行時に SmartScreen の警告（「Windows によって PC が保護されました」）が出ることがあります。「詳細情報」→「実行」で起動できます。

### 更新

MRHC 本体の更新は内蔵の自己更新で行えます。

- **Web UI から**: 新しいバージョンがあると画面右上の ⋮ に赤丸が付きます。⋮ →「更新を確認」→「アップデート」でダウンロード・検証・差し替えまで自動で行われ、**次に MRHC を起動し直した時から**新バージョンになります（差し替え自体は稼働中のワールドに影響しません）。そのまま「今すぐ終了する」を押せばワールドを順に停止して MRHC が終了するので、あとは起動し直すだけです。
- **コマンドラインから**: `./mrhc update`（Windows: `mrhc.exe update`）。MRHC が起動できない状態からの復旧手段としても使えます。

従来どおり、MRHC を停止してから install.sh を再実行（Linux）または zip を同じ場所に上書き展開（Windows）しても更新できます。設定・データはアーカイブに含まれないため、どの方法でも保持されます。

> install.sh の再実行は、展開フォルダの名前を変えている場合は別フォルダが新規作成されるため、tar.gz を手動で展開して中身を上書きしてください。

### データの置き場所

設定・状態・ダウンロードした Resonite 本体は、すべて**実行ファイルと同じフォルダ内**に保存されます（フォルダごと移動・バックアップ可能）。別の場所に置きたい場合は `-data <dir>` で指定できます。

### 困ったとき

- **管理パスワードを忘れた** — サーバー機のコマンドラインで `./mrhc reset-password`（Windows: `mrhc.exe reset-password`）を実行すると、旧パスワードなしで再設定できます。
- **更新の途中で失敗して起動できなくなった** — 実行ファイルの隣に `mrhc.exe.old`（Linux: `mrhc.old`）が残っていれば、それを `mrhc.exe`（`mrhc`）に名前を戻すと元のバージョンに復旧できます。
- **セットアップを最初からやり直したい** — フォルダ内の `mrhc.config.json` を削除してもう一度起動すると、ウィザードが再実行されます。
- **ポートを変えたい／ポートが使用中と表示される** — `mrhc.config.json` の `"port"` を他の番号に書き換えて、もう一度起動してください。
- **同じ LAN からセッションに入れない／見つからない** — サーバー機側で LAN からの UDP 受信を許可してください。
  - **Windows**: ネットワークの設定で、接続中のネットワークを「**プライベート ネットワーク**」に変更します（設定 → ネットワークとインターネット → イーサネット（または Wi-Fi））。
  - **Linux**: ファイアウォールが有効な場合は LAN からの UDP を許可します（`192.168.1.0/24` の部分はお使いの LAN のアドレス帯に合わせてください）。
    - ufw の場合（Ubuntu 系・CachyOS・Manjaro など）: `sudo ufw allow from 192.168.1.0/24 proto udp`
    - firewalld の場合（Fedora・RHEL 系・openSUSE など）: `sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="192.168.1.0/24" protocol value="udp" accept' && sudo firewall-cmd --reload`
    - 素の Arch Linux など、ファイアウォールが標準で無効な環境では設定不要です。
- **Steam Guard をオフにできない** — スマホの「モバイル認証器」を設定済みのアカウントは、スマホの Steam アプリ側（Steamガード → 認証機器を削除）で解除してから、Steam の 設定 → セキュリティ でオフにします。
- **表示言語を変えたい** — `mrhc.config.json` の `"language"` を `"ja"` / `"en"` に書き換えて再起動します（Web UI の表示言語は画面右上の切替で別管理）。

## ドキュメント
- **設計書**: [docs/DESIGN.md](docs/DESIGN.md)
- **Resoniteドメイン事実**（コンソールコマンド・出力書式・起動方法など）: [docs/resonite-domain-facts.md](docs/resonite-domain-facts.md)

## ステータス
v2 を実装中。コア機能は実装済み（CLIセットアップ・ヘッドレス起動/停止/再起動・ライブログ(SSE)・全タブの Web UI・スケジュール再起動・Steam（DepotDownloader）経由の Resonite 入手/更新・MRHC 自身の自己更新（Web UI / CLI）・依存検出と導入案内・Windows / Linux 単一バイナリ）。残りは ARM 実機検証とリリース準備。

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
