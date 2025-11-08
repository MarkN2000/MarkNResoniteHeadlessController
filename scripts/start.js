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

const loadOrCreateAuthConfig = () => {
  let authConfig;

  if (fs.existsSync(AUTH_CONFIG_PATH)) {
    try {
      authConfig = JSON.parse(fs.readFileSync(AUTH_CONFIG_PATH, 'utf-8'));
    } catch (error) {
      console.warn('[WARN] config/auth.json の読み込みに失敗しました。テンプレートまたはデフォルトを使用します:', error);
    }
  }

  if (!authConfig && fs.existsSync(AUTH_CONFIG_TEMPLATE)) {
    try {
      authConfig = JSON.parse(fs.readFileSync(AUTH_CONFIG_TEMPLATE, 'utf-8'));
    } catch (error) {
      console.warn('[WARN] auth.json.example の読み込みに失敗しました。デフォルトを使用します:', error);
    }
  }

  if (!authConfig) {
    authConfig = {
      jwtSecret: 'your-secret-key-change-in-production',
      jwtExpiresIn: '24h',
      password: '',
    };
  }

  if ('defaultPassword' in authConfig) {
    delete authConfig.defaultPassword;
  }

  return authConfig;
};

const saveAuthConfig = (config) => {
  fs.mkdirSync(path.dirname(AUTH_CONFIG_PATH), { recursive: true });
  fs.writeFileSync(AUTH_CONFIG_PATH, JSON.stringify(config, null, 2), 'utf-8');
};

const updateAuthPassword = (password) => {
  const authConfig = loadOrCreateAuthConfig();
  authConfig.password = password;
  saveAuthConfig(authConfig);
  console.log(`[INFO] config/auth.json を更新しました。`);
};

const loadAuthPassword = () => {
  try {
    if (!fs.existsSync(AUTH_CONFIG_PATH)) {
      return null;
    }

    const data = JSON.parse(fs.readFileSync(AUTH_CONFIG_PATH, 'utf-8'));
    const password = typeof data.password === 'string' ? data.password : null;
    return password;
  } catch (error) {
    console.warn('[WARN] config/auth.json の読み込みに失敗しました:', error);
    return null;
  }
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

const saveHeadlessCredentials = (username, password) => {
  const authConfig = loadOrCreateAuthConfig();
  authConfig.headlessCredentials = {
    username,
    password,
    updatedAt: new Date().toISOString(),
  };
  saveAuthConfig(authConfig);
  console.log(
    `[INFO] Headless資格情報を保存しました: ${path.relative(
      ROOT_DIR,
      AUTH_CONFIG_PATH,
    )} (headlessCredentials)`,
  );
};

const loadSavedHeadlessCredentials = () => {
  try {
    if (!fs.existsSync(AUTH_CONFIG_PATH)) {
      return null;
    }

    const raw = fs.readFileSync(AUTH_CONFIG_PATH, 'utf-8');
    const json = JSON.parse(raw);
    const credentials = json?.headlessCredentials;
    const username =
      typeof credentials?.username === 'string' ? credentials.username.trim() : '';
    const password =
      typeof credentials?.password === 'string' ? credentials.password : '';

    if (!username || !password) {
      return null;
    }

    return { username, password };
  } catch (error) {
    console.warn('[WARN] Headless資格情報の読み込みに失敗しました:', error);
    return null;
  }
};

const ensureDefaultHeadlessConfig = () => {
  fs.mkdirSync(HEADLESS_CONFIG_DIR, { recursive: true });

  const defaultPath = path.join(HEADLESS_CONFIG_DIR, 'default.json');
  if (fs.existsSync(defaultPath)) {
    return defaultPath;
  }

  let config;
  for (const candidate of HEADLESS_CONFIG_TEMPLATE_CANDIDATES) {
    if (candidate && fs.existsSync(candidate)) {
      try {
        config = JSON.parse(fs.readFileSync(candidate, 'utf-8'));
        break;
      } catch (error) {
        console.warn(`[WARN] テンプレートの読み込みに失敗しました: ${candidate}`, error);
      }
    }
  }

  if (!config) {
    config = loadHeadlessTemplate();
  }

  fs.writeFileSync(defaultPath, JSON.stringify(config, null, 2), 'utf-8');
  console.log(`[INFO] Headlessデフォルト設定を作成しました: ${path.relative(ROOT_DIR, defaultPath)}`);
  return defaultPath;
};

const updateHeadlessConfigFile = (configPath, username, password) => {
  try {
    const config = JSON.parse(fs.readFileSync(configPath, 'utf-8'));
    config.loginCredential = username;
    config.loginPassword = password;

    if (!Array.isArray(config.startWorlds)) {
      config.startWorlds = [];
    }

    fs.writeFileSync(configPath, JSON.stringify(config, null, 2), 'utf-8');
    console.log(`[INFO] Headless設定を更新しました: ${path.relative(ROOT_DIR, configPath)}`);
  } catch (error) {
    console.warn(`[WARN] Headless設定の更新に失敗しました (${configPath}):`, error);
  }
};

const applyHeadlessCredentialsToConfigs = (username, password) => {
  const defaultPath = ensureDefaultHeadlessConfig();

  const entries = fs.readdirSync(HEADLESS_CONFIG_DIR, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isFile()) continue;
    if (!entry.name.toLowerCase().endsWith('.json')) continue;
    if (entry.name === 'credentials.json') continue;

    const targetPath = path.join(HEADLESS_CONFIG_DIR, entry.name);
    try {
      updateHeadlessConfigFile(targetPath, username, password);
    } catch (error) {
      console.warn(`[WARN] Headless設定ファイルにアクセスできません (${targetPath}):`, error);
    }
  }

  // 確実に default.json が最新化されるよう明示呼び出し
  if (fs.existsSync(defaultPath)) {
    updateHeadlessConfigFile(defaultPath, username, password);
  }
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
    saveHeadlessCredentials(credentials.headlessUsername, credentials.headlessPassword);
    applyHeadlessCredentialsToConfigs(credentials.headlessUsername, credentials.headlessPassword);
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

const ensureCredentialsConfigured = async () => {
  const savedAuthPassword = loadAuthPassword();
  const savedHeadlessCredentials = loadSavedHeadlessCredentials();

  const needsAuthPassword = !savedAuthPassword; // パスワードが設定されていない場合
  const needsHeadlessCredentials = !savedHeadlessCredentials;

  if (needsAuthPassword || needsHeadlessCredentials) {
    console.log('');
    console.log('--- 初期パスワード／Headless資格情報が未設定です。 ---');
    const credentials = await promptForInitialCredentials();
    if (credentials) {
      updateAuthPassword(credentials.appPassword);
      saveHeadlessCredentials(credentials.headlessUsername, credentials.headlessPassword);
      applyHeadlessCredentialsToConfigs(credentials.headlessUsername, credentials.headlessPassword);
      return;
    }
    console.warn('[WARN] 資格情報が設定されていません。必要に応じて config/auth.json と config/headless/*.json を手動で更新してください。');
    return;
  }

  // 既存の資格情報でプリセットを整備
  applyHeadlessCredentialsToConfigs(savedHeadlessCredentials.username, savedHeadlessCredentials.password);
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
    } else {
      await ensureCredentialsConfigured();
    }

    startApplication();
  } catch (error) {
    console.error('\n[ERROR] 処理中にエラーが発生しました:', error.message);
    waitForExit(1);
  }
};

void main();

