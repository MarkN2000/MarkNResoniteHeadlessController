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

3. **自動セットアップの実行**
```bash
# Windowsの場合
scripts\setup.bat

# または手動で
npm run setup
```

このスクリプトが以下を自動的に行います：
- 設定ファイルのサンプルから実際のファイルを生成
- 共通型定義のビルド

4. **設定ファイルの編集**

生成された設定ファイルを環境に合わせて編集してください：

- `.env` - 環境変数（シークレットキー、パス等）
- `config/auth.json` - 認証設定（パスワード等）
- `config/security.json` - セキュリティ設定（CIDR範囲等）

⚠️ **重要**: 本番環境では必ず以下を変更してください：
- `.env` の `AUTH_SHARED_SECRET`（JWTシークレット）
- `.env` の `MOD_API_KEY`（Mod APIキー）
- `.env` の `RESONITE_HEADLESS_PATH`（Resonite実行ファイルパス）
- `config/auth.json` の `password`（管理者パスワード）

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
DEFAULT_PASSWORD=admin123
RESONITE_HEADLESS_PATH=C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe
```

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
- デフォルトパスワード: `admin123`（変更推奨）

#### 本番環境

1. **ビルド**
```bash
npm run build
```

2. **起動**
```bash
# Windowsの場合
scripts\start-production.bat

# または
npm start
```

3. **アクセス**
- WebUI & API: `http://localhost:8080`
- パスワード: `.env` または `config/auth.json` で設定したもの

#### 配布用Zipの作成（Windows）

```bash
npm run package:zip
```

コマンド実行時に `npm run build` も自動で走り、最新ビルド成果物を含む `dist/MarkNResoniteHeadlessController.zip` が生成されます。`.env` や実運用の `config/*.json` は Zip に含まれません。既にビルド済みの場合は `powershell -File scripts/package-distribution.ps1 -SkipBuild` を使うと再ビルドを省略できます。

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

### 共通型定義（shared パッケージ）について

`shared/`ディレクトリには、バックエンドとフロントエンドで共有する型定義を配置しています。

**現在の実装（開発環境）:**
- `tsx`を使用して`.ts`ファイルを直接インポート
- パス: `import type { Type } from '../../../shared/src/index.js'`
- 開発中はビルド不要で動作

**将来の本番対応（TODO）:**
- `tsconfig.json`の`paths`を使用したエイリアス設定に移行予定
- 設定例:
  ```json
  {
    "compilerOptions": {
      "baseUrl": ".",
      "paths": {
        "@shared/*": ["../shared/src/*"]
      }
    }
  }
  ```
- 使用例: `import type { Type } from '@shared/index.js'`
- これにより開発環境と本番環境で同じコードが動作

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

### 📊 実装進捗状況

#### ✅ 実装済み（約95%完成）
- **フロントエンド**: 自動再起動設定タブUI（95%完成）
  - ✅ 強制再起動・手動トリガーボタン
  - ✅ 再起動トリガー設定UI
  - ✅ 再起動前アクション設定UI
  - ✅ フェールセーフ設定UI
  - ✅ ステータス表示（動的データ連携）
  - ✅ 設定の保存・リセット機能
  
- **バックエンド基盤**: 設定管理とトリガー監視（100%完成）
  - ✅ 設定ファイル管理（`restart.json`, `restart-status.json`）
  - ✅ APIエンドポイント（GET/POST）
  - ✅ トリガー監視システム（予定/高負荷）
    - ※ユーザー0時再起動は基本機能実装済みだが統合未完了（将来実装予定）
  - ✅ 優先度管理とクールダウン機能
  
- **再起動実行システム**: （100%完成）
  - ✅ 実際の再起動処理（stop → 確認 → start）
  - ✅ **待機制御システム（完全実装）**
    - ✅ ユーザー0人待機（設定可能な待機時間）
    - ✅ 強制再起動タイムアウト（最大待機時間）
    - ✅ アクション実行タイミング制御（強制再起動の何分前）
    - ✅ 1分ごとのユーザー数チェック（Present人数を監視）
    - ✅ タイマー/インターバルの適切な管理とクリーンアップ
  - ✅ チャットメッセージ送信アクション（各セッションのAFKではないユーザーに個別送信）
  - ✅ アイテムスポーン警告アクション（全セッションへの個別スポーン + dynamicImpulseString送信）
    - ✅ アイテムURL管理機能（設定ファイル保存）
    - ✅ 2種類のアイテムプリセット（とらぞセッション閉店アナウンス、テキスト読み上げ）
    - ✅ UIでのアイテム選択とURL編集機能
  - ✅ セッション設定変更アクション（全セッションに個別適用）
  - ✅ フェールセーフのリトライ機能
  - ✅ 予定再起動時のconfig切り替え処理
  - ✅ **パフォーマンス最適化**: worldsコマンドの重複実行を削減（1回の取得で全アクションに共有）

