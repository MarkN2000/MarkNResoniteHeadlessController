import { Server } from 'socket.io';
import { processManager } from '../services/processManager.js';
import { systemMetricsCollector } from '../services/systemMetrics.js';
import { steamUpdateBus } from '../services/steamUpdateBus.js';
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

  // SteamCMDアップデートのイベントを中継（ルートハンドラ -> バス -> WS）
  steamUpdateBus.on('log', (text) => {
    namespace.emit('update:log', text);
  });
  steamUpdateBus.on('status', (state) => {
    namespace.emit('update:status', state);
  });
  steamUpdateBus.on('progress', (progress) => {
    namespace.emit('update:progress', progress);
  });
  // Resonite 最新バージョン確認の結果を中継
  steamUpdateBus.on('check-result', (result) => {
    namespace.emit('update:check-result', result);
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

    // SteamCMDアップデートが進行中、または直近の結果が残っている場合は
    // 接続したクライアントだけにスナップショットを送る（再接続時の状態復元に使用）
    const updateSnapshot = steamUpdateBus.getSnapshot();
    if (updateSnapshot.state !== null) {
      socket.emit('update:snapshot', updateSnapshot);
    }

    // 既にバージョン確認が 1 度走っていれば、その結果を接続クライアントへ即送する。
    // これでフロントは onMount 前でも赤ドットバッジの初期表示を復元できる。
    const checkSnapshot = steamUpdateBus.getCheckSnapshot();
    if (checkSnapshot) {
      socket.emit('update:check-snapshot', checkSnapshot);
    }

    socket.on('disconnect', () => {
      console.log(`[WebSocket] Client ${clientIp} disconnected`);
    });
  });

  console.log('[WebSocket] Socket handlers registered on /server namespace');
};
