import type { Request, Response, NextFunction } from 'express';
import { isIpAllowed, getClientIp, logSecurityEvent, debugClientInfo } from '../utils/cidr.js';

/**
 * CIDR制限ミドルウェア
 * 許可されたIP範囲からのアクセスのみを許可
 */
export const cidrRestriction = (req: Request, res: Response, next: NextFunction) => {
  // 開発環境でのデバッグ情報を出力
  const { clientIp, isAllowed } = debugClientInfo(req);
  
  // IPアドレスが許可されているかチェック
  if (isAllowed) {
    // 許可されたIPからのアクセス
    logSecurityEvent('ACCESS_ALLOWED', clientIp, {
      path: req.path,
      method: req.method
    });
    next();
  } else {
    // 拒否されたIPからのアクセス
    logSecurityEvent('ACCESS_DENIED', clientIp, {
      userAgent: req.headers['user-agent'],
      path: req.path,
      method: req.method
    });
    
    res.status(403).json({ 
      error: 'Access denied',
      message: 'Your IP address is not allowed to access this service'
    });
  }
};

/**
 * オプショナルCIDR制限ミドルウェア
 * ログは記録するが、アクセスは許可
 */
export const optionalCidrRestriction = (req: Request, res: Response, next: NextFunction) => {
  const clientIp = getClientIp(req);
  
  if (isIpAllowed(clientIp)) {
    logSecurityEvent('ACCESS_ALLOWED', clientIp, {
      path: req.path,
      method: req.method
    });
  } else {
    logSecurityEvent('ACCESS_WARNING', clientIp, {
      userAgent: req.headers['user-agent'],
      path: req.path,
      method: req.method,
      message: 'Access from non-whitelisted IP'
    });
  }
  
  next();
};
