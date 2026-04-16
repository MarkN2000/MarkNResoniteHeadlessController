import 'dotenv/config';
import express from 'express';
import cors from 'cors';
import { createServer } from 'http';
import { Server as SocketIOServer } from 'socket.io';
import path from 'path';
import { fileURLToPath } from 'url';
import { SERVER_PORT } from './config/index.js';
import { createApiRouter } from './http/index.js';
import { registerSocketHandlers } from './ws/index.js';
import { cidrRestriction } from './middleware/cidr.js';
import { getCorsConfig } from './config/cors.js';
import { startRateLimitCleanup } from './utils/rateLimit.js';
import { systemMetricsCollector } from './services/systemMetrics.js';
import { processManager } from './services/processManager.js';
import { RestartManager } from './services/restartManager.js';
import { SteamUpdateChecker } from './services/steamUpdateChecker.js';
import { steamUpdateBus } from './services/steamUpdateBus.js';
import { loadSteamConfig } from './services/steamConfig.js';
import fs from 'fs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// 必要なディレクトリを作成
const ensureDirectories = () => {
  const configDir = path.join(__dirname, '../../../config');
  const backendConfigDir = path.join(__dirname, '../config');
  
  if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
    console.log('[App] Created config directory:', configDir);
  }
  
  if (!fs.existsSync(backendConfigDir)) {
    fs.mkdirSync(backendConfigDir, { recursive: true });
    console.log('[App] Created backend config directory:', backendConfigDir);
  }
};

// ディレクトリの作成を実行
ensureDirectories();

const app = express();

// 環境に応じたCORS設定を適用
const corsConfig = getCorsConfig();
app.use(cors(corsConfig));

// CIDR制限を適用（Socket.IOパスは除外）
app.use((req, res, next) => {
  // Socket.IOリクエストはスキップ（WebSocket接続時に個別にチェック）
  if (req.path.startsWith('/socket.io')) {
    return next();
  }
  cidrRestriction(req, res, next);
});

app.use(express.json({ limit: '10mb' })); // リクエストサイズ制限
app.use(express.urlencoded({ extended: true, limit: '10mb' })); // URLエンコードされたデータの制限

// ProcessManagerを初期化（ランタイム状態の読み込み）
await processManager.initialize();

// Resonite 最新バージョン確認の checker を初期化し、結果を WS バスへ流す
const steamUpdateChecker = new SteamUpdateChecker(() => loadSteamConfig());

// RestartManagerを初期化（steamUpdateChecker / loadSteamConfig を渡して再起動時アップデートに対応）
const restartManager = new RestartManager(processManager, steamUpdateChecker, () => loadSteamConfig());
restartManager.initialize().catch((error) => {
  console.error('[App] Failed to initialize RestartManager:', error);
});
steamUpdateChecker.on('result', (result) => {
  steamUpdateBus.emitCheckResult(result);
});

// APIルートを設定（RestartManager, ProcessManager, SteamUpdateCheckerを渡す）
const apiRouter = createApiRouter(restartManager, processManager, steamUpdateChecker);
app.use('/api', apiRouter);

// フロントエンドの静的ファイルを配信（本番環境、または開発環境でビルド済みの場合）
// 開発環境: backend/src/app.ts から見て ../../frontend/build
// 本番環境: dist/backend/src/app.js から見て ../../../../frontend/build
// 両方に対応するため、プロジェクトルートを取得
const projectRoot = path.resolve(__dirname, process.env.NODE_ENV === 'production' ? '../../../../' : '../../');
const frontendBuildPath = path.join(projectRoot, 'frontend/build');

if (fs.existsSync(frontendBuildPath)) {
  // 静的ファイル配信
  app.use(express.static(frontendBuildPath));
  
  // SPAのフォールバック: 存在しないルートはindex.htmlを返す
  app.get('*', (req, res, next) => {
    // API、WebSocket、静的ファイルリクエストは除外
    if (req.path.startsWith('/api') || req.path.startsWith('/socket.io')) {
      return next();
    }
    res.sendFile(path.join(frontendBuildPath, 'index.html'));
  });
  
  console.log('[Static] Serving frontend from:', frontendBuildPath);
} else if (process.env.NODE_ENV === 'production') {
  console.warn('[Static] Frontend build directory not found:', frontendBuildPath);
  console.warn('[Static] Please run "npm run build" to build the frontend.');
}

const httpServer = createServer(app);
const io = new SocketIOServer(httpServer, {
  cors: {
    origin: true, // 本番環境でのテスト用に全てのオリジンを許可
    credentials: true,
    methods: ['GET', 'POST']
  },
  transports: ['polling', 'websocket'], // pollingを先に試す
  allowEIO3: true, // Engine.IO v3互換性
  pingTimeout: 60000,
  pingInterval: 25000
});

console.log('[Socket.IO] Server initialized');
console.log('[Socket.IO] CORS enabled for localhost and local network');
console.log('[Socket.IO] Transports: websocket, polling');

registerSocketHandlers(io);

// システムメトリクスの収集を開始
systemMetricsCollector.start();

// レート制限のクリーンアップタイマーを開始
startRateLimitCleanup();

httpServer.listen(SERVER_PORT, () => {
  console.log(`Backend listening on port ${SERVER_PORT}`);
  console.log(`WebSocket server available at ws://localhost:${SERVER_PORT}`);

  // Resonite 最新バージョンの定期チェックを開始。
  // 起動直後にいきなり走らせると他の初期化処理と競合するため少し遅らせる。
  // 間隔は ENV で上書き可能。既定は 60 分。
  const initialDelayMs = Number(process.env.STEAM_UPDATE_CHECK_INITIAL_DELAY_MS) || 60 * 1000;
  const intervalMs = Number(process.env.STEAM_UPDATE_CHECK_INTERVAL_MS) || 60 * 60 * 1000;
  steamUpdateChecker.startPeriodic({ initialDelayMs, intervalMs });
});
