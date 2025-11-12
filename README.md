# MarkN Resonite Headless Controller

Resoniteのヘッドレスサーバーを、ローカルネットワーク内のPCやスマートフォンのブラウザから簡単に操作・管理するためのWebアプリケーションです。

## 使い方（配布Zipからのセットアップ）

1. 配布された `MarkNResoniteHeadlessController.zip` を任意のディレクトリに展開します。
2. Node.js 20 以上がインストールされていることを確認します。
3. 初回セットアップは `start.bat` を実行（ダブルクリック）します。指示にしたがってパスワードなどを設定してください。
4. 2回目以降も `start.bat` を実行するだけでバックエンドを起動できます（セットアップはスキップされます）。
5. ローカルネットワーク内でブラウザから `<ヘッドレスPCのIPアドレス（例：192.168.1.~）>:8080>` にアクセスし、初回セットアップで設定したパスワードでログインしてください。

アップデートはフォルダごと上書きしてください。

## 技術スタック

- **バックエンド**: Node.js + Express + Socket.io
- **フロントエンド**: Svelte + Tailwind CSS + Skeleton UI
- **認証**: JWT (jsonwebtoken)
- **セキュリティ**: bcryptjs, CIDR制限, レート制限
- **外部連携**: REST API

## 開発者向けセットアップ

### 前提条件
- Node.js 18.x LTS以上
- Resonite Headless Server
- Windows 10/11

#### 配布用Zipの作成（Windows）

```bash
npm run package:zip
```

コマンド実行時に `npm run build` も自動で走り、最新ビルド成果物を含む `dist/MarkNResoniteHeadlessController.zip` が生成されます。`.env` や実運用の `config/*.json` は Zip に含まれません。既にビルド済みの場合は `powershell -File scripts/package-distribution.ps1 -SkipBuild` を使うと再ビルドを省略できます。

## API

ローカルネットワーク内からpostリクエストでAPIを呼び出して、ヘッドレスサーバーを操作できます。

### エンドポイント（単一）
```http
POST /api/headless
Content-Type: application/json; charset=utf-8
```

### 認証
- API Key はリクエストボディに含めます（ヘッダー/クエリは使用しません）。
- API Key はアプリのログインパスワード（`config/auth.json` の `password` または環境変数 `DEFAULT_PASSWORD`）と同じものを使用します。

### セキュリティ制限（維持）
- **CIDR制限**: ローカルネットワーク内からのアクセスのみ許可（デフォルト: `192.168.0.0/16`, `10.0.0.0/8`, `127.0.0.1`）
- **レート制限**: 15分間に1000リクエストまで（`X-RateLimit-*` ヘッダ付与）
- **CORS**: ローカル/プライベートIPのオリジンのみ許可
- **リクエストサイズ**: 10MBまで

### リクエストボディ（共通）
```json
{
  "version": 1,
  "timestamp": "2025-11-11T03:00:00.000Z",
  "apiKey": "your-api-key",
  "action": "sessionlist",
  "params": {},
  "requestId": "abc-123"
}
```
- `version`: number（必須、現行は 1 固定）
- `timestamp`: string（必須、ISO8601）
- `apiKey`: string（必須）
- `action`: string（必須、実装済み: `sessionlist`, `focus`, `invite`, `setaccesslevel`, `setrole`, `startworld`）
- `params`: object（任意、アクション固有の引数。詳細は各アクションの説明を参照）
- `requestId`: string（任意、相関ID）

### レスポンス（共通）
成功時（200 OK）
```json
{
  "ok": true,
  "action": "sessionlist",
  "timestamp": "2025-11-11T03:00:01.234Z",
  "requestId": "abc-123",
  "data": { /* アクション固有の構造化データ */ }
}
```
- `ok`: boolean（常に `true`）
- `action`: string（リクエストの `action` を反映）
- `timestamp`: string（サーバー処理時刻、ISO8601）
- `requestId`: string（任意、リクエストに含まれていた場合のみ）
- `data`: any（アクション固有の結果データ）

失敗時の例
```json
{
  "ok": false,
  "action": "sessionlist",
  "timestamp": "2025-11-11T03:00:01.234Z",
  "requestId": "abc-123",
  "error": { "code": "INVALID_API_KEY", "message": "Invalid API key" }
}
```
HTTPステータス例：
- `401 Unauthorized`（APIキー不正/未指定）
- `400 Bad Request`（必須項目/型不正, `version` 不正 等）
- `403 Forbidden`（CIDR制限）
- `404 Not Found`（未知の `action`）
- `415 Unsupported Media Type`（JSON以外のContent-Type）
- `429 Too Many Requests`（レート制限超過）
- `500 Internal Server Error`（実行時例外）

### 実装済みアクション

#### 1. `sessionlist` - セッション一覧取得

ヘッドレスの `worlds` コマンドを実行し、アクティブなセッション一覧を構造化データで返します。

