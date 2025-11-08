import { spawn } from 'node:child_process';
import type { ChildProcessWithoutNullStreams } from 'node:child_process';
import path from 'node:path';
import fs from 'node:fs';
import { EventEmitter } from 'node:events';
import { HEADLESS_EXECUTABLE, HEADLESS_CONFIG_DIR, LOG_RING_BUFFER_SIZE, RUNTIME_STATE_PATH } from '../config/index.js';
import { getHeadlessCredentials } from '../config/headlessCredentials.js';
import { LogBuffer } from './logBuffer.js';
import type { LogEntry } from './logBuffer.js';
import iconv from 'iconv-lite';

export interface HeadlessStatus {
  running: boolean;
  pid?: number;
  configPath?: string;
  startedAt?: string;
  exitCode?: number | null;
  signal?: NodeJS.Signals | null;
  userName?: string;
  userId?: string;
}

interface RuntimeState {
  lastUsedConfigPath: string | null;
  lastUsedConfigName: string | null;
  isRunning: boolean;
}

export interface ExecuteCommandOptions {
  stopWhen?: (entry: LogEntry) => boolean;
  settleDurationMs?: number;
}

export class ProcessManager extends EventEmitter {
  private child?: ChildProcessWithoutNullStreams;
  private status: HeadlessStatus = { running: false };
  private readonly logBuffer = new LogBuffer(LOG_RING_BUFFER_SIZE);
  private stopPromise?: Promise<void>;
  private lastUserName?: string;
  private lastUserId?: string;

  constructor() {
    super();
    // 起動時にランタイム状態を読み込む
    this.loadRuntimeState();
  }

  /**
   * ランタイム状態をファイルから読み込む
   */
  private loadRuntimeState(): void {
    try {
      if (fs.existsSync(RUNTIME_STATE_PATH)) {
        const content = fs.readFileSync(RUNTIME_STATE_PATH, 'utf-8');
        const state: RuntimeState = JSON.parse(content);
        
        // 最後に起動したコンフィグパスをステータスに反映
        if (state.lastUsedConfigPath) {
          this.status.configPath = state.lastUsedConfigPath;
        }
      } else {
        // ファイルが存在しない場合はデフォルト状態を作成
        console.log('[ProcessManager] Runtime state file not found, creating default runtime-state.json');
        const defaultState: RuntimeState = {
          lastUsedConfigPath: null,
          lastUsedConfigName: null,
          isRunning: false
        };
        this.saveRuntimeState(null, 'stop');
      }
    } catch (error) {
      console.error('[ProcessManager] Failed to load runtime state:', error);
      // エラー時はデフォルト状態を作成
      const defaultState: RuntimeState = {
        lastUsedConfigPath: null,
        lastUsedConfigName: null,
        isRunning: false
      };
      try {
        fs.writeFileSync(RUNTIME_STATE_PATH, JSON.stringify(defaultState, null, 2), 'utf-8');
        console.log('[ProcessManager] Created default runtime state file');
      } catch (writeError) {
        console.error('[ProcessManager] Failed to create default runtime state file:', writeError);
      }
    }
  }

  /**
   * ランタイム状態をファイルに保存する
   */
  private saveRuntimeState(configPath: string | null, event: 'start' | 'stop'): void {
    try {
      const state: RuntimeState = {
        lastUsedConfigPath: configPath,
        lastUsedConfigName: configPath ? path.basename(configPath) : null,
        isRunning: event === 'start'
      };

      // 既存の状態を読み込んで更新
      if (fs.existsSync(RUNTIME_STATE_PATH)) {
        const content = fs.readFileSync(RUNTIME_STATE_PATH, 'utf-8');
        const existingState: RuntimeState = JSON.parse(content);
        
        if (event === 'start') {
          state.lastUsedConfigPath = configPath;
          state.lastUsedConfigName = configPath ? path.basename(configPath) : null;
          state.isRunning = true;
        } else {
          state.lastUsedConfigPath = existingState.lastUsedConfigPath;
          state.lastUsedConfigName = existingState.lastUsedConfigName;
          state.isRunning = false;
        }
      }

      fs.writeFileSync(RUNTIME_STATE_PATH, JSON.stringify(state, null, 2), 'utf-8');
    } catch (error) {
      console.error('[ProcessManager] Failed to save runtime state:', error);
    }
  }

