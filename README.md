# MarkN Resonite Headless Controller

Resoniteのヘッドレスサーバーを、ローカルネットワーク内のPCやスマートフォンのブラウザから簡単に操作・管理するためのWebアプリケーションです。

## 概要

- **WebUI**: ブラウザから直感的にヘッドレスサーバーを操作
- **Mod連携**: Resonite ModからのAPI連携機能
- **セキュリティ**: JWT認証、CIDR制限、レート制限による多層防御
- **リアルタイム**: WebSocketによるリアルタイムログ・ステータス表示

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

#### 方法1: 自動セットアップ（推奨・Windows）

1. **リポジトリのクローン**
```bash
git clone <repository-url>
cd MarkNResoniteHeadlessController
```

2. **依存関係のインストール**
```bash
npm install

# フロントエンド用のadapter-staticをインストール（重要）
npm install --save-dev @sveltejs/adapter-static --workspace frontend
```

3. **初期セットアップの実行**
```bash
# Windowsの場合（初回セットアップと本番起動を兼ねる）
start.bat

# または手動でセットアップのみ実施
npm run setup
```

`start.bat` は初回実行時に以下を自動的に行い、そのまま本番モードでバックエンドを起動します：
- 設定ファイルのサンプルから実際のファイルを生成
- バックエンド依存関係のインストール
- プリビルド済み資産の確認
- 管理画面ログイン用パスワードと Headless 資格情報の入力・保存
- 入力された Headless 資格情報は `config/auth.json` の `headlessCredentials` として保存され、既存のプリセット設定（`config/headless/*.json`）へ自動反映されます。
- `.setup_completed` が存在する環境でも、パスワードや Headless 資格情報が未設定／初期値の場合には再度入力を促し、設定を反映します。

4. **設定ファイルの編集**

生成された設定ファイルを環境に合わせて編集してください：

- `.env` - 環境変数（シークレットキー、パス等）
- `config/auth.json` - 認証設定（パスワード等）
- `config/security.json` - セキュリティ設定（CIDR範囲等）

⚠️ **重要**: 本番環境では必ず以下を変更してください：
- `.env` の `AUTH_SHARED_SECRET`（JWTシークレット）
- `.env` の `MOD_API_KEY`（Mod APIキー）
- `.env` の `RESONITE_HEADLESS_PATH`（Resonite実行ファイルパス）

#### 方法2: 手動セットアップ

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
# 環境変数
cp env.example .env

# 認証設定
cp config/auth.json.example config/auth.json

# セキュリティ設定
cp config/security.json.example config/security.json

# ランタイム状態
cp config/runtime-state.json.example config/runtime-state.json

# 再起動設定
cp backend/config/restart.json.example backend/config/restart.json
cp backend/config/restart-status.json.example backend/config/restart-status.json
```

4. **環境変数の編集（.env）**
```bash
NODE_ENV=development
SERVER_PORT=8080
AUTH_SHARED_SECRET=your-secret-key-change-in-production
MOD_API_KEY=mod-secret-key
RESONITE_HEADLESS_PATH=C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe
```

初回セットアップではポート番号の入力を求められ、回答が `.env` の `PORT` / `SERVER_PORT` に自動反映されます。手動で変更する場合も `.env` を編集してください。

### 起動

#### 開発環境

1. **開発サーバーの起動（バックエンド + フロントエンド）**
```bash
npm run dev
```

または個別に起動：

```bash
# バックエンドのみ
npm run dev --workspace backend

# フロントエンドのみ
npm run dev --workspace frontend
```

2. **アクセス**
- WebUI: `http://localhost:5173`
- API: `http://localhost:8080/api`

#### 本番環境

1. **ビルド**
```bash
npm run build
```

2. **起動**
```bash
# Windowsの場合
start.bat

# または（Node.jsから直接実行）
node scripts/start.js

# 既存のバックエンドワークスペーススクリプトを使う場合
npm start
```

3. **アクセス**
- WebUI & API: `http://localhost:<設定したポート>`
- パスワード: `.env` または `config/auth.json` で設定したもの

#### 配布用Zipの作成（Windows）

```bash
npm run package:zip
```

コマンド実行時に `npm run build` も自動で走り、最新ビルド成果物を含む `dist/MarkNResoniteHeadlessController.zip` が生成されます。`.env` や実運用の `config/*.json` は Zip に含まれません。既にビルド済みの場合は `powershell -File scripts/package-distribution.ps1 -SkipBuild` を使うと再ビルドを省略できます。

