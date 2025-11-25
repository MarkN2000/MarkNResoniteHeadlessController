import fs from 'fs';
import path from 'path';
import https from 'https';
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
const DEFAULT_STEAMCMD_INSTALL_DIR = 'C:/steamcmd';
const STEAM_CONFIG_FILE = path.join(CONFIG_DIR, 'steam.json');
const STEAMCMD_DOWNLOAD_URL =
  'https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip';

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

/**
 * SteamCMD がインストールされているか確認
 */
const checkSteamCmdInstalled = async (
  installDir = DEFAULT_STEAMCMD_INSTALL_DIR,
) => {
  const steamcmdExePath = path.join(installDir, 'steamcmd.exe');
  try {
    await fs.promises.access(steamcmdExePath);
    return { installed: true, path: steamcmdExePath };
  } catch {
    return { installed: false, path: steamcmdExePath };
  }
};

/**
 * URL からファイルをダウンロード
 */
const downloadFile = (url, destPath) =>
  new Promise((resolve, reject) => {
    const file = fs.createWriteStream(destPath);

    https
      .get(url, (response) => {
        if (response.statusCode !== 200) {
          file.close();
          if (fs.existsSync(destPath)) {
            fs.unlinkSync(destPath);
          }
          reject(
            new Error(
              `HTTP ${response.statusCode}: ${response.statusMessage || ''}`,
            ),
          );
          return;
        }

        const totalSize = parseInt(
          response.headers['content-length'] || '0',
          10,
        );
        let downloadedSize = 0;

        response.on('data', (chunk) => {
          downloadedSize += chunk.length;
          if (totalSize > 0) {
            const percent = ((downloadedSize / totalSize) * 100).toFixed(1);
            process.stdout.write(
              `\r[SteamCMD] ダウンロード進捗: ${percent}%`,
            );
          }
        });

        response.pipe(file);

        file.on('finish', () => {
          file.close();
          console.log(''); // 改行
          resolve();
        });

        file.on('error', (err) => {
          file.close();
          if (fs.existsSync(destPath)) {
            fs.unlinkSync(destPath);
          }
          reject(err);
        });
      })
      .on('error', (err) => {
        file.close();
        if (fs.existsSync(destPath)) {
          fs.unlinkSync(destPath);
        }
        reject(err);
      });
  });

/**
 * SteamCMD を自動インストール
 */
