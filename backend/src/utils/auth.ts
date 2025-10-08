import jwt from 'jsonwebtoken';
import bcrypt from 'bcryptjs';
import fs from 'fs';
import path from 'path';

interface ResoniteConfigSettings {
  loginUsername: string;
  loginUserid: string;
  loginPassword: string; // plain by design per spec
  isEncrypted: boolean; // keep flag for future switch
}

interface UserCredentialsSettings {
  username: string;
  userId: string;
  password: string;
  isEncrypted: boolean;
  lastUpdated: string;
}

interface AuthConfig {
  jwtSecret: string;
  jwtExpiresIn: string;
  defaultPassword: string;
  userCredentials?: UserCredentialsSettings;
  resoniteConfig?: ResoniteConfigSettings;
}

let authConfig: AuthConfig | null = null;

const loadAuthConfig = (): AuthConfig => {
  if (!authConfig) {
    const configPath = path.join(process.cwd(), '..', 'config', 'auth.json');
    const configData = fs.readFileSync(configPath, 'utf-8');
    authConfig = JSON.parse(configData);
  }
  return authConfig;
};

export const getAuthConfig = (): AuthConfig => loadAuthConfig();

const AUTH_PATH = () => path.join(process.cwd(), '..', 'config', 'auth.json');

export const saveAuthConfig = (config: AuthConfig) => {
  const filePath = AUTH_PATH();
  fs.writeFileSync(filePath, JSON.stringify(config, null, 2), 'utf-8');
  authConfig = config; // update cache
};

export const getResoniteConfigSettings = (): ResoniteConfigSettings => {
  const cfg = loadAuthConfig();
  return cfg.resoniteConfig ?? { loginUsername: '', loginUserid: '', loginPassword: '', isEncrypted: false };
};

export const updateResoniteConfigSettings = (update: Partial<ResoniteConfigSettings>) => {
  const cfg = loadAuthConfig();
  const current = cfg.resoniteConfig ?? { loginUsername: '', loginUserid: '', loginPassword: '', isEncrypted: false };
  const next: ResoniteConfigSettings = {
    loginUsername: update.loginUsername ?? current.loginUsername ?? '',
    loginUserid: update.loginUserid ?? current.loginUserid ?? '',
    loginPassword: update.loginPassword ?? current.loginPassword ?? '',
    isEncrypted: update.isEncrypted ?? current.isEncrypted ?? false
  };
  const merged: AuthConfig = { ...cfg, resoniteConfig: next };
  saveAuthConfig(merged);
  return next;
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
  return config.defaultPassword;
};
