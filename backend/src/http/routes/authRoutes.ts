import { Router } from 'express';
import { generateToken, hashPassword, comparePassword, getDefaultPassword, getResoniteConfigSettings, updateResoniteConfigSettings, saveAuthConfig, getAuthConfig } from '../../utils/auth.js';
import { authenticateToken } from '../../middleware/auth.js';
import { strictRateLimit, normalRateLimit } from '../../middleware/rateLimit.js';
import type { AuthenticatedRequest } from '../../middleware/auth.js';
import { cidrAllowlistMiddleware } from '../../utils/cidr.js';

const router = Router();

// ログイン（厳しいレート制限を適用）
router.post('/login', strictRateLimit, async (req, res) => {
  try {
    const { password } = req.body;
    
    if (!password) {
      return res.status(400).json({ error: 'Password is required' });
    }

    // デフォルトパスワードとの比較
    const defaultPassword = getDefaultPassword();
    const isValid = password === defaultPassword;

    if (!isValid) {
      return res.status(401).json({ error: 'Invalid password' });
    }

    // JWTトークン生成
    const token = generateToken({ 
      authenticated: true, 
      timestamp: Date.now() 
    });

    res.json({ 
      token,
      message: 'Login successful' 
    });
  } catch (error) {
    console.error('Login error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// 認証状態確認
router.get('/verify', authenticateToken, (req: AuthenticatedRequest, res) => {
  res.json({ 
    authenticated: true,
    user: req.user 
  });
});

// ログアウト（クライアント側でトークンを削除）
router.post('/logout', authenticateToken, (req: AuthenticatedRequest, res) => {
  res.json({ message: 'Logout successful' });
});

export { router as authRoutes };

// Resonite 認証設定: 取得（JWT + CIDR + レート制限）
router.get('/resonite', authenticateToken, cidrAllowlistMiddleware, normalRateLimit, (_req, res) => {
  const settings = getResoniteConfigSettings();
  // パスワードはマスクして返す
  res.json({
    loginUsername: settings.loginUsername,
    loginUserid: settings.loginUserid,
    loginPassword: settings.loginPassword ? '********' : '',
    isEncrypted: settings.isEncrypted
  });
});

// Resonite 認証設定: 更新（JWT + CIDR + レート制限）
router.put('/resonite', authenticateToken, cidrAllowlistMiddleware, normalRateLimit, (req, res) => {
  const { loginUsername, loginUserid, loginPassword, isEncrypted } = req.body ?? {};

  if (loginUsername !== undefined && typeof loginUsername !== 'string') {
    return res.status(400).json({ error: 'loginUsername must be string' });
  }
  if (loginUserid !== undefined && typeof loginUserid !== 'string') {
    return res.status(400).json({ error: 'loginUserid must be string' });
  }
  if (loginPassword !== undefined && typeof loginPassword !== 'string') {
    return res.status(400).json({ error: 'loginPassword must be string' });
  }
  if (isEncrypted !== undefined && typeof isEncrypted !== 'boolean') {
    return res.status(400).json({ error: 'isEncrypted must be boolean' });
  }

  const updated = updateResoniteConfigSettings({ loginUsername, loginUserid, loginPassword, isEncrypted });
  res.json({ ok: true, settings: { ...updated, loginPassword: updated.loginPassword ? '********' : '' } });
});

// Resonite 認証設定: 削除（JWT + CIDR + レート制限）
router.delete('/resonite', authenticateToken, cidrAllowlistMiddleware, normalRateLimit, (_req, res) => {
  const cfg = getAuthConfig();
  const cleared = { loginUsername: '', loginUserid: '', loginPassword: '', isEncrypted: false };
  const merged = { ...cfg, resoniteConfig: cleared } as any;
  saveAuthConfig(merged);
  res.json({ ok: true });
});
