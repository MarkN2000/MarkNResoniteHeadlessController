# MarkN Resonite Headless Controller

Resoniteのヘッドレスサーバーを、ローカルネットワーク内のPCやスマートフォンのブラウザから簡単に操作・管理するためのWebアプリケーションです。

## 概要

- **WebUI**: ブラウザから直感的にヘッドレスサーバーを操作
- **Mod連携**: Resonite ModからのAPI連携機能
- **セキュリティ**: JWT認証、CIDR制限、レート制限による多層防御
- **リアルタイム**: WebSocketによるリアルタイムログ・ステータス表示

## アーキテクチャ

```
[WebUI Browser] ←→ [Backend Server] ←→ [Resonite Headless]
[Resonite Mod]  ←→ [Mod API]        ←→ [Resonite Headless]
```

## 主要機能

### WebUI機能
- **ダッシュボード**: サーバー状態監視、ユーザー管理、ワールド操作
- **新規ワールド**: テンプレート/URLからのワールド起動、ワールド検索機能
- **フレンド管理**: フレンドリクエスト、メッセージ送信
- **設定管理**: 設定ファイルの作成・編集
- **コマンド実行**: 任意のヘッドレスコマンドの実行

### Mod連携機能
- **REST API**: Modからのコマンド送信
- **軽量認証**: API Key認証による高速アクセス
- **ローカル制限**: ローカルネットワーク内からのアクセスのみ許可

## セキュリティ機能

### 認証・認可
- **JWT認証**: WebUI用のトークンベース認証
- **API Key認証**: Mod用の軽量認証
- **CIDR制限**: ローカルネットワーク内からのアクセスのみ許可

### レート制限
- **ログイン試行**: 15分間に5回まで
- **API呼び出し**: 15分間に1000回まで
- **セキュリティ情報**: 15分間に1000回まで
- **リクエストサイズ**: 10MBまで

### CORS設定
- **開発環境**: localhost系のみ許可
- **本番環境**: 指定されたドメインのみ許可
- **Mod用**: ローカルネットワーク内のIPのみ許可

## 技術スタック

- **バックエンド**: Node.js + Express + Socket.io
- **フロントエンド**: Svelte + Tailwind CSS + Skeleton UI
- **認証**: JWT (jsonwebtoken)
- **セキュリティ**: bcryptjs, CIDR制限, レート制限
- **外部連携**: REST API

## セットアップ

### 前提条件
- Node.js 18.x LTS以上
- Resonite Headless Server
- Windows 10/11

### インストール

1. **リポジトリのクローン**
```bash
git clone <repository-url>
cd MarkNResoniteHeadlessController
```

2. **依存関係のインストール**
```bash
npm install
```

3. **設定ファイルの準備**
```bash
# 認証設定
cp config/auth.json.example config/auth.json
# セキュリティ設定
cp config/security.json.example config/security.json
```

4. **環境変数の設定**
```bash
# .envファイルを作成
NODE_ENV=development
AUTH_SHARED_SECRET=your-secret-key-change-in-production
SERVER_PORT=8080
MOD_API_KEY=mod-secret-key
```

### 起動

1. **バックエンドの起動**
```bash
npm run dev --workspace backend
```

2. **フロントエンドの起動**
```bash
npm run dev --workspace frontend
```

3. **アクセス**
- WebUI: `http://localhost:5173`
- デフォルトパスワード: `admin123`

## Mod連携API

### 認証
ModからのAPIアクセスにはAPI Key認証が必要です。

```javascript
// リクエストヘッダー
headers: {
  'Content-Type': 'application/json',
  'X-Mod-Api-Key': 'mod-secret-key'
}
```

### エンドポイント

#### コマンド実行
```http
POST /api/mod/command
Content-Type: application/json
X-Mod-Api-Key: mod-secret-key

{
  "command": "say Hello from Mod!",
  "options": {}
}
```

#### サーバー状態取得
```http
GET /api/mod/status
X-Mod-Api-Key: mod-secret-key
```

#### ログ取得
```http
GET /api/mod/logs
X-Mod-Api-Key: mod-secret-key
```

