import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { execFileSync, execSync, spawn } from 'child_process';
import readline from 'node:readline/promises';
import { stdin as input, stdout as output } from 'node:process';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT_DIR = path.resolve(__dirname, '..');
const BACKEND_DIR = path.join(ROOT_DIR, 'backend');
const CONFIG_DIR = path.join(ROOT_DIR, 'config');
const HEADLESS_CONFIG_DIR = path.join(CONFIG_DIR, 'headless');
const AUTH_CONFIG_PATH = path.join(CONFIG_DIR, 'auth.json');
const AUTH_CONFIG_TEMPLATE = path.join(CONFIG_DIR, 'auth.json.example');
const HEADLESS_CONFIG_TARGET = path.join(HEADLESS_CONFIG_DIR, 'default.json');
const HEADLESS_CONFIG_TEMPLATE_CANDIDATES = [
  path.join(HEADLESS_CONFIG_DIR, 'default.template.json'),
  path.join(ROOT_DIR, 'sample', 'default.json'),
];
const SETUP_MARKER = path.join(ROOT_DIR, '.setup_completed');
const SETUP_SCRIPT = path.join(__dirname, 'setup.js');
const APP_ENTRY = path.join(BACKEND_DIR, 'dist', 'backend', 'src', 'app.js');

const divider = () => {
  console.log('======================================');
};

const waitForExit = (code = 0) => {
  if (!process.stdin.isTTY) {
    process.exit(code);
    return;
  }

  console.log('何かキーを押すと終了します...');
  process.stdin.resume();
  process.stdin.setEncoding('utf8');
  process.stdin.once('data', () => {
    process.exit(code);
  });
};

const ensureFileExists = (filePath, friendlyName) => {
  if (!fs.existsSync(filePath)) {
    throw new Error(`${friendlyName} が見つかりませんでした: ${filePath}`);
  }
};

const promptForInitialCredentials = async () => {
  if (!process.stdin.isTTY) {
    console.warn('[WARN] 対話的な入力が利用できません。config/auth.json と config/headless/*.json を手動で更新してください。');
    return null;
  }

  const rl = readline.createInterface({ input, output });

  const askNonEmpty = async (question, { confirm = false, mask = false } = {}) => {
    while (true) {
      const value = mask ? await rl.question(question, { signal: undefined }) : await rl.question(question);
      const trimmed = value.trim();
      if (!trimmed) {
        console.log('値が空です。もう一度入力してください。');
        continue;
      }
      if (confirm) {
        const confirmation = mask
          ? await rl.question('（確認）もう一度入力してください: ', { signal: undefined })
          : await rl.question('（確認）もう一度入力してください: ');
        if (trimmed !== confirmation.trim()) {
          console.log('入力が一致しません。もう一度やり直してください。');
          continue;
        }
      }
      return trimmed;
    }
  };

  const appPassword = await askNonEmpty('アプリのログインパスワードを入力してください: ', { confirm: true });
  const headlessUsername = await askNonEmpty('Headlessのユーザー名を入力してください: ');
  const headlessPassword = await askNonEmpty('Headlessのパスワードを入力してください: ', { confirm: true });

  rl.close();
  console.log('');

  return {
    appPassword,
    headlessUsername,
    headlessPassword,
  };
};

const updateAuthPassword = (password) => {
  let authConfig;
  if (fs.existsSync(AUTH_CONFIG_PATH)) {
    authConfig = JSON.parse(fs.readFileSync(AUTH_CONFIG_PATH, 'utf-8'));
  } else if (fs.existsSync(AUTH_CONFIG_TEMPLATE)) {
    authConfig = JSON.parse(fs.readFileSync(AUTH_CONFIG_TEMPLATE, 'utf-8'));
  } else {
    authConfig = {
      jwtSecret: 'your-secret-key-change-in-production',
      jwtExpiresIn: '24h',
      defaultPassword: 'admin123',
      password: 'admin123',
    };
  }

  authConfig.password = password;
  authConfig.defaultPassword = password;

  fs.mkdirSync(path.dirname(AUTH_CONFIG_PATH), { recursive: true });
  fs.writeFileSync(AUTH_CONFIG_PATH, JSON.stringify(authConfig, null, 2), 'utf-8');
  console.log(`[INFO] config/auth.json を更新しました。`);
};

