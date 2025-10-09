import { Router } from 'express';
import { serverRoutes } from './routes/serverRoutes.js';
import { authRoutes } from './routes/authRoutes.js';
import { securityRoutes } from './routes/securityRoutes.js';
import { modRoutes } from './routes/modRoutes.js';

export const apiRouter = Router();

apiRouter.use('/auth', authRoutes);
apiRouter.use('/server', serverRoutes);
apiRouter.use('/security', securityRoutes);
apiRouter.use('/mod', modRoutes);

apiRouter.use((err: unknown, _req, res, _next) => {
  const message = err instanceof Error ? err.message : 'Unknown error';
  console.error('[API ERROR]', err);
  res.status(500).json({ error: message });
});
