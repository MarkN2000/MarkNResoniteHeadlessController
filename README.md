<p align="right"><b>日本語</b> | <a href="README.en.md">English</a></p>

# MarkN Resonite Headless Controller (MRHC)

![MRHC の操作画面](docs/images/screenshot.jpg)

Resonite のヘッドレスサーバーを、LAN 内のブラウザから操作・管理するツールです。

**対応プラットフォーム:** Windows (x64) ／ Linux (x64) ／ Linux (ARM)

ランタイム不要の単一バイナリで動作し、Resonite 本体は MRHC が自分のフォルダ内へ自動でダウンロードします。

## 事前に確認

MRHC を使うには、次の準備が必要です。

- **Resonite のヘッドレスコード（必須）** — Resonite を[サブスクリプション](https://account.resonite.com/)（Stripe）で月 $10 以上のプランで支援し、ヘッドレス用のコードを取得している必要があります（→ [取得方法](#ヘッドレスコードの取得方法)）。
- **Steam のサブアカウント** — Resonite 本体の自動ダウンロード／更新に Steam を使います。MRHC はこのパスワードを保存し、二段階認証には対応していないため、普段使いとは別の **サブアカウントを新しく作成** し、その **Steam Guard（二段階認証）は必ずオフ** にしてください（→ [やり方](#steam-guard-off)）。
- **Resonite 本体は自動ダウンロード** — このソフトが Resonite を取得するため、事前にダウンロードしておく必要はありません。Steam クライアントのインストールも不要です。
- **空き容量に余裕のある場所へ** — Resonite 本体とキャッシュで相応の容量を使います（古いキャッシュを自動で削除する機能があります）。
- **設置場所は先に決めてください** — Resonite 本体や設定はフォルダ内に保存され、その絶対パスが記録されます。**一度起動（特に Resonite のダウンロード）した後は、フォルダを移動・改名しないでください。** 使いたい場所に置いてから起動します。
- **接続は LAN 内から** — Web UI（管理画面）には **同じネットワーク内からのみ** アクセスできます。外出先から使うには VPN や Tailscale などのセットアップが必要です（→ [VPS で動かす場合](#install-vps)）。

## インストール

環境を選んでください: [Windows](#install-windows) ／ [Linux（x64 / ARM）](#install-linux) ／ [VPS（Oracle Cloud 等）](#install-vps)

<a id="install-windows"></a>
### Windows

1. [リリースページ](https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest)から `mrhc-windows-amd64.zip` をダウンロードします。
2. **使いたい場所で展開**し、フォルダ内の `mrhc.exe` を実行します（起動後はフォルダを移動しないでください）。
3. 初回はセットアップウィザード（日本語／英語）が起動します。管理パスワード・ポートを設定すると、続けて Resonite 本体のダウンロードが始まります。**ダウンロードには時間がかかります。「今すぐサーバーを起動しますか?」と表示されるまでお待ちください。**

起動したら、ブラウザで `http://localhost:8080`（既定）を開くと Web UI が使えます。

> **SmartScreen の警告について** — 未署名のため、初回実行時に「Windows によって PC が保護されました」と表示されることがあります。「詳細情報」→「実行」で起動できます。
>
> **データの置き場所** — 設定・状態・ダウンロードした Resonite 本体は、すべて `mrhc.exe` と同じフォルダ内にまとまっています。バックアップはこのフォルダごとコピーしてください（前述のとおり、起動後の移動・改名は避けてください）。

<a id="install-linux"></a>
### Linux（x64 / ARM）

置きたい場所へ移動し、次の 1 行を実行します。x64・ARM のどちらも同じコマンドで、アーキテクチャは自動で判定されます。

```sh
cd ~   # 置きたい場所へ（例: ホーム直下。必ず存在します）
curl -fsSL https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download/install.sh | sh
```

実行した場所に `mrhc-linux-amd64/`（ARM では `mrhc-linux-arm64/`）が作られるので、その中で起動します。

```sh
cd mrhc-linux-amd64   # ARM では mrhc-linux-arm64
./mrhc
```

初回はセットアップウィザード（日本語／英語）が起動します。管理パスワード・ポートを設定すると Resonite 本体のダウンロードが始まります（時間がかかります。「今すぐサーバーを起動しますか?」と表示されるまでお待ちください）。完了するとそのままサーバーが立ち上がり、ブラウザで `http://<サーバーの IP>:8080`（既定）から Web UI が使えます。

.NET ランタイムや DepotDownloader は MRHC が自動で取得するため、ファイアウォール以外に事前準備は基本的に不要です（freetype2 など依存が不足している場合は、セットアップ時に導入の案内があります）。

> **データの置き場所** — 設定・状態・Resonite 本体は、すべて `mrhc` と同じフォルダ内にまとまっています（別の場所に置きたい場合は `-data <dir>` で指定できます）。起動後はフォルダを移動・改名しないでください。手動で展開したい場合は、[最新リリース](https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest)の `mrhc-linux-amd64.tar.gz` ／ `mrhc-linux-arm64.tar.gz` を好きな場所に展開してください（実行権を保持しているため `chmod +x` は不要です）。

**ファイアウォール（参考）** — 同じ LAN の他ユーザーがセッションに参加できない・見つからないときは、LAN からの UDP 受信を許可します（`192.168.1.0/24` はお使いの LAN のアドレス帯に合わせてください）。

- ufw（Ubuntu 系・CachyOS など）: `sudo ufw allow from 192.168.1.0/24 proto udp`
- firewalld（Fedora・RHEL 系・openSUSE など）: `sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="192.168.1.0/24" protocol value="udp" accept' && sudo firewall-cmd --reload`
- 素の Arch Linux など、ファイアウォールが標準で無効な環境では設定は不要です。

<a id="install-vps"></a>
### VPS（Oracle Cloud 等）

> ⚠️ **重要: Web UI は LAN 内からしか開けません。** VPS 上で動かす場合、インターネットから直接アクセスすることはできません。**SSH トンネル** または **Tailscale などの VPN** で「同じネットワークにいる」状態を作ってからアクセスします。Web UI は平文 HTTP のため、ポートを直接インターネットに公開しないでください（管理パスワードが平文で流れてしまいます）。

**おすすめ構成（本節はこれを前提に説明します）**

- Oracle Cloud の **Ampere A1（ARM）** インスタンス
- OS は **Ubuntu**

**1. 導入**（Linux ARM と同じ） — SSH でログインし、置きたい場所で次を実行します。

```sh
curl -fsSL https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download/install.sh | sh
cd mrhc-linux-arm64 && ./mrhc
```

**2. Web UI にアクセスする**（次のどちらかの方法で「同じネットワークにいる」状態を作ります）

**方法 A: SSH トンネル**（追加インストール不要・ポート開放も不要）

手元の PC で次を実行します（スマホからは Termius などの SSH アプリでも可）。

```sh
ssh -L 8080:localhost:8080 <ユーザー>@<VM の IP>
```

接続したまま、手元のブラウザで `http://localhost:8080` を開きます。SSH（22 番）が通っている時点でファイアウォールは通っているので、追加の開放は不要です。

**方法 B: Tailscale**（VPN。スマホからも使いやすい）

VPS と手元の端末を同じ Tailscale ネットワーク（tailnet）に入れると、その間は同一ネットワーク扱いになり、Web UI に直接アクセスできます。

1. VPS に Tailscale を入れて接続します。

   ```sh
   curl -fsSL https://tailscale.com/install.sh | sh
   sudo tailscale up
   ```

   表示される URL を開いてログイン・承認します。
2. 手元の PC ／ スマホにも [Tailscale](https://tailscale.com/download) を入れ、同じアカウントでログインします。
3. VPS の Tailscale IP を確認し（VPS で `tailscale ip -4`）、手元のブラウザで `http://<その IP>:8080` を開きます。

**3.（任意）セッションへの直接参加を速くする — UDP ポートの開放**

ポートを開けなくても、Resonite のリレー経由で他ユーザーはセッションに参加できます（その分だけ遅延が増えます）。直接接続で遅延を抑えたい場合のみ設定してください。クラウド VM のファイアウォールは **2 層**（クラウド側のセキュリティルール ＋ VM 内）あり、両方で開放が必要です。

1. **ポート番号を固定する** — Web UI の「コンフィグ」タブでワールドを開き、「詳細フィールド」から `forcePort` に任意の番号（例: `32100`）を設定 → 保存 → ヘッドレスを再起動します（既定はランダムポートのため、固定が必要です）。
2. **クラウド側を開放する** — Oracle Cloud コンソール → 対象 VCN の「セキュリティリスト」（または NSG）→ イングレス規則を追加します（ソース `0.0.0.0/0`・プロトコル `UDP`・宛先ポート `32100`）。
3. **VM 内（Ubuntu）を開放する** — Oracle の Ubuntu は raw iptables が有効です（ufw は既定で無効）。

   ```sh
   sudo iptables -I INPUT -p udp --dport 32100 -j ACCEPT
   sudo netfilter-persistent save
   ```

   `-I INPUT` は規則を **先頭に挿入** します（Oracle 既定の末尾 REJECT より前に置くため。`-A`〔末尾追記〕では REJECT に弾かれて効きません）。

> 誰でもセッションに参加できるようになりますが、アクセス制御はファイアウォールではなく Resonite 側の accessLevel で行います。

## アップデート

MRHC 本体は、内蔵の自己更新機能でアップデートできます。

- **Web UI から** — 新しいバージョンがあると、画面右上の ⋮ に赤丸が付きます。⋮ →「更新を確認」→「アップデート」で、ダウンロード・検証・差し替えまで自動で行われ、**次に MRHC を起動し直した時から** 新バージョンになります（差し替え自体は稼働中のワールドに影響しません）。続けて「今すぐ終了する」を押すとワールドを順に停止して MRHC が終了するので、あとは起動し直すだけです。
- **コマンドラインから** — `./mrhc update`（Windows: `mrhc.exe update`）。MRHC が起動できない状態からの復旧手段としても使えます。

> 手動でアップデートする場合は、MRHC を停止してから install.sh を再実行（Linux）、または zip を同じ場所へ上書き展開（Windows）してください。設定・データはアーカイブに含まれないため、どの方法でも保持されます。

## ヘッドレスコードの取得方法

Resonite のヘッドレスサーバーは **非公開ベータ** として配布されており、ダウンロード・起動には「ヘッドレスコード」が必要です。

1. Resonite を[サブスクリプション](https://account.resonite.com/)（Stripe・手数料が安く公式推奨）で支援し、ヘッドレスが利用できるティア（月 $10 以上）になります。
2. Resonite を起動し、フレンドにいる **Resonite bot** へ **`/headlessCode`** とメッセージを送ります。
3. 返ってきたコードを、MRHC のセットアップウィザード（または「設定 → Steam」のブランチコード欄）に入力します。

> コードは変更されることがあります。更新後に動かなくなったら、もう一度 `/headlessCode` で最新のコードを取得して設定し直してください。

## 困ったとき

- **ヘッドレスのログを見たい** — Web UI の「ログ」タブで、Resonite ヘッドレスのログファイル（`<インストール先>/Headless/Logs`）を選んで表示・コピーできます（読み取り専用・停止中でも閲覧可能）。大きいログは末尾のみ表示されます。稼働中の現行ログは、OS によっては読み取れないことがあります。
- **ディスク容量が気になる（キャッシュ）** — 設定タブの「キャッシュ管理」で、Resonite キャッシュ（既定 `headless-cache`）の合計サイズ確認・全削除（ヘッドレス停止中のみ）ができます。「キャッシュ自動削除」を ON にすると、停止のたびに、最終更新が指定日数（既定 30 日）より古いキャッシュを自動で掃除します。削除しても、必要なものは次回に自動で再取得されます。
- **管理パスワードを忘れた** — サーバー機のコマンドラインで `./mrhc reset-password`（Windows: `mrhc.exe reset-password`）を実行すると、旧パスワードなしで再設定できます。
- **アップデートの途中で失敗して起動できなくなった** — 実行ファイルの隣に `mrhc.exe.old`（Linux: `mrhc.old`）が残っていれば、それを `mrhc.exe`（`mrhc`）に名前を戻すと元のバージョンに復旧できます。
- **セットアップを最初からやり直したい** — フォルダ内の `mrhc.config.json` を削除してもう一度起動すると、ウィザードが再実行されます。
- **ポートを変えたい／ポートが使用中と表示される** — `mrhc.config.json` の `"port"` を他の番号に書き換えて、もう一度起動してください。
- **同じ LAN からセッションに入れない／見つからない** — サーバー機側で、LAN からの UDP 受信を許可してください。
  - **Windows**: 接続中のネットワークを「**プライベート ネットワーク**」に変更します（設定 → ネットワークとインターネット → イーサネット、または Wi-Fi）。
  - **Linux**: ファイアウォールが有効な場合は、LAN からの UDP を許可します（[Linux のインストール](#install-linux)の「ファイアウォール（参考）」を参照）。
- <a id="steam-guard-off"></a>**Steam Guard をオフにできない** — スマホの「モバイル認証器」を設定済みのアカウントは、まずスマホの Steam アプリ側で解除し（Steam ガード → 認証機器を削除）、その後で Steam の「設定 → セキュリティ」からオフにします。
- **表示言語を変えたい** — `mrhc.config.json` の `"language"` を `"ja"` ／ `"en"` に書き換えて再起動します（Web UI の表示言語は、画面右上の切り替えで別管理です）。

## 主要機能

LAN 内のブラウザから、ヘッドレスサーバーを丸ごと運用できます。

**起動・停止・状態監視**
- ヘッドレス（Resonite プロセス）の起動／通常停止（安全に約2分で停止）／強制停止／再起動
- 稼働状況のライブ表示：参加者（在席／離席）、ワールド情報、稼働時間、セッション URL
- Resonite 出力のライブログ（SSE）と、ログファイルの閲覧・コピー
- サーバーのシステム使用率（CPU／メモリ／ディスク空き）の表示
- 任意のコンソールコマンドの送信

**セッション運営**
- 新しいセッションを開く：テンプレート／レコード URL／**キーワード（ワールド名・タグ）でワールド検索**して起動、お気に入り登録
- セッション設定の編集：名前・アクセスレベル・最大人数・説明・一覧から隠す
- 参加者の操作：キック／BAN／ミュート／リスポーン／ロール変更／メッセージ送信
- アイテムのスポーン（URL 指定）とダイナミックインパルスの送信（タグ＋値）

**ユーザー管理**
- ユーザー名／ユーザー ID での検索、フレンド申請の送受信・解除
- フレンドリクエストの承認、フォーカス中セッションへの招待
- 全セッション対象の BAN と BAN 解除

**コンフィグ（ヘッドレス設定）**
- 複数のコンフィグを作成・複製・リネーム・保存して切り替え
- 公式スキーマ準拠でワールド単位に詳細設定（アクセスレベル・タグ・AFK キック・自動保存・自動復帰・ロール・自動招待・forcePort など）
- スキーマの全項目を「詳細フィールド」から追加可能

**自動再起動・メンテナンス**
- 予定再起動（一度きり／毎日／毎週）。事前告知（dynamicImpulse）や Private 化・改名などの事前アクション付き
- 参加者がいるときは退出を待ってから安全に再起動
- クラッシュからの自動復帰（暴走を防ぐクラッシュ回数ガード付き）
- 予定再起動のついでに Resonite を自動アップデート
- キャッシュ管理（停止時の自動削除・手動全削除・サイズ確認）

**セットアップ・配布**
- Resonite 本体を DepotDownloader で自動ダウンロード／更新（全 OS・ARM 対応・.NET ランタイムも自動設置）
- 不足依存（freetype2 等）の検出・導入案内
- MRHC 自身の自己アップデート（Web UI／CLI）
- 日本語／英語対応（セットアップウィザード・Web UI）、ランタイム不要の単一バイナリ（Windows／Linux x64・ARM）

## ドキュメント

- 設計書: [docs/DESIGN.md](docs/DESIGN.md)
- Resonite ドメイン事実（コンソールコマンド・出力書式・起動方法など）: [docs/resonite-domain-facts.md](docs/resonite-domain-facts.md)

## ビルド / 開発

前提: **Go 1.26+** と **Node 20+**。

```sh
# 1) フロントエンドをビルド（web/dist を生成 → Go が埋め込む）
cd web && npm install && npm run build && cd ..

# 2) バイナリをビルド
go build -o bin/mrhc ./cmd/mrhc                                              # 現在の OS 向け
GOOS=windows GOARCH=amd64 go build -o bin/mrhc-windows-amd64.exe ./cmd/mrhc  # Windows (x64)
GOOS=linux   GOARCH=amd64 go build -o bin/mrhc-linux-amd64      ./cmd/mrhc   # Linux (x64)
GOOS=linux   GOARCH=arm64 go build -o bin/mrhc-linux-arm64      ./cmd/mrhc   # Linux (ARM64)
```

いずれも **CGO 不要の純 Go**（依存は `golang.org/x/{crypto,sys,term,text}` のみ）なので、環境変数を変えるだけでクロスビルドできます。リリース用の全ターゲット一括ビルドは GitHub Actions（`.github/workflows/release.yml`）で行います。

> ⚠️ `web/dist` はビルド成果物のため **git 管理外** です。`go build`（embed.FS で同梱）の前に、必ずフロントエンドをビルドしてください。

開発時:

- バックエンド: `go run ./cmd/mrhc -data ./bin/devdata`（初回は対話セットアップ）
- フロントエンド: `cd web && npm run dev`（`/api` を `:8080` のバックエンドへプロキシ）

## ライセンス

MIT — [LICENSE](LICENSE)

本ソフトは Resonite のヘッドレスサーバーを操作するツールです。利用にあたっては、Resonite の[ガイドライン](https://resonite.com/policies/Guidelines.html)・利用規約に従ってください。
