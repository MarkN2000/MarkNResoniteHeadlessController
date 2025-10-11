import { EventEmitter } from 'node:events';
import { promises as fs } from 'fs';
import path from 'path';
import type { RestartConfig, RestartStatus } from '../../../shared/src/index.js';
import { loadRestartConfig } from './restartConfig.js';
import { ProcessManager } from './processManager.js';
import { ScheduledRestartWatcher } from './scheduledRestartWatcher.js';
import { HighLoadWatcher } from './highLoadWatcher.js';
import { UserZeroWatcher } from './userZeroWatcher.js';

const STATUS_FILE = path.join(process.cwd(), 'config', 'restart-status.json');

// 高負荷トリガーの無効化期間（ミリ秒）
const HIGH_LOAD_COOLDOWN_MS = 30 * 60 * 1000; // 30分

/**
 * 再起動マネージャー
 * 自動再起動機能の中核を担うサービス
 */
export class RestartManager extends EventEmitter {
  private config: RestartConfig | null = null;
  private status: RestartStatus;
  private processManager: ProcessManager;
  private restartInProgress = false;
  private waitingForUsers = false;
  private serverStartTime: Date | null = null;
  
  // タイマー管理
  private waitTimer: NodeJS.Timeout | null = null;
  private forceRestartTimer: NodeJS.Timeout | null = null;
  private actionTimer: NodeJS.Timeout | null = null;
  private userCheckInterval: NodeJS.Timeout | null = null;
  
  // 待機制御用フラグ
  private actionsExecuted = false;
  private zeroUserWaitStartTime: number | null = null;
  
  // トリガー監視
  private scheduledWatcher: ScheduledRestartWatcher;
  private highLoadWatcher: HighLoadWatcher;
  private userZeroWatcher: UserZeroWatcher;

  constructor(processManager: ProcessManager) {
    super();
    this.processManager = processManager;
    this.scheduledWatcher = new ScheduledRestartWatcher();
    this.highLoadWatcher = new HighLoadWatcher();
    this.userZeroWatcher = new UserZeroWatcher();
    
    // 初期ステータス
    this.status = {
      nextScheduledRestart: {
        scheduleId: null,
        datetime: null,
        configFile: null
      },
      currentUptime: 0,
      lastRestart: {
        timestamp: null,
        trigger: null
      },
      highLoadTriggerDisabledUntil: null,
      restartInProgress: false,
      waitingForUsers: false
    };
    
    // イベントリスナー設定
    this.setupEventListeners();
  }

  /**
   * 初期化
   */
  async initialize(): Promise<void> {
    try {
      // 設定を読み込み
      this.config = await loadRestartConfig();
      
      // ステータスファイルを読み込み
      await this.loadStatus();
      
      // トリガー監視を開始
      this.startTriggerWatchers();
      
      console.log('[RestartManager] Initialized successfully');
    } catch (error) {
      console.error('[RestartManager] Initialization failed:', error);
      throw error;
    }
  }

  /**
   * トリガー監視を開始
   */
  private startTriggerWatchers(): void {
    if (!this.config) return;
    
    // 予定再起動の監視を開始
    this.scheduledWatcher.start(
      this.config.triggers.scheduled.schedules,
      this.config.triggers.scheduled.enabled
    );
    
    // 高負荷監視を開始
    this.highLoadWatcher.start(
      this.config.triggers.highLoad.enabled,
      this.config.triggers.highLoad.cpuThreshold,
      this.config.triggers.highLoad.memoryThreshold,
      this.config.triggers.highLoad.durationMinutes,
      this.status.highLoadTriggerDisabledUntil
    );
    
    // ユーザー0監視を有効化
    this.userZeroWatcher.enable(
      this.config.triggers.userZero.enabled,
      this.config.triggers.userZero.minUptimeMinutes
    );
    
    // 次回の予定再起動を更新
    this.updateNextScheduledRestart();
  }

