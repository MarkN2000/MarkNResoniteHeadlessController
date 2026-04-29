import { spawn } from 'node:child_process';
import type { ChildProcessWithoutNullStreams } from 'node:child_process';
import path from 'node:path';
import { promises as fs } from 'node:fs';
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

interface QueuedCommand {
  command: string;
  timeoutMs: number;
  options?: ExecuteCommandOptions;
  resolve: (value: LogEntry[]) => void;
  reject: (reason?: any) => void;
}

export class ProcessManager extends EventEmitter {
  private child?: ChildProcessWithoutNullStreams;
  private status: HeadlessStatus = { running: false };
  private readonly logBuffer = new LogBuffer(LOG_RING_BUFFER_SIZE);
  private stopPromise?: Promise<void>;
  private lastUserName?: string;
  private lastUserId?: string;
  private commandQueue: QueuedCommand[] = [];
  private isExecutingCommand = false;
  // P-5: lastUsedConfigPath のメモリキャッシュ
  private cachedLastConfigPath: string | null = null;

  constructor() {
    super();
  }

  /**
   * 非同期初期化（起動時にランタイム状態を読み込む）
   */
  async initialize(): Promise<void> {
    await this.loadRuntimeState();
  }

  /**
   * ランタイム状態をファイルから読み込む
   */
  private async loadRuntimeState(): Promise<void> {
    try {
      const content = await fs.readFile(RUNTIME_STATE_PATH, 'utf-8');
      const state: RuntimeState = JSON.parse(content);

      // 最後に起動したコンフィグパスをステータスに反映
      if (state.lastUsedConfigPath) {
        this.status.configPath = state.lastUsedConfigPath;
        this.cachedLastConfigPath = state.lastUsedConfigPath;
      }
    } catch (error: any) {
      if (error.code === 'ENOENT') {
        // ファイルが存在しない場合はデフォルト状態を作成
        console.log('[ProcessManager] Runtime state file not found, creating default runtime-state.json');
        await this.saveRuntimeState(null, 'stop');
      } else {
        console.error('[ProcessManager] Failed to load runtime state:', error);
        // エラー時はデフォルト状態を作成
        try {
          await this.saveRuntimeState(null, 'stop');
          console.log('[ProcessManager] Created default runtime state file');
        } catch (writeError) {
          console.error('[ProcessManager] Failed to create default runtime state file:', writeError);
        }
      }
    }
  }

  /**
   * ランタイム状態をファイルに保存する
   */
  private async saveRuntimeState(configPath: string | null, event: 'start' | 'stop'): Promise<void> {
    try {
      let state: RuntimeState;

      if (event === 'start') {
        state = {
          lastUsedConfigPath: configPath,
          lastUsedConfigName: configPath ? path.basename(configPath) : null,
          isRunning: true
        };
      } else {
        // stop時は既存のconfigPath/configNameを保持する
        state = {
          lastUsedConfigPath: this.cachedLastConfigPath ?? configPath,
          lastUsedConfigName: (this.cachedLastConfigPath ?? configPath)
            ? path.basename(this.cachedLastConfigPath ?? configPath ?? '')
            : null,
          isRunning: false
        };
      }

      // キャッシュを更新
      this.cachedLastConfigPath = state.lastUsedConfigPath;
      await fs.writeFile(RUNTIME_STATE_PATH, JSON.stringify(state, null, 2), 'utf-8');
    } catch (error) {
      console.error('[ProcessManager] Failed to save runtime state:', error);
    }
  }

  /**
   * 最後に起動したコンフィグファイルのパスを取得
   */
  getLastStartedConfigPath(): string | null {
    return this.cachedLastConfigPath;
  }

  getStatus(): HeadlessStatus {
    return this.status;
  }

  getLogs(limit?: number): LogEntry[] {
    return this.logBuffer.toArray(limit);
  }

  async listConfigs(): Promise<string[]> {
    try {
      const files = await fs.readdir(HEADLESS_CONFIG_DIR);
      return files
        .filter(file => file.toLowerCase().endsWith('.json'))
        .map(file => path.join(HEADLESS_CONFIG_DIR, file));
    } catch (error: any) {
      if (error.code === 'ENOENT') {
        return [];
      }
      throw error;
    }
  }

  async loadConfig(configPath: string): Promise<any> {
    const fullPath = path.isAbsolute(configPath) ? configPath : path.join(HEADLESS_CONFIG_DIR, configPath);
    const content = await fs.readFile(fullPath, 'utf-8');
    return JSON.parse(content);
  }

  async deleteConfig(configPath: string): Promise<void> {
    const fullPath = path.isAbsolute(configPath) ? configPath : path.join(HEADLESS_CONFIG_DIR, configPath);
    await fs.unlink(fullPath);
  }

