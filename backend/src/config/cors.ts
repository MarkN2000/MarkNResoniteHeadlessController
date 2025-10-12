interface CorsConfig {
  origin: string | string[] | boolean | ((origin: string | undefined, callback: (err: Error | null, allow?: boolean) => void) => void);
  credentials: boolean;
  methods: string[];
  allowedHeaders: string[];
}

/**
 * 開発環境のCORS設定
 */
const developmentCors: CorsConfig = {
  origin: [
    'http://localhost:5173', // Vite開発サーバー
    'http://localhost:3000',  // 代替ポート
    'http://127.0.0.1:5173', // ローカルホスト代替
    'http://127.0.0.1:3000'
  ],
  credentials: true,
  methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
  allowedHeaders: [
    'Content-Type',
    'Authorization',
    'X-Requested-With',
    'Accept',
    'Origin'
  ]
};

/**
 * Mod専用のCORS設定（ローカルネットワークのみ）
 */
export const modCors: CorsConfig = {
  origin: (origin: string | undefined, callback: (err: Error | null, allow?: boolean) => void) => {
    // ローカルネットワーク内のIPのみ許可
    if (!origin || 
        origin.startsWith('http://192.168.') ||
        origin.startsWith('http://10.') ||
        origin.startsWith('http://127.0.0.1:') ||
        origin.startsWith('http://localhost:')) {
      callback(null, true);
    } else {
      callback(new Error('Not allowed by CORS'));
    }
  },
  credentials: false, // Mod用は認証情報不要
  methods: ['GET', 'POST'],
  allowedHeaders: [
    'Content-Type',
    'X-Mod-Api-Key'
  ]
};

/**
 * 本番環境のCORS設定
 */
const productionCors: CorsConfig = {
  origin: (origin: string | undefined, callback: (err: Error | null, allow?: boolean) => void) => {
    // originがない場合（同一オリジン）は許可
    if (!origin) {
      callback(null, true);
      return;
    }
    
    // localhost系は常に許可（ローカル本番環境用）
    if (origin.startsWith('http://localhost:') || 
        origin.startsWith('http://127.0.0.1:') ||
        origin.startsWith('https://localhost:') || 
        origin.startsWith('https://127.0.0.1:')) {
      callback(null, true);
      return;
    }
    
    // ローカルネットワーク内のIPを許可
    if (origin.startsWith('http://192.168.') || 
        origin.startsWith('http://10.')) {
      callback(null, true);
      return;
    }
    
    // その他の本番ドメイン
    const allowedDomains = [
      'https://yourdomain.com',
      'https://www.yourdomain.com'
    ];
    
    if (allowedDomains.includes(origin)) {
      callback(null, true);
    } else {
      console.log(`[CORS] Blocked origin: ${origin}`);
      callback(new Error('Not allowed by CORS'));
    }
  },
  credentials: true,
  methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
  allowedHeaders: [
    'Content-Type',
    'Authorization',
    'X-Requested-With',
    'Accept',
    'Origin'
  ]
};

/**
 * 環境に応じたCORS設定を取得
 */
export const getCorsConfig = (): CorsConfig => {
  const isDevelopment = process.env.NODE_ENV === 'development' || 
                       process.env.NODE_ENV !== 'production';
  
  return isDevelopment ? developmentCors : productionCors;
};

/**
 * 動的オリジンチェック（Socket.IO用）
 */
export const dynamicOriginCheck = (origin: string | undefined, callback: (error: Error | null, allow?: boolean) => void) => {
  // originがない場合（同一オリジン）は許可
  if (!origin) {
    callback(null, true);
    return;
  }
  
  // localhost系は常に許可
  if (origin.startsWith('http://localhost:') || 
      origin.startsWith('http://127.0.0.1:') ||
      origin.startsWith('https://localhost:') || 
      origin.startsWith('https://127.0.0.1:')) {
    callback(null, true);
    return;
  }
  
  // ローカルネットワーク内のIPを許可
  if (origin.startsWith('http://192.168.') || 
      origin.startsWith('http://10.')) {
    callback(null, true);
    return;
  }
  
  const isDevelopment = process.env.NODE_ENV === 'development' || 
                       process.env.NODE_ENV !== 'production';
  
  if (isDevelopment) {
    callback(null, true);
    return;
  }
  
  // 本番環境: 許可されたドメインのみ
  const allowedOrigins = [
    'https://yourdomain.com',
    'https://www.yourdomain.com'
  ];
  
  if (allowedOrigins.includes(origin)) {
    callback(null, true);
  } else {
    console.log(`[WebSocket CORS] Blocked origin: ${origin}`);
    callback(new Error('Not allowed by CORS'), false);
  }
};
