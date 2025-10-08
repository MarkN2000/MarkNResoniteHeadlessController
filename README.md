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