const loadHeadlessTemplate = () => {
  for (const candidate of HEADLESS_CONFIG_TEMPLATE_CANDIDATES) {
    if (candidate && fs.existsSync(candidate)) {
      try {
        return JSON.parse(fs.readFileSync(candidate, 'utf-8'));
      } catch (error) {
        console.warn(`[WARN] テンプレートの読み込みに失敗しました: ${candidate}`, error);
      }
    }
  }

  return {
    $schema: 'https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json',
    comment: 'Auto-generated headless config',
    universeId: null,
    tickRate: 60,
    maxConcurrentAssetTransfers: 128,
    usernameOverride: null,
    loginCredential: '',
    loginPassword: '',
    startWorlds: [],
    dataFolder: null,
    cacheFolder: null,
    logsFolder: null,
    allowedUrlHosts: null,
    autoSpawnItems: null,
  };
};

const updateHeadlessCredentials = (username, password) => {
  fs.mkdirSync(HEADLESS_CONFIG_DIR, { recursive: true });

  let config;
  if (fs.existsSync(HEADLESS_CONFIG_TARGET)) {
    try {
      config = JSON.parse(fs.readFileSync(HEADLESS_CONFIG_TARGET, 'utf-8'));
    } catch (error) {
      console.warn(`[WARN] 既存の headless 設定を読み込めませんでした。テンプレートから再生成します: ${HEADLESS_CONFIG_TARGET}`);
      config = loadHeadlessTemplate();
    }
  } else {
    config = loadHeadlessTemplate();
  }

  config.loginCredential = username;
  config.loginPassword = password;

  if (!Array.isArray(config.startWorlds)) {
    config.startWorlds = [];
  }

  fs.writeFileSync(HEADLESS_CONFIG_TARGET, JSON.stringify(config, null, 2), 'utf-8');
  console.log(`[INFO] Headless設定を更新しました: ${path.relative(ROOT_DIR, HEADLESS_CONFIG_TARGET)}`);
};

const runInitialSetup = async () => {
  divider();
  console.log(' MarkN Resonite Headless Controller');
  console.log(' Initial Setup');
  divider();
  console.log('');

  console.log('[1/3] setup.js を実行しています...');
  execFileSync(process.execPath, [SETUP_SCRIPT], {
    stdio: 'inherit',
    cwd: ROOT_DIR,
  });
  console.log('');

  console.log('[2/3] backend の依存関係をインストールしています...');
  execSync('npm install --omit=dev', {
    stdio: 'inherit',
    cwd: BACKEND_DIR,
  });
  console.log('');

  console.log('[3/3] 追加のビルドは不要です。プリビルド資産を使用します。');
  console.log('');

  const credentials = await promptForInitialCredentials();
  if (credentials) {
    updateAuthPassword(credentials.appPassword);
    updateHeadlessCredentials(credentials.headlessUsername, credentials.headlessPassword);
  }

  divider();
  console.log(' Setup Complete!');
  divider();
  console.log('');
  console.log('Next steps:');
  console.log('  1. 環境に合わせて .env を更新');
  console.log('  2. config/security.json でアクセス制限を調整');
  console.log('  3. config/headless/default.json を確認し、必要に応じてワールド設定を編集');
  console.log('');

  fs.writeFileSync(
    SETUP_MARKER,
    `Setup completed on ${new Date().toISOString()}${process.platform === 'win32' ? '\r\n' : '\n'}`,
  );
};

const startApplication = () => {
  divider();
  console.log(' MarkN Resonite Headless Controller');
  console.log(' Production Mode');
  divider();
  console.log('');
  console.log('[INFO] Starting backend server...');
  console.log('Port: 8080');
  console.log('');

  ensureFileExists(APP_ENTRY, 'バックエンドのビルド成果物 (backend/dist/backend/src/app.js)');

  const child = spawn(process.execPath, [APP_ENTRY], {
    stdio: 'inherit',
    cwd: ROOT_DIR,
    env: {
      ...process.env,
      NODE_ENV: 'production',
      APP_ROOT: ROOT_DIR,
    },
  });

  child.on('error', (error) => {
    console.error('[ERROR] アプリケーションの起動に失敗しました:', error.message);
    waitForExit(1);
  });

  child.on('close', (code) => {
    console.log('');
    console.log(`アプリケーションが終了しました。終了コード: ${code}`);
    waitForExit(code ?? 0);
  });
};

const main = async () => {
  try {
    if (!fs.existsSync(SETUP_MARKER)) {
      await runInitialSetup();
    }

    startApplication();
  } catch (error) {
    console.error('\n[ERROR] 処理中にエラーが発生しました:', error.message);
    waitForExit(1);
  }
};

void main();