### 使用例

```javascript
// Modからのコマンド送信例
const response = await fetch('http://192.168.1.100:8080/api/mod/command', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Mod-Api-Key': 'mod-secret-key'
  },
  body: JSON.stringify({
    command: 'say Hello from Mod!'
  })
});

const result = await response.json();
console.log(result);
```

## 設定ファイル

### 認証設定 (`config/auth.json`)
```json
{
  "jwtSecret": "your-secret-key-change-in-production",
  "jwtExpiresIn": "24h",
  "defaultPassword": "admin123"
}
```

### セキュリティ設定 (`config/security.json`)
```json
{
  "allowedCidrs": ["192.168.0.0/16", "10.0.0.0/8"]
}
```

### 環境変数
```bash
# 開発環境
NODE_ENV=development
AUTH_SHARED_SECRET=your-secret-key
SERVER_PORT=8080
MOD_API_KEY=mod-secret-key

# 本番環境
NODE_ENV=production
AUTH_SHARED_SECRET=your-production-secret
SERVER_PORT=8080
MOD_API_KEY=your-production-mod-key
```

## セキュリティ設定

### CIDR制限
ローカルネットワーク内からのアクセスのみ許可：
- `192.168.0.0/16` (家庭用ルーター)
- `10.0.0.0/8` (企業ネットワーク)
- `127.0.0.1` (localhost)

### レート制限
- **ログイン**: 15分間に5回まで（ブルートフォース攻撃防止）
- **API**: 15分間に1000回まで（過度な使用防止）
- **セキュリティ**: 15分間に1000回まで（監視機能用）

### CORS設定
- **WebUI**: 開発環境ではlocalhost、本番環境では指定ドメイン
- **Mod API**: ローカルネットワーク内のIPのみ許可

## トラブルシューティング

### よくある問題

1. **ログインできない**
   - デフォルトパスワード: `admin123`
   - 設定ファイル: `config/auth.json`を確認

2. **Mod APIにアクセスできない**
   - API Key: `mod-secret-key`を確認
   - ローカルネットワーク内からのアクセスか確認
   - CIDR制限の設定を確認

3. **CORSエラー**
   - 開発環境: `http://localhost:5173`からのアクセスか確認
   - 本番環境: 許可されたドメインからのアクセスか確認

### ログの確認
バックエンドのターミナルで以下のログを確認：
- `[SECURITY]`: セキュリティイベント
- `[DEBUG]`: デバッグ情報
- `[RATE_LIMIT]`: レート制限イベント

## 開発

### プロジェクト構造
```
MarkNResoniteHeadlessController/
├── agent/                  # AI協業用ドキュメント
├── backend/                # Node.js バックエンド
│   ├── src/
│   │   ├── app.ts         # エントリポイント
│   │   ├── config/        # 設定管理
│   │   ├── http/          # REST API
│   │   ├── ws/            # WebSocket
│   │   ├── services/      # ビジネスロジック
│   │   ├── middleware/    # ミドルウェア
│   │   └── utils/         # ユーティリティ
│   └── package.json
├── frontend/               # Svelte フロントエンド
│   ├── src/
│   │   ├── routes/        # ページ
│   │   ├── lib/           # ライブラリ
│   │   └── stores/        # 状態管理
│   └── package.json
├── config/                 # 設定ファイル
│   ├── auth.json          # 認証設定
│   ├── security.json      # セキュリティ設定
│   └── headless/          # Resonite設定
└── shared/                 # 共通型定義
```

### 開発コマンド
```bash
# バックエンド開発
npm run dev --workspace backend

# フロントエンド開発
npm run dev --workspace frontend

# 全体的なビルド
npm run build

# テスト
npm run test
```

## ライセンス

このプロジェクトはMITライセンスの下で公開されています。

## 貢献

プルリクエストやイシューの報告を歓迎します。

## 更新履歴

### v1.0.0
- 基本的なWebUI機能
- JWT認証システム
- CIDR制限
- レート制限
- Mod連携API
- ワールド検索機能
- セキュリティ強化

---

