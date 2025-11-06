import fs from 'node:fs';
import path from 'node:path';

import { HEADLESS_CONFIG_DIR } from './index.js';

export interface HeadlessCredentials {
  username: string;
  password: string;
}

const CREDENTIALS_FILE = path.join(HEADLESS_CONFIG_DIR, 'credentials.json');

const isNonEmptyString = (value: unknown): value is string =>
  typeof value === 'string' && value.trim().length > 0;

export const getHeadlessCredentials = (): HeadlessCredentials | null => {
  try {
    if (!fs.existsSync(CREDENTIALS_FILE)) {
      return null;
    }

    const raw = fs.readFileSync(CREDENTIALS_FILE, 'utf-8');
    const json = JSON.parse(raw);
    const username = isNonEmptyString(json.username) ? json.username.trim() : '';
    const password = isNonEmptyString(json.password) ? json.password : '';

    if (!username && !password) {
      return null;
    }

    return {
      username,
      password,
    };
  } catch (error) {
    console.warn('[HeadlessCredentials] Failed to read credentials:', error);
    return null;
  }
};

