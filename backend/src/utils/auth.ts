import jwt from 'jsonwebtoken';
import bcrypt from 'bcryptjs';
import fs from 'fs';
import path from 'path';

interface AuthConfig {
  jwtSecret: string;
  jwtExpiresIn: string;
  defaultPassword?: string; // 互換のため残す
  password?: string; // プレーンパスワード（ユーザー設定）
}

let authConfig: AuthConfig | null = null;

const getConfigPath = (): string => {
  return path.join(process.cwd(), '..', 'config', 'auth.json');
};

const loadAuthConfig = (): AuthConfig => {
  if (!authConfig) {
    const configPath = getConfigPath();
    const configData = fs.readFileSync(configPath, 'utf-8');
    authConfig = JSON.parse(configData);
  }
  return authConfig;
};

const saveAuthConfig = (config: AuthConfig) => {
  const configPath = getConfigPath();
  // インデントと整形を一定にする
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2), 'utf-8');
  authConfig = null; // 次回読み込みでリロード
};

export const generateToken = (payload: any): string => {
  const config = loadAuthConfig();
  return jwt.sign(payload, config.jwtSecret, { expiresIn: config.jwtExpiresIn });
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