## 🔄 自動再起動機能（開発中メモ）

### 設計概要
再起動機能は大きく2つの要素で構成：
1. **再起動トリガー（条件）**: どのような条件で再起動するか
2. **再起動前アクション**: 条件が満たされた後、どのように処理するか

**重要な制約・仕様**:
- 再起動前に、起動に使ったコンフィグファイルが存在するか確認、なければ再起動は無効化
- サーバー停止中は全トリガーを一時停止（起動時にリセット）
- 複数トリガーが同時発動した場合は優先順位に従って1回のみ実行（優先順位: 1.予定 > 2.ユーザー0 > 3.高負荷）
- 高負荷トリガーは起動（再起動含む）から30分間は無効（再起動ループ防止）

### 使用ライブラリ
- **systeminformation**: システムリソース監視（CPU使用率、メモリ使用率の取得）
  - システム全体の使用率を監視
  - クロスプラットフォーム対応
  - インストール: `npm install systeminformation`

### 1. 再起動トリガー

#### 予定再起動（最優先）
- 複数の再起動予定をリスト形式で管理
- 各予定には以下を設定：
  - **再起動タイプ**:
    - 指定日時: 特定の年月日時刻に再起動（例：2025年10月15日 3:00）
    - 毎週: 毎週特定曜日の指定時刻（例：毎週日曜 3:00）
    - 毎日: 毎日指定時刻（例：毎日 3:00）
  - **起動コンフィグファイル**: プルダウンから選択（`config/headless/`内のファイル）
- 実装方法: 再起動トリガー時に、最後に使用したコンフィグファイルのパスを予定で指定されたコンフィグファイルで上書き
- 予定はカード形式でリスト表示され、追加・編集・削除が可能
- 各予定カードにはon/offトグルボタンを配置

#### 高負荷時
- システム全体のメモリ使用率とCPU使用率のどちらかが閾値超過を一定時間継続（例：メモリが80%を10分連続超過）
- 起動（再起動含む）から30分間は監視を無効化（再起動ループ防止・固定値）
- 再起動時は最後に使用したconfigで起動

#### ユーザー0
- 全セッションのheadlessアカウントを除いた合計人数がAFKも含めて0人（つまり、worldsコマンドを実行して、すべてのセッションのusersの値が1）に**変化**したタイミングで再起動をトリガー
- 「複数人→0人」に減った瞬間のみ発動（0人が継続している場合は無視）
- 最後に起動してから指定時間（例：4時間）経っていない場合は無視
- 再起動時は最後に使用したconfigで起動


### 2. 再起動前アクション

#### 待機制御（全トリガー対象）
- ユーザーが0になってから指定時間待機（例：5分）
- 強制再起動までのタイムアウト（例：120分待ってもユーザーがいたら強制実行）
- 強制再起動の何分前に強制再起動前通知・アクションを実行するか（例：2分、この値が以下の全アクションに適用）

#### メッセージで警告
- 強制再起動X分前に全セッションのAFKではないユーザーにチャットメッセージ送信（メッセージ内容を設定）
#### アイテムで警告
- 強制再起動X分前にメッセージアイテムをスポーンして、10秒後にdynamicImpulseString で特定の文字列と指定のメッセージをくっつけたstringを送る（アイテムの種類、メッセージを設定）

#### アクセスレベルをプライベートに変更
- 強制再起動X分前に全セッションに対してアクセスレベルをプライベートに変更（onoffトグルボタン、デフォルトoff）
#### MaxUserを1に変更
- 強制再起動X分前に全セッションに対してMaxUserを1に変更（onoffトグルボタン、デフォルトon）
#### セッション名を変更
- 強制再起動X分前に全セッションに対してセッション名を「指定のテキスト」に変更（セッション名編集用のテキストフィールド、onoffトグルボタン、デフォルトoff）





#### UI/UX
- コンフィグ作成タブ や ダッシュボードタブ のuiとできる限り同じ構成、デザインにする

### UI構成イメージ

