import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { execFileSync, execSync, spawn } from 'child_process';
import readline from 'node:readline/promises';
import { stdin as input, stdout as output } from 'node:process';
import net from 'node:net';
import crypto from 'crypto';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT_DIR = path.resolve(__dirname, '..');
const BACKEND_DIR = path.join(ROOT_DIR, 'backend');
const CONFIG_DIR = path.join(ROOT_DIR, 'config');
const ENV_PATH = path.join(ROOT_DIR, '.env');
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
const DEFAULT_SERVER_PORT = 8080;
const DEFAULT_HEADLESS_PATH = 'C:/Program Files (x86)/Steam/steamapps/common/Resonite/Headless/Resonite.exe';

const divider = () => {
  console.log('======================================');
};

const generateSecureSecret = () => {
  return crypto.randomBytes(32).toString('base64');
};

const parseKeyValueLine = (line) => {
  const trimmed = line.trim();
  if (!trimmed || trimmed.startsWith('#')) {
    return null;
  }

  const idx = trimmed.indexOf('=');
  if (idx === -1) {
    return null;
  }

  const key = trimmed.slice(0, idx).trim();
  const value = trimmed.slice(idx + 1).trim();
  return { key, value };
};

const readEnvLines = () => {
  if (!fs.existsSync(ENV_PATH)) {
    return [];
  }

  const raw = fs.readFileSync(ENV_PATH, 'utf-8').replace(/\r\n?/g, '\n');
  const lines = raw.split('\n');
  if (lines.length && lines[lines.length - 1] === '') {
    lines.pop();
  }
  return lines;
};

const writeEnvLines = (lines) => {
  const content = lines.join('\n');
  const terminator = content ? `${content}\n` : '';
  fs.writeFileSync(ENV_PATH, terminator, 'utf-8');
};

const replaceEnvValue = (lines, key, value) => {
  const upperKey = key.toUpperCase();
  let found = false;

  const updated = lines.map((line) => {
    const kv = parseKeyValueLine(line);
    if (kv && kv.key.toUpperCase() === upperKey) {
      found = true;
      return `${key}=${value}`;
    }
    return line;
  });

  if (!found) {
    updated.push(`${key}=${value}`);
  }

  return updated;
};

const saveServerPort = (port) => {
  const portValue = String(port);
  let lines = readEnvLines();
  lines = replaceEnvValue(lines, 'PORT', portValue);
  lines = replaceEnvValue(lines, 'SERVER_PORT', portValue);
  writeEnvLines(lines);
  console.log(`[INFO] .env のポート設定を更新しました (PORT=${portValue}).`);
};

const parsePort = (value) => {
  if (typeof value !== 'string') {
    return null;
  }
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }
  const num = Number(trimmed);
  if (!Number.isInteger(num) || num < 1 || num > 65535) {
    return null;
  }
  return num;
};

const loadPortFromLines = (lines, key) => {
  for (const line of lines) {
    const kv = parseKeyValueLine(line);
    if (kv && kv.key.toUpperCase() === key.toUpperCase()) {
      const parsed = parsePort(kv.value);
      if (parsed) {
        return parsed;
      }
    }
  }
  return null;
};

const loadSavedServerPort = () => {
  const candidates = [process.env.PORT, process.env.SERVER_PORT];
  for (const candidate of candidates) {
    const parsed = parsePort(candidate);
    if (parsed) {
      return parsed;
    }
  }

  const lines = readEnvLines();
  const fromPort = loadPortFromLines(lines, 'PORT');
  if (fromPort) {
    return fromPort;
  }
  return loadPortFromLines(lines, 'SERVER_PORT');
};

const applyServerPortToProcess = (port) => {
  const portValue = String(port);
  process.env.PORT = portValue;
  process.env.SERVER_PORT = portValue;
};

const isPortAvailable = (port) =>
  new Promise((resolve) => {
    const server = net.createServer();

    server.once('error', (error) => {
      if (error.code !== 'EADDRINUSE' && error.code !== 'EACCES') {
        console.warn(`[WARN] ポート ${port} の使用状況確認中にエラーが発生しました:`, error);
      }
      resolve(false);
    });

    server.once('listening', () => {
      server.close(() => resolve(true));
    });

    server.listen(port, '0.0.0.0');
  });

