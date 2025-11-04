import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const currentDir = path.dirname(fileURLToPath(import.meta.url));

const resolveProjectRoot = (): string => {
  const candidates = [
    process.env.APP_ROOT,
    process.cwd(),
    path.resolve(process.cwd(), '..'),
    path.resolve(currentDir, '../../../..'),
    path.resolve(currentDir, '../../..'),
  ].filter((candidate): candidate is string => Boolean(candidate));

  for (const candidate of candidates) {
    if (fs.existsSync(path.join(candidate, 'package.json'))) {
      return candidate;
    }
  }

  return candidates[0] ?? currentDir;
};

export const PROJECT_ROOT = resolveProjectRoot();
const CONFIG_DIR = path.join(PROJECT_ROOT, 'config');

export const HEADLESS_EXECUTABLE =
  process.env.RESONITE_HEADLESS_PATH ||
  process.env.HEADLESS_EXECUTABLE ||
  'C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe';

export const HEADLESS_CONFIG_DIR =
  process.env.RESONITE_CONFIG_DIR ||
  process.env.HEADLESS_CONFIG_DIR ||
  path.join(CONFIG_DIR, 'headless');

export const RUNTIME_STATE_PATH =
  process.env.RUNTIME_STATE_PATH || path.join(CONFIG_DIR, 'runtime-state.json');

export const LOG_RING_BUFFER_SIZE = Number(process.env.LOG_RING_BUFFER_SIZE || 1000);

export const SERVER_PORT = Number(process.env.PORT || 8080);
