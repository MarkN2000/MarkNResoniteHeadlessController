import { Router } from 'express';
import { generateToken, hashPassword, comparePassword, getDefaultPassword } from '../../utils/auth.js';
import { authenticateToken } from '../../middleware/auth.js';
import type { AuthenticatedRequest } from '../../middleware/auth.js';

const router = Router();

// ログイン
router.post('/login', async (req, res) => {
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