  /**
   * 最後に起動したコンフィグファイルのパスを取得
   */
  getLastStartedConfigPath(): string | null {
    try {
      if (fs.existsSync(RUNTIME_STATE_PATH)) {
        const content = fs.readFileSync(RUNTIME_STATE_PATH, 'utf-8');
        const state: RuntimeState = JSON.parse(content);
        return state.lastUsedConfigPath;
      }
    } catch (error) {
      console.error('[ProcessManager] Failed to get last started config path:', error);
    }
    return null;
  }

  getStatus(): HeadlessStatus {
    return this.status;
  }

  getLogs(limit?: number): LogEntry[] {
    return this.logBuffer.toArray(limit);
  }

  listConfigs(): string[] {
    if (!fs.existsSync(HEADLESS_CONFIG_DIR)) {
      return [];
    }
    return fs
      .readdirSync(HEADLESS_CONFIG_DIR)
      .filter(file => file.toLowerCase().endsWith('.json'))
      .map(file => path.join(HEADLESS_CONFIG_DIR, file));
  }

  async loadConfig(configPath: string): Promise<any> {
    const fullPath = path.isAbsolute(configPath) ? configPath : path.join(HEADLESS_CONFIG_DIR, configPath);
    
    if (!fs.existsSync(fullPath)) {
      throw new Error(`Config file not found: ${configPath}`);
    }

    const content = fs.readFileSync(fullPath, 'utf-8');
    return JSON.parse(content);
  }

  async deleteConfig(configPath: string): Promise<void> {
    const fullPath = path.isAbsolute(configPath) ? configPath : path.join(HEADLESS_CONFIG_DIR, configPath);
    
    if (!fs.existsSync(fullPath)) {
      throw new Error(`Config file not found: ${configPath}`);
    }

    fs.unlinkSync(fullPath);
  }

  async generateConfig(name: string, username: string, password: string, configData: any, overwrite = false): Promise<string> {
    // 設定ディレクトリが存在しない場合は作成
    if (!fs.existsSync(HEADLESS_CONFIG_DIR)) {
      fs.mkdirSync(HEADLESS_CONFIG_DIR, { recursive: true });
    }

    const configPath = path.join(HEADLESS_CONFIG_DIR, `${name}.json`);
    
    // 既存のファイルがある場合の挙動
    if (fs.existsSync(configPath)) {
      if (!overwrite) {
        throw new Error(`Config file already exists: ${name}.json`);
      }
    }

    let resolvedUsername = typeof username === 'string' ? username.trim() : '';
    let resolvedPassword = typeof password === 'string' ? password : '';

    if (!resolvedUsername || !resolvedPassword) {
      const defaults = getHeadlessCredentials();
      if (defaults) {
        if (!resolvedUsername && defaults.username) {
          resolvedUsername = defaults.username;
        }
        if (!resolvedPassword && defaults.password) {
          resolvedPassword = defaults.password;
        }
      }
    }

    if (!resolvedUsername || !resolvedPassword) {
      console.warn(
        `[ProcessManager] ユーザー名またはパスワードが空です。config/auth.json の headlessCredentials を設定するか、明示的に指定してください。`
      );
    }

    // 設定ファイルを生成
    const config = {
      "$schema": "https://raw.githubusercontent.com/Yellow-Dog-Man/JSONSchemas/main/schemas/HeadlessConfig.schema.json",
      "comment": configData.comment || `${name}の設定ファイル`,
      "universeId": configData.universeId || null,
      "tickRate": configData.tickRate || 60.0,
      "maxConcurrentAssetTransfers": configData.maxConcurrentAssetTransfers || 128,
      // 仕様: デフォルトは null。入力がなければ null を採用
      "usernameOverride": typeof configData.usernameOverride === 'string' && configData.usernameOverride.trim() !== ''
        ? configData.usernameOverride
        : null,
      "loginCredential": resolvedUsername ?? '',
      "loginPassword": resolvedPassword ?? '',
      "startWorlds": configData.startWorlds || [],
      "dataFolder": configData.dataFolder || null,
      "cacheFolder": configData.cacheFolder || null,
      "logsFolder": configData.logsFolder || null,
      "allowedUrlHosts": configData.allowedUrlHosts || null,
      "autoSpawnItems": configData.autoSpawnItems || null
    };

    fs.writeFileSync(configPath, JSON.stringify(config, null, 2), 'utf8');
    return configPath;
  }

