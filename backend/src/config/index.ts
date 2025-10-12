import path from 'node:path';
import { fileURLToPath } from 'node:url';

const currentDir = path.dirname(fileURLToPath(import.meta.url));
// 本番環境: dist/backend/src/config -> プロジェクトルート は ../../../../
// 開発環境: src/config -> プロジェクトルート は ../..
// process.cwd() を使用してプロジェクトルート（バックエンドディレクトリ）を取得し、さらに1つ上へ
const ROOT_DIR = path.resolve(process.cwd(), '..');

export const HEADLESS_EXECUTABLE =
  process.env.RESONITE_HEADLESS_PATH ||
  process.env.HEADLESS_EXECUTABLE ||
  'C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe';

export const HEADLESS_CONFIG_DIR =
  process.env.RESONITE_CONFIG_DIR ||
  process.env.HEADLESS_CONFIG_DIR ||
  path.join(ROOT_DIR, 'config', 'headless');

export const RUNTIME_STATE_PATH =
  process.env.RUNTIME_STATE_PATH || path.join(ROOT_DIR, 'config', 'runtime-state.json');

export const LOG_RING_BUFFER_SIZE = Number(process.env.LOG_RING_BUFFER_SIZE || 1000);

export const SERVER_PORT = Number(process.env.PORT || 8080);
