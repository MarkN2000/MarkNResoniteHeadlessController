import { io, Socket } from 'socket.io-client';
import type { HeadlessStatus, LogEntry } from './api';

// 開発環境ではViteプロキシを使用、本番環境では環境変数から取得
const SOCKET_BASE = import.meta.env.VITE_BACKEND_SOCKET_URL || '';

export interface ServerSocketEvents {
  status: (status: HeadlessStatus) => void;
  log: (entry: LogEntry) => void;
  logs: (entries: LogEntry[]) => void;
}

export const connectServerSocket = (handlers: ServerSocketEvents): Socket => {
  const socket = io(`${SOCKET_BASE}/server`, {
    transports: ['websocket'],
    autoConnect: true,
    reconnection: true,
    reconnectionAttempts: 5,
    reconnectionDelay: 1000
  });

  socket.on('connect', () => {
    console.log('[WebSocket] Connected to server');
  });

  socket.on('connect_error', (error) => {
    console.error('[WebSocket] Connection error:', error.message);
  });

  socket.on('disconnect', (reason) => {
    console.warn('[WebSocket] Disconnected:', reason);
  });

  socket.on('status', handlers.status);
  socket.on('log', handlers.log);
  socket.on('logs', handlers.logs);

  return socket;
};
