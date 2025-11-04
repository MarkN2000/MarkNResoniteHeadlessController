import jwt from 'jsonwebtoken';
import bcrypt from 'bcryptjs';
import fs from 'fs';
import path from 'path';
import { PROJECT_ROOT } from '../config/index.js';

interface AuthConfig {
  jwtSecret: string;
  jwtExpiresIn: string;
  defaultPassword?: string; // 互換のため残す
  password?: string; // プレーンパスワード（ユーザー設定）
}

let authConfig: AuthConfig | null = null;

const getConfigPath = (): string => path.join(PROJECT_ROOT, 'config', 'auth.json');

const loadAuthConfig = (): AuthConfig => {
  if (!authConfig) {
    const configPath = getConfigPath();
    
    try {
      const configData = fs.readFileSync(configPath, 'utf-8');
      const fileConfig = JSON.parse(configData);
      
      // 環境変数で上書き可能
      authConfig = {
        jwtSecret: process.env.AUTH_SHARED_SECRET || fileConfig.jwtSecret,
        jwtExpiresIn: process.env.JWT_EXPIRES_IN || fileConfig.jwtExpiresIn,
        defaultPassword: process.env.DEFAULT_PASSWORD || fileConfig.defaultPassword,
        password: process.env.DEFAULT_PASSWORD || fileConfig.password,
      };
    } catch (error: any) {
      if (error.code === 'ENOENT') {
        // ファイルが存在しない場合はデフォルト設定を作成
        console.log('[Auth] Config file not found, creating default auth.json');
        const defaultConfig: AuthConfig = {
          jwtSecret: process.env.AUTH_SHARED_SECRET || 'your-secret-key-change-in-production',
          jwtExpiresIn: process.env.JWT_EXPIRES_IN || '24h',
          defaultPassword: process.env.DEFAULT_PASSWORD || 'admin123',
          password: process.env.DEFAULT_PASSWORD || 'admin123',
        };
        saveAuthConfig(defaultConfig);
        authConfig = defaultConfig;
      } else {
        console.error('[Auth] Failed to load auth config:', error);
        // エラー時はデフォルト設定を使用
        authConfig = {
          jwtSecret: process.env.AUTH_SHARED_SECRET || 'your-secret-key-change-in-production',
          jwtExpiresIn: process.env.JWT_EXPIRES_IN || '24h',
          defaultPassword: process.env.DEFAULT_PASSWORD || 'admin123',
          password: process.env.DEFAULT_PASSWORD || 'admin123',
        };
      }
    }
  }
  return authConfig;
};

const saveAuthConfig = (config: AuthConfig) => {
  const configPath = getConfigPath();
  // インデントと整形を一定にする
  fs.mkdirSync(path.dirname(configPath), { recursive: true });
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2), 'utf-8');
  authConfig = null; // 次回読み込みでリロード
};

export const generateToken = (payload: any): string => {
  const config = loadAuthConfig();
  return jwt.sign(payload, config.jwtSecret, { expiresIn: config.jwtExpiresIn } as jwt.SignOptions);
};

export const verifyToken = (token: string): any => {
  const config = loadAuthConfig();
  return jwt.verify(token, config.jwtSecret);
};

export const hashPassword = async (password: string): Promise<string> => {
  return bcrypt.hash(password, 10);
};

export const comparePassword = async (password: string, hashedPassword: string): Promise<boolean> => {
  return bcrypt.compare(password, hashedPassword);
};

export const getDefaultPassword = (): string => {
  const config = loadAuthConfig();
  // 後方互換: defaultPassword が残っている場合に参照
  return config.defaultPassword || '';
};

export const getPlainPassword = (): string => {
  const config = loadAuthConfig();
  // 優先: password（ユーザー設定）。未設定時はdefaultPasswordへフォールバック
  return (config.password && String(config.password)) || getDefaultPassword();
};

export const updatePlainPassword = (newPassword: string) => {
  const config = loadAuthConfig();
  const nextConfig: AuthConfig = { ...config, password: newPassword };
  saveAuthConfig(nextConfig);
};
