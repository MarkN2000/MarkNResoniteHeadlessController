interface RateLimitEntry {
  count: number;
  resetTime: number;
}

interface RateLimitConfig {
  windowMs: number; // 時間窓（ミリ秒）
  maxRequests: number; // 最大リクエスト数
  skipSuccessfulRequests?: boolean; // 成功したリクエストをスキップするか
}

// メモリ内のレート制限ストア
const rateLimitStore = new Map<string, RateLimitEntry>();

// デフォルト設定
const defaultConfig: RateLimitConfig = {
  windowMs: 15 * 60 * 1000, // 15分
  maxRequests: 100, // 100リクエスト
  skipSuccessfulRequests: false
};

/**
 * レート制限をチェック
 */
export const checkRateLimit = (
  key: string, 
  config: RateLimitConfig = defaultConfig
): { allowed: boolean; remaining: number; resetTime: number } => {
  const now = Date.now();
  const entry = rateLimitStore.get(key);
  
  // エントリが存在しないか、リセット時間が過ぎている場合
  if (!entry || now > entry.resetTime) {
    const newEntry: RateLimitEntry = {
      count: 1,
      resetTime: now + config.windowMs
    };
    rateLimitStore.set(key, newEntry);
    
    return {
      allowed: true,
      remaining: config.maxRequests - 1,
      resetTime: newEntry.resetTime
    };
  }
  
  // リクエスト数が上限に達している場合
  if (entry.count >= config.maxRequests) {
    return {
      allowed: false,
      remaining: 0,
      resetTime: entry.resetTime
    };
  }
  
  // リクエスト数を増加
  entry.count++;
  rateLimitStore.set(key, entry);
  
  return {
    allowed: true,
    remaining: config.maxRequests - entry.count,
    resetTime: entry.resetTime
  };
};

/**
 * 成功したリクエストのカウントを減らす（オプション）
 */
export const decrementRateLimit = (key: string): void => {
  const entry = rateLimitStore.get(key);
  if (entry && entry.count > 0) {
    entry.count--;
    rateLimitStore.set(key, entry);
  }
};

/**
 * 古いエントリをクリーンアップ
 */
export const cleanupRateLimit = (): void => {
  const now = Date.now();
  for (const [key, entry] of rateLimitStore.entries()) {
    if (now > entry.resetTime) {
      rateLimitStore.delete(key);
    }
  }
};

/**
 * レート制限キーを生成
 */
export const generateRateLimitKey = (req: any): string => {
  const clientIp = req.ip || req.connection?.remoteAddress || 'unknown';
  const userAgent = req.headers['user-agent'] || 'unknown';
  
  // IPアドレスとUser-Agentの組み合わせでキーを生成
  return `${clientIp}:${userAgent}`.substring(0, 100); // キーの長さを制限
};

/**
 * 定期的なクリーンアップを開始
 */
export const startRateLimitCleanup = (): void => {
  // 5分ごとにクリーンアップ
  setInterval(cleanupRateLimit, 5 * 60 * 1000);
};

// 起動時にクリーンアップを開始
startRateLimitCleanup();
