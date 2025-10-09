import 'dotenv/config';
import express from 'express';
import cors from 'cors';
import { createServer } from 'http';
import { Server as SocketIOServer } from 'socket.io';
import { SERVER_PORT } from './config/index.js';
import { apiRouter } from './http/index.js';
import { registerSocketHandlers } from './ws/index.js';
import { cidrRestriction } from './middleware/cidr.js';
import { getCorsConfig, dynamicOriginCheck } from './config/cors.js';

const app = express();

// CIDR制限を最初に適用
app.use(cidrRestriction);

// 環境に応じたCORS設定を適用
const corsConfig = getCorsConfig();
app.use(cors(corsConfig));

app.use(express.json({ limit: '10mb' })); // リクエストサイズ制限
app.use(express.urlencoded({ extended: true, limit: '10mb' })); // URLエンコードされたデータの制限
app.use('/api', apiRouter);

const httpServer = createServer(app);
const io = new SocketIOServer(httpServer, {
  cors: {
    origin: dynamicOriginCheck,
    credentials: true,
    methods: ['GET', 'POST']
  }
});

console.log('[Socket.IO] Server initialized');

registerSocketHandlers(io);

httpServer.listen(SERVER_PORT, () => {
  console.log(`Backend listening on port ${SERVER_PORT}`);
  console.log(`WebSocket server available at ws://localhost:${SERVER_PORT}`);
});
