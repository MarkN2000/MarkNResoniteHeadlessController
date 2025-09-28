import { io, Socket } from 'socket.io-client';
import type { HeadlessStatus, LogEntry } from './api';

const SOCKET_BASE = import.meta.env.VITE_BACKEND_SOCKET_URL || 'http://localhost:8080';

export interface ServerSocketEvents {
  status: (status: HeadlessStatus) => void;
  log: (entry: LogEntry) => void;
  logs: (entries: LogEntry[]) => void;
}

export const connectServerSocket = (handlers: ServerSocketEvents): Socket => {
  const socket = io(`${SOCKET_BASE}/server`, {
    transports: ['websocket'],
    autoConnect: true
  });

  socket.on('status', handlers.status);
  socket.on('log', handlers.log);
  socket.on('logs', handlers.logs);

  return socket;
};