  /**
   * イベントリスナーの設定
   */
  private setupEventListeners(): void {
    // プロセス開始イベント
    this.processManager.on('started', () => {
      this.serverStartTime = new Date();
      
      // 高負荷トリガーを30分間無効化
      const disabledUntil = new Date(Date.now() + HIGH_LOAD_COOLDOWN_MS);
      this.status.highLoadTriggerDisabledUntil = disabledUntil.toISOString();
      
      // ユーザー0監視にサーバー起動時刻を設定
      this.userZeroWatcher.setServerStartTime(this.serverStartTime);
      
      this.saveStatus();
      
      console.log('[RestartManager] Server started, high load trigger disabled until', disabledUntil.toISOString());
    });
    
    // プロセス停止イベント
    this.processManager.on('stopped', () => {
      this.serverStartTime = null;
      
      // 待機中のタイマーをクリア
      this.clearAllTimers();
      this.waitingForUsers = false;
      this.status.waitingForUsers = false;
      
      // 監視を停止
      this.scheduledWatcher.stop();
      this.highLoadWatcher.stop();
      this.userZeroWatcher.setServerStartTime(null);
      
      this.saveStatus();
      
      console.log('[RestartManager] Server stopped, all timers cleared');
    });
    
    // 予定再起動トリガーイベント
    this.scheduledWatcher.on('trigger', (scheduleId: string, configFile: string) => {
      console.log(`[RestartManager] Scheduled restart triggered: ${scheduleId}, config: ${configFile}`);
      
      // コンフィグファイルを更新してから再起動
      this.updateLastUsedConfigAndRestart('scheduled', scheduleId, configFile);
    });
    
    // 高負荷トリガーイベント
    this.highLoadWatcher.on('trigger', () => {
      console.log('[RestartManager] High load restart triggered');
      
      // 最後に使用したコンフィグで再起動
      this.triggerRestart('highLoad').catch((error) => {
        console.error('[RestartManager] High load restart failed:', error);
      });
    });
    
    // ユーザー0トリガーイベント
    this.userZeroWatcher.on('trigger', () => {
      console.log('[RestartManager] User zero restart triggered');
      
      // 最後に使用したコンフィグで再起動
      this.triggerRestart('userZero').catch((error) => {
        console.error('[RestartManager] User zero restart failed:', error);
      });
    });
  }

  /**
   * 設定を再読み込み
   */
  async reloadConfig(): Promise<void> {
    try {
      this.config = await loadRestartConfig();
      
      // トリガー監視を再開
      this.startTriggerWatchers();
      
      console.log('[RestartManager] Config reloaded');
    } catch (error) {
      console.error('[RestartManager] Failed to reload config:', error);
      throw error;
    }
  }

  /**
   * 現在の設定を取得
   */
  getConfig(): RestartConfig | null {
    return this.config;
  }

  /**
   * 現在のステータスを取得
   */
  getStatus(): RestartStatus {
    // 稼働時間を更新
    if (this.serverStartTime) {
      const uptime = Math.floor((Date.now() - this.serverStartTime.getTime()) / 1000);
      this.status.currentUptime = uptime;
    } else {
      this.status.currentUptime = 0;
    }
    
    return { ...this.status };
  }

  /**
   * ステータスをファイルから読み込み
   */
  private async loadStatus(): Promise<void> {
    try {
      const data = await fs.readFile(STATUS_FILE, 'utf-8');
      const savedStatus = JSON.parse(data) as RestartStatus;
      
      // 一部のフィールドは保持
      this.status.lastRestart = savedStatus.lastRestart;
      this.status.highLoadTriggerDisabledUntil = savedStatus.highLoadTriggerDisabledUntil;
      
      console.log('[RestartManager] Status loaded from file');
    } catch (error: any) {
      if (error.code === 'ENOENT') {
        // ファイルが存在しない場合は新規作成
        await this.saveStatus();
      } else {
        console.error('[RestartManager] Failed to load status:', error);
      }
    }
  }

  /**
   * ステータスをファイルに保存
   */
  private async saveStatus(): Promise<void> {
    try {
      await fs.mkdir(path.dirname(STATUS_FILE), { recursive: true });
      await fs.writeFile(STATUS_FILE, JSON.stringify(this.status, null, 2), 'utf-8');
    } catch (error) {
      console.error('[RestartManager] Failed to save status:', error);
    }
  }