**リクエスト例:**
```json
{
  "version": 1,
  "timestamp": "2025-11-11T03:00:00.000Z",
  "apiKey": "your-login-password",
  "action": "sessionlist",
  "params": {},
  "requestId": "req-001"
}
```

**レスポンス例（成功時）:**
```json
{
  "ok": true,
  "action": "sessionlist",
  "timestamp": "2025-11-11T03:00:01.234Z",
  "requestId": "req-001",
  "data": [
    {
      "index": 0,
      "name": "セッション１",
      "users": 2,
      "present": 1,
      "accessLevel": "LAN",
      "maxUsers": 16
    },
    {
      "index": 1,
      "name": "MarkN_headless World 1",
      "users": 1,
      "present": 0,
      "accessLevel": "Private",
      "maxUsers": 32
    }
  ]
}
```

#### 2. `focus` - セッションにフォーカスしてステータス取得

指定されたインデックスのセッションにフォーカスし、そのセッションのステータス情報を構造化データで返します。

**リクエスト例:**
```json
{
  "version": 1,
  "timestamp": "2025-11-11T03:30:00.000Z",
  "apiKey": "your-login-password",
  "action": "focus",
  "params": {
    "index": 0
  },
  "requestId": "req-002"
}
```

**レスポンス例（成功時）:**
```json
{
  "ok": true,
  "action": "focus",
  "timestamp": "2025-11-11T03:30:01.234Z",
  "requestId": "req-002",
  "data": {
    "name": "セッション１",
    "sessionId": "S-170a0f5e-bbc8-427c-8b50-7a494ee89b62",
    "currentUsers": 2,
    "presentUsers": 0,
    "maxUsers": 16,
    "uptime": "00:23:35.0453801",
    "accessLevel": "LAN",
    "hiddenFromListing": false,
    "mobileFriendly": false,
    "description": "",
    "tags": [],
    "users": ["MarkN_headless", "MarkN"]
  }
}
```

**注意事項:**
- `params.index` は必須で、0以上の整数を指定してください
- 存在しないインデックスを指定した場合、エラーが返される可能性があります

#### 3. `invite` - ユーザーを招待

現在フォーカス中のセッションにフレンドを招待します。

**リクエスト例:**
```json
{
  "version": 1,
  "timestamp": "2025-11-11T17:46:00.000Z",
  "apiKey": "your-login-password",
  "action": "invite",
  "params": {
    "username": "MarkN"
  },
  "requestId": "req-003"
}
```

**レスポンス例（成功時）:**
```json
{
  "ok": true,
  "action": "invite",
  "timestamp": "2025-11-11T17:46:01.234Z",
  "requestId": "req-003",
  "data": {
    "success": true,
    "message": "Invite sent!"
  }
}
```

**注意事項:**
- `params.username` は必須で、招待するユーザー名を指定してください
- ユーザーはフレンドリストに登録されている必要があります
- フォーカス中のセッションがない場合、エラーが返される可能性があります

#### 4. `setaccesslevel` - セッションのアクセスレベル変更

現在フォーカス中のセッションのアクセスレベルを変更します。

**リクエスト例:**
```json
{
  "version": 1,
  "timestamp": "2025-11-11T17:47:00.000Z",
  "apiKey": "your-login-password",
  "action": "setaccesslevel",
  "params": {
    "accessLevel": "Private"
  },
  "requestId": "req-004"
}
```

**レスポンス例（成功時）:**
```json
{
  "ok": true,
  "action": "setaccesslevel",
  "timestamp": "2025-11-11T17:47:01.234Z",
  "requestId": "req-004",
  "data": {
    "success": true,
    "message": "World セッション１ now has access level Private",
    "accessLevel": "Private"
  }
}
```

**注意事項:**
- `params.accessLevel` は必須で、以下のいずれかを指定してください: `Private`, `LAN`, `Friends`, `Anyone`
- フォーカス中のセッションがない場合、エラーが返される可能性があります

#### 5. `setrole` - ユーザーの権限変更

現在フォーカス中のセッションのユーザーの権限（ロール）を変更します。

**リクエスト例:**
```json
{
  "version": 1,
  "timestamp": "2025-11-11T17:48:00.000Z",
  "apiKey": "your-login-password",
  "action": "setrole",
  "params": {
    "username": "MarkN",
    "role": "Admin"
  },
  "requestId": "req-005"
}
```

**レスポンス例（成功時）:**
```json
{
  "ok": true,
  "action": "setrole",
  "timestamp": "2025-11-11T17:48:01.234Z",
  "requestId": "req-005",
  "data": {
    "success": true,
    "message": "MarkN now has role Admin!",
    "username": "MarkN",
    "role": "Admin"
  }
}
```

**注意事項:**
- `params.username` は必須で、権限を変更するユーザー名を指定してください
- `params.role` は必須で、以下のいずれかを指定してください: `Admin`, `Builder`, `Moderator`, `Guest`, `Spectator`
- フォーカス中のセッションに該当ユーザーが存在しない場合、エラーが返される可能性があります