```
┌─────────────────────────────────────────────────────────────────┐
│ 自動再起動設定                                                │
└─────────────────────────────────────────────────────────────────┘

[強制再起動ボタン] [手動再起動トリガー]

┌─────────────────────────────────────────────────────────────────┐
│ 1️⃣ 再起動トリガー設定                                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ 📅 予定再起動                        [+ 予定を追加] │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ 予定 #1                                    [ON/OFF Toggle]│   │
│ │ タイプ: [○指定日時 ○毎週 ○毎日]                         │   │
│ │ • 指定日時: [2025]年[10]月[15]日 [03]:[00]              │   │
│ │   または                                                 │   │
│ │ • 毎週: [日曜▼] [03]:[00]                               │   │
│ │   または                                                 │   │
│ │ • 毎日: [03]:[00]                                        │   │
│ │ 起動コンフィグ: [プルダウン: default.json▼]             │   │
│ │                                            [編集] [削除] │   │
│ └───────────────────────────────────────────────────────────┘   │
│ ┌───────────────────────────────────────────────────────────┐   │
│ │ 予定 #2                                    [ON/OFF Toggle]│   │
│ │ タイプ: [○指定日時 ●毎週 ○毎日]                         │   │
│ │ • 毎週: [土曜▼] [02]:[00]                               │   │
│ │ 起動コンフィグ: [test.json▼]                            │   │
│ │                                            [編集] [削除] │   │
│ └───────────────────────────────────────────────────────────┘   │
│                                                                 │
│ ⚡ 高負荷時再起動  CPU閾値: [80]%  メモリ閾値: [80]%  継続時間: [10]分  [ON/OFF Toggle]│
│                                                                 │
│ 👤 ユーザー0時再起動  最小稼働時間: [240]分（起動後この時間未満は無視）  [ON/OFF Toggle]│
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ 2️⃣ 再起動前アクション設定（全トリガー対象）                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   再起動待機: [5]分  │ 
│ 強制実行: [120]分  │ 
│ アクション実行タイミング: [2]分前 │
│                                                                 │
│   💬 チャットメッセージ送信（AFKではないユーザー対象）  [ON/OFF Toggle]│
│      メッセージ: [テキスト入力エリア                          ]│
│                                                                 │
│   📦 アイテムスポーン警告  アイテム種類[プルダウン] [ON/OFF Toggle]  │
│      メッセージ: [テキスト入力エリア                          ]│
│                                                                 │
│   🚫 アクセスレベル変更→プライベート  [ON/OFF Toggle: OFF]    │
│   👥 MaxUser変更→1  [ON/OFF Toggle: ON]                       │
│   📝 セッション名変更  変更後の名前: [テキスト入力]  [ON/OFF Toggle: OFF]│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ 3️⃣ フェールセーフ設定                                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ 🔄 再起動失敗時の処理  リトライ回数: [3]回  リトライ間隔: [30]秒│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ 📊 ステータス表示                                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ • 次回の予定再起動: 2025/10/13（日） 03:00 (予定#1)           │
│ • 現在の稼働時間: 5時間32分                                    │
│ • 現在のCPU使用率: 45% / メモリ使用率: 62%                     │
│ • 現在の合計ユーザー数: 3人（Headless除く）                    │
│ • 最後の再起動: 2025/10/11 22:30 (トリガー: 予定#2)           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

                        [設定を保存]  [リセット]
```

### 実装時の検討事項

1. **設定の保存場所**: JSONファイル (`config/restart.json`)
   - 予定再起動のリストも含む
   - 各予定には一意のIDを付与
   
2. **予定再起動の実装方法**:
   - 予定トリガー発動時に、`runtime-state.json`の最後に使用したコンフィグファイルのパスを予定で指定されたコンフィグファイルで上書き
   - その後通常の再起動処理を実行
   - 指定日時の予定は実行後に自動的に無効化またはリストから削除
   
3. **バックエンドでの実装**: 定期的な条件チェックのインターバル（トリガーの条件ごとに負荷や値の変動間隔が異なるので、個別に設定）
   - 予定再起動: 1分ごとにチェック
   - 高負荷: 1分ごとにチェック
   - ユーザー0: worldsコマンド実行時にチェック
   
