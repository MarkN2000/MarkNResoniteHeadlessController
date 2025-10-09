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

### 1. 再起動トリガー（条件）

#### 時間ベース
- 毎日指定時刻（例：毎日3:00 AM）
- 毎週特定曜日の指定時刻（例：毎週日曜3:00 AM）
- 指定時間ごと（例：6時間ごと）
- 累計稼働時間（例：連続稼働48時間経過後）

#### リソースベース
- メモリ使用率が閾値超過（例：80%を10分連続超過）
- CPU使用率が閾値超過
- プロセスのメモリリーク検知（メモリ使用量が徐々に増加している場合）

#### ユーザー状況ベース
- 全セッション合計ユーザー数が0 + 最終再起動から指定時間経過
- 全セッションが空の状態が指定時間継続

#### 複合条件
- 複数条件を「AND」「OR」で組み合わせ
  - 例：「毎日3時 AND 全セッション空」
  - 例：「メモリ80%超過 OR 48時間経過」

### 2. 再起動前アクション

#### 通知系
- 全ユーザーにチャットメッセージ送信
- メッセージアイテムをスポーン
- 再起動までのカウントダウン表示（例：10分前、5分前、1分前）
- Discord/他の外部通知連携（将来的に）

#### セッション制御
- アクセスレベルをプライベートに変更
- 新規ユーザーの入室を禁止
- セッション説明文に「まもなく再起動」と表示

#### 待機制御
- ユーザーが0になるまで待機
- ユーザーが0になってから指定時間待機（例：5分）
- 強制再起動までのタイムアウト（例：60分待ってもユーザーがいたら強制実行）
- 「今すぐ再起動」ボタン（手動でトリガー可能）

### 3. 追加機能

#### 安全機能
- 再起動スケジュールのプレビュー（次回再起動予定時刻の表示）
- 再起動履歴ログ
- ドライラン（実際には再起動せずに条件チェックのみ）
- 再起動のキャンセル機能（条件を満たしたが、手動でキャンセル）

#### UI/UX
- 複数の再起動プロファイル保存（例：「毎日定期」「週末メンテ」など）
- 有効/無効の切り替え
- 次回再起動までの残り時間表示

### UI構成イメージ

```
┌─ その他タブ ─────────────────────────────┐
│                                          │
│ 【自動再起動設定】                        │
│                                          │
│ ■ 有効/無効トグル                        │
│                                          │
│ ┌─ 再起動トリガー ───────────────────┐  │
│ │ □ 時間ベース                          │  │
│ │   ├ 毎日 [03:00] に実行               │  │
│ │   └ 累計稼働 [48] 時間後              │  │
│ │                                       │  │
│ │ □ リソースベース                      │  │
│ │   └ メモリ [80]% を [10]分 連続超過   │  │
│ │                                       │  │
│ │ □ ユーザー状況                        │  │
│ │   └ 全セッション空 + [2]時間 経過後   │  │
│ │                                       │  │
│ │ 条件組み合わせ: [AND / OR]            │  │
│ └───────────────────────────────────┘  │
│                                          │
│ ┌─ 再起動前アクション ───────────────┐  │
│ │ ☑ ユーザー退出まで待機                │  │
│ │   ├ 最大待機時間: [60] 分             │  │
│ │   └ 退出後の待機: [5] 分              │  │
│ │                                       │  │
│ │ ☑ 全ユーザーに通知                    │  │
│ │   ├ [10]分前に警告                    │  │
│ │   └ メッセージ: [メンテナンスの...]   │  │
│ │                                       │  │
│ │ ☑ セッションをプライベート化           │  │
│ └───────────────────────────────────┘  │
│                                          │
│ ┌─ ステータス ────────────────────────┐  │
│ │ 次回予定: 2025/10/10 03:00           │  │
│ │ 最終実行: 2025/10/09 03:00           │  │
│ │ 現在状態: 待機中                      │  │
│ └───────────────────────────────────┘  │
│                                          │
│ [設定を保存] [今すぐ再起動]              │
└──────────────────────────────────────┘
```

### 実装時の検討事項

1. **設定の保存場所**: JSONファイル (`config/restart.json`) or データベース？
2. **バックエンドでの実装**: 定期的な条件チェックのインターバル（例：1分ごと）
3. **再起動方法**: 現在の`stopServer()`の後、自動で`startServer()`を呼ぶ
4. **フェールセーフ**: 再起動が失敗した場合の処理（リトライ、アラート）
5. **複数コンフィグ対応**: 各コンフィグごとに個別の再起動設定が必要か？