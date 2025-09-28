import { spawn } from 'node:child_process';
import type { ChildProcessWithoutNullStreams } from 'node:child_process';
import path from 'node:path';
import fs from 'node:fs';
import { EventEmitter } from 'node:events';
import { HEADLESS_EXECUTABLE, HEADLESS_CONFIG_DIR, LOG_RING_BUFFER_SIZE } from '../config/index.js';
import { LogBuffer } from './logBuffer.js';
import type { LogEntry } from './logBuffer.js';

export interface HeadlessStatus {
  running: boolean;
  pid?: number;
  configPath?: string;
  startedAt?: string;
  exitCode?: number | null;
  signal?: NodeJS.Signals | null;
}

export class ProcessManager extends EventEmitter {
  private child?: ChildProcessWithoutNullStreams;
  private status: HeadlessStatus = { running: false };
  private readonly logBuffer = new LogBuffer(LOG_RING_BUFFER_SIZE);
  private stopPromise?: Promise<void>;

  constructor() {
    super();
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
    this.stopPromise = undefined;

    this.status = {
      running: true,
      pid: child.pid ?? undefined,
      configPath: resolvedConfig,
      startedAt: new Date().toISOString(),
      exitCode: undefined,
      signal: undefined
    };
    this.emit('status', { ...this.status });

    child.stdout.on('data', data => {
      const entry = this.logBuffer.push('stdout', data.toString());
      emitLog(this, entry);
    });

    child.stderr.on('data', data => {
      const entry = this.logBuffer.push('stderr', data.toString());
      emitLog(this, entry);
    });

    const handleExit = (code: number | null, signal: NodeJS.Signals | null) => {
      this.child = undefined;
      this.status = {
        running: false,
        pid: undefined,
        configPath: resolvedConfig,
        exitCode: code,
        signal
      };
      this.emit('status', { ...this.status });
      this.stopPromise = undefined;
    };

    child.on('close', handleExit);
    child.on('exit', handleExit);
    child.on('error', err => {
      const entry = this.logBuffer.push('stderr', `Process error: ${err.message}`);
      emitLog(this, entry);
    });
  }

  private sendCommand(command: string): void {
    if (!this.child || !this.child.stdin) return;
    try {
      this.child.stdin.write(`${command}\n`);
      emitLog(this, this.logBuffer.push('stdout', `> ${command}`));
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      const entry = this.logBuffer.push('stderr', `Failed to write command: ${message}`);
      emitLog(this, entry);
    }
  }

  stop(gracePeriodMs = 10000, killTimeoutMs = 15000): Promise<void> {
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
        this.stopPromise = undefined;
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
}

export const processManager = new ProcessManager();

const emitLog = (manager: ProcessManager, entry: LogEntry) => {
  manager.emit('log', entry);
};