  start(configPath?: string): void {
    if (this.child) {
      throw new Error('Headless process already running');
    }

    const configs = this.listConfigs();
    const resolvedConfig = configPath ?? configs[0];
    if (!resolvedConfig) {
      throw new Error('No headless config file found. Please place one under config/headless');
    }

    if (!fs.existsSync(resolvedConfig)) {
      throw new Error(`Config file not found: ${resolvedConfig}`);
    }

    const args = ['-HeadlessConfig', resolvedConfig];
    const child = spawn(HEADLESS_EXECUTABLE, args, { cwd: path.dirname(HEADLESS_EXECUTABLE) });
    this.child = child;
    this.stopPromise = undefined as any;

    this.status = {
      running: true,
      pid: child.pid as number | undefined,
      configPath: resolvedConfig,
      startedAt: new Date().toISOString(),
      exitCode: undefined as any,
      signal: undefined as any,
      userName: this.lastUserName,
      userId: this.lastUserId
    };
    this.emit('status', { ...this.status });

    // ランタイム状態を保存
    this.saveRuntimeState(resolvedConfig, 'start');

    child.stdout.on('data', data => {
      const message = this.decodeBuffer(data as Buffer);
      this.updateAccountFromLog(message);
      const entry = this.logBuffer.push('stdout', message);
      emitLog(this, entry);
    });

    child.stderr.on('data', data => {
      const message = this.decodeBuffer(data as Buffer);
      const entry = this.logBuffer.push('stderr', message);
      emitLog(this, entry);
    });

    const handleExit = (code: number | null, signal: NodeJS.Signals | null) => {
      this.child = undefined as any;
      this.status = {
        running: false,
        pid: undefined as any,
        configPath: resolvedConfig,
        exitCode: code,
        signal,
        userName: this.lastUserName,
        userId: this.lastUserId
      };
      this.emit('status', { ...this.status });
      this.stopPromise = undefined as any;
      
      // ランタイム状態を保存
      this.saveRuntimeState(resolvedConfig, 'stop');
    };

    child.on('close', handleExit);
    child.on('exit', handleExit);
    child.on('error', err => {
      const entry = this.logBuffer.push('stderr', `Process error: ${err.message}`);
      emitLog(this, entry);
    });
  }

  public sendCommand(command: string): void {
    if (!this.child || !this.child.stdin) return;
    try {
      const payload = `${command}\n`;
      const encoded = iconv.encode(payload, 'shift_jis');
      this.child.stdin.write(encoded);
      emitLog(this, this.logBuffer.push('stdout', `> ${command}`));
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      const entry = this.logBuffer.push('stderr', `Failed to write command: ${message}`);
      emitLog(this, entry);
    }
  }

