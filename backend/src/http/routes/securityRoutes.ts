import { Router } from 'express';
import fs from 'fs';
import path from 'path';
import { authenticateToken } from '../../middleware/auth.js';
import { optionalCidrRestriction } from '../../middleware/cidr.js';
import { lenientRateLimit } from '../../middleware/rateLimit.js';
import { getClientIp, logSecurityEvent, isIpAllowed } from '../../utils/cidr.js';
import type { AuthenticatedRequest } from '../../middleware/auth.js';

const router = Router();

// 認証とレート制限を適用
router.use(authenticateToken);
router.use(lenientRateLimit);

// セキュリティ設定の取得
router.get('/config', (req: AuthenticatedRequest, res) => {
  try {
    const configPath = path.join(process.cwd(), '..', 'config', 'security.json');
    const configData = fs.readFileSync(configPath, 'utf-8');
    const config = JSON.parse(configData);
    
    res.json(config);
  } catch (error) {
    console.error('Failed to load security config:', error);
    res.status(500).json({ error: 'Failed to load security configuration' });
  }
});

// 現在のクライアントIP情報の取得
router.get('/client-info', (req: AuthenticatedRequest, res) => {
  const clientIp = getClientIp(req);
  const isAllowed = isIpAllowed(clientIp);
  
  res.json({
    clientIp,
    isAllowed,
    userAgent: req.headers['user-agent'],
    forwardedFor: req.headers['x-forwarded-for'],
    realIp: req.headers['x-real-ip']
  });
});

// セキュリティログの取得（簡易版）
router.get('/logs', (req: AuthenticatedRequest, res) => {
  // 実際の実装では、ログファイルから読み取る
  res.json({
    message: 'Security logs endpoint - to be implemented',
    note: 'Currently security events are logged to console'
  });
});

export { router as securityRoutes };