#### 🐛 修正済みの重大なバグ
- **待機制御が未実装だった問題**（2024-10-12修正）
  - 症状: 再起動トリガー発動時にユーザー数をチェックせず、即座に再起動が実行されていた
  - 原因: `executePreRestartActions`でユーザー数チェックのロジックが完全に欠けていた
  - 修正: 完全な待機制御システムを実装
    - ユーザー0人になるまで待機 → 0人になったら指定時間待機 → 再起動
    - 強制再起動タイムアウト（最大待機時間）の実装
    - 強制再起動の何分前にアクション実行する制御
    - 1分ごとのユーザー数チェックインターバル
- **worldsコマンドの3重実行問題**（2024-10-12修正）
  - 症状: アクション実行時にworldsコマンドが3回連続で実行されていた
  - 原因: `sendChatMessage`、`spawnWarningItem`、`applySessionChanges`がそれぞれ独立してworldsを取得
  - 修正: `executeActions`で1回だけworldsを取得し、各メソッドに渡すように変更
- **強制再起動ボタンが待機制御を実行していた問題**（2024-10-12修正）
  - 症状: 「強制再起動」ボタンを押してもユーザーがいる場合は最大120分待機していた
  - 原因: `trigger === 'forced'`の場合でも`executePreRestartActions()`を実行していた
  - 修正: `trigger === 'forced'`の場合は待機制御とアクションをスキップして即座に再起動
    - **強制再起動（forced）**: 待機制御・アクションをスキップ → 即座に再起動
    - **手動再起動トリガー（manual）**: 待機制御・アクションを実行 → 再起動
- **🔴 ユーザー数カウントの重大なバグ**（2024-10-12修正）
  - 症状: ユーザーがいるのに0人と判定され、アクションが実行されずに即座に再起動されていた
  - 原因: `parseWorldsOutput()`が`Present`人数を取得しておらず、`getTotalUserCount()`のロジックが複雑で誤動作していた
  - 修正: 
    - `parseWorldsOutput()`を修正して`present`フィールドも取得するように変更
    - `getTotalUserCount()`をシンプルな`reduce`に変更し、`present`人数を正しく合計
    - **詳細なデバッグログを追加**（原因特定のため）:
      - worlds コマンドの raw output を出力
      - パース結果（各セッションの Users / Present 人数）
      - 総ユーザー数（Present の合計）
      - 初回チェックと定期チェックの結果
      - ゼロユーザー待機の経過時間
      - アクション実行の詳細（どのアクションが実行されたか）
  - **この修正により、待機制御が正しく動作するようになりました**
- **🔴🔴🔴 最重大バグ: sendCommand() vs executeCommand() の使用ミス**（2024-10-12修正）
  - **症状**: `worlds` と `users` コマンドが `undefined` を返し、全てのアクションが実行できなかった
  - **原因**: `ProcessManager.sendCommand()` は **void を返す**メソッドで、コマンドを送信するだけで出力を取得しない
    - `sendCommand()`: コマンドを stdin に書き込むだけ（戻り値なし）
    - `executeCommand()`: コマンドを実行して LogEntry[] を返す（出力を取得）
  - **修正**: 出力を必要とするコマンドを `executeCommand()` に変更
    - `getTotalUserCount()`: `executeCommand('worlds')` に変更
    - `executeActions()`: `executeCommand('worlds')` に変更
    - `sendChatMessage()`: `executeCommand('users')` に変更
  - **この修正により、コマンド出力が正しく取得でき、アクションが実行されるようになりました**
- **🔴 チャットメッセージのコマンド構文エラー**（2024-10-12修正）
  - **症状**: `message "MarkN" 🔄 サーバーが間もなく再起動します。` → `Invalid number of arguments.`
  - **原因**: 絵文字を含むメッセージが正しくエスケープされていない。メッセージ部分も引用符で囲む必要がある
  - **修正**: `message "${username}" "${message}"` に変更
- **🔴 worlds の重複パース問題**（2024-10-12修正）
  - **症状**: 2つのセッションなのに4つとしてパースされ、各アクションが2回実行されていた
  - **原因**: `executeCommand('worlds')` の出力に同じ内容が複数回含まれていた
  - **修正**: `parseWorldsOutput()` で `Map` を使用して index をキーに重複を除去
- **🔴 dynamicImpulseString のタグ修正**（2024-10-12修正）
  - **症状**: `Triggered 0 receivers` - インパルスを受信するオブジェクトがない
  - **原因**: タグが "MRHC" だったが、アイテム側は "MRHC.play" を期待していた
  - **修正**: タグを `MRHC.play` に変更
    ```typescript
    dynamicimpulsestring MRHC.play "${message}"
    ```

