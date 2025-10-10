import { Router } from 'express';
import cors from 'cors';
import { processManager } from '../../services/processManager.js';
import { cidrRestriction } from '../../middleware/cidr.js';
import { lenientRateLimit } from '../../middleware/rateLimit.js';
import { modCors } from '../../config/cors.js';
import { getPlainPassword } from '../../utils/auth.js';
import type { ExecuteCommandOptions } from '../../services/processManager.js';

const router = Router();

// Mod専用の軽量認証（APIキーベース）
// 既定ではアプリのログインパスワードと同一キーを使用
const modApiKey = process.env.MOD_API_KEY || getPlainPassword();

const authenticateMod = (req: any, res: any, next: any) => {
  const apiKey = req.headers['x-mod-api-key'] || req.query.apiKey;
  
  if (!apiKey || apiKey !== modApiKey) {
    return res.status(401).json({ error: 'Invalid API key' });
  }
  
  next();
};

// Mod専用ルート（認証は軽量、CORSは制限的）
router.use(cors(modCors)); // Mod専用CORS設定
router.use(authenticateMod);
router.use(cidrRestriction); // ローカルネットワークのみ
router.use(lenientRateLimit); // 緩いレート制限

// コマンド実行
router.post('/command', async (req, res) => {
  try {
    const { command, options } = req.body;
    
    if (!command) {
      return res.status(400).json({ error: 'Command is required' });
    }

    const result = await processManager.executeCommand(command, options);
    
    res.json({ 
      success: true, 
      result,
      timestamp: new Date().toISOString()
    });
  } catch (error) {
    console.error('Mod command error:', error);
    res.status(500).json({ 
      error: 'Command execution failed',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// サーバー状態取得
router.get('/status', (req, res) => {
  res.json(processManager.getStatus());
});

// ログ取得（最新100件）
router.get('/logs', (req, res) => {
  const logs = processManager.getLogs(100);
  res.json(logs);
});

// ワールド一覧取得
router.get('/worlds', async (req, res) => {
  try {
    const output = await processManager.executeCommand('worlds');
    res.json({ 
      success: true, 
      result: output,
      timestamp: new Date().toISOString()
    });
  } catch (error) {
    console.error('Mod worlds error:', error);
    res.status(500).json({ 
      error: 'Failed to get worlds',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// ユーザー一覧取得
router.get('/users', async (req, res) => {
  try {
    const output = await processManager.executeCommand('users');
    res.json({ 
      success: true, 
      result: output,
      timestamp: new Date().toISOString()
    });
  } catch (error) {
    console.error('Mod users error:', error);
    res.status(500).json({ 
      error: 'Failed to get users',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// サーバー起動
router.post('/start', async (req, res) => {
  try {
    const { configPath } = req.body;
    processManager.start(configPath);
    res.json({ 
      success: true,
      timestamp: new Date().toISOString()
    });
  } catch (error) {
    console.error('Mod start error:', error);
    res.status(500).json({ 
      error: 'Failed to start server',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

// サーバー停止
router.post('/stop', async (req, res) => {
  try {
    await processManager.stop();
    res.json({ 
      success: true,
      timestamp: new Date().toISOString()
    });
  } catch (error) {
    console.error('Mod stop error:', error);
    res.status(500).json({ 
      error: 'Failed to stop server',
      message: error instanceof Error ? error.message : 'Unknown error'
    });
  }
});

export { router as modRoutes };