#### 配布Zipからのセットアップ（エンドユーザー向け）

1. 配布された `MarkNResoniteHeadlessController.zip` を任意のディレクトリに展開します。
2. Node.js 20 以上がインストールされていることを確認します。
3. Windowsでは `start.bat` を実行（ダブルクリック可）します。初回はセットアップが自動で実施され、アプリ用パスワード・Headless資格情報・使用ポート（デフォルト8080）の入力を求められます。その他の環境では `node scripts/start.js` または `npm run setup` と既存手順を利用してください。
4. `.env` や `config/*.json` が未生成の場合は自動的に作成されます。必要に応じて `env.example` を `.env` に複製し、シークレットやパスを調整してください。
5. 2回目以降も `start.bat` を実行するだけでバックエンドを起動できます（セットアップはスキップされます）。
6. ブラウザで `http://localhost:<設定したポート>` にアクセスし、初回セットアップで設定したパスワードでログインしてください。

⚠️ **本番環境の注意点**:
- 必ず `.env` で `NODE_ENV=production` を設定
- セキュリティ設定を厳密に確認
- ファイアウォールでポート8080を適切に設定
- 可能であればリバースプロキシ（nginx等）の使用を推奨

## Mod連携API

Resonite MODから本アプリケーションのAPIを呼び出して、ヘッドレスサーバーを操作することができます。

### 認証

ModからのAPIアクセスには、API Key認証が必要です。API Keyは以下のいずれかの方法で指定できます：

#### 方法1: リクエストヘッダー（推奨）
```csharp
// C# Example
var request = new HttpRequestMessage(HttpMethod.Post, "http://192.168.1.100:8080/api/mod/command");
request.Headers.Add("X-Mod-Api-Key", "mod-secret-key");
request.Content = new StringContent(jsonBody, Encoding.UTF8, "application/json");
```

#### 方法2: クエリパラメータ
```
http://192.168.1.100:8080/api/mod/command?apiKey=mod-secret-key
```

### セキュリティ制限

- **CIDR制限**: ローカルネットワーク内からのアクセスのみ許可（デフォルト: `192.168.0.0/16`, `10.0.0.0/8`, `127.0.0.1`）
- **レート制限**: 15分間に1000リクエストまで
- **API Key**: `config/auth.json` または環境変数 `MOD_API_KEY` で設定

### エンドポイント一覧

#### 1. コマンド実行（任意のヘッドレスコマンド）

```http
POST /api/mod/command
Content-Type: application/json
X-Mod-Api-Key: mod-secret-key

{
  "command": "say Hello from Mod!",
  "options": {}
}
```

**レスポンス:**
```json
{
  "success": true,
  "result": [
    {
      "message": "Command executed successfully",
      "timestamp": "2025-10-12T12:00:00.000Z"
    }
  ],
  "timestamp": "2025-10-12T12:00:00.000Z"
}
```

**使用例:**
```csharp
// C# Example - チャットメッセージ送信
var command = new {
    command = "say Hello from Mod!"
};
var json = JsonSerializer.Serialize(command);
var response = await httpClient.PostAsync(
    "http://192.168.1.100:8080/api/mod/command",
    new StringContent(json, Encoding.UTF8, "application/json")
);

// ダイナミックインパルス送信
var impulseCommand = new {
    command = "dynamicimpulsestring MessageBoard \"New message from MOD\""
};
```

#### 2. サーバー状態取得

```http
GET /api/mod/status
X-Mod-Api-Key: mod-secret-key
```

**レスポンス:**
```json
{
  "isRunning": true,
  "startedAt": "2025-10-12T08:00:00.000Z",
  "uptime": 14400000,
  "focusedWorldId": "0",
  "lastUsedConfig": "default.json"
}
```

#### 3. ログ取得（最新100件）

```http
GET /api/mod/logs
X-Mod-Api-Key: mod-secret-key
```

**レスポンス:**
```json
[
  {
    "message": "World started",
    "timestamp": "2025-10-12T08:00:00.000Z"
  },
  ...
]
```

#### 4. ワールド一覧取得

```http
GET /api/mod/worlds
X-Mod-Api-Key: mod-secret-key
```