#### ⚠️ 残りタスク（優先度低）
1. **予定再起動のカード機能** - UI/UX向上（後回し可）

#### 🔮 将来実装予定
- **ユーザー0時再起動機能** - 必要性は低いため、将来的に実装予定
  - ユーザー数の定期チェック統合が必要（WebSocketまたはポーリング）
  - 基本的な監視機能は実装済みだが、統合作業が未完了

---

### 設計概要
再起動機能は大きく2つの要素で構成：
1. **再起動トリガー（条件）**: どのような条件で再起動するか
2. **再起動前アクション**: 条件が満たされた後、どのように処理するか

**重要な制約・仕様**:
- 再起動前に、起動に使ったコンフィグファイルが存在するか確認、なければ再起動は無効化
- サーバー停止中は全トリガーを一時停止（起動時にリセット）
- 複数トリガーが同時発動した場合は優先順位に従って1回のみ実行（優先順位: 1.予定 > 2.高負荷）
  - ※ユーザー0時再起動は将来実装予定のため、現在は対象外
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

##### データ構造
```typescript
interface ScheduledRestart {
  id: string;                    // 一意のID（UUID）
  enabled: boolean;              // 有効/無効トグル
  type: 'once' | 'weekly' | 'daily';  // 再起動タイプ
  dateTime?: string;             // ISO 8601形式（type === 'once'）
  dayOfWeek?: 0 | 1 | 2 | 3 | 4 | 5 | 6;  // 0=日曜（type === 'weekly'）
  time?: string;                 // HH:mm形式（type === 'weekly' | 'daily'）
  configFile: string;            // 起動するコンフィグファイル名
}
```

##### 優先度制御（30分前制御方式）
予定再起動の**30分前から**制御を開始し、確実に予定通りの再起動を実行：

**30分前の時点で:**
- 新規の高負荷トリガーを無効化（発動しない）
- 待機中の再起動があればキャンセルし、lastusedコンフィグを予定のものに変更
- ステータスに「予定再起動準備中」と表示
- 強制再起動ボタンは動作可能（ただしlastusedコンフィグは予定のものを維持）

**予定時刻到達時:**
- サーバー起動中: 通常の再起動フロー実行（待機制御・アクション実行）→ 予定のコンフィグで再起動
- サーバー停止中: lastusedコンフィグを予定のものに変更 → 次回起動時に予定のコンフィグが使用される

**予定実行後:**
- `type === 'once'` の予定は自動的に無効化
- 準備中フラグをクリア
- 高負荷トリガーを再有効化

##### 監視方式
- チェック間隔: 1分ごと
- 判定方式: 現在時刻と予定時刻を分単位で比較

#### 高負荷時
- システム全体のメモリ使用率とCPU使用率のどちらかが閾値超過を一定時間継続（例：メモリが80%を10分連続超過）
- 起動（再起動含む）から30分間は監視を無効化（再起動ループ防止・固定値）
- 再起動時は最後に使用したconfigで起動

#### ユーザー0（将来実装予定）
- 全セッションのheadlessアカウントを除いた合計人数がAFKも含めて0人（つまり、worldsコマンドを実行して、すべてのセッションのusersの値が1）に**変化**したタイミングで再起動をトリガー
- 「複数人→0人」に減った瞬間のみ発動（0人が継続している場合は無視）
- 最後に起動してから指定時間（例：4時間）経っていない場合は無視
- 再起動時は最後に使用したconfigで起動
- **注**: 必要性が低いため、将来的に実装予定。基本的な監視機能は実装済み


### 2. 再起動前アクション

#### 待機制御（全トリガー対象）
- ユーザーが0になってから指定時間待機（例：5分）
- 強制再起動までのタイムアウト（例：120分待ってもユーザーがいたら強制実行）
- 強制再起動の何分前に強制再起動前通知・アクションを実行するか（例：2分、この値が以下の全アクションに適用）

#### メッセージで警告
- 強制再起動X分前に全セッションのAFKではないユーザーにチャットメッセージ送信（メッセージ内容を設定）
#### アイテムで警告
- 強制再起動X分前にメッセージアイテムをスポーンして、10秒後にdynamicImpulseString で `MRHC.play` タグと指定のメッセージを送信する（アイテムの種類、メッセージを設定）

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
│ 👤 ユーザー0時再起動（将来実装予定）  最小稼働時間: [240]分  [ON/OFF Toggle]│
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
   - ユーザー0: 将来実装予定（worldsコマンド実行時にチェックする予定）
   
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