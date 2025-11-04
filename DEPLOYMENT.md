# 配布・デプロイメントガイド

## 📦 配布前のチェックリスト

### 必須項目

- [ ] `@sveltejs/adapter-static` のインストール
- [ ] `.gitignore` の確認（機密情報が含まれていないか）
- [ ] サンプル設定ファイル（`.example`）の存在確認
- [ ] README.mdのセットアップ手順の確認
- [ ] デフォルトパスワードの変更を促す警告の確認

### 推奨項目

- [ ] セキュリティ監査の実施
- [ ] パフォーマンステスト
- [ ] 本番環境での動作確認
- [ ] バックアップ・リストア手順の確認

## 🔧 初回セットアップ手順（配布先ユーザー向け）

### 1. 依存関係のインストール

まず、必要なパッケージをインストールします：

```bash
# プロジェクトルートで実行
npm install

# フロントエンド用のadapter-staticをインストール（重要）
npm install --save-dev @sveltejs/adapter-static --workspace frontend
```

### 2. 自動セットアップの実行

Windowsの場合：
```bash
scripts\setup.bat
```

または手動で：
```bash
npm run setup
```

### 3. 設定ファイルの編集

生成された以下のファイルを環境に合わせて編集：

#### `.env`
```env
NODE_ENV=production
SERVER_PORT=8080
AUTH_SHARED_SECRET=ランダムで強力な文字列に変更
MOD_API_KEY=ランダムで強力な文字列に変更
RESONITE_HEADLESS_PATH=C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe
```

⚠️ **セキュリティ重要**: `AUTH_SHARED_SECRET` と `MOD_API_KEY` は必ず変更してください！

推奨される強力なキーの生成方法：
```bash
# Node.jsで生成
node -e "console.log(require('crypto').randomBytes(32).toString('hex'))"
```

#### `config/auth.json`
```json
{
  "jwtSecret": "AUTH_SHARED_SECRETと同じ値",
  "jwtExpiresIn": "24h",
  "password": "強力な管理者パスワードに変更"
}
```

#### `config/security.json`
```json
{
  "allowedCidrs": [
    "192.168.0.0/16",
    "10.0.0.0/8",
    "127.0.0.1/32"
  ]
}
```

必要に応じてCIDR範囲を調整してください。

### 4. ビルド

```bash
npm run build
```

### 5. 起動

```bash
# Windowsの場合
scripts\start-production.bat

# または
npm start
```

### 6. アクセス

ブラウザで `http://localhost:8080` にアクセスし、設定したパスワードでログインしてください。

## 🚀 デプロイメント方法

### オプション1: 直接実行（シンプル）

最もシンプルな方法。開発・テスト環境に最適。

**起動:**
```bash
npm start
```

**停止:**
- Ctrl+C でプロセスを停止

**デメリット:**
- ターミナルを閉じるとサーバーも停止
- 自動再起動なし
- ログ管理が手動

### オプション2: PM2（推奨）

プロセス管理ツールを使用。本番環境に推奨。

**インストール:**
```bash
npm install -g pm2
```

**PM2設定ファイル作成** (`ecosystem.config.cjs`):
```javascript
module.exports = {
  apps: [{
    name: 'mrhc-backend',
    script: 'dist/app.js',
    cwd: './backend',
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    env: {
      NODE_ENV: 'production',
      PORT: 8080
    },
    error_file: './logs/err.log',
    out_file: './logs/out.log',
    log_date_format: 'YYYY-MM-DD HH:mm:ss Z'
  }]
};
```

**起動:**
```bash
pm2 start ecosystem.config.cjs
```

**停止:**
```bash
pm2 stop mrhc-backend
```

**再起動:**
```bash
pm2 restart mrhc-backend
```

**ログ確認:**
```bash
pm2 logs mrhc-backend
```

**自動起動設定（Windows起動時）:**
```bash
pm2 startup
pm2 save
```

### オプション3: Windowsサービス

Windows Serviceとして登録し、バックグラウンドで常時実行。

**node-windows のインストール:**
```bash
npm install -g node-windows
```

**サービス登録スクリプト** (`scripts/install-service.js`):
```javascript
import { Service } from 'node-windows';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const svc = new Service({
  name: 'MRHC Backend',
  description: 'MarkN Resonite Headless Controller Backend Service',
  script: path.join(__dirname, '..', 'backend', 'dist', 'app.js'),
  env: {
    name: 'NODE_ENV',
    value: 'production'
  }
});

svc.on('install', () => {
  svc.start();
  console.log('Service installed and started');
});

svc.install();
```

**登録:**
```bash
node scripts/install-service.js
```

**削除:**
```bash
node scripts/uninstall-service.js
```

## 🔐 セキュリティ設定

### ファイアウォール設定

**ポート8080を開放:**

Windows Firewall:
```powershell
# 管理者権限でPowerShellを実行
New-NetFirewallRule -DisplayName "MRHC Backend" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow
```

### リバースプロキシ（推奨）

**nginx の例:**

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket対応
    location /socket.io/ {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

### HTTPS化（Let's Encrypt）

本番環境では必ずHTTPSを使用してください。

```bash
# Certbotのインストール（nginx使用時）
sudo apt-get install certbot python3-certbot-nginx

# 証明書の取得
sudo certbot --nginx -d your-domain.com
```

## 🔄 アップデート手順

### 1. 最新版の取得

```bash
git pull origin main
```

### 2. 依存関係の更新

```bash
npm install
```

### 3. ビルド

```bash
npm run build
```

### 4. サーバーの再起動

**直接実行の場合:**
- Ctrl+C で停止 → `npm start` で再起動

**PM2の場合:**
```bash
pm2 restart mrhc-backend
```

**Windowsサービスの場合:**
```powershell
Restart-Service "MRHC Backend"
```

## 📊 ログとモニタリング

### ログファイルの場所

- **バックエンド**: コンソール出力（PM2使用時は `./logs/` 以下）
- **Resoniteヘッドレス**: Resoniteのログディレクトリ

### PM2でのモニタリング

```bash
# リアルタイムモニタリング
pm2 monit

# ステータス確認
pm2 status

# ログ確認
pm2 logs mrhc-backend
```

## 🐛 トラブルシューティング

### ビルドに失敗する

1. Node.jsのバージョンを確認（20.x以上必要）
   ```bash
   node --version
   ```

2. `node_modules` を削除して再インストール
   ```bash
   rmdir /s /q node_modules
   npm install
   ```

3. キャッシュをクリア
   ```bash
   npm cache clean --force
   ```

### 起動できない

1. ポート8080が既に使用されていないか確認
   ```bash
   netstat -ano | findstr :8080
   ```

2. 設定ファイルが正しく作成されているか確認
   - `.env`
   - `config/auth.json`
   - `config/security.json`

3. ログを確認
   ```bash
   npm start
   ```

### WebSocketが接続できない

1. CORS設定を確認（`backend/src/config/cors.ts`）
2. ファイアウォール設定を確認
3. リバースプロキシを使用している場合、WebSocket設定を確認

## 📝 バックアップ

定期的にバックアップを取ることを推奨します：

### バックアップ対象

- `config/` ディレクトリ全体
- `.env` ファイル
- `backend/config/` ディレクトリ（再起動設定等）
- Resoniteヘッドレスの設定・ワールドデータ

### バックアップスクリプト例

```bash
@echo off
set BACKUP_DIR=backups\%date:~0,4%%date:~5,2%%date:~8,2%_%time:~0,2%%time:~3,2%
mkdir %BACKUP_DIR%

xcopy config %BACKUP_DIR%\config /E /I
copy .env %BACKUP_DIR%\.env
xcopy backend\config %BACKUP_DIR%\backend\config /E /I

echo Backup completed: %BACKUP_DIR%
```

## 🎯 パフォーマンスチューニング

### Node.js メモリ制限の引き上げ

大量のセッションを扱う場合：

```bash
# package.jsonのstartスクリプトを変更
"start": "cd backend && NODE_ENV=production node --max-old-space-size=4096 dist/app.js"
```

### PM2でのクラスタリング

```javascript
// ecosystem.config.cjs
module.exports = {
  apps: [{
    name: 'mrhc-backend',
    script: 'dist/app.js',
    cwd: './backend',
    instances: 'max',  // CPUコア数に応じて自動調整
    exec_mode: 'cluster',
    // ...
  }]
};
```

## 📞 サポート

問題が発生した場合：

1. このドキュメントのトラブルシューティングセクションを確認
2. GitHubのIssuesで既存の問題を検索
3. 新しいIssueを作成（詳細な情報を含める）

---

**重要**: 本番環境にデプロイする前に、必ずテスト環境で十分な検証を行ってください。