**レスポンス:**
```json
[
  {
    "index": 0,
    "name": "My World",
    "sessionId": "S-abc123...",
    "activeUsers": 3,
    "totalUsers": 5,
    "maxUsers": 16,
    "accessLevel": "Anyone"
  },
  ...
]
```

#### 5. フォーカス中ワールドのユーザー一覧取得

```http
GET /api/mod/users
X-Mod-Api-Key: mod-secret-key
```

**レスポンス:**
```json
[
  {
    "username": "User1",
    "userId": "U-abc123...",
    "role": "Admin",
    "isPresent": true,
    "ping": 45
  },
  ...
]
```

#### 6. サーバー起動

```http
POST /api/mod/start
Content-Type: application/json
X-Mod-Api-Key: mod-secret-key

{
  "config": "default.json"
}
```

#### 7. サーバー停止

```http
POST /api/mod/stop
X-Mod-Api-Key: mod-secret-key
```

### 実装例

#### C# (Resonite MOD)

```csharp
using System;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Threading.Tasks;

public class HeadlessControllerClient
{
    private readonly HttpClient _httpClient;
    private readonly string _baseUrl;
    private readonly string _apiKey;

    public HeadlessControllerClient(string baseUrl, string apiKey)
    {
        _baseUrl = baseUrl;
        _apiKey = apiKey;
        _httpClient = new HttpClient();
        _httpClient.DefaultRequestHeaders.Add("X-Mod-Api-Key", apiKey);
    }

    // 任意のコマンドを実行
    public async Task<bool> ExecuteCommandAsync(string command)
    {
        try
        {
            var payload = new { command };
            var json = JsonSerializer.Serialize(payload);
            var content = new StringContent(json, Encoding.UTF8, "application/json");
            
            var response = await _httpClient.PostAsync(
                $"{_baseUrl}/api/mod/command",
                content
            );
            
            return response.IsSuccessStatusCode;
        }
        catch (Exception ex)
        {
            Console.WriteLine($"Command execution failed: {ex.Message}");
            return false;
        }
    }

    // チャットメッセージを送信
    public async Task SendChatMessageAsync(string message)
    {
        await ExecuteCommandAsync($"say {message}");
    }

    // ダイナミックインパルスを送信
    public async Task SendDynamicImpulseAsync(string tag, string value)
    {
        await ExecuteCommandAsync($"dynamicimpulsestring {tag} \"{value}\"");
    }

    // サーバー状態を取得
    public async Task<ServerStatus> GetStatusAsync()
    {
        try
        {
            var response = await _httpClient.GetAsync($"{_baseUrl}/api/mod/status");
            var json = await response.Content.ReadAsStringAsync();
            return JsonSerializer.Deserialize<ServerStatus>(json);
        }
        catch
        {
            return null;
        }
    }
}

// 使用例
var client = new HeadlessControllerClient("http://192.168.1.100:8080", "mod-secret-key");

// コマンド実行
await client.ExecuteCommandAsync("save");

// チャットメッセージ送信
await client.SendChatMessageAsync("Hello from MOD!");

// ダイナミックインパルス送信
await client.SendDynamicImpulseAsync("MRHC.play", "Announcement message");

// サーバー状態確認
var status = await client.GetStatusAsync();
if (status?.isRunning == true)
{
    Console.WriteLine($"Server is running for {status.uptime}ms");
}
```

### エラーハンドリング

APIはエラー時に適切なHTTPステータスコードとエラーメッセージを返します：

- **401 Unauthorized**: API Keyが無効または未指定
- **403 Forbidden**: CIDR制限により拒否
- **429 Too Many Requests**: レート制限超過
- **400 Bad Request**: リクエストが不正
- **500 Internal Server Error**: サーバー内部エラー

```json
{
  "error": "Command execution failed",
  "message": "Process is not running"
}
```

### トラブルシューティング

1. **401エラーが返される**
   - API Keyが正しいか確認
   - `config/auth.json` または環境変数 `MOD_API_KEY` を確認

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
   - 初回セットアップで設定したパスワードを再確認
   - `start.bat` を再実行してパスワードを再設定
   - 設定ファイル: `config/auth.json` を確認

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
| `dynamicImpulseString` | 指定されたタグと文字列値で非同期ダイナミックインパルスを送信 | `dynamicimpulsestring <tag> <value>` |
| `dynamicImpulseInt` | 指定されたタグと整数値で非同期ダイナミックインパルスを送信 | `dynamicimpulseint <tag> <value>` |
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