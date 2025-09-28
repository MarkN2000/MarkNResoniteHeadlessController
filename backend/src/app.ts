import 'dotenv/config';
import express from 'express';
import cors from 'cors';
import { createServer } from 'http';
import { Server as SocketIOServer } from 'socket.io';
import { SERVER_PORT } from './config/index.js';
import { apiRouter } from './http/index.js';
import { registerSocketHandlers } from './ws/index.js';

const app = express();

app.use(cors());
app.use(express.json());
app.use('/api', apiRouter);

const httpServer = createServer(app);
const io = new SocketIOServer(httpServer, {
  cors: {
    origin: '*'
  }
});

registerSocketHandlers(io);

httpServer.listen(SERVER_PORT, () => {
  console.log(`Backend listening on port ${SERVER_PORT}`);
});
