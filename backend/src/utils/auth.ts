import jwt from 'jsonwebtoken';
import bcrypt from 'bcryptjs';
import fs from 'fs';
import path from 'path';

interface AuthConfig {
  jwtSecret: string;
  jwtExpiresIn: string;
  defaultPassword: string;
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