4. **再起動方法**: 現在の`stopServer()`の後、停止したのを確認して、5秒後に自動で`startServer()`を呼ぶ

5. **フェールセーフ**: 再起動が失敗した場合の処理（リトライ回数、リトライ間隔）

6. **UIデザインの統一ルール**:
   - **全セクション共通**: コンフィグ作成タブの「基本設定」「セッション設定」と同じデザインパターンを採用
   - **レイアウト構造**:
     ```html
     <div class="card status-card">
       <!-- 入力フォームの場合 -->
       <form class="status-form" on:submit|preventDefault={() => {}}>
         <label>
           <span>項目名</span>
           <div class="field-row">
             <input type="text" />
             <button>ボタン</button>
           </div>
         </label>
       </form>
       
       <!-- 読み取り専用表示の場合 -->
       <div class="status-display-list">
         <div class="status-display-item">
           <span class="status-display-label">項目名</span>
           <div class="field-row">
             <div class="status-display-value">値</div>
           </div>
         </div>
       </div>
     </div>
     ```
   - **スタイル特徴**:
     - 各項目は暗い紺色の背景 (`#11151d`) に配置
     - 左側にタイトル（白色、太字）、右側に値や入力欄を配置
     - 入力欄や値表示エリアは `rgba(17, 21, 29, 0.6)` の背景
     - 値は右寄せで表示
     - 項目間のギャップは `0.75rem`
   - **適用対象**: 
     - 1️⃣ 再起動トリガー設定（入力フォーム）
     - 2️⃣ 再起動前アクション設定（入力フォーム + トグル）
     - 3️⃣ フェールセーフ設定（入力フォーム）
     - 📊 ステータス表示（読み取り専用、実装済み）

---

## 📋 Resoniteヘッドレスコマンドリファレンス

### アカウント管理

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `login` | Resoniteアカウントにログイン | `login <username/email> <password>` |
| `logout` | 現在のResoniteアカウントからログアウト | `logout` |

### フレンド管理

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `message` | フレンドリストのユーザーにメッセージを送信 | `message <friend name> <message>` |
| `invite` | 現在フォーカス中のワールドにフレンドを招待 | `invite <friend name>` |
| `friendRequests` | すべての受信したフレンドリクエストを一覧表示 | `friendRequests` |
| `acceptFriendRequest` | フレンドリクエストを承認 | `acceptfriendrequest <friend name>` |
| `sendFriendRequest` | フレンドリクエストを送信 | `sendFriendRequest <friend name>` |
| `removeFriend` | ヘッドレスからフレンドを削除 | `removeFriend <friend name>` |

### ワールド管理

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `worlds` | すべてのアクティブなワールドを一覧表示 | `worlds` |
| `focus` | 特定のワールドにフォーカス | `focus <world id>` |
| `startWorldURL` | Resonite URLから新しいワールドを開始 | `startworldurl <record URL>` |
| `startWorldTemplate` | テンプレートから新しいワールドを開始 | `startworldtemplate <template name>` |
| `status` | 現在のワールドのステータスを表示 | `status` |
| `close` | 現在フォーカス中のワールドを閉じる | `close` |
| `save` | 現在フォーカス中のワールドを保存 | `save` |
| `restart` | 現在フォーカス中のワールドを再起動 | `restart` |

### セッション情報

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `sessionURL` | 現在のセッションのResonite URLをコンソールに出力 | `sessionurl` |
| `sessionID` | 現在のセッションのIDをコンソールに出力 | `sessionid` |
| `copySessionURL` | 現在のセッションのResonite URLをクリップボードにコピー | `copysessionurl` |
| `copySessionID` | 現在のセッションのIDをクリップボードにコピー | `copysessionid` |