  /**
   * 再起動をトリガー（優先度制御付き）
   */
  async triggerRestart(
    trigger: 'scheduled' | 'highLoad' | 'userZero' | 'manual' | 'forced',
    scheduleId?: string
  ): Promise<void> {
    // 既に再起動処理中の場合はスキップ
    if (this.restartInProgress) {
      console.log('[RestartManager] Restart already in progress, skipping trigger:', trigger);
      return;
    }
    
    // サーバーが停止中の場合はスキップ
    if (!this.processManager.getStatus().running) {
      console.log('[RestartManager] Server not running, skipping trigger:', trigger);
      return;
    }
    
    this.restartInProgress = true;
    this.status.restartInProgress = true;
    
    console.log(`[RestartManager] Restart triggered by: ${trigger}${scheduleId ? ` (schedule #${scheduleId})` : ''}`);
    
    try {
      // 強制再起動の場合は待機制御をスキップして即座に再起動
      if (trigger === 'forced') {
        console.log('[RestartManager] Forced restart: Skipping wait control and actions');
        await this.executeRestart(trigger, scheduleId);
      } else {
        // その他のトリガーは待機制御とアクションを実行
        await this.executePreRestartActions();
        await this.executeRestart(trigger, scheduleId);
      }
      
    } catch (error) {
      console.error('[RestartManager] Restart failed:', error);
      throw error;
    } finally {
      this.restartInProgress = false;
      this.status.restartInProgress = false;
      this.saveStatus();
    }
  }

  /**
   * 再起動前アクションを実行（待機制御付き）
   */
  private async executePreRestartActions(): Promise<void> {
    if (!this.config) return;
    
    const { preRestartActions } = this.config;
    const { waitControl } = preRestartActions;
    
    console.log('[RestartManager] Starting pre-restart wait control...');
    console.log(`[RestartManager] - Wait for zero users: ${waitControl.waitForZeroUsers} minutes`);
    console.log(`[RestartManager] - Force restart timeout: ${waitControl.forceRestartTimeout} minutes`);
    console.log(`[RestartManager] - Action timing: ${waitControl.actionTiming} minutes before restart`);
    
    // フラグをリセット
    this.actionsExecuted = false;
    this.zeroUserWaitStartTime = null;
    this.waitingForUsers = true;
    this.status.waitingForUsers = true;
    
    const forceRestartTime = waitControl.forceRestartTimeout * 60 * 1000; // 分→ミリ秒
    const actionTime = waitControl.actionTiming * 60 * 1000; // 分→ミリ秒
    const actionDelay = forceRestartTime - actionTime; // アクション実行までの待機時間
    const zeroUserWaitTime = waitControl.waitForZeroUsers * 60 * 1000; // ユーザー0人待機時間
    
    const startTime = Date.now();
    
    // 待機制御のPromise
    await new Promise<void>((resolve) => {
      // 1. 強制再起動タイマー（最終的な強制再起動）
      this.forceRestartTimer = setTimeout(async () => {
        console.log('[RestartManager] Force restart timeout reached');
        
        // アクションがまだ実行されていない場合は実行
        if (!this.actionsExecuted) {
          console.log('[RestartManager] Executing actions before forced restart');
          await this.executeActions();
          this.actionsExecuted = true;
        }
        
        resolve();
      }, forceRestartTime);
      
      // 2. アクション実行タイマー（強制再起動のX分前）
      if (actionDelay > 0) {
        this.actionTimer = setTimeout(async () => {
          if (!this.actionsExecuted) {
            console.log(`[RestartManager] Action timing reached (${waitControl.actionTiming} minutes before forced restart)`);
            await this.executeActions();
            this.actionsExecuted = true;
          }
        }, actionDelay);
      }
      
      // 3. ユーザー数チェックインターバル（1分ごと）
      const USER_CHECK_INTERVAL = 60 * 1000; // 1分
      
      this.userCheckInterval = setInterval(async () => {
        const elapsedMinutes = Math.floor((Date.now() - startTime) / 60000);
        console.log(`[RestartManager] User check (elapsed: ${elapsedMinutes} minutes)...`);
        
        const userCount = await this.getTotalUserCount();
        
        if (userCount < 0) {
          console.log('[RestartManager] Failed to get user count, skipping this check');
          return;
        }
        
        console.log(`[RestartManager] Current user count: ${userCount}`);
        
        if (userCount === 0) {
          // ユーザーが0人の場合
          if (this.zeroUserWaitStartTime === null) {
            // 0人待機を開始
            this.zeroUserWaitStartTime = Date.now();
            console.log(`[RestartManager] Users reached zero. Waiting ${waitControl.waitForZeroUsers} minutes...`);
          } else {
            // 0人待機中
            const zeroWaitElapsed = Date.now() - this.zeroUserWaitStartTime;
            const zeroWaitRemaining = zeroUserWaitTime - zeroWaitElapsed;
            
            console.log(`[RestartManager] Zero user wait: ${Math.floor(zeroWaitRemaining / 60000)} minutes remaining`);
            
            if (zeroWaitElapsed >= zeroUserWaitTime) {
              // 待機時間が経過したので再起動
              console.log('[RestartManager] Zero user wait completed. Proceeding to restart.');
              
              // アクションがまだ実行されていない場合は実行
              if (!this.actionsExecuted) {
                console.log('[RestartManager] Executing actions before restart');
                await this.executeActions();
                this.actionsExecuted = true;
              }
              
              // タイマーをクリアしてresolve
              if (this.forceRestartTimer) clearTimeout(this.forceRestartTimer);
              if (this.actionTimer) clearTimeout(this.actionTimer);
              if (this.userCheckInterval) clearInterval(this.userCheckInterval);
              
              resolve();
            }
          }
        } else {
          // ユーザーがいる場合は0人待機をリセット
          if (this.zeroUserWaitStartTime !== null) {
            console.log('[RestartManager] Users joined. Resetting zero user wait.');
            this.zeroUserWaitStartTime = null;
          }
        }
      }, USER_CHECK_INTERVAL);
      
      // 初回チェックを即座に実行
      (async () => {
        const userCount = await this.getTotalUserCount();
        if (userCount >= 0) {
          console.log(`[RestartManager] Initial user count: ${userCount}`);
          if (userCount === 0) {
            this.zeroUserWaitStartTime = Date.now();
            console.log(`[RestartManager] Users already zero. Waiting ${waitControl.waitForZeroUsers} minutes...`);
          }
        }
      })();
    });
    
    // タイマーをクリア
    this.clearAllTimers();
    this.waitingForUsers = false;
    this.status.waitingForUsers = false;
    
    console.log('[RestartManager] Pre-restart wait control completed');
  }

  /**
   * 各種アクションを実行
   */
  private async executeActions(): Promise<void> {
    if (!this.config) return;
    
    const { preRestartActions } = this.config;
    
    console.log('[RestartManager] Executing pre-restart actions...');
    
    try {
      // worldsコマンドを1回だけ実行して全アクションで共有
      console.log('[RestartManager] Fetching active worlds...');
      const worldsOutput = await this.processManager.sendCommand('worlds');
      const worlds = this.parseWorldsOutput(worldsOutput);
      
      if (worlds.length === 0) {
        console.log('[RestartManager] No active worlds found. Skipping actions.');
        return;
      }
      
      console.log(`[RestartManager] Found ${worlds.length} active world(s)`);
      
      // チャットメッセージ送信
      if (preRestartActions.chatMessage.enabled) {
        await this.sendChatMessage(preRestartActions.chatMessage.message, worlds);
      }
      
      // アイテムスポーン
      if (preRestartActions.itemSpawn.enabled) {
        await this.spawnWarningItem(
          preRestartActions.itemSpawn.itemType,
          preRestartActions.itemSpawn.message,
          worlds
        );
      }
      
      // セッション設定変更
      await this.applySessionChanges(preRestartActions.sessionChanges, worlds);
      
    } catch (error) {
      console.error('[RestartManager] Failed to execute some actions:', error);
      // エラーが発生しても再起動は続行
    }
  }

  /**
   * チャットメッセージを全セッションのAFKではないユーザーに送信
   */
  private async sendChatMessage(message: string, worlds: Array<{ index: number; name: string; users: number; present: number }>): Promise<void> {
    console.log('[RestartManager] Sending chat message to all active users:', message);
    
    try {
      // 各セッションに対して処理
      for (const world of worlds) {
        try {
          // セッションにフォーカス
          await this.processManager.sendCommand(`focus ${world.index}`);
          
          // そのセッションのユーザー一覧を取得
          const usersOutput = await this.processManager.sendCommand('users');
          const users = this.parseUsersOutput(usersOutput);
          
          // AFKではないユーザーにメッセージを送信
          for (const user of users) {
            if (!user.present) {
              // AFKユーザーはスキップ
              console.log(`[RestartManager] Skipping AFK user: ${user.username}`);
              continue;
            }
            
            // アクティブなユーザーにメッセージ送信
            await this.processManager.sendCommand(`message "${user.username}" ${message}`);
            console.log(`[RestartManager] Sent message to ${user.username} in ${world.name}`);
          }
        } catch (error) {
          console.error(`[RestartManager] Failed to send message to world ${world.name}:`, error);
          // エラーが発生しても次のセッションに進む
        }
      }
      
      console.log('[RestartManager] Chat message sent to all active users');
      
    } catch (error) {
      console.error('[RestartManager] Failed to send chat message:', error);
    }
  }

  /**
   * 全セッションの総ユーザー数を取得（Present人数の合計）
   */
  private async getTotalUserCount(): Promise<number> {
    try {
      const worldsOutput = await this.processManager.sendCommand('worlds');
      const worlds = this.parseWorldsOutput(worldsOutput);
      
      // Present（実際にいる）ユーザーの合計を計算
      const totalUsers = worlds.reduce((sum, world) => sum + world.present, 0);
      
      console.log(`[RestartManager] Total present users: ${totalUsers}`);
      
      return totalUsers;
    } catch (error) {
      console.error('[RestartManager] Failed to get total user count:', error);
      return -1; // エラーの場合は-1を返す
    }
  }

  /**
   * worldsコマンドの出力をパース
   * 例: [0] MarkN_headless_test             Users: 1    Present: 0      AccessLevel: LAN        MaxUsers: 16
   */
  private parseWorldsOutput(output: string): Array<{ index: number; name: string; users: number; present: number }> {
    const worlds: Array<{ index: number; name: string; users: number; present: number }> = [];
    const lines = output.split('\n');
    
    for (const line of lines) {
      // [0] で始まる行を探す
      const match = line.match(/^\[(\d+)\]\s+(.+?)\s+Users:\s+(\d+)\s+Present:\s+(\d+)/);
      if (match) {
        const index = parseInt(match[1], 10);
        const name = match[2].trim();
        const users = parseInt(match[3], 10);
        const present = parseInt(match[4], 10);
        
        worlds.push({ index, name, users, present });
        console.log(`[RestartManager] Parsed world: [${index}] ${name} - Users: ${users}, Present: ${present}`);
      }
    }
    
    return worlds;
  }

  /**
   * usersコマンドの出力をパース
   * 例: MarkN_headless  ID: U-1NzqeqewOpM       Role: Admin     Present: False  Ping: 0 ms      FPS: 60.073162  Silenced: False
   */
  private parseUsersOutput(output: string): Array<{ username: string; userId: string; present: boolean }> {
    const users: Array<{ username: string; userId: string; present: boolean }> = [];
    const lines = output.split('\n');
    
    for (const line of lines) {
      // ユーザー情報の行をパース
      const match = line.match(/^(.+?)\s+ID:\s+(U-[^\s]+)\s+.*Present:\s+(True|False)/);
      if (match) {
        const username = match[1].trim();
        const userId = match[2].trim();
        const present = match[3] === 'True';
        
        users.push({ username, userId, present });
      }
    }
    
    return users;
  }

  /**
   * 警告アイテムを全セッションにスポーン
   */
  private async spawnWarningItem(itemType: string, message: string, worlds: Array<{ index: number; name: string; users: number; present: number }>): Promise<void> {
    console.log(`[RestartManager] Spawning warning item: ${itemType}`);
    
    if (!this.config) return;
    
    try {
      const itemUrl = this.config.preRestartActions.itemSpawn.itemUrl;
      
      if (!itemUrl) {
        console.error('[RestartManager] Item URL not configured');
        return;
      }
      
      // 各セッションにアイテムをスポーン
      for (const world of worlds) {
        try {
          // セッションにフォーカス
          await this.processManager.sendCommand(`focus ${world.index}`);
          console.log(`[RestartManager] Spawning item in ${world.name}...`);
          
          // アイテムをスポーン
          await this.processManager.sendCommand(`spawn ${itemUrl} true`);
          console.log(`[RestartManager] Spawned ${itemType} in ${world.name}`);
          
        } catch (error) {
          console.error(`[RestartManager] Failed to spawn item in ${world.name}:`, error);
          // エラーが発生しても次のセッションに進む
        }
      }
      
      console.log('[RestartManager] Waiting 10 seconds before sending dynamic impulse...');
      // 10秒待機
      await new Promise(resolve => setTimeout(resolve, 10000));
      
      // 全セッションに対してdynamicImpulseStringを送信
      for (const world of worlds) {
        try {
          // セッションにフォーカス
          await this.processManager.sendCommand(`focus ${world.index}`);
          
          // dynamicImpulseStringでメッセージを送信
          await this.processManager.sendCommand(`dynamicimpulsestring MRHC "${message}"`);
          console.log(`[RestartManager] Sent dynamic impulse to ${world.name}`);
          
        } catch (error) {
          console.error(`[RestartManager] Failed to send dynamic impulse to ${world.name}:`, error);
        }
      }
      
      console.log('[RestartManager] Warning items spawned in all worlds');
      
    } catch (error) {
      console.error('[RestartManager] Failed to spawn warning item:', error);
    }
  }

  /**
   * セッション設定を全セッションに対して変更
   */
  private async applySessionChanges(changes: RestartConfig['preRestartActions']['sessionChanges'], worlds: Array<{ index: number; name: string; users: number; present: number }>): Promise<void> {
    console.log('[RestartManager] Applying session changes to all worlds...');
    
    try {
      // 各セッションに対して設定変更を適用
      for (const world of worlds) {
        try {
          // セッションにフォーカス
          await this.processManager.sendCommand(`focus ${world.index}`);
          console.log(`[RestartManager] Applying changes to ${world.name}...`);
          
          // アクセスレベルをプライベートに変更
          if (changes.setPrivate) {
            await this.processManager.sendCommand('accesslevel Private');
            console.log(`[RestartManager] Set ${world.name} to Private`);
          }
          
          // MaxUserを1に変更
          if (changes.setMaxUserToOne) {
            await this.processManager.sendCommand('maxusers 1');
            console.log(`[RestartManager] Set MaxUsers to 1 for ${world.name}`);
          }
          
          // セッション名を変更
          if (changes.changeSessionName.enabled && changes.changeSessionName.newName) {
            await this.processManager.sendCommand(`name "${changes.changeSessionName.newName}"`);
            console.log(`[RestartManager] Changed name of ${world.name} to "${changes.changeSessionName.newName}"`);
          }
          
        } catch (error) {
          console.error(`[RestartManager] Failed to apply changes to ${world.name}:`, error);
          // エラーが発生しても次のセッションに進む
        }
      }
      
      console.log('[RestartManager] Session changes applied to all worlds');
      
    } catch (error) {
      console.error('[RestartManager] Failed to apply session changes:', error);
    }
  }

  /**
   * 再起動を実行
   */
  private async executeRestart(
    trigger: 'scheduled' | 'highLoad' | 'userZero' | 'manual' | 'forced',
    scheduleId?: string
  ): Promise<void> {
    if (!this.config) {
      throw new Error('Config not loaded');
    }
    
    const { failsafe } = this.config;
    let retryCount = 0;
    
    while (retryCount <= failsafe.retryCount) {
      try {
        // サーバーを停止
        await this.processManager.stop();
        
        // 5秒待機
        await new Promise(resolve => setTimeout(resolve, 5000));
        
        // 最後に使用したコンフィグで起動
        const lastConfig = this.processManager.getLastStartedConfigPath();
        if (!lastConfig) {
          throw new Error('No last config found');
        }
        
        // コンフィグファイルが存在するか確認
        try {
          await fs.access(lastConfig);
        } catch {
          throw new Error(`Config file not found: ${lastConfig}`);
        }
        
        // サーバーを起動
        await this.processManager.start(lastConfig);
        
        // 再起動成功
        this.status.lastRestart = {
          timestamp: new Date().toISOString(),
          trigger,
          scheduleId
        };
        
        await this.saveStatus();
        
        console.log('[RestartManager] Restart completed successfully');
        return;
        
      } catch (error) {
        retryCount++;
        console.error(`[RestartManager] Restart attempt ${retryCount} failed:`, error);
        
        if (retryCount <= failsafe.retryCount) {
          console.log(`[RestartManager] Retrying in ${failsafe.retryIntervalSeconds} seconds...`);
          await new Promise(resolve => setTimeout(resolve, failsafe.retryIntervalSeconds * 1000));
        } else {
          throw new Error(`Restart failed after ${failsafe.retryCount} retries`);
        }
      }
    }
  }

  /**
   * すべてのタイマーをクリア
   */
  private clearAllTimers(): void {
    if (this.waitTimer) {
      clearTimeout(this.waitTimer);
      this.waitTimer = null;
    }
    if (this.forceRestartTimer) {
      clearTimeout(this.forceRestartTimer);
      this.forceRestartTimer = null;
    }
    if (this.actionTimer) {
      clearTimeout(this.actionTimer);
      this.actionTimer = null;
    }
    if (this.userCheckInterval) {
      clearInterval(this.userCheckInterval);
      this.userCheckInterval = null;
    }
    
    // フラグをリセット
    this.actionsExecuted = false;
    this.zeroUserWaitStartTime = null;
  }

  /**
   * 次回の予定再起動を計算して更新
   */
  updateNextScheduledRestart(): void {
    const next = this.scheduledWatcher.getNextScheduledRestart();
    
    if (next) {
      this.status.nextScheduledRestart = {
        scheduleId: next.scheduleId,
        datetime: next.datetime,
        configFile: next.configFile
      };
    } else {
      this.status.nextScheduledRestart = {
        scheduleId: null,
        datetime: null,
        configFile: null
      };
    }
  }

  /**
   * 最後に使用したコンフィグを更新してから再起動
   */
  private async updateLastUsedConfigAndRestart(
    trigger: 'scheduled' | 'highLoad' | 'userZero' | 'manual' | 'forced',
    scheduleId: string | undefined,
    configFile: string
  ): Promise<void> {
    try {
      // runtime-state.jsonを更新
      const runtimeStatePath = path.join(process.cwd(), 'config', 'runtime-state.json');
      const configPath = path.join(process.cwd(), 'config', 'headless', configFile);
      
      // ファイルの存在確認
      try {
        await fs.access(configPath);
      } catch {
        console.error(`[RestartManager] Config file not found: ${configPath}`);
        return;
      }
      
      // runtime-state.jsonを更新
      const runtimeState = {
        lastStartedConfigPath: configPath,
        lastStartedAt: new Date().toISOString(),
        lastStoppedAt: null
      };
      
      await fs.writeFile(runtimeStatePath, JSON.stringify(runtimeState, null, 2), 'utf-8');
      
      // 再起動を実行
      await this.triggerRestart(trigger, scheduleId);
      
    } catch (error) {
      console.error('[RestartManager] Failed to update config and restart:', error);
    }
  }

  /**
   * ユーザー数をチェック（外部から呼び出し可能）
   */
  checkUserCount(totalUsers: number): void {
    this.userZeroWatcher.checkUserCount(totalUsers);
  }

  /**
   * クリーンアップ
   */
  cleanup(): void {
    this.clearAllTimers();
    this.scheduledWatcher.cleanup();
    this.highLoadWatcher.cleanup();
    this.userZeroWatcher.cleanup();
    this.removeAllListeners();
  }
}

