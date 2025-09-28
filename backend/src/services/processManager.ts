import { spawn, ChildProcessWithoutNullStreams } from 'node:child_process';
import path from 'node:path';
import fs from 'node:fs';
import { EventEmitter } from 'node:events';
import { HEADLESS_EXECUTABLE, HEADLESS_CONFIG_DIR, LOG_RING_BUFFER_SIZE } from '../config/index.js';
import { LogBuffer, LogEntry } from './logBuffer.js';

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
      this.emit('log', entry);
    });

    child.stderr.on('data', data => {
      const entry = this.logBuffer.push('stderr', data.toString());
      this.emit('log', entry);
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
    };

    child.on('close', handleExit);
    child.on('exit', handleExit);
    child.on('error', err => {
      const entry = this.logBuffer.push('stderr', `Process error: ${err.message}`);
      this.emit('log', entry);
    });
  }

  stop(): void {
    if (!this.child) {
      throw new Error('Headless process is not running');
    }
    this.child.kill();
  }
}

export const processManager = new ProcessManager();
