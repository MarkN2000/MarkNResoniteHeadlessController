import type { Request, Response, NextFunction } from 'express';
import { checkRateLimit, generateRateLimitKey, decrementRateLimit } from '../utils/rateLimit.js';
import { logSecurityEvent } from '../utils/cidr.js';

interface RateLimitConfig {
  windowMs: number;
  maxRequests: number;
  skipSuccessfulRequests?: boolean;
}

/**
 * レート制限ミドルウェア
 */
export const rateLimit = (config: RateLimitConfig) => {
  return (req: Request, res: Response, next: NextFunction) => {
    const key = generateRateLimitKey(req);
    const result = checkRateLimit(key, config);
    
    // レスポンスヘッダーを設定
    res.set({
      'X-RateLimit-Limit': config.maxRequests.toString(),
      'X-RateLimit-Remaining': result.remaining.toString(),
      'X-RateLimit-Reset': new Date(result.resetTime).toISOString()
    });
    
    if (!result.allowed) {
      logSecurityEvent('RATE_LIMIT_EXCEEDED', req.ip || 'unknown', {
        key,
        path: req.path,
        method: req.method,
        userAgent: req.headers['user-agent']
      });
      
      return res.status(429).json({
        error: 'Too Many Requests',
        message: 'Rate limit exceeded. Please try again later.',
        retryAfter: Math.ceil((result.resetTime - Date.now()) / 1000)
      });
    }
    
    // 成功したリクエストをスキップする設定の場合
    if (config.skipSuccessfulRequests) {
      const originalSend = res.send;
      res.send = function(data) {
        if (res.statusCode < 400) {
          decrementRateLimit(key);
        }
        return originalSend.call(this, data);
      };
    }
    
    next();
  };
};

/**
 * 厳しいレート制限（ログイン試行用）
 */
export const strictRateLimit = rateLimit({
  windowMs: 15 * 60 * 1000, // 15分
  maxRequests: 5, // 5回まで
  skipSuccessfulRequests: true
});

/**
 * 通常のレート制限（API用）
 */
export const normalRateLimit = rateLimit({
  windowMs: 15 * 60 * 1000, // 15分
  maxRequests: 1000, // 1000回まで
  skipSuccessfulRequests: false
});

/**
 * 緩いレート制限（読み取り専用API用）
 */
export const lenientRateLimit = rateLimit({
  windowMs: 15 * 60 * 1000, // 15分
  maxRequests: 1000, // 1000回まで
  skipSuccessfulRequests: false
});
