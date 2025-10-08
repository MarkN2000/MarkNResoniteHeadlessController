interface CorsConfig {
  origin: string | string[] | boolean;
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
  origin: (origin, callback) => {
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
  origin: [
    // 本番環境のドメインを指定
    'https://yourdomain.com',
    'https://www.yourdomain.com'
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
 * 環境に応じたCORS設定を取得
 */
export const getCorsConfig = (): CorsConfig => {
  const isDevelopment = process.env.NODE_ENV === 'development' || 
                       process.env.NODE_ENV !== 'production';
  
  return isDevelopment ? developmentCors : productionCors;
};

/**
 * 動的オリジンチェック（本番環境用）
 */
export const dynamicOriginCheck = (origin: string | undefined): boolean => {
  const isDevelopment = process.env.NODE_ENV === 'development' || 
                       process.env.NODE_ENV !== 'production';
  
  if (isDevelopment) {
    // 開発環境: localhost系を許可
    return !origin || 
           origin.startsWith('http://localhost:') || 
           origin.startsWith('http://127.0.0.1:');
  }
  
  // 本番環境: 許可されたドメインのみ
  const allowedOrigins = [
    'https://yourdomain.com',
    'https://www.yourdomain.com'
  ];
  
  return !origin || allowedOrigins.includes(origin);
};
