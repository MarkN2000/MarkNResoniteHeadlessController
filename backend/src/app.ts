import 'dotenv/config';
import express from 'express';
import cors from 'cors';
import { createServer } from 'http';
import { Server as SocketIOServer } from 'socket.io';

const app = express();
const port = process.env.PORT ?? 8080;

app.use(cors());
app.use(express.json());

const httpServer = createServer(app);
const io = new SocketIOServer(httpServer, {
  cors: {
    origin: '*'
  }
});

io.on('connection', socket => {
  console.log('socket connected', socket.id);
});

app.get('/health', (_req, res) => {
  res.json({ status: 'ok' });
});

httpServer.listen(port, () => {
  console.log(`Backend listening on port ${port}`);
});
