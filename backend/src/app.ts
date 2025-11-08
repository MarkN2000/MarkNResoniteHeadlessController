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
import { getCorsConfig, dynamicOriginCheck } from './config/cors.js';
import { systemMetricsCollector } from './services/systemMetrics.js';
import { processManager } from './services/processManager.js';
import { RestartManager } from './services/restartManager.js';
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

// RestartManagerを初期化
const restartManager = new RestartManager(processManager);
restartManager.initialize().catch((error) => {
  console.error('[App] Failed to initialize RestartManager:', error);
});

// APIルートを設定（RestartManagerを渡す）
const apiRouter = createApiRouter(restartManager);
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

httpServer.listen(SERVER_PORT, () => {
  console.log(`Backend listening on port ${SERVER_PORT}`);
  console.log(`WebSocket server available at ws://localhost:${SERVER_PORT}`);
});