  async generateConfig(name: string, username: string, password: string, configData: any, overwrite = false): Promise<string> {
    // 設定ディレクトリが存在しない場合は作成
    await fs.mkdir(HEADLESS_CONFIG_DIR, { recursive: true });

    const configPath = path.join(HEADLESS_CONFIG_DIR, `${name}.json`);

    // 既存のファイルがある場合の挙動
    if (!overwrite) {
      try {
        await fs.access(configPath);
        throw new Error(`Config file already exists: ${name}.json`);
      } catch (error: any) {
        if (error.code !== 'ENOENT') throw error;
        // ENOENT = ファイルが存在しない = OK
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

    await fs.writeFile(configPath, JSON.stringify(config, null, 2), 'utf8');
    return configPath;
  }

  async start(configPath?: string): Promise<void> {
    if (this.child) {
      throw new Error('Headless process already running');
    }

    const configs = await this.listConfigs();
    const resolvedConfig = configPath ?? configs[0];
    if (!resolvedConfig) {
      throw new Error('No headless config file found. Please place one under config/headless');
    }

    try {
      await fs.access(resolvedConfig);
    } catch {
      throw new Error(`Config file not found: ${resolvedConfig}`);
    }

    const args = ['-HeadlessConfig', resolvedConfig];
    const child = spawn(HEADLESS_EXECUTABLE, args, { cwd: path.dirname(HEADLESS_EXECUTABLE) });
    this.child = child;
    this.stopPromise = undefined as any;

    // EPIPE 等の stdin 書き込みエラーを握りつぶす（プロセス全体を落とさないため）。
    // 停止中の sendCommand 競合などで write が失敗しても、Unhandled 'error' event で
    // バックエンドが終了するのを防ぐ。
    child.stdin?.on('error', (err) => {
      const entry = this.logBuffer.push('stderr', `stdin error (suppressed): ${err.message}`);
      emitLog(this, entry);
    });

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
    this.emit('started');

    // ランタイム状態を保存（非同期、完了を待たない）
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

    let exitHandled = false;
    const handleExit = (code: number | null, signal: NodeJS.Signals | null) => {
      // B-1: 二重実行を防止
      if (exitHandled) return;
      exitHandled = true;

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
      this.emit('stopped');
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
    const stdin = this.child.stdin;
    // パイプが既に閉じている／閉じる途中の場合は書き込まない（EPIPE 防止）。
    // stop() が走り始めて stdin が閉じた後に並列の sendCommand が走るレース対策。
    if (!stdin.writable || stdin.destroyed) return;
    try {
      const payload = `${command}\n`;
      const encoded = iconv.encode(payload, 'shift_jis');
      stdin.write(encoded);
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
    // 停止中なら即時 reject。並列に残った再起動前アクションが新しいコマンドを
    // キューに積んで stdin 書き込みでクラッシュするのを防ぐ。
    if (this.stopPromise) {
      throw new Error('Headless process is stopping');
    }

    return new Promise((resolve, reject) => {
      // キューに追加
      this.commandQueue.push({
        command,
        timeoutMs,
        options,
        resolve,
        reject
      });

      // キューを処理開始（既に実行中でない場合）
      this.processQueue();
    });
  }

  /**
   * キューから順次コマンドを処理する
   */
  private processQueue(): void {
    // 既に実行中、またはキューが空の場合は何もしない
    if (this.isExecutingCommand || this.commandQueue.length === 0) {
      return;
    }

    // 次のコマンドを取得
    const queued = this.commandQueue.shift();
    if (!queued) {
      return;
    }

    this.isExecutingCommand = true;

    const { command, timeoutMs, options, resolve, reject } = queued;
    const collectorKey = this.logBuffer.nextId();
    const collector = this.logBuffer.createResponseCollector(collectorKey);

    const stopWhen = options?.stopWhen;
    const settleDurationMs = options?.settleDurationMs ?? 80;

    const handleExit = () => {
      cleanup();
      this.isExecutingCommand = false;
      reject(new Error('Headless process exited during command execution'));
      // キューに残っているコマンドをすべて拒否
      this.rejectAllQueuedCommands(new Error('Headless process exited'));
      // 次のコマンドを処理
      this.processQueue();
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
      this.isExecutingCommand = false;
      resolve(collector.collect());
      // 次のコマンドを処理
      this.processQueue();
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
  }

  /**
   * キューに残っているすべてのコマンドを拒否する
   */
  private rejectAllQueuedCommands(reason: Error): void {
    while (this.commandQueue.length > 0) {
      const queued = this.commandQueue.shift();
      if (queued) {
        queued.reject(reason);
      }
    }
  }

  stop(gracePeriodMs = 60000, killTimeoutMs = 70000): Promise<void> {
    const child = this.child;
    if (!child) {
      return Promise.reject(new Error('Headless process is not running'));
    }

    // キューに残っているコマンドをすべて拒否
    this.rejectAllQueuedCommands(new Error('Headless process is stopping'));
    this.isExecutingCommand = false;

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
      return iconv.decode(data, 'utf8');
    } catch (utfError) {
      try {
        return iconv.decode(data, 'shift_jis');
      } catch (sjisError) {
        return data.toString('utf8');
      }
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
