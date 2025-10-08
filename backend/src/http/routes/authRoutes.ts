import { Router } from 'express';
import { generateToken, getPlainPassword, updatePlainPassword } from '../../utils/auth.js';
import { authenticateToken } from '../../middleware/auth.js';
import { strictRateLimit } from '../../middleware/rateLimit.js';
import type { AuthenticatedRequest } from '../../middleware/auth.js';

const router = Router();

// ログイン（厳しいレート制限を適用）
router.post('/login', strictRateLimit, async (req, res) => {
  try {
    const { password } = req.body;

    if (!password) {
      return res.status(400).json({ error: 'Password is required' });
    }

    const currentPassword = getPlainPassword();
    const isValid = password === currentPassword;

    if (!isValid) {
      return res.status(401).json({ error: 'Invalid password' });
    }

    const token = generateToken({ authenticated: true, timestamp: Date.now() });

    res.json({ token, message: 'Login successful' });
  } catch (error) {
    console.error('Login error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// パスワード更新（要認証）
router.post('/password/update', authenticateToken, async (req: AuthenticatedRequest, res) => {
  try {
    const { newPassword } = req.body;

    if (!newPassword || typeof newPassword !== 'string') {
      return res.status(400).json({ error: 'newPassword is required' });
    }

    // 過度な複雑化を避ける: 最小長のみ簡易チェック
    if (newPassword.length < 4) {
      return res.status(400).json({ error: 'Password must be at least 4 characters' });
    }

    updatePlainPassword(newPassword);

    res.json({ success: true, message: 'Password updated' });
  } catch (error) {
    console.error('Password update error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// 認証状態確認
router.get('/verify', authenticateToken, (req: AuthenticatedRequest, res) => {
  res.json({ authenticated: true, user: req.user });
});

// ログアウト（クライアント側でトークンを削除）
router.post('/logout', authenticateToken, (req: AuthenticatedRequest, res) => {
  res.json({ message: 'Logout successful' });
});

export { router as authRoutes };
