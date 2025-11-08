import fs from 'node:fs';
import path from 'node:path';

import { PROJECT_ROOT } from './index.js';

export interface HeadlessCredentials {
  username: string;
  password: string;
}

const AUTH_CONFIG_PATH = path.join(PROJECT_ROOT, 'config', 'auth.json');

const isNonEmptyString = (value: unknown): value is string =>
  typeof value === 'string' && value.trim().length > 0;

export const getHeadlessCredentials = (): HeadlessCredentials | null => {
  try {
    if (!fs.existsSync(AUTH_CONFIG_PATH)) {
      return null;
    }

    const raw = fs.readFileSync(AUTH_CONFIG_PATH, 'utf-8');
    const json = JSON.parse(raw);
    const credentials = json?.headlessCredentials;
    const username = isNonEmptyString(credentials?.username) ? credentials.username.trim() : '';
    const password = isNonEmptyString(credentials?.password) ? credentials.password : '';

    if (!username || !password) {
      return null;
    }

    return {
      username,
      password,
    };
  } catch (error) {
    console.warn('[HeadlessCredentials] Failed to read headless credentials from auth.json:', error);
    return null;
  }
};