### セッション設定

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `name` | 現在フォーカス中のワールドの名前を設定 | `name <new name>` |
| `accessLevel` | 現在フォーカス中のワールドのアクセスレベルを設定 | `accesslevel <access level name>` |
| `hideFromListing` | セッションをリストから非表示にするかどうかを設定 | `hidefromlisting <true/false>` |
| `description` | 現在フォーカス中のワールドの説明を設定 | `description <new description>` |
| `maxUsers` | 現在フォーカス中のワールドのユーザー制限を設定 | `maxusers <number of users>` |
| `awayKickInterval` | 現在フォーカス中のワールドの離席キック間隔を設定 | `awaykickinterval <interval in minutes>` |

### ユーザー管理

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `users` | 現在フォーカス中のワールドのすべてのユーザーを一覧表示 | `users` |
| `kick` | セッションから指定されたユーザーをキック | `kick <username>` |
| `silence` | セッション内で指定されたユーザーをサイレンス | `silence <username>` |
| `unsilence` | 指定されたユーザーからサイレンスを解除 | `unsilence <username>` |
| `respawn` | 指定されたユーザーをリスポーン | `respawn <username>` |
| `role` | 指定されたユーザーにロールを割り当て（Admin, Builder, Moderator, Guest, Spectator） | `role <username> <role>` |

### BANシステム

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `ban` | このサーバーがホストするすべてのセッションから指定されたユーザーをBAN | `ban <username>` |
| `unban` | 指定されたユーザーのBANを解除 | `unban <username>` |
| `listbans` | すべてのアクティブなBANを一覧表示 | `listbans` |
| `banByName` | Resoniteユーザー名でユーザーをBANする | `banbyname <Resonite username>` |
| `unbanByName` | Resoniteユーザー名でユーザーのBANを解除 | `unbanbyname <Resonite username>` |
| `banByID` | ResoniteユーザーIDでユーザーをBAN | `banbyid <user ID>` |
| `unbanByID` | ResoniteユーザーIDでユーザーのBANを解除 | `unbanbyid <user ID>` |

### アイテム・アセット

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `import` | 現在フォーカス中のワールドにアセットをインポート | `import <file path or Resonite URL>` |
| `importMinecraft` | Minecraftワールドをインポート（Minewaysのインストールが必要） | `importminecraft <folder containing Minecraft world with the level.dat file>` |
| `spawn` | インベントリから保存されたアイテムをルートにスポーン | `spawn <Resonite url> <active state>` |

### ダイナミックインパルス

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `dynamicImpulse` | 指定されたタグでシーンルートにダイナミックインパルスを送信 | `dynamicimpulse <tag>` |
| `dynamicImpulseString` | 指定されたタグと文字列値で非同期ダイナミックインパルスを送信 | `dynamicimpulsesstring <tag> <value>` |
| `dynamicImpulseInt` | 指定されたタグと整数値で非同期ダイナミックインパルスを送信 | `dynamicimpulsesint <tag> <value>` |
| `dynamicImpulseFloat` | 指定されたタグと浮動小数点値で非同期ダイナミックインパルスを送信 | `dynamicimpulsefloat <tag> <value>` |

### システム管理

| コマンド | 説明 | 使用方法 |
|---------|------|---------|
| `saveConfig` | 現在の設定を元のconfigファイルに保存 | `saveconfig <filename>` (オプション、未指定の場合は上書き保存) |
| `gc` | 完全なガベージコレクションを強制実行 | `gc` |
| `shutdown` | ヘッドレスをシャットダウン（ワールド状態は保存されません） | `shutdown` |
| `tickRate` | サーバーの最大シミュレーションレートを設定 | `tickrate <ticks per second>` |
| `log` | 対話型シェルをログ出力モードに切り替え（Enterキーで復帰） | `log` |
| `debugWorldState` | ワールドの状態に関するデバッグ情報を出力 | `debugWorldState` |
| `version` | ヘッドレスが実行中のバージョン番号を出力 | `version` |

### 使用例

```bash
# ワールドを開始
startworldtemplate Grid

# ワールドにフォーカス
focus 0

# ワールド名を変更
name My Awesome World

# アクセスレベルを変更
accesslevel Friends

# 最大ユーザー数を設定
maxusers 16

# ダイナミックインパルスでメッセージを送信
dynamicimpulsestring MessageBoard "Hello from Headless!"

# ワールドを保存
save
```