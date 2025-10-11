import { promises as fs } from 'fs';
import path from 'path';
import type { RestartConfig } from '../../../shared/src/index.js';

const CONFIG_DIR = path.join(process.cwd(), 'config');
const RESTART_CONFIG_FILE = path.join(CONFIG_DIR, 'restart.json');

// デフォルト設定
const DEFAULT_CONFIG: RestartConfig = {
  triggers: {
    scheduled: {
      enabled: true,
      schedules: []
    },
    highLoad: {
      enabled: false,
      cpuThreshold: 80,
      memoryThreshold: 80,
      durationMinutes: 10
    },
    userZero: {
      enabled: false,
      minUptimeMinutes: 240
    }
  },
  preRestartActions: {
    waitControl: {
      waitForZeroUsers: 5,
      forceRestartTimeout: 120,
      actionTiming: 2
    },
    chatMessage: {
      enabled: false,
      message: '🔄 サーバーが間もなく再起動します。'
    },
    itemSpawn: {
      enabled: false,
      itemType: 'とらぞセッション閉店アナウンス',
      itemUrl: 'resrec:///U-MarkN/R-d347f78c-d30a-4664-9b6f-2984078880a8',
      message: '🔄 サーバー再起動通知'
    },
    sessionChanges: {
      setPrivate: false,
      setMaxUserToOne: true,
      changeSessionName: {
        enabled: false,
        newName: '[再起動します]'
      }
    }
  },
  failsafe: {
    retryCount: 3,
    retryIntervalSeconds: 30
  }
};

/**
 * 再起動設定を読み込む
 */
export async function loadRestartConfig(): Promise<RestartConfig> {
  try {
    const data = await fs.readFile(RESTART_CONFIG_FILE, 'utf-8');
    const config = JSON.parse(data) as RestartConfig;
    
    // 設定のバリデーション
    validateConfig(config);
    
    return config;
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      // ファイルが存在しない場合はデフォルト設定を作成
      await saveRestartConfig(DEFAULT_CONFIG);
      return DEFAULT_CONFIG;
    }
    
    console.error('[RestartConfig] Failed to load config:', error);
    // エラー時はデフォルト設定を返す
    return DEFAULT_CONFIG;
  }
}

/**
 * 再起動設定を保存する
 */
export async function saveRestartConfig(config: RestartConfig): Promise<void> {
  try {
    // ディレクトリが存在しない場合は作成
    await fs.mkdir(CONFIG_DIR, { recursive: true });
    
    // 設定のバリデーション
    validateConfig(config);
    
    // JSON形式で保存（整形付き）
    await fs.writeFile(
      RESTART_CONFIG_FILE,
      JSON.stringify(config, null, 2),
      'utf-8'
    );
    
    console.log('[RestartConfig] Config saved successfully');
  } catch (error) {
    console.error('[RestartConfig] Failed to save config:', error);
    throw new Error('設定の保存に失敗しました');
  }
}

/**
 * 設定のバリデーション
 */
function validateConfig(config: RestartConfig): void {
  // 基本的な構造チェック
  if (!config.triggers || !config.preRestartActions || !config.failsafe) {
    throw new Error('設定の構造が不正です');
  }
  
  // 数値の範囲チェック
  const { highLoad, userZero } = config.triggers;
  
  if (highLoad.cpuThreshold < 0 || highLoad.cpuThreshold > 100) {
    throw new Error('CPU閾値は0〜100の範囲で指定してください');
  }
  
  if (highLoad.memoryThreshold < 0 || highLoad.memoryThreshold > 100) {
    throw new Error('メモリ閾値は0〜100の範囲で指定してください');
  }
  
  if (highLoad.durationMinutes < 1) {
    throw new Error('継続時間は1分以上で指定してください');
  }
  
  if (userZero.minUptimeMinutes < 0) {
    throw new Error('最小稼働時間は0分以上で指定してください');
  }
  
  // 予定再起動のバリデーション
  for (const schedule of config.triggers.scheduled.schedules) {
    if (!schedule.id || !schedule.type || !schedule.configFile) {
      throw new Error('予定再起動の設定が不正です');
    }
    
    // タイプ別のバリデーション
    if (schedule.type === 'specific' && !schedule.specificDate) {
      throw new Error('指定日時の設定が不正です');
    }
    if (schedule.type === 'weekly' && (schedule.weeklyDay === undefined || !schedule.weeklyTime)) {
      throw new Error('毎週の設定が不正です');
    }
    if (schedule.type === 'daily' && !schedule.dailyTime) {
      throw new Error('毎日の設定が不正です');
    }
  }
  
  // フェールセーフのバリデーション
  if (config.failsafe.retryCount < 0) {
    throw new Error('リトライ回数は0回以上で指定してください');
  }
  
  if (config.failsafe.retryIntervalSeconds < 1) {
    throw new Error('リトライ間隔は1秒以上で指定してください');
  }
}

/**
 * デフォルト設定を取得
 */
export function getDefaultRestartConfig(): RestartConfig {
  return JSON.parse(JSON.stringify(DEFAULT_CONFIG));
}

