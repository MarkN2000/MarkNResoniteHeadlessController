import path from 'node:path';
import { Router } from 'express';
import { processManager } from '../../services/processManager.js';

export const serverRoutes = Router();

serverRoutes.get('/status', (_req, res) => {
  res.json(processManager.getStatus());
});

serverRoutes.get('/logs', (req, res) => {
  const limit = req.query.limit ? Number(req.query.limit) : undefined;
  res.json(processManager.getLogs(limit));
});

serverRoutes.get('/configs', (_req, res) => {
  const configs = processManager.listConfigs().map(filePath => ({
    path: filePath,
    name: path.basename(filePath)
  }));
  res.json(configs);
});

serverRoutes.post('/start', (req, res, next) => {
  try {
    const { configPath } = req.body ?? {};
    processManager.start(configPath);
    res.json({ ok: true });
  } catch (error) {
    next(error);
  }
});

serverRoutes.post('/stop', async (_req, res, next) => {
  try {
    await processManager.stop();
    res.json({ ok: true });
  } catch (error) {
    next(error);
  }
});
