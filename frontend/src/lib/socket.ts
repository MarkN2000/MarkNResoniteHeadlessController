import { io } from 'socket.io-client';
import type { Socket } from 'socket.io-client';
import type { HeadlessStatus, LogEntry, SystemMetrics } from './api';
// SteamCMDアップデート関連の型は shared パッケージから再エクスポート
// （バックエンドと完全一致を保証するため、フロント側で重複定義しない）
import type {
  SteamUpdateState,
  SteamUpdateProgress,
  SteamUpdateSnapshot,
  SteamUpdateCheckResult
} from '../../../shared/src/index.js';

export type { SteamUpdateState, SteamUpdateProgress, SteamUpdateSnapshot, SteamUpdateCheckResult };

// 開発環境では直接バックエンドに接続、本番環境では現在のオリジンを使用
const getSocketBase = () => {
  // 環境変数が設定されている場合はそれを使用（開発環境）
  if (import.meta.env.VITE_BACKEND_SOCKET_URL) {
    return import.meta.env.VITE_BACKEND_SOCKET_URL;
  }
  
  // ブラウザ環境の場合、現在のオリジンを使用（本番環境）
  if (typeof window !== 'undefined') {
    return window.location.origin;
  }
  
  // フォールバック（SSR等）
  return 'http://localhost:8080';
};

const SOCKET_BASE = getSocketBase();

export interface ServerSocketEvents {
  status: (status: HeadlessStatus) => void;
  log: (entry: LogEntry) => void;
  logs: (entries: LogEntry[]) => void;
  metrics: (metrics: SystemMetrics) => void;
  updateLog?: (text: string) => void;
  updateStatus?: (state: SteamUpdateState) => void;
  updateProgress?: (progress: SteamUpdateProgress) => void;
  updateSnapshot?: (snapshot: SteamUpdateSnapshot) => void;
  updateCheckResult?: (result: SteamUpdateCheckResult) => void;
  updateCheckSnapshot?: (result: SteamUpdateCheckResult) => void;
}

export const connectServerSocket = (handlers: ServerSocketEvents): Socket => {
  console.log('[Socket] Connecting to:', SOCKET_BASE);
  console.log('[Socket] Full URL:', `${SOCKET_BASE}/server`);
  
  const socket = io(`${SOCKET_BASE}/server`, {
    transports: ['polling', 'websocket'], // pollingを先に試す
    autoConnect: true,
    reconnection: true,
    reconnectionAttempts: 5,
    reconnectionDelay: 1000,
    timeout: 10000,
    forceNew: false,
    withCredentials: true
  });

  socket.on('connect', () => {
    console.log('[WebSocket] Connected to server');
    console.log('[WebSocket] Transport:', socket.io.engine.transport.name);
    console.log('[WebSocket] Socket ID:', socket.id);
  });

  socket.on('connect_error', (error) => {
    console.error('[WebSocket] Connection error:', error.message);
    console.error('[WebSocket] Error details:', error);
    console.error('[WebSocket] Attempting to connect to:', SOCKET_BASE);
  });

  socket.on('disconnect', (reason) => {
    console.warn('[WebSocket] Disconnected:', reason);
  });

  socket.on('error', (error) => {
    console.error('[WebSocket] Socket error:', error);
  });

  socket.on('status', handlers.status);
  socket.on('log', handlers.log);
  socket.on('logs', handlers.logs);
  socket.on('metrics', handlers.metrics);

  if (handlers.updateLog) {
    socket.on('update:log', handlers.updateLog);
  }
  if (handlers.updateStatus) {
    socket.on('update:status', handlers.updateStatus);
  }
  if (handlers.updateProgress) {
    socket.on('update:progress', handlers.updateProgress);
  }
  if (handlers.updateSnapshot) {
    socket.on('update:snapshot', handlers.updateSnapshot);
  }
  if (handlers.updateCheckResult) {
    socket.on('update:check-result', handlers.updateCheckResult);
  }
  if (handlers.updateCheckSnapshot) {
    socket.on('update:check-snapshot', handlers.updateCheckSnapshot);
  }

  return socket;
};