const promptForServerPort = async (defaultPort = DEFAULT_SERVER_PORT) => {
  if (!process.stdin.isTTY) {
    return defaultPort;
  }

  const rl = readline.createInterface({ input, output });
  let resolvedPort = defaultPort;

  while (true) {
    const answer = await rl.question(
      `使用するポート番号を入力してください (Enterで ${defaultPort}): `,
    );
    const trimmed = answer.trim();
    const candidate = trimmed ? parsePort(trimmed) : defaultPort;

    if (!candidate) {
      console.log('1〜65535 の範囲でポート番号を入力してください。');
      continue;
    }

    const available = await isPortAvailable(candidate);
    if (!available) {
      console.log(`ポート ${candidate} は既に使用中です。別のポートを入力してください。`);
      continue;
    }

    resolvedPort = candidate;
    break;
  }

  rl.close();
  console.log('');
  return resolvedPort;
};

const ensureServerPortConfigured = async () => {
  let port = loadSavedServerPort();

  if (port) {
    const available = await isPortAvailable(port);
    if (!available) {
      if (process.stdin.isTTY) {
        console.log('');
        console.log(`--- 保存されているポート ${port} は使用中です。別のポートを指定してください。 ---`);
        port = await promptForServerPort(port);
        saveServerPort(port);
      } else {
        console.warn(
          `[WARN] 保存されているポート ${port} は使用中の可能性があります。必要に応じて .env を更新してください。`,
        );
      }
    }
  } else if (process.stdin.isTTY) {
    console.log('');
    console.log('--- サーバーのポート番号が未設定です。 ---');
    port = await promptForServerPort(DEFAULT_SERVER_PORT);
    saveServerPort(port);
  } else {
    port = DEFAULT_SERVER_PORT;
    console.warn(
      `[WARN] 対話的な入力が利用できません。デフォルトポート ${port} を使用します。必要に応じて .env を手動で更新してください。`,
    );
    const available = await isPortAvailable(port);
    if (!available) {
      console.warn(`[WARN] ポート ${port} は使用中かもしれません。手動で変更してください。`);
    }
    saveServerPort(port);
  }

  if (!port) {
    port = DEFAULT_SERVER_PORT;
  }

  applyServerPortToProcess(port);
  return port;
};

const validateHeadlessPath = (filePath) => {
  if (!filePath || typeof filePath !== 'string') {
    return { valid: false, reason: 'パスが指定されていません。' };
  }

  const trimmed = filePath.trim();
  if (!trimmed) {
    return { valid: false, reason: 'パスが空です。' };
  }

  // パスが Headless/Resonite.exe または Headless\Resonite.exe で終わっているか確認
  const normalizedPath = trimmed.replace(/\\/g, '/');
  if (!normalizedPath.toLowerCase().endsWith('headless/resonite.exe')) {
    return {
      valid: false,
      reason: 'パスは Headless/Resonite.exe で終わる必要があります。',
    };
  }

  // ファイルが存在するか確認
  if (!fs.existsSync(trimmed)) {
    return { valid: false, reason: '指定されたファイルが存在しません。' };
  }

  return { valid: true };
};

const loadSavedHeadlessPath = () => {
  // 環境変数から確認
  if (process.env.RESONITE_HEADLESS_PATH) {
    const validation = validateHeadlessPath(process.env.RESONITE_HEADLESS_PATH);
    if (validation.valid) {
      return process.env.RESONITE_HEADLESS_PATH;
    }
  }

  // .env ファイルから読み込み
  const lines = readEnvLines();
  for (const line of lines) {
    const kv = parseKeyValueLine(line);
    if (kv && kv.key.toUpperCase() === 'RESONITE_HEADLESS_PATH') {
      const validation = validateHeadlessPath(kv.value);
      if (validation.valid) {
        return kv.value;
      }
    }
  }

  return null;
};

const saveHeadlessPath = (headlessPath) => {
  const pathValue = String(headlessPath);
  let lines = readEnvLines();
  lines = replaceEnvValue(lines, 'RESONITE_HEADLESS_PATH', pathValue);
  writeEnvLines(lines);
  console.log(`[INFO] .env の RESONITE_HEADLESS_PATH を更新しました。`);
};

