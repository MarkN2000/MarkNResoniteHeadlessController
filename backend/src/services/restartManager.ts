import { EventEmitter } from 'node:events';
import { promises as fs } from 'fs';
import path from 'path';
import type { RestartConfig, RestartStatus } from '../../../shared/src/index.js';
import { loadRestartConfig } from './restartConfig.js';
import { ProcessManager } from './processManager.js';
import { ScheduledRestartWatcher } from './scheduledRestartWatcher.js';
import { HighLoadWatcher } from './highLoadWatcher.js';
import { UserZeroWatcher } from './userZeroWatcher.js';
import { PROJECT_ROOT, RUNTIME_STATE_PATH } from '../config/index.js';

const STATUS_FILE = path.join(PROJECT_ROOT, 'config', 'restart-status.json');

// 高負荷トリガーの無効化期間（ミリ秒）
const HIGH_LOAD_COOLDOWN_MS = 30 * 60 * 1000; // 30分

// ユーザー0人待機時間（固定値5秒）
const ZERO_USER_WAIT_TIME_MS = 5000; // 5秒

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
      waitingForUsers: false,
      scheduledRestartPreparing: {
        preparing: false,
        scheduleId: null,
        scheduledTime: null,
        configFile: null
      }
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
      
      // 予定再起動準備中フラグをクリア
      // ※サーバー停止時でも、lastusedコンフィグは予定のものが保持される
      this.clearScheduledRestartPreparation();
      
      // 監視を停止
      this.scheduledWatcher.stop();
      this.highLoadWatcher.stop();
      this.userZeroWatcher.setServerStartTime(null);
      
      this.saveStatus();
      
      console.log('[RestartManager] Server stopped, all timers and flags cleared');
    });
    
    // 予定再起動準備イベント（30分前）
    this.scheduledWatcher.on('preparing', (scheduleId: string, configFile: string, scheduledTime: string) => {
      console.log(`[RestartManager] Scheduled restart preparing: ${scheduleId}, config: ${configFile}, time: ${scheduledTime}`);
      
      // 30分前制御を開始
      this.startScheduledRestartPreparation(scheduleId, configFile, scheduledTime);
    });
    
    // 予定再起動トリガーイベント
    this.scheduledWatcher.on('trigger', (scheduleId: string, configFile: string) => {
      console.log(`[RestartManager] Scheduled restart triggered: ${scheduleId}, config: ${configFile}`);
      
      // コンフィグファイルを更新してから再起動
      this.updateLastUsedConfigAndRestart('scheduled', scheduleId, configFile);
    });
    
    // 高負荷トリガーイベント
    this.highLoadWatcher.on('trigger', () => {
      // 予定再起動準備中の場合はスキップ
      if (this.status.scheduledRestartPreparing.preparing) {
        console.log('[RestartManager] High load trigger skipped: Scheduled restart is preparing');
        return;
      }
      
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
        await this.executePreRestartActions(scheduleId);
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
   * 再起動前アクションのみを実行（再起動は行わない）
   * 手動トリガーの「トリガー後終了」ボタン用
   */
  async triggerPreRestartActionsOnly(scheduleId?: string): Promise<void> {
    // 既に再起動処理中（または待機制御中）の場合はスキップ
    if (this.restartInProgress) {
      console.log('[RestartManager] Restart already in progress, skipping actions-only trigger');
      return;
    }

    // サーバーが停止中の場合はスキップ
    if (!this.processManager.getStatus().running) {
      console.log('[RestartManager] Server not running, skipping actions-only trigger');
      return;
    }

    this.restartInProgress = true;
    this.status.restartInProgress = true;
    console.log('[RestartManager] Actions-only trigger started (no restart will be performed)');

    try {
      await this.executePreRestartActions(scheduleId);
      console.log('[RestartManager] Actions-only trigger completed (no restart executed)');
    } catch (error) {
      console.error('[RestartManager] Actions-only trigger failed:', error);
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
  private async executePreRestartActions(scheduleId?: string): Promise<void> {
    if (!this.config) return;
    
    const { preRestartActions } = this.config;
    const globalWaitControl = preRestartActions.waitControl;
    
    let waitControl = { ...globalWaitControl };
    let usingScheduleOverride = false;
    
    if (scheduleId) {
      const schedule = this.config.triggers.scheduled.schedules.find((s) => s.id === scheduleId);
      if (schedule?.waitControl) {
        waitControl = {
          forceRestartTimeout: schedule.waitControl.forceRestartTimeout,
          actionTiming: schedule.waitControl.actionTiming
        };
        usingScheduleOverride = true;
      }
    }
    
    console.log('[RestartManager] Starting pre-restart wait control...');
    console.log(`[RestartManager] - Wait for zero users: 5 seconds (fixed)`);
    console.log(
      `[RestartManager] - Force restart timeout: ${waitControl.forceRestartTimeout} minutes${
        usingScheduleOverride ? ` (schedule override${scheduleId ? `: ${scheduleId}` : ''})` : ''
      }`
    );
    console.log(
      `[RestartManager] - Action timing: ${waitControl.actionTiming} minutes before restart${
        usingScheduleOverride ? ` (schedule override${scheduleId ? `: ${scheduleId}` : ''})` : ''
      }`
    );
    
    // フラグをリセット
    this.actionsExecuted = false;
    this.zeroUserWaitStartTime = null;
    this.waitingForUsers = true;
    this.status.waitingForUsers = true;
    
    const forceRestartTime = waitControl.forceRestartTimeout * 60 * 1000; // 分→ミリ秒
    const actionTime = waitControl.actionTiming * 60 * 1000; // 分→ミリ秒
    const actionDelay = forceRestartTime - actionTime; // アクション実行までの待機時間
    
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
            console.log(
              `[RestartManager] Action timing reached (${waitControl.actionTiming} minutes before forced restart)`
            );
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
            console.log(`[RestartManager] ⚠️ Users reached zero. Waiting 5 seconds...`);
            console.log(
              `[RestartManager] Zero user wait will complete at: ${new Date(
                this.zeroUserWaitStartTime + ZERO_USER_WAIT_TIME_MS
              ).toLocaleString()}`
            );
          } else {
            // 0人待機中
            const zeroWaitElapsed = Date.now() - this.zeroUserWaitStartTime;
            const zeroWaitRemaining = ZERO_USER_WAIT_TIME_MS - zeroWaitElapsed;
            const secondsRemaining = Math.floor(zeroWaitRemaining / 1000);
            
            console.log(`[RestartManager] Zero user wait: ${secondsRemaining}s remaining (elapsed: ${Math.floor(zeroWaitElapsed / 1000)}s)`);
            
            if (zeroWaitElapsed >= ZERO_USER_WAIT_TIME_MS) {
              // 待機時間が経過したので再起動
              console.log('[RestartManager] ✓ Zero user wait completed. Proceeding to restart.');
              
              // アクションがまだ実行されていない場合は実行
              if (!this.actionsExecuted) {
                console.log('[RestartManager] Executing actions before restart');
                await this.executeActions();
                this.actionsExecuted = true;
              } else {
                console.log('[RestartManager] Actions already executed');
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
            console.log(`[RestartManager] ✓ Users joined (${userCount}). Resetting zero user wait.`);
            this.zeroUserWaitStartTime = null;
          }
        }
      }, USER_CHECK_INTERVAL);
      
      // 初回チェックを即座に実行
      (async () => {
        console.log('[RestartManager] Starting initial user count check...');
        const userCount = await this.getTotalUserCount();
        console.log(`[RestartManager] Initial user count result: ${userCount}`);
        
        if (userCount < 0) {
          console.error('[RestartManager] Initial user count check failed (returned -1)');
          return;
        }
        
        if (userCount === 0) {
          this.zeroUserWaitStartTime = Date.now();
          console.log(`[RestartManager] ⚠️ Users already zero at start. Waiting 5 seconds...`);
        } else {
          console.log(`[RestartManager] ✓ Users present (${userCount}). Will wait for them to leave.`);
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
    if (!this.config) {
      console.error('[RestartManager] ⚠️ Cannot execute actions: config is null');
      return;
    }
    
    const { preRestartActions } = this.config;
    
    console.log('[RestartManager] ========================================');
    console.log('[RestartManager] 🎯 EXECUTING PRE-RESTART ACTIONS');
    console.log('[RestartManager] ========================================');
    
    try {
      // worldsコマンドを1回だけ実行して全アクションで共有
      console.log('[RestartManager] Fetching active worlds...');
      const logEntries = await this.processManager.executeCommand('worlds', 3000);
      const worldsOutput = logEntries.map(entry => entry.message).join('\n');
      const worlds = this.parseWorldsOutput(worldsOutput);
      
      if (worlds.length === 0) {
        console.warn('[RestartManager] ⚠️ No active worlds found. Skipping actions.');
        return;
      }
      
      console.log(`[RestartManager] ✓ Found ${worlds.length} active world(s)`);
      console.log('[RestartManager] Enabled actions:');
      console.log(`[RestartManager]   - Chat message: ${preRestartActions.chatMessage.enabled}`);
      console.log(`[RestartManager]   - Item spawn: ${preRestartActions.itemSpawn.enabled}`);
      console.log(`[RestartManager]   - Session changes: ${preRestartActions.sessionChanges.setPrivate || preRestartActions.sessionChanges.setMaxUserToOne || preRestartActions.sessionChanges.changeSessionName.enabled}`);
      
      // チャットメッセージ送信
      if (preRestartActions.chatMessage.enabled) {
        console.log('[RestartManager] [1/3] Sending chat messages...');
        await this.sendChatMessage(preRestartActions.chatMessage.message, worlds);
        console.log('[RestartManager] ✓ Chat messages completed');
      } else {
        console.log('[RestartManager] [1/3] Chat message disabled, skipping');
      }
      
      // アイテムスポーン
      if (preRestartActions.itemSpawn.enabled) {
        console.log('[RestartManager] [2/3] Spawning warning items...');
        await this.spawnWarningItem(
          preRestartActions.itemSpawn.itemType,
          preRestartActions.itemSpawn.message,
          worlds
        );
        console.log('[RestartManager] ✓ Item spawn completed');
      } else {
        console.log('[RestartManager] [2/3] Item spawn disabled, skipping');
      }
      
      // セッション設定変更
      console.log('[RestartManager] [3/3] Applying session changes...');
      await this.applySessionChanges(preRestartActions.sessionChanges, worlds);
      console.log('[RestartManager] ✓ Session changes completed');
      
      console.log('[RestartManager] ========================================');
      console.log('[RestartManager] ✓ ALL ACTIONS COMPLETED');
      console.log('[RestartManager] ========================================');
      
    } catch (error) {
      console.error('[RestartManager] ❌ Failed to execute some actions:', error);
      // エラーが発生しても再起動は続行
    }
  }

  /**
   * チャットメッセージを全セッションのAFKではないユーザーに送信
   */
  private async sendChatMessage(message: string, worlds: Array<{ index: number; name: string; users: number; present: number }>): Promise<void> {
    console.log('[RestartManager] Sending chat message to all active users:', message);
    
    // 改行を<br>に置き換え
    const formattedMessage = message.replace(/\n/g, '<br>');
    
    try {
      // 各セッションに対して処理
      for (const world of worlds) {
        try {
          // セッションにフォーカス
          await this.processManager.sendCommand(`focus ${world.index}`);
          
          // そのセッションのユーザー一覧を取得
          const logEntries = await this.processManager.executeCommand('users', 3000);
          const usersOutput = logEntries.map(entry => entry.message).join('\n');
          const users = this.parseUsersOutput(usersOutput);
          
          // AFKではないユーザーにメッセージを送信
          for (const user of users) {
            if (!user.present) {
              // AFKユーザーはスキップ
              console.log(`[RestartManager] Skipping AFK user: ${user.username}`);
              continue;
            }
            
            // アクティブなユーザーにメッセージ送信
            // メッセージ全体を引用符で囲む
            await this.processManager.sendCommand(`message "${user.username}" "${formattedMessage}"`);
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
      const logEntries = await this.processManager.executeCommand('worlds', 3000);
      const worldsOutput = logEntries.map(entry => entry.message).join('\n');
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
    console.log('[RestartManager] === RAW WORLDS OUTPUT START ===');
    console.log(output);
    console.log('[RestartManager] === RAW WORLDS OUTPUT END ===');
    
    // 重複を除去するためにMapを使用
    const worldsMap = new Map<number, { index: number; name: string; users: number; present: number }>();
    const lines = output.split('\n');
    
    console.log(`[RestartManager] Parsing ${lines.length} lines...`);
    
    for (const line of lines) {
      if (!line.trim()) continue; // 空行をスキップ
      
      // [0] で始まる行を探す（タブ文字にも対応）
      const match = line.match(/^\[(\d+)\]\s+(.+?)\s+Users:\s+(\d+)[\s\t]+Present:\s+(\d+)/);
      if (match) {
        const index = parseInt(match[1]!, 10);
        const name = match[2]!.trim();
        const users = parseInt(match[3]!, 10);
        const present = parseInt(match[4]!, 10);
        
        // indexが既に存在する場合は上書きしない（最初のものを保持）
        if (!worldsMap.has(index)) {
          worldsMap.set(index, { index, name, users, present });
          console.log(`[RestartManager] ✓ Parsed world: [${index}] ${name} - Users: ${users}, Present: ${present}`);
        } else {
          console.log(`[RestartManager] ⚠️ Skipping duplicate world: [${index}] ${name}`);
        }
      } else {
        // マッチしなかった行を出力（デバッグ用）
        if (line.includes('[')) {
          console.log(`[RestartManager] ✗ Failed to parse line: "${line}"`);
        }
      }
    }
    
    const worlds = Array.from(worldsMap.values());
    console.log(`[RestartManager] Total unique worlds: ${worlds.length}`);
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
        const username = match[1]!.trim();
        const userId = match[2]!.trim();
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
          await this.processManager.sendCommand(`dynamicimpulsestring MRHC.play "${message}"`);
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
   * コンフィグファイル名を解決（"__previous__"の場合は現在のコンフィグを使用）
   */
  private async resolveConfigFile(configFile: string): Promise<{ fileName: string; fullPath: string } | null> {
    // "__previous__"の場合は現在のコンフィグを取得
    if (configFile === '__previous__') {
      try {
        const runtimeStateData = await fs.readFile(RUNTIME_STATE_PATH, 'utf-8');
        const runtimeState = JSON.parse(runtimeStateData);
        
        if (runtimeState.lastUsedConfigPath) {
          const fileName = path.basename(runtimeState.lastUsedConfigPath);
          console.log(`[RestartManager] Resolved "__previous__" to: ${fileName}`);
          return {
            fileName,
            fullPath: runtimeState.lastUsedConfigPath
          };
        } else {
          console.error('[RestartManager] No previous config found in runtime-state.json');
          return null;
        }
      } catch (error: any) {
        // runtime-state.jsonが存在しない場合は、ProcessManagerから取得を試みる
        if (error.code === 'ENOENT') {
          console.log('[RestartManager] runtime-state.json not found, trying ProcessManager as fallback');
          
          // まずProcessManager.getLastStartedConfigPath()を試す
          let lastConfigPath = this.processManager.getLastStartedConfigPath();
          
          // それでも取得できない場合は、現在のステータスから取得を試みる
          if (!lastConfigPath) {
            const status = this.processManager.getStatus();
            if (status.configPath) {
              lastConfigPath = status.configPath;
              console.log('[RestartManager] Using configPath from ProcessManager status');
            }
          }
          
          if (lastConfigPath) {
            const fileName = path.basename(lastConfigPath);
            console.log(`[RestartManager] Resolved "__previous__" to: ${fileName} (from ProcessManager)`);
            return {
              fileName,
              fullPath: lastConfigPath
            };
          } else {
            console.error('[RestartManager] No previous config found. Please start the server at least once or specify a config file name instead of "__previous__"');
            return null;
          }
        } else {
          console.error('[RestartManager] Failed to read runtime-state.json:', error);
          return null;
        }
      }
    }
    
    // 通常のコンフィグファイル名の場合
    const configPath = path.join(PROJECT_ROOT, 'config', 'headless', configFile);
    return {
      fileName: configFile,
      fullPath: configPath
    };
  }

  /**
   * 予定再起動の30分前制御を開始
   */
  private async startScheduledRestartPreparation(
    scheduleId: string,
    configFile: string,
    scheduledTime: string
  ): Promise<void> {
    console.log('[RestartManager] ========================================');
    console.log('[RestartManager] 🔔 SCHEDULED RESTART PREPARATION STARTED');
    console.log(`[RestartManager] Schedule ID: ${scheduleId}`);
    console.log(`[RestartManager] Config File: ${configFile}`);
    console.log(`[RestartManager] Scheduled Time: ${scheduledTime}`);
    console.log('[RestartManager] ========================================');
    
    // 既に準備中の場合は何もしない
    if (this.status.scheduledRestartPreparing.preparing) {
      console.log('[RestartManager] Already preparing for another scheduled restart');
      return;
    }
    
    // 準備中フラグをセット
    this.status.scheduledRestartPreparing = {
      preparing: true,
      scheduleId,
      scheduledTime,
      configFile
    };
    
    // 1. 待機中の再起動をキャンセル
    if (this.waitingForUsers) {
      console.log('[RestartManager] Cancelling waiting restart...');
      this.clearAllTimers();
      this.waitingForUsers = false;
      this.status.waitingForUsers = false;
      console.log('[RestartManager] ✓ Waiting restart cancelled');
    }
    
    // 2. コンフィグファイルを解決
    const resolved = await this.resolveConfigFile(configFile);
    if (!resolved) {
      console.error('[RestartManager] ⚠️ Preparation failed: Cannot resolve config file');
      this.clearScheduledRestartPreparation();
      return;
    }
    
    // 3. lastusedコンフィグを予定のものに変更
    try {
      console.log(`[RestartManager] Checking config file: ${resolved.fullPath}`);
      
      // ファイルの存在確認
      try {
        await fs.access(resolved.fullPath);
        console.log(`[RestartManager] Config file exists: ${resolved.fullPath}`);
      } catch {
        console.error(`[RestartManager] Config file not found: ${resolved.fullPath}`);
        console.error('[RestartManager] ⚠️ Preparation failed: Config file not found');
        this.clearScheduledRestartPreparation();
        return;
      }
      
      // runtime-state.jsonを更新
      const runtimeState = {
        lastUsedConfigPath: resolved.fullPath,
        lastUsedConfigName: resolved.fileName,
        isRunning: true
      };
      
      await fs.writeFile(RUNTIME_STATE_PATH, JSON.stringify(runtimeState, null, 2), 'utf-8');
      console.log(`[RestartManager] ✓ Updated lastUsedConfig to: ${resolved.fileName}`);
      console.log(`[RestartManager] ✓ runtime-state.json path: ${RUNTIME_STATE_PATH}`);
      
    } catch (error) {
      console.error('[RestartManager] Failed to update runtime state:', error);
      console.error('[RestartManager] ⚠️ Preparation failed: Cannot update config');
      this.clearScheduledRestartPreparation();
      return;
    }
    
    // 4. 高負荷トリガーは既にイベントハンドラーで無効化済み
    console.log('[RestartManager] ✓ High load trigger disabled (handled in event listener)');
    
    // ステータスを保存
    await this.saveStatus();
    
    console.log('[RestartManager] ========================================');
    console.log('[RestartManager] ✓ PREPARATION COMPLETED');
    console.log('[RestartManager] High load triggers will be skipped until scheduled time');
    console.log('[RestartManager] Force restart button remains available');
    console.log('[RestartManager] ========================================');
  }

  /**
   * 予定再起動準備中フラグをクリア
   */
  private clearScheduledRestartPreparation(): void {
    this.status.scheduledRestartPreparing = {
      preparing: false,
      scheduleId: null,
      scheduledTime: null,
      configFile: null
    };
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
      // コンフィグファイルを解決
      const resolved = await this.resolveConfigFile(configFile);
      if (!resolved) {
        console.error('[RestartManager] Cannot resolve config file, restart aborted');
        return;
      }
      
      console.log(`[RestartManager] Updating config to: ${resolved.fullPath}`);
      
      // ファイルの存在確認
      try {
        await fs.access(resolved.fullPath);
        console.log(`[RestartManager] Config file exists: ${resolved.fullPath}`);
      } catch {
        console.error(`[RestartManager] Config file not found: ${resolved.fullPath}`);
        return;
      }
      
      // runtime-state.jsonを更新
      const runtimeState = {
        lastUsedConfigPath: resolved.fullPath,
        lastUsedConfigName: resolved.fileName,
        isRunning: true
      };
      
      await fs.writeFile(RUNTIME_STATE_PATH, JSON.stringify(runtimeState, null, 2), 'utf-8');
      console.log(`[RestartManager] runtime-state.json updated with config: ${resolved.fileName}`);
      
      // 再起動を実行
      await this.triggerRestart(trigger, scheduleId);
      
      // 予定再起動の準備中フラグをクリア
      if (trigger === 'scheduled') {
        this.clearScheduledRestartPreparation();
        await this.saveStatus();
        console.log('[RestartManager] ✓ Scheduled restart preparation cleared');
      }
      
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

