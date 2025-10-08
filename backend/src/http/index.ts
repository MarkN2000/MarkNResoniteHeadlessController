import { Router } from 'express';
import { serverRoutes } from './routes/serverRoutes.js';
import { authRoutes } from './routes/authRoutes.js';

export const apiRouter = Router();

apiRouter.use('/auth', authRoutes);
apiRouter.use('/server', serverRoutes);

apiRouter.use((err: unknown, _req, res, _next) => {
  const message = err instanceof Error ? err.message : 'Unknown error';
  res.status(500).json({ error: message });
});