const promptForHeadlessPath = async (defaultPath = DEFAULT_HEADLESS_PATH) => {
  if (!process.stdin.isTTY) {
    return defaultPath;
  }

  const rl = readline.createInterface({ input, output });
  let resolvedPath = defaultPath;

  while (true) {
    const answer = await rl.question(
      `Resonite.exeのパスを入力（未入力の場合 ${defaultPath} となります): `,
    );
    const trimmed = answer.trim();
    const candidate = trimmed || defaultPath;

    const validation = validateHeadlessPath(candidate);
    if (!validation.valid) {
      console.log(`エラー: ${validation.reason}`);
      continue;
    }

    resolvedPath = candidate;
    break;
  }

  rl.close();
  console.log('');
  return resolvedPath;
};

const ensureHeadlessPathConfigured = async () => {
  let headlessPath = loadSavedHeadlessPath();

  if (headlessPath) {
    const validation = validateHeadlessPath(headlessPath);
    if (!validation.valid) {
      if (process.stdin.isTTY) {
        console.log('');
        console.log(`--- 保存されているパスが無効です: ${validation.reason} ---`);
        console.log('--- Headlessの実行ファイルパスを変更する場合は入力してください ---');
        headlessPath = await promptForHeadlessPath(DEFAULT_HEADLESS_PATH);
        saveHeadlessPath(headlessPath);
      } else {
        console.warn(
          `[WARN] 保存されているパスが無効です: ${validation.reason}。必要に応じて .env を更新してください。`,
        );
        headlessPath = DEFAULT_HEADLESS_PATH;
        saveHeadlessPath(headlessPath);
      }
    }
  } else if (process.stdin.isTTY) {
    console.log('');
    console.log('--- Headlessの実行ファイルパスを変更する場合は入力してください ---');
    headlessPath = await promptForHeadlessPath(DEFAULT_HEADLESS_PATH);
    saveHeadlessPath(headlessPath);
  } else {
    headlessPath = DEFAULT_HEADLESS_PATH;
    console.warn(
      `[WARN] 対話的な入力が利用できません。デフォルトパス ${headlessPath} を使用します。必要に応じて .env を手動で更新してください。`,
    );
    saveHeadlessPath(headlessPath);
  }

  if (!headlessPath) {
    headlessPath = DEFAULT_HEADLESS_PATH;
    saveHeadlessPath(headlessPath);
  }

  return headlessPath;
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

  const appPassword = await askNonEmpty('アプリのログインパスワードを設定してください: ', { confirm: true });
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

const isDefaultSecret = (secret) => {
  if (!secret || typeof secret !== 'string') {
    return true;
  }
  const trimmed = secret.trim();
  return (
    !trimmed ||
    trimmed === 'your-secret-key' ||
    trimmed === 'your-secret-key-change-in-production' ||
    trimmed.length < 16
  );
};

const ensureAuthSecretConfigured = () => {
  let secret = null;
  let needsEnvUpdate = false;
  let needsAuthJsonUpdate = false;

  // .env ファイルが存在しない場合は作成（setup.js で作成されるはずだが念のため）
  if (!fs.existsSync(ENV_PATH)) {
    console.log('[INFO] .env ファイルが存在しないため、作成します。');
    writeEnvLines([]);
  }

  // .env ファイルから AUTH_SHARED_SECRET を読み込む
  const envLines = readEnvLines();
  let envSecret = null;
  for (const line of envLines) {
    const kv = parseKeyValueLine(line);
    if (kv && kv.key.toUpperCase() === 'AUTH_SHARED_SECRET') {
      envSecret = kv.value;
      break;
    }
  }

  // 環境変数からも確認
  if (!envSecret && process.env.AUTH_SHARED_SECRET) {
    envSecret = process.env.AUTH_SHARED_SECRET;
  }

  // デフォルト値の場合は生成が必要
  if (isDefaultSecret(envSecret)) {
    secret = generateSecureSecret();
    needsEnvUpdate = true;
    console.log('[INFO] AUTH_SHARED_SECRET を自動生成しました。');
  } else {
    secret = envSecret;
  }

  // auth.json の jwtSecret を確認
  let authConfig = loadOrCreateAuthConfig();
  if (isDefaultSecret(authConfig.jwtSecret)) {
    // 環境変数または生成したシークレットを使用
    authConfig.jwtSecret = secret;
    needsAuthJsonUpdate = true;
    console.log('[INFO] auth.json の jwtSecret を自動設定しました。');
  }

  // .env ファイルを更新
  if (needsEnvUpdate) {
    let updatedLines = replaceEnvValue(envLines, 'AUTH_SHARED_SECRET', secret);
    writeEnvLines(updatedLines);
    console.log('[INFO] .env の AUTH_SHARED_SECRET を更新しました。');
    // 更新後の行を再読み込み（NODE_ENV のチェック用）
    envLines.length = 0;
    envLines.push(...readEnvLines());
  }

  // NODE_ENV=production を設定（本番環境）
  const hasNodeEnv = envLines.some((line) => {
    const kv = parseKeyValueLine(line);
    return kv && kv.key.toUpperCase() === 'NODE_ENV';
  });
  if (!hasNodeEnv) {
    const updatedLines = replaceEnvValue(envLines, 'NODE_ENV', 'production');
    writeEnvLines(updatedLines);
    console.log('[INFO] .env の NODE_ENV を production に設定しました。');
  } else {
    // 既存の NODE_ENV を確認し、development の場合は production に変更
    const nodeEnvLine = envLines.find((line) => {
      const kv = parseKeyValueLine(line);
      return kv && kv.key.toUpperCase() === 'NODE_ENV';
    });
    if (nodeEnvLine) {
      const kv = parseKeyValueLine(nodeEnvLine);
      if (kv && kv.value.toLowerCase() === 'development') {
        const updatedLines = replaceEnvValue(envLines, 'NODE_ENV', 'production');
        writeEnvLines(updatedLines);
        console.log('[INFO] .env の NODE_ENV を production に更新しました。');
      }
    }
  }

  // auth.json を更新
  if (needsAuthJsonUpdate) {
    saveAuthConfig(authConfig);
    console.log('[INFO] config/auth.json の jwtSecret を更新しました。');
  }

  return secret;
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

  // シークレットの自動生成・設定
  ensureAuthSecretConfigured();

  const credentials = await promptForInitialCredentials();
  if (credentials) {
    updateAuthPassword(credentials.appPassword);
    saveHeadlessCredentials(credentials.headlessUsername, credentials.headlessPassword);
    applyHeadlessCredentialsToConfigs(credentials.headlessUsername, credentials.headlessPassword);
  }

  await ensureHeadlessPathConfigured();
  await ensureServerPortConfigured();

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
  // シークレットの自動生成・設定（初回起動時にも実行）
  ensureAuthSecretConfigured();

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
      
      // 資格情報入力後にHeadlessパスとポート設定を促す
      console.log('');
      await ensureHeadlessPathConfigured();
      await ensureServerPortConfigured();
      return true; // 資格情報を設定したことを示す
    }
    console.warn('[WARN] 資格情報が設定されていません。必要に応じて config/auth.json と config/headless/*.json を手動で更新してください。');
    return false;
  }

  // 既存の資格情報でプリセットを整備
  applyHeadlessCredentialsToConfigs(savedHeadlessCredentials.username, savedHeadlessCredentials.password);
  return false; // 既存の資格情報を使用したことを示す
};

const startApplication = () => {
  divider();
  console.log(' MarkN Resonite Headless Controller');
  console.log(' Production Mode');
  divider();
  console.log('');
  console.log('[INFO] Starting backend server...');
  const port = process.env.PORT || String(DEFAULT_SERVER_PORT);
  console.log(`Port: ${port}`);
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

  // サーバー起動を少し待ってからリンクを表示
  setTimeout(() => {
    console.log('');
    console.log('======================================');
    console.log(' サーバーが起動しました！');
    console.log('======================================');
    console.log('');
    console.log(` アクセス: http://localhost:${port}/`);
    console.log('');
  }, 2000);

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
      const credentialsWereSet = await ensureCredentialsConfigured();
      // 資格情報を設定した場合は、その時点でHeadlessパスとポート設定も完了している
      // 資格情報が既に設定済みの場合は、ここでHeadlessパスとポート設定を確認
      if (!credentialsWereSet) {
        await ensureHeadlessPathConfigured();
        await ensureServerPortConfigured();
      }
    }

    startApplication();
  } catch (error) {
    console.error('\n[ERROR] 処理中にエラーが発生しました:', error.message);
    waitForExit(1);
  }
};

void main();

