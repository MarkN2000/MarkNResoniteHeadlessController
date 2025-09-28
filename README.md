# **MarkN Resonite Headless Controller 構想案**

Resoniteのヘッドレスサーバーを、ローカルネットワーク内のPCやスマートフォンのブラウザから簡単に操作・管理するためのツールの構想案です。

## **1\. プロジェクトの目的**

* **操作の簡略化**: CUI（黒い画面）に不慣れなユーザーでも、GUIを通して直感的にサーバーの起動、設定、管理を行えるようにする。  
* **リモート管理**: ローカルネットワーク内の別デバイス（スマホ、タブレット等）からサーバーの状態監視や操作を可能にする。  
* **多機能化**: コマンドをGUIのボタンやフォームに置き換えることで、ワールドの保存、ユーザーの管理、チャットの送信などを簡単に行えるようにする。  
* **外部連携**: Resonite Modなど、外部アプリケーションからのヘッドレスサーバー操作を可能にし、より高度な自動化や連携を実現する。

## **2\. 想定されるアーキテクチャ（仕組み）**

ヘッドレスサーバーとブラウザを仲介する「管理ツール（バックエンド）」をサーバーPC上で動作させる構成を提案します。

\[ブラウザ (フロントエンド)\] \<=\> \[管理ツール (バックエンド)\] \<=\> \[Resoniteヘッドレスサーバー\]  
      (HTTP/WebSocket)                (標準入出力)

1. **ブラウザ (フロントエンド)**  
   * ユーザーが操作する画面。HTML, CSS, JavaScriptで構築。  
   * 管理ツールに対し、操作リクエスト（サーバー起動など）を送信する。  
   * 管理ツールからサーバーログやステータスをリアルタイムで受信し、表示する。  
2. **管理ツール (バックエンド)**  
   * ヘッドレスサーバーと同じPCで動作するWebサーバー。  
   * ブラウザからのリクエストを受け取り、Resoniteヘッドレスサーバーのプロセスを起動・停止する。  
   * ヘッドレスサーバーの標準入力(STDIN)にコマンドを書き込む。  
   * ヘッドレスサーバーの標準出力(STDOUT)からログやコマンドの実行結果を常時読み取り、WebSocket経由でブラウザに送信する。  
   * **外部連携API**: Resonite Modなどの外部アプリケーションからのリクエストを受け付けるためのREST APIエンドポイントを提供する。  
3. **Resonite ヘッドレスサーバー**  
   * 公式提供のサーバー本体。管理ツールによって制御される。

## **3\. UI構造と機能案**

直感的な操作を実現するため、UIを以下の3つのエリアに分割し、各エリアに機能を配置します。各機能の実装は、公式コマンドリファレンスを参考にします。