const installSteamCmd = async (installDir = DEFAULT_STEAMCMD_INSTALL_DIR) => {
  const steamcmdExePath = path.join(installDir, 'steamcmd.exe');
  const zipPath = path.join(installDir, 'steamcmd.zip');

  try {
    // インストールディレクトリを作成
    await fs.promises.mkdir(installDir, { recursive: true });

    console.log(`[SteamCMD] SteamCMDをダウンロードしています...`);
    console.log(`[SteamCMD] URL: ${STEAMCMD_DOWNLOAD_URL}`);

    // ZIP ファイルをダウンロード
    await downloadFile(STEAMCMD_DOWNLOAD_URL, zipPath);

    console.log(`[SteamCMD] SteamCMDを展開しています...`);
    console.log(`[SteamCMD] インストール先: ${installDir}`);

    // PowerShell の Expand-Archive で ZIP を展開
    const escapedZipPath = zipPath.replace(/'/g, "''");
    const escapedDestDir = installDir.replace(/'/g, "''");
    const command = `powershell -Command "Expand-Archive -Path '${escapedZipPath}' -DestinationPath '${escapedDestDir}' -Force"`;

    execSync(command, { stdio: 'inherit' });

    // ZIP ファイルを削除
    if (fs.existsSync(zipPath)) {
      await fs.promises.unlink(zipPath);
    }

    // インストール確認
    try {
      await fs.promises.access(steamcmdExePath);
      console.log(`[SteamCMD] インストールが完了しました: ${steamcmdExePath}`);
      return { success: true, path: steamcmdExePath };
    } catch (error) {
      return {
        success: false,
        error: `SteamCMDのインストールは完了しましたが、実行ファイルが見つかりません: ${
          error && error.message ? error.message : String(error)
        }`,
      };
    }
  } catch (error) {
    return {
      success: false,
      error: `SteamCMDのインストールに失敗しました: ${
        error && error.message ? error.message : String(error)
      }`,
    };
  }
};

/**
 * SteamCMD のインストールを確認し、必要に応じて自動インストール
 * （初回セットアップ時のみ呼び出される想定）
 */
const ensureSteamCmdInstalled = async () => {
  console.log('');
  console.log('--- SteamCMDの確認中... ---');

  // まず config/steam.json に設定済みのパスを確認
  let steamCmdPath = null;
  if (fs.existsSync(STEAM_CONFIG_FILE)) {
    try {
      const config = JSON.parse(fs.readFileSync(STEAM_CONFIG_FILE, 'utf-8'));
      if (config && config.steamCmd && typeof config.steamCmd.path === 'string') {
        steamCmdPath = config.steamCmd.path;
      }
    } catch {
      // 壊れている場合は無視して再生成を試みる
    }
  }

  if (steamCmdPath) {
    try {
      await fs.promises.access(steamCmdPath);
      console.log(`[SteamCMD] 既にインストールされています: ${steamCmdPath}`);
      return { installed: true, path: steamCmdPath };
    } catch {
      // 設定ファイル上のパスに存在しない場合は続行
    }
  }

  // デフォルトのインストール先を確認
  const defaultCheck = await checkSteamCmdInstalled();
  if (defaultCheck.installed) {
    console.log(`[SteamCMD] 既にインストールされています: ${defaultCheck.path}`);
    return { installed: true, path: defaultCheck.path };
  }

  // インストールされていない場合は自動インストール
  console.log(
    `[SteamCMD] SteamCMDが見つかりません。自動インストールを開始します...`,
  );
  console.log(`[SteamCMD] インストール先: ${DEFAULT_STEAMCMD_INSTALL_DIR}`);

  const result = await installSteamCmd();

  if (!result.success) {
    console.warn(`[WARN] ${result.error}`);
    console.warn(
      '[WARN] SteamCMDは手動でインストールする必要があります（Resoniteのアップデート機能は使用できません）。',
    );
    return { installed: false, error: result.error };
  }

  // config/steam.json を作成／更新して、バックエンドからも認識できるようにする
  try {
    await fs.promises.mkdir(CONFIG_DIR, { recursive: true });

    let steamConfig = {};
    if (fs.existsSync(STEAM_CONFIG_FILE)) {
      try {
        steamConfig = JSON.parse(fs.readFileSync(STEAM_CONFIG_FILE, 'utf-8'));
      } catch {
        // 破損している場合はデフォルトから作り直す
        steamConfig = {};
      }
    }

    if (!steamConfig.steamCmd) {
      steamConfig.steamCmd = {};
    }
    steamConfig.steamCmd.path = result.path;
    steamConfig.steamCmd.autoDetect = true;

    if (!steamConfig.resonite) {
      steamConfig.resonite = {
        appId: '2519830',
        installDir:
          'C:/Program Files (x86)/Steam/steamapps/common/Resonite',
        autoDetectFromExecutable: true,
      };
    }

    if (!steamConfig.account) {
      steamConfig.account = {
        username: '',
        password: '',
        useSteamGuardFile: false,
        steamGuardFile: '',
      };
    }

    fs.writeFileSync(
      STEAM_CONFIG_FILE,
      JSON.stringify(steamConfig, null, 2),
      'utf-8',
    );
    console.log(
      `[SteamCMD] 設定ファイルを更新しました: ${path.relative(
        ROOT_DIR,
        STEAM_CONFIG_FILE,
      )}`,
    );
  } catch (error) {
    console.warn(
      `[WARN] SteamCMDの設定ファイル更新に失敗しました: ${
        error && error.message ? error.message : String(error)
      }`,
    );
  }

  return { installed: true, path: result.path };
};

/**
 * Steamアカウント資格情報を config/steam.json に保存（未設定の場合のみ）
 * フロントエンドからは設定せず、バックエンドPCの対話入力でのみ設定する想定
 */
const ensureSteamAccountConfigured = async () => {
  console.log('');
  console.log('--- Steamアカウント資格情報の確認中... ---');

  // 対話入力不可の場合はスキップ（手動で config/steam.json を編集してもらう）
  if (!process.stdin.isTTY) {
    if (!fs.existsSync(STEAM_CONFIG_FILE)) {
      console.warn(
        '[WARN] 対話的な入力が利用できません。Steam資格情報を設定するには config/steam.json を手動で作成・編集してください。',
      );
    }
    return;
  }

  let steamConfig = {};
  if (fs.existsSync(STEAM_CONFIG_FILE)) {
    try {
      steamConfig = JSON.parse(fs.readFileSync(STEAM_CONFIG_FILE, 'utf-8'));
    } catch {
      console.warn(
        '[WARN] config/steam.json の読み込みに失敗しました。デフォルト設定から再生成します。',
      );
      steamConfig = {};
    }
  }

  if (!steamConfig.account) {
    steamConfig.account = {
      username: '',
      password: '',
      useSteamGuardFile: false,
      steamGuardFile: '',
    };
  }

  const currentUsername =
    typeof steamConfig.account.username === 'string'
      ? steamConfig.account.username.trim()
      : '';
  const currentPassword =
    typeof steamConfig.account.password === 'string'
      ? steamConfig.account.password
      : '';

  if (currentUsername && currentPassword) {
    console.log('[Steam] 既にSteamアカウント資格情報が設定されています。');
    return;
  }

  const rl = readline.createInterface({ input, output });

  const askNonEmpty = async (question, { confirm = false, mask = false } = {}) => {
    while (true) {
      const value = mask
        ? await rl.question(question, { signal: undefined })
        : await rl.question(question);
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

  console.log('');
  console.log('--- Steamアカウント資格情報が未設定です。 ---');
  const username = await askNonEmpty('Steamのユーザー名を入力してください: ');
  const password = await askNonEmpty('Steamのパスワードを入力してください: ', {
    confirm: true,
    mask: true,
  });

  rl.close();
  console.log('');

  steamConfig.account.username = username;
  steamConfig.account.password = password;

  // 既存の構造がなければ最低限のデフォルトを補完
  if (!steamConfig.steamCmd) {
    steamConfig.steamCmd = {
      path: path.join(DEFAULT_STEAMCMD_INSTALL_DIR, 'steamcmd.exe'),
      autoDetect: true,
    };
  }
  if (!steamConfig.resonite) {
    steamConfig.resonite = {
      appId: '2519830',
      installDir: 'C:/Program Files (x86)/Steam/steamapps/common/Resonite',
      autoDetectFromExecutable: true,
    };
  }

  fs.mkdirSync(CONFIG_DIR, { recursive: true });
  fs.writeFileSync(STEAM_CONFIG_FILE, JSON.stringify(steamConfig, null, 2), 'utf-8');

  console.log(
    `[Steam] Steamアカウント資格情報を保存しました: ${path.relative(
      ROOT_DIR,
      STEAM_CONFIG_FILE,
    )}`,
  );
};

/**
 * 毎回起動時に Steam にログインを試行する
 * - 信頼済み状態であればそのまま成功
 * - Guard がリセットされている場合は、Guardコード入力を求めて再試行
 */
const ensureSteamLoggedIn = async () => {
  console.log('');
  console.log('--- Steamへのログインを確認中... ---');

  // 対話入力がない環境では、ここでのログイン確認はスキップ（バックエンドは通常どおり起動）
  if (!process.stdin.isTTY) {
    console.log(
      '[Steam] 対話的な入力が利用できないため、起動時のSteamログイン確認はスキップします。必要に応じて手動で steamcmd を実行してGuardコードを入力してください。',
    );
    return;
  }

  if (!fs.existsSync(STEAM_CONFIG_FILE)) {
    console.log(
      '[Steam] config/steam.json が存在しません。Steam資格情報を設定するには start.bat を対話的に実行し直してください。',
    );
    return;
  }

  let steamConfig;
  try {
    steamConfig = JSON.parse(fs.readFileSync(STEAM_CONFIG_FILE, 'utf-8'));
  } catch (error) {
    console.warn(
      `[WARN] config/steam.json の読み込みに失敗しました: ${
        error && error.message ? error.message : String(error)
      }`,
    );
    return;
  }

  const steamCmdPath = steamConfig?.steamCmd?.path;
  const username =
    typeof steamConfig?.account?.username === 'string'
      ? steamConfig.account.username.trim()
      : '';
  const password =
    typeof steamConfig?.account?.password === 'string'
      ? steamConfig.account.password
      : '';

  if (!steamCmdPath || !username || !password) {
    console.log(
      '[Steam] SteamCMDパスまたはアカウント資格情報が未設定のため、起動時ログインはスキップします。config/steam.json を確認してください。',
    );
    return;
  }

  const runLogin = (guardCode = '') =>
    new Promise((resolve) => {
      const args = ['+login', username, password];
      if (guardCode) {
        args.push(guardCode);
      }
      args.push('+quit');

      console.log(`[Steam] steamcmd を起動します: ${steamCmdPath}`);

      const child = spawn(steamCmdPath, args, {
        cwd: path.dirname(steamCmdPath),
        stdio: ['ignore', 'pipe', 'pipe'],
      });

      let stdout = '';
      let stderr = '';
      let sawGuardPrompt = false;

      const TIMEOUT_MS = 5 * 60 * 1000; // 5分
      const timeout = setTimeout(() => {
        console.warn(
          `[WARN] Steamログインが ${TIMEOUT_MS}ms 以内に完了しませんでした。プロセスを終了します。`,
        );
        try {
          child.kill();
        } catch {
          // ignore
        }
      }, TIMEOUT_MS);

      child.stdout.on('data', (data) => {
        const text = data.toString('utf-8');
        stdout += text;
        process.stdout.write(text);

        const lower = text.toLowerCase();
        if (
          lower.includes('steam guard') ||
          lower.includes('two-factor') ||
          lower.includes('two factor')
        ) {
          sawGuardPrompt = true;
        }
      });

      child.stderr.on('data', (data) => {
        const text = data.toString('utf-8');
        stderr += text;
        process.stderr.write(text);
      });

      child.on('close', (code) => {
        clearTimeout(timeout);
        const rawLog = stdout + stderr;

        if (code === 0) {
          resolve({ success: true, requiresGuard: false, rawLog });
        } else if (sawGuardPrompt) {
          resolve({ success: false, requiresGuard: true, rawLog });
        } else {
          resolve({ success: false, requiresGuard: false, rawLog });
        }
      });

      child.on('error', (error) => {
        clearTimeout(timeout);
        console.error('[Steam] steamcmd の起動に失敗しました:', error);
        resolve({ success: false, requiresGuard: false, rawLog: String(error) });
      });
    });

  // まずはGuardコードなしでログインを試行
  const first = await runLogin();
  if (first.success) {
    console.log('[Steam] Steamへのログインに成功しました（Guardコード不要）。');
    return;
  }

  if (!first.requiresGuard) {
    console.warn(
      '[WARN] Steamログインに失敗しました。ログを確認し、必要に応じて steamcmd を手動で実行して原因を調査してください。',
    );
    return;
  }

  // Guardコードが必要な場合は、ユーザーに入力してもらう
  const rl = readline.createInterface({ input, output });
  const guardCode = await rl.question('Steam Guard コードを入力してください（6桁）: ');
  rl.close();

  const trimmedCode = guardCode.trim();
  if (!trimmedCode) {
    console.warn('[WARN] Steam Guard コードが入力されませんでした。ログインはスキップされます。');
    return;
  }

  const second = await runLogin(trimmedCode);
  if (second.success) {
    console.log('[Steam] Steam Guard コードを使用してSteamへのログインに成功しました。');
  } else {
    console.warn(
      '[WARN] Steam Guard コードを使用したログインに失敗しました。コードが正しいか、時間切れでないかを確認してください。',
    );
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

  // SteamCMD のインストール確認と自動インストール
  await ensureSteamCmdInstalled();

  // Steamアカウント資格情報を設定（未設定の場合のみ対話で入力）
  await ensureSteamAccountConfigured();

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

      // 既存環境でも SteamCMD と Steamアカウント資格情報を確認
      await ensureSteamCmdInstalled();
      await ensureSteamAccountConfigured();
    }

    startApplication();
  } catch (error) {
    console.error('\n[ERROR] 処理中にエラーが発生しました:', error.message);
    waitForExit(1);
  }
};

void main();

