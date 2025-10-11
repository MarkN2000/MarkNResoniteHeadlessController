import { Router } from 'express';
import { serverRoutes } from './routes/serverRoutes.js';
import { authRoutes } from './routes/authRoutes.js';
import { securityRoutes } from './routes/securityRoutes.js';
import { modRoutes } from './routes/modRoutes.js';
import { createRestartRoutes } from './routes/restartRoutes.js';
import type { RestartManager } from '../services/restartManager.js';

export function createApiRouter(restartManager?: RestartManager): Router {
  const apiRouter = Router();

  apiRouter.use('/auth', authRoutes);
  apiRouter.use('/server', serverRoutes);
  apiRouter.use('/security', securityRoutes);
  apiRouter.use('/mod', modRoutes);
  
  // RestartManagerが提供されている場合のみrestartルートを追加
  if (restartManager) {
    apiRouter.use('/restart', createRestartRoutes(restartManager));
  }

  apiRouter.use((err: unknown, _req, res, _next) => {
    const message = err instanceof Error ? err.message : 'Unknown error';
    console.error('[API ERROR]', err);
    res.status(500).json({ error: message });
  });
  
  return apiRouter;
}

// 後方互換性のため、デフォルトエクスポートも提供
export const apiRouter = createApiRouter();