* **コマンドリファレンス**: [https://wiki.resonite.com/Headless\_server\_software/Commands](https://wiki.resonite.com/Headless_server_software/Commands)

### **ヘッダーエリア (常時表示)**

画面最上部に常に表示され、最も重要なサーバー全体の状況確認と操作を行います。

* **アプリケーション名**: MarkN Resonite Headless Controller  
* **プロファイル選択**: 起動に使う設定プロファイル（config.json）を選択するプルダウンメニュー。  
* **サーバーステータス**: サーバーが起動中か停止中かが一目でわかるインジケーター。  
* **サーバー操作**: \[起動\] \[停止\] \[再起動\] ボタン。

### **サイドバーエリア (常時表示)**

画面の左側に常に表示され、ナビゲーションとリアルタイム監視の役割を担います。

* **セッション一覧と選択**: /worlds コマンドで取得したアクティブな全セッションを一覧表示します。一覧からセッションをクリックすると、そのセッションが操作対象として選択（/focus）され、メインコンテンツエリアの内容が更新されます。  
* **PCリソース監視**: 管理ツールが動作するPCのCPU使用率、メモリ使用率をリアルタイムで表示する簡易グラフ。  
* **ライブログ**: サーバーログをリアルタイムで表示します。エラーや警告などのログレベルに応じた色分けや、キーワードでのフィルタリング機能も提供します。このエリアは折りたたみやサイズ変更が可能であることを想定します。

### **メインコンテンツエリア (タブ切り替え)**

画面の大部分を占めるこのエリアは、タブによって表示する内容を切り替えます。サイドバーで選択したセッションに対する詳細な情報表示と操作がここで行われます。

#### **タブ①: ダッシュボード (デフォルト)**

選択中のセッションに関する主要な情報を集約し、基本的な操作を行います。

* **ステータス表示**: 選択されたセッションに対して/status コマンドが実行され、以下の情報がリアルタイムで表示されます。  
  * **ワールド名 / セッション名**  
  * **セッションID (SessionID)**  
  * **人数 (オンライン/在席/最大) (Current/Present/Max Users)**  
  * **アクセスレベル (Access Level)**  
  * **リスト非表示 (Hidden from listing)**  
  * **説明 (Description)**  
  * **参加ユーザー (Users)**  
* **ユーザーリスト**: /users コマンドで接続ユーザー情報を一覧表示します。各ユーザーの横に操作ボタンや権限設定UIを配置します。  
  * **表示項目**: Username, UserID  
  * **個別操作**:  
    * **ボタン**: \[ミュート\] (/silence), \[ミュート解除\] (/unsilence), \[リスポーン\] (/respawn), \[キック\] (/kick), \[BAN\] (/ban), \[BAN解除\] (/unban)  
    * **権限設定**: プルダウンメニューから新しい権限を選択して設定 (/role) できるようにします。  
* **ワールド操作**: 選択中のセッションに対する操作ボタンを配置します。  
  * \[ワールドを保存\] (/save), \[ワールドを閉じる\] (/close)

#### **タブ②: 新規ワールド**

新しいセッションを開始するための専用エリアです。

* **テンプレートから**: テンプレート名を選択するUIを提供し、セッションを開始します (/startWorldTemplate)。  
* **URLから**: ResoniteのワールドURLを入力するフォームを提供し、セッションを開始します (/startWorldURL)。

#### **タブ③: フレンド管理**

フレンドに関するすべての操作をここで行います。

* **フレンドリクエスト一覧**: /friendRequests で保留中のリクエストを一覧表示します。  
* **リクエストへの対応**: 各リクエストの横に\[承認\] (/acceptFriendRequest) ボタンと\[拒否\] (/removeFriend) ボタンを配置します。  
* **フレンドリクエスト送信**: フレンド名を入力してフレンド申請 (/sendFriendRequest) を送るフォーム。  
* **フレンドへのメッセージ送信**: フレンド名とメッセージを入力して送信 (/message) するための専用フォーム。  
* **フレンド解除**: フレンド名を指定してフレンドを解除 (/removeFriend) するUI。  
* **BANリスト管理**: /listbans でBANユーザーを一覧表示し、解除できるUIを提供します。

#### **タブ④: 設定**

このツールの動作設定や、サーバーの自動化設定を行います。

* **設定プロファイル管理**: 専用フォルダ内の設定JSONファイルをプロファイルとして管理します。  
  * プロファイル一覧の表示と読み込み。  
  * GUIでの設定編集と上書き保存。  
  * 既存プロファイルやプリセットをテンプレートとした新規作成機能。  
* **条件付き自動再起動**: GUIから再起動ルール（例: 毎日定刻、セッションが無人になったら等）を設定可能にします。

#### **タブ⑤: コマンド**

ヘッドレスサーバーに対して、任意のコマンドを手動で実行します。

* **コマンド送信フォーム**:  
  * **手動コマンド実行**: 任意のコマンドを直接入力して実行するための入力フォーム。

## **4\. セキュリティ**

### **GUIアクセス認証 (JWTの導入)**

* **目的**: ローカルネットワーク内の第三者による不正な操作を防ぐため、管理GUIへのアクセスにパスワード認証を導入する。  
* **採用技術**: 認証には、標準的でセキュアな**JWT (JSON Web Token)** を採用する。  
* **認証フロー**:  
  1. **パスワード入力**: ユーザーがGUIに初めてアクセスすると、パスワード入力画面が表示される。  
  2. **JWT発行**: パスワードが正しい場合、バックエンドはJWT（認証トークン）を生成してブラウザに返す。  
  3. **トークンを利用した通信**: ブラウザは受け取ったJWTを保持し、これ以降のWebSocket接続やAPIリクエストの際に、このトークンを付与して送信する。  
  4. **サーバー側検証**: バックエンドは、リクエストの都度JWTの有効性を検証し、無効な場合はアクセスを拒否する。

#### **セキュリティ方針の決定事項**

- **共通シークレット管理**: `.env` などの環境変数に `AUTH_SHARED_SECRET` を定義し、リポジトリには含めない。開発環境では `.env.local` を使用し、本番ではOSの環境変数またはSecret Managerを利用する。
- **パスワード・JWTシークレット**: 認証用初期パスワードとJWTサインシークレットは `AUTH_SHARED_SECRET` から直接読み取って利用する。平文のハードコードは禁止。
- **APIキー認証**: APIキーは `sha256(AUTH_SHARED_SECRET + "api")` など固定サフィックス付きハッシュで派生させ、利用者には共通パスワードと同じ値として案内する。必要な連携先にのみ発行し、JWTと併用して多層防御とする。
- **ローカルネットワーク制限**: `config/security.json` に許可CIDRを定義し、バックエンドでアクセス元IPを検証する。初期値は `192.168.0.0/16` と `10.0.0.0/8` を想定。

### **外部連携API（Resonite Modからの利用）**

* **方針**: ローカルネットワーク内からのアクセスに限定した**REST API**を通じて行う。  
* **セキュリティ**: APIへのアクセスはローカルネットワークに限定。必要に応じて、APIキーによる認証も追加で実装可能。  
* **具体的なAPIエンドポイント（例）**:  
  * POST /api/sessions: 新しいヘッドレスセッションを作成する。  
  * POST /api/sessions/{id}/invite: 既存セッションへ招待を送信する。  
  * GET /api/status: ヘッドレスサーバー全体の現在の状態を取得する。

## **5\. 技術スタック**

* **バックエンド**: **Node.js**  
  * **理由**: リアルタイム通信や外部プロセスの監視といった、このツールの中心的な役割である非同期処理に非常に適しているため。フロントエンドと言語を統一できるメリットも大きい。  
* **フロントエンド**: **Svelte**  
  * **理由**: シンプルな記述で直感的に開発でき、動作が非常に高速かつ軽量なため。個人開発で素早く高品質なUIを構築するのに最適。

## **6\. 開発ロードマップ案**

1. **フェーズ1 (最小構成)**: サーバーの起動・停止機能と、整形されていない生ログの表示機能。  
2. **フェーズ2 (基本機能)**: 設定のGUI編集機能（**ファイルベース**）、/status と /users コマンドによる情報表示機能、PCリソース監視機能、**GUIアクセス認証（JWT）の実装**。  
3. **フェーズ3 (応用機能)**: チャット送信、ユーザーリストからの個別操作、ワールド管理機能、**フレンド管理機能**、条件付き自動再起動機能の実装。  
4. **フェーズ4 (外部連携)**: 外部連携用のREST APIの設計と実装。UI/UXの全体的な改善。

## **7\. 実装の参考とアプローチ**

開発にあたり、以下のオープンソースプロジェクトから得られる知見は非常に有益です。

* **参考リポジトリ**: [hantabaru1014/baru-reso-headless-controller](https://github.com/hantabaru1014/baru-reso-headless-controller)

### **フロントエンド (Svelte) のアプローチ**

* **コンポーネント分割**: ダッシュボードの各機能（サーバー情報、ログ、ユーザーリスト等）を個別のコンポーネントファイルに分割し、管理しやすくする。  
* **ロジックの分離**: WebSocket接続のような中核的なロジックを専用のファイル（例: websocket.ts）に分離し、UIコンポーネントから呼び出す形にすることで、コードの見通しを良くする。  
* **状態管理**: Svelteの「ストア」機能を活用し、サーバーの状態やログ、ユーザーリストといったアプリケーション全体で共有するデータを一元管理する。

### **バックエンド (Node.js) のAPI設計**

* **責務の分離**: REST APIのエンドポイントは、/start、/stop、/send\_commandのように、URLを見るだけで役割が明確に分かるシンプルで直感的な命名規則を採用する。

### **認証のアプローチ**

* Node.jsでJWTを扱うためのライブラリ（例: jsonwebtoken）を活用し、トークンの発行と検証を安全に実装する。

## **8\. 開発環境情報**

### **Resoniteヘッドレスサーバー環境**
- **インストールパス**: `C:\Program Files (x86)\Steam\steamapps\common\Resonite\Headless\Resonite.exe`
- **動作確認**: 完了済み
- **設定ファイル管理**: 独自フォルダ内で複数設定ファイルを管理
- **設定ファイル指定方法**: `Resonite.exe -HeadlessConfig Config/設定ファイル名.json`

### **推奨開発環境**
- **Node.js**: 18.x LTS 以上 (推奨: 20.x LTS)
- **npm**: 9.x 以上
- **開発用ポート**: 3000 (フロントエンド), 8080 (バックエンド)
- **OS**: Windows 10/11 (Resoniteサーバー環境に合わせる)

## **9\. プロジェクト構造**

```
MarkNResoniteHeadlessController/
├── agent/                  # AI協業用ドキュメント (開発計画・タスク分解)
├── backend/                # Node.js バックエンド (API・WebSocket・プロセス管理)
│   ├── src/
│   │   ├── app.ts         # エントリポイント
│   │   ├── config/        # 設定読み込み・バリデーション
│   │   ├── http/          # REST API ルート
│   │   ├── ws/            # WebSocket ハンドラ
│   │   ├── services/      # ドメインロジック (プロセス管理・ログ管理)
│   │   └── utils/         # 共通ユーティリティ
│   ├── tests/             # バックエンドテスト
│   └── package.json
├── config/                 # ランタイム設定
│   ├── headless/          # Resonite 用カスタム config.json 群
│   └── security.json      # 許可CIDR やポリシー設定
├── docs/                   # 追加ドキュメント (API仕様・セットアップ)
├── frontend/               # Svelte フロントエンド
│   ├── src/
│   │   ├── App.svelte     # ルートコンポーネント
│   │   ├── components/    # UIコンポーネント
│   │   ├── routes/        # ページ/レイアウト
│   │   └── stores/        # 状態管理 (Svelte stores)
│   ├── static/            # 静的アセット
│   └── package.json
├── scripts/                # メンテ・ビルド用スクリプト
├── shared/                 # フロント/バックエンド共通型定義・ユーティリティ
├── .env.example            # 環境変数テンプレート (AUTH_SHARED_SECRET 等)
├── package.json            # ルートワークスペース設定 (npm workspaces)
└── README.md
```

### **ディレクトリ運用ポリシー**
- ルート `package.json` で backend・frontend を npm workspaces として管理
- `config/headless` に Resonite 用設定ファイルを保管し、`Resonite.exe -HeadlessConfig` で指定 ([参考](https://wiki.resonite.com/Headless_server_software/Configuration_file))
- `shared/` で TypeScript 型や共通定数を集約し、型整合性を維持
- `scripts/` に開発・デプロイスクリプトを集約し、CI/CD から再利用

## **10\. 技術的詳細の決定**

### **WebSocketライブラリ**
- **採用**: Socket.io
- **理由**: 自動リトライ・名前空間・Room などリアルタイムUIに必要な機能が揃っており、Resonite ログ配信やステータス通知を簡潔に実装可能
- **運用**: バックエンドで Socket.io サーバーを構築し、フロントエンドは公式クライアントで接続。JWT 検証済みのソケットのみイベントを購読させる。

### **状態管理**
- **採用**: Svelte stores (標準機能)
- **理由**: 規模が中程度であり、Svelte のリアクティブストアで十分柔軟かつ軽量。必要に応じて `derived` / `readable` stores を活用し、APIレスポンスやWebSocketイベントを集約する。
- **運用**: `frontend/src/stores/` に zustand など外部ライブラリを追加する前提の拡張余地を残しつつ、標準ストアで開始。

### **UIフレームワーク**
- **採用**: Tailwind CSS + Skeleton UI + カスタムコンポーネント
- **理由**: Tailwind の柔軟性に Skeleton UI のコンポーネント/テーマ機能を乗せることで、ダッシュボード向けUIを効率良く構築できる。Skeleton は Svelte 向けのTailwind拡張で、アクセシビリティやダークモードを標準サポートし、開発を迅速化できる ([Skeleton UI](https://github.com/skeletonlabs/skeleton))。
- **運用**: Tailwind設定 (`tailwind.config.cjs`) に Skeleton プリセットを適用し、テーマカラーを共通管理。Skeletonのコンポーネントをベースにしつつ、必要に応じて Tailwind ユーティリティや自作コンポーネントを `frontend/src/components/` に配置する。
- **ブランドカラー**: Resonite公式ブランドガイドに掲載されているパレットを優先的に使用し、Tailwindのカラートークンへ反映することでUI全体の統一感を維持する ([Resonite Branding](https://wiki.resonite.com/Branding))。

### **ログ保存**
- **採用**: メモリ + ファイルのハイブリッド
- **理由**: UI 表示向けにはバックエンドでリングバッファ (例: 最新 1,000 行) を保持し、即時表示を実現。恒久保存やトラブルシュート向けにはローテーション付きファイル出力を行う。
- **運用**: `services/logService.ts` でヘッドレスプロセスの stdout/stderr を監視し、`winston` 等でファイル出力。UI には WebSocket 経由でストリーム配信し、履歴要求にはファイル読み出しで対応。

## **11\. 付録: コマンドリファレンス一覧**

| Command | Description | Usage |
| :---- | :---- | :---- |
| saveConfig | 現在の設定を元の設定ファイルに保存します。 | saveconfig \<filename\> (optional) |
| login | Resoniteアカウントにログインします。 | login \<username/email\> \<password\> |
| logout | 現在のResoniteアカウントからログアウトします。 | logout |
| message | フレンドリストのユーザーにメッセージを送信します。 | message \<friend name\> \<message\> |
| invite | 現在フォーカスしているワールドにフレンドを招待します。 | invite \<friend name\> |
| friendRequests | 受信したすべてのフレンドリクエストを一覧表示します。 | friendRequests |
| acceptFriendRequest | フレンドリクエストを承認します。 | acceptfriendrequest \<friend name\> |
| sendFriendRequest | フレンドリクエストを送信します。 | sendFriendRequest \<friend name\> |
| removeFriend | フレンドリストからフレンドを削除します。 | removeFriend \<friend name\> |
| worlds | アクティブなすべてのワールドを一覧表示します。 | worlds |
| focus | 特定のワールドにフォーカスします。 | focus \<world id\> |
| startWorldURL | Resonite URLから新しいワールドを開始します。 | startworldurl \<record URL\> |
| startWorldTemplate | テンプレートから新しいワールドを開始します。 | startworldtemplate \<template name\> |
| status | 現在のワールドのステータスを表示します。 | status |
| sessionURL | 現在のセッションのResonite URLをコンソールに出力します。 | sessionurl |
| sessionID | 現在のセッションIDをコンソールに出力します。 | sessionid |
| copySessionURL | 現在のセッションのResonite URLをクリップボードにコピーします。 | copysessionurl |
| copySessionID | 現在のセッションIDをクリップボードにコピーします。 | copysessionid |
| users | 現在フォーカスしているワールドの全ユーザーを一覧表示します。 | users |
| close | 現在フォーカスしているワールドを閉じます。 | close |
| save | 現在フォーカスしているワールドを保存します。 | save |
| restart | 現在フォーカスしているワールドを再起動します。 | restart |
| kick | 指定したユーザーをセッションからキックします。 | kick \<username\> |
| silence | 指定したユーザーをセッション内でサイレンスします。 | silence \<username\> |
| unsilence | 指定したユーザーのサイレンスを解除します。 | unsilence \<username\> |
| ban | 指定したユーザーをこのサーバーがホストする全セッションからBANします。 | ban \<username\> |
| unban | 指定したユーザーのBANを解除します。 | unban \<username\> |
| listbans | アクティブなBANをすべて一覧表示します。 | listbans |
| banByName | 指定したResoniteユーザー名を全セッションからBANします。 | banbyname \<Resonite username\> |
| unbanByName | 指定したResoniteユーザー名のBANを解除します。 | unbanbyname \<Resonite username\> |
| banByID | 指定したResoniteユーザーIDを全セッションからBANします。 | banbyid \<user ID\> |
| unbanByID | 指定したResoniteユーザーIDのBANを解除します。 | unbanbyid \<user ID\> |
| respawn | 指定したユーザーをリスポーンさせます。 | respawn \<username\> |
| role | 指定したユーザーに役割を割り当てます。 (Admin, Builder, Moderator, Guest, Spectator) | role \<username\> \<role\> |
| name | 現在フォーカスしているワールドの新しい名前を設定します。 | name \<new name\> |
| accessLevel | 現在フォーかしているワールドの新しいアクセスレベルを設定します。 | accesslevel \<access level name\> |
| hideFromListing | セッションをリストに表示するかどうかを設定します。 | hidefromlisting \<true/false\> |
| description | 現在フォーカスしているワールドの新しい説明を設定します。 | description \<new description\> |
| maxUsers | 現在フォーカスしているワールドのユーザー上限を設定します。 | maxusers \<number of users\> |
| awayKickInterval | 現在フォーカスしているワールドの離席キック間隔を設定します。 | awaykickinterval \<interval in minutes\> |
| import | アセットを現在フォーカスしているワールドにインポートします。 | import \<file path or Resonite URL\> |
| importMinecraft | Minecraftワールドをインポートします。(Minewaysのインストールが必要) | importminecraft \<level.datを含むフォルダ\> |
| dynamicImpulse | 指定したタグでシーンルートにダイナミックインパルスを送信します。 | dynamicimpulse \<tag\> |
| dynamicImpulseString | 指定したタグと文字列値で非同期ダイナミックインパルスを送信します。 | dynamicimpulsesstring \<tag\> \<value\> |
| dynamicImpulseInt | 指定したタグと整数値で非同期ダイナミックインパルスを送信します。 | dynamicimpulsesint \<tag\> \<value\> |
| dynamicImpulseFloat | 指定したタグと浮動小数点数値で非同期ダイナミックインパルスを送信します。 | dynamicimpulsefloat \<tag\> \<value\> |
| spawn | インベントリから保存済みアイテムをルートにスポーンさせます。 | spawn \<Resonite url\> \<active state\> |
| gc | 完全なガベージコレクションを強制実行します。 | gc |
| shutdown | ヘッドレスをシャットダウンします。(ワールドの状態は保存されません) | shutdown |
| tickRate | サーバーの最大シミュレーションレートを設定します。 | tickrate \<ticks per second\> |
| log | 対話シェルをログ出力に切り替えます。(再度Enterで復帰) | log |
| debugWorldState | 問題診断のためにワールドの状態に関するデバッグ情報を出力します。 | debugWorldState |
| version | ヘッドレスのバージョン番号を出力します。 | version |
