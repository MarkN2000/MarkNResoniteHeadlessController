import { Server } from 'socket.io';
import { processManager } from '../services/processManager.js';
import { isIpAllowed, getClientIp, logSecurityEvent } from '../utils/cidr.js';

export const registerSocketHandlers = (io: Server): void => {
  const namespace = io.of('/server');

  namespace.on('connection', socket => {
    // WebSocket接続のCIDRチェック
    const clientIp = getClientIp(socket.request);
    console.log(`[WebSocket] Connection attempt from ${clientIp}`);
    
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
    
    socket.emit('status', processManager.getStatus());
    socket.emit('logs', processManager.getLogs(200));

    const handleLog = (entry: ReturnType<typeof processManager.getLogs>[number]) => {
      socket.emit('log', entry);
    };
    const handleStatus = (status: ReturnType<typeof processManager.getStatus>) => {
      socket.emit('status', status);
    };

    processManager.on('log', handleLog);
    processManager.on('status', handleStatus);

    socket.on('disconnect', () => {
      console.log(`[WebSocket] Client ${clientIp} disconnected`);
      processManager.off('log', handleLog);
      processManager.off('status', handleStatus);
    });
  });
  
  console.log('[WebSocket] Socket handlers registered on /server namespace');
};