  async executeCommand(command: string, timeoutMs = 3000, options?: ExecuteCommandOptions): Promise<LogEntry[]> {
    if (!this.child) {
      throw new Error('Headless process is not running');
    }

    const collectorKey = this.logBuffer.nextId();
    const collector = this.logBuffer.createResponseCollector(collectorKey);

    const stopWhen = options?.stopWhen;
    const settleDurationMs = options?.settleDurationMs ?? 80;

    return new Promise((resolve, reject) => {
      const handleExit = () => {
        cleanup();
        reject(new Error('Headless process exited during command execution'));
      };

      const cleanup = () => {
        clearTimeout(timer);
        if (settleTimer) clearTimeout(settleTimer);
        this.off('status', handleExit);
        this.off('log', handleLog);
        collector.dispose();
      };

      const finalize = () => {
        cleanup();
        resolve(collector.collect());
      };

      const timer = setTimeout(() => {
        finalize();
      }, timeoutMs);

      let settleTimer: NodeJS.Timeout | null = null;

      const handleLog = (entry: LogEntry) => {
        if (entry.id < collectorKey) return;
        if (!stopWhen) return;
        if (!stopWhen(entry)) return;
        if (settleTimer) return;
        settleTimer = setTimeout(finalize, settleDurationMs);
      };

      this.on('status', handleExit);
      if (stopWhen) {
        this.on('log', handleLog);
      }
      this.sendCommand(command);
    });
  }

  stop(gracePeriodMs = 20000, killTimeoutMs = 25000): Promise<void> {
    const child = this.child;
    if (!child) {
      return Promise.reject(new Error('Headless process is not running'));
    }

    if (this.stopPromise) {
      return this.stopPromise;
    }

    this.stopPromise = new Promise((resolve, reject) => {
      const cleanup = () => {
        clearTimeout(forceTimer);
        clearTimeout(killTimer);
        child.off('exit', handleExit);
        child.off('error', handleError);
        this.stopPromise = undefined as any;
      };

      const handleExit = () => {
        cleanup();
        resolve();
      };

      const handleError = (error: Error) => {
        this.logBuffer.push('stderr', `Process error during shutdown: ${error.message}`);
        cleanup();
        reject(error);
      };

      child.once('exit', handleExit);
      child.once('error', handleError);

      this.sendCommand('shutdown');

      const forceTimer = setTimeout(() => {
        if (child.killed) return;
        emitLog(this, this.logBuffer.push('stderr', 'Graceful shutdown timed out, sending SIGTERM'));
        child.kill('SIGTERM');
      }, gracePeriodMs);

      const killTimer = setTimeout(() => {
        if (child.killed) return;
        emitLog(this, this.logBuffer.push('stderr', 'Process unresponsive, forcing termination'));
        child.kill('SIGKILL');
      }, killTimeoutMs);
    });

    return this.stopPromise;
  }

  private decodeBuffer(data: Buffer): string {
    try {
      return iconv.decode(data, 'shift_jis');
    } catch (error) {
      return data.toString('utf8');
    }
  }

  private updateAccountFromLog(message: string): void {
    const loginMatch = message.match(/Logging in as\s+(?<user>[^\s]+)/i);
    if (loginMatch?.groups?.user) {
      const user = loginMatch.groups.user.trim();
      if (user) {
        let updated = false;
        if (user !== this.lastUserName) {
          this.lastUserName = user;
          updated = true;
        }
        if (updated) this.emitStatusUpdate();
      }
      return;
    }

    const userIdMatch = message.match(/Initializing SignalR: UserLogin:\s*(?<id>U-[A-Za-z0-9]+)/i);
    if (userIdMatch?.groups?.id) {
      const id = userIdMatch.groups.id.trim();
      if (id && id !== this.lastUserId) {
        this.lastUserId = id;
        this.emitStatusUpdate();
      }
    }
  }

  private emitStatusUpdate(): void {
    this.status = { 
      ...this.status, 
      userName: this.lastUserName as string | undefined, 
      userId: this.lastUserId as string | undefined 
    };
    this.emit('status', { ...this.status });
  }
}

export const processManager = new ProcessManager();

const emitLog = (manager: ProcessManager, entry: LogEntry) => {
  manager.emit('log', entry);
};
