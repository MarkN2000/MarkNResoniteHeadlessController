import path from 'node:path';

const ROOT_DIR = path.resolve(__dirname, '../../..');

export const HEADLESS_EXECUTABLE = process.env.HEADLESS_EXECUTABLE ||
  'C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe';

export const HEADLESS_CONFIG_DIR = process.env.HEADLESS_CONFIG_DIR ||
  path.join(ROOT_DIR, 'config', 'headless');

export const LOG_RING_BUFFER_SIZE = Number(process.env.LOG_RING_BUFFER_SIZE || 1000);

export const SERVER_PORT = Number(process.env.PORT || 8080);
