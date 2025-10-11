import 'dotenv/config';
import express from 'express';
import cors from 'cors';
import { createServer } from 'http';
import { Server as SocketIOServer } from 'socket.io';
import { SERVER_PORT } from './config/index.js';
import { createApiRouter } from './http/index.js';
import { registerSocketHandlers } from './ws/index.js';
import { cidrRestriction } from './middleware/cidr.js';
import { getCorsConfig, dynamicOriginCheck } from './config/cors.js';
import { systemMetricsCollector } from './services/systemMetrics.js';
import { processManager } from './services/processManager.js';
import { RestartManager } from './services/restartManager.js';

const app = express();

// CIDR制限を最初に適用
app.use(cidrRestriction);

// 環境に応じたCORS設定を適用
const corsConfig = getCorsConfig();
app.use(cors(corsConfig));

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

const httpServer = createServer(app);
const io = new SocketIOServer(httpServer, {
  cors: {
    origin: dynamicOriginCheck,
    credentials: true,
    methods: ['GET', 'POST']
  },
  transports: ['websocket', 'polling']
});

console.log('[Socket.IO] Server initialized with CORS');
console.log('[Socket.IO] Allowed origins: localhost:5173, localhost:3000, 127.0.0.1:5173, 127.0.0.1:3000');

registerSocketHandlers(io);

// システムメトリクスの収集を開始
systemMetricsCollector.start();

httpServer.listen(SERVER_PORT, () => {
  console.log(`Backend listening on port ${SERVER_PORT}`);
  console.log(`WebSocket server available at ws://localhost:${SERVER_PORT}`);
});