#### 6. `startworld` - セッションの開始

指定されたURLから新しいセッションを開始し、開始されたセッションのURLを返します。

**リクエスト例:**
```json
{
  "version": 1,
  "timestamp": "2025-11-11T17:59:00.000Z",
  "apiKey": "your-login-password",
  "action": "startworld",
  "params": {
    "url": "resrec:///U-MarkN/R-a8166d98-ef9f-4efb-8a86-dbed9d8be48d"
  },
  "requestId": "req-006"
}
```

**レスポンス例（成功時）:**
```json
{
  "ok": true,
  "action": "startworld",
  "timestamp": "2025-11-11T17:59:15.234Z",
  "requestId": "req-006",
  "data": {
    "success": true,
    "sessionUrl": "res-steam://76561198384468054/0/S-aac647b6-241c-47ae-b5d7-29365df96c24",
    "sessionId": "S-aac647b6-241c-47ae-b5d7-29365df96c24",
    "worldName": "地下貯水施設"
  }
}
```

**注意事項:**
- `params.url` は必須で、セッションURL（`resrec://`, `res-steam://` など）を指定してください
- ワールドの読み込みには時間がかかる場合があります（最大30秒まで待機）
- 無効なURLや読み込めないワールドの場合、エラーが返される可能性があります
- ワールド名はプロンプトから自動抽出されますが、取得できない場合もあります

### エラーハンドリング

APIはエラー時に適切なHTTPステータスコードとエラーメッセージを返します：

- **401 Unauthorized**: API Keyが無効または未指定
- **403 Forbidden**: CIDR制限により拒否
- **429 Too Many Requests**: レート制限超過
- **400 Bad Request**: リクエストが不正（`version`/`timestamp`/`action` 不備等）
- **404 Not Found**: 未知の `action`
- **415 Unsupported Media Type**: JSON以外のContent-Type
- **500 Internal Server Error**: サーバー内部エラー

```json
{
  "ok": false,
  "action": "sessionlist",
  "timestamp": "2025-11-11T03:00:01.234Z",
  "requestId": "abc-123",
  "error": { "code": "INTERNAL_ERROR", "message": "Failed to execute action" }
}
```

### トラブルシューティング

1. **401エラーが返される**
   - API Keyが正しいか確認
   - `config/auth.json` の `password`（または環境変数 `DEFAULT_PASSWORD`）を確認
   - API Keyはアプリのログインパスワードと同じ値を使用します

2. **403エラーが返される**
   - アクセス元IPアドレスがCIDR範囲内か確認
   - `config/security.json` の `allowedCidrs` を確認

3. **接続できない**
   - バックエンドサーバーが起動しているか確認
   - ファイアウォールでポート8080が開いているか確認
   - ローカルネットワーク内からアクセスしているか確認

## 設定ファイル

### 認証設定 (`config/auth.json`)
```json
{
  "jwtSecret": "your-secret-key-change-in-production",
  "jwtExpiresIn": "24h",
  "password": ""
}
```

**⚠️ セキュリティ注意事項:**
- ✅ `jwtSecret` は**初回セットアップ時に自動生成・設定**されます（ユーザー操作不要）
- `jwtSecret` は環境変数 `AUTH_SHARED_SECRET` で上書き可能です（環境変数が優先されます）
- `password` はインストール時に必ず設定されるため、ここでの設定は不要です
- 必要に応じて手動で変更することも可能です

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
DEFAULT_PASSWORD=your-login-password

# 本番環境
NODE_ENV=production
AUTH_SHARED_SECRET=your-production-secret
SERVER_PORT=8080
DEFAULT_PASSWORD=your-production-login-password
```

**環境変数の優先順位:**
- 環境変数（`process.env.*`）が設定ファイルより優先されます
- ✅ 初回セットアップ時に `AUTH_SHARED_SECRET` が自動生成され、`.env` ファイルに自動設定されます
- 必要に応じて手動で変更することも可能です（例: `openssl rand -base64 32` で生成した値を設定）

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
- **Headless API**: ローカルネットワーク内のIPのみ許可

## トラブルシューティング

### よくある問題

1. **ログインできない**
   - 初回セットアップで設定したパスワードを再確認
   - `start.bat` を再実行してパスワードを再設定
   - 設定ファイル: `config/auth.json` を確認

2. **Headless APIにアクセスできない**
   - API Keyが正しいか確認
   - ローカルネットワーク内からのアクセスか確認
   - CIDR制限の設定を確認

3. **CORSエラー**
   - 本番環境: 許可されたドメインからのアクセスか確認

### ログ
バックエンドのターミナルで以下のログを確認：
- `[SECURITY]`: セキュリティイベント
- `[DEBUG]`: デバッグ情報
- `[RATE_LIMIT]`: レート制限イベント

### GitHub Actions

   git tag v1.0.0
   git push origin v1.0.0
