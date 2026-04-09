import { Server } from 'socket.io';
import { processManager } from '../services/processManager.js';
import { systemMetricsCollector } from '../services/systemMetrics.js';
import { isIpAllowed, getClientIp, logSecurityEvent } from '../utils/cidr.js';

export const registerSocketHandlers = (io: Server): void => {
  const namespace = io.of('/server');

  // namespace レベルで1つだけリスナーを登録し、全クライアントに一括配信
  processManager.on('log', (entry) => {
    namespace.emit('log', entry);
  });
  processManager.on('status', (status) => {
    namespace.emit('status', status);
  });
  systemMetricsCollector.on('metrics', (metrics) => {
    namespace.emit('metrics', metrics);
  });

  namespace.on('connection', socket => {
    // WebSocket接続のCIDRチェック
    const clientIp = getClientIp(socket.request);
    console.log(`[WebSocket] Connection attempt from ${clientIp}`);
    console.log(`[WebSocket] Request headers:`, socket.request.headers);

    if (!isIpAllowed(clientIp)) {
      console.error(`[WebSocket] Access denied for ${clientIp}`);
      logSecurityEvent('WEBSOCKET_ACCESS_DENIED', clientIp, {
        userAgent: socket.request.headers['user-agent']
      });
      socket.disconnect(true);
      return;
    }

    console.log(`[WebSocket] Access allowed for ${clientIp}`);
    logSecurityEvent('WEBSOCKET_ACCESS_ALLOWED', clientIp);

    // 初回接続時に現在の状態を送信
    socket.emit('status', processManager.getStatus());
    socket.emit('logs', processManager.getLogs(200));

    const currentMetrics = systemMetricsCollector.getCurrentMetrics();
    if (currentMetrics) {
      socket.emit('metrics', currentMetrics);
    }

    socket.on('disconnect', () => {
      console.log(`[WebSocket] Client ${clientIp} disconnected`);
    });
  });

  console.log('[WebSocket] Socket handlers registered on /server namespace');
};
