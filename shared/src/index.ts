export interface ServerStatus {
  running: boolean;
  profile: string | null;
  pid?: number;
}

export interface LogEntry {
  timestamp: string;
  level: 'info' | 'warn' | 'error';
  message: string;
}

// 自動再起動設定の型定義
export interface ScheduledRestartEntry {
  id: string;
  enabled: boolean;
  type: 'once' | 'weekly' | 'daily';
  // 指定日時の場合（type === 'once'）
  specificDate?: {
    year: number;
    month: number;
    day: number;
    hour: number;
    minute: number;
  };
  // 毎週の場合（type === 'weekly'）
  weeklyDay?: number; // 0=日曜, 1=月曜, ..., 6=土曜
  weeklyTime?: {
    hour: number;
    minute: number;
  };
  // 毎日の場合（type === 'daily'）
  dailyTime?: {
    hour: number;
    minute: number;
  };
  configFile: string; // 起動するコンフィグファイル名
}

export interface RestartConfig {
  // トリガー設定
  triggers: {
    // 予定再起動
    scheduled: {
      enabled: boolean;
      schedules: ScheduledRestartEntry[];
    };
    // 高負荷時再起動
    highLoad: {
      enabled: boolean;
      cpuThreshold: number; // パーセンテージ
      memoryThreshold: number; // パーセンテージ
      durationMinutes: number; // 継続時間（分）
    };
    // ユーザー0時再起動
    userZero: {
      enabled: boolean;
      minUptimeMinutes: number; // 最小稼働時間（分）
    };
  };
  
  // 再起動前アクション設定
  preRestartActions: {
    // 待機制御
    waitControl: {
      forceRestartTimeout: number; // 強制実行までのタイムアウト（分）
      actionTiming: number; // アクション実行タイミング（強制再起動の何分前）
    };
    // チャットメッセージ送信
    chatMessage: {
      enabled: boolean;
      message: string;
    };
    // アイテムスポーン警告
    itemSpawn: {
      enabled: boolean;
      itemType: string;
      itemUrl: string;
      message: string;
    };
    // セッション設定変更
    sessionChanges: {
      setPrivate: boolean;
      setMaxUserToOne: boolean;
      changeSessionName: {
        enabled: boolean;
        newName: string;
      };
    };
  };
  
  // フェールセーフ設定
  failsafe: {
    retryCount: number;
    retryIntervalSeconds: number;
  };
}

// 再起動ステータス（リアルタイム情報）
export interface RestartStatus {
  // 次回の予定再起動
  nextScheduledRestart: {
    scheduleId: string | null;
    datetime: string | null; // ISO 8601形式
    configFile: string | null;
  };
  // 現在の稼働時間
  currentUptime: number; // 秒数
  // 最後の再起動情報
  lastRestart: {
    timestamp: string | null; // ISO 8601形式
    trigger: 'scheduled' | 'highLoad' | 'userZero' | 'manual' | 'forced' | null;
    scheduleId?: string;
  };
  // 高負荷トリガーの無効化状態
  highLoadTriggerDisabledUntil: string | null; // ISO 8601形式
  // 再起動処理中フラグ
  restartInProgress: boolean;
  // 待機中フラグ
  waitingForUsers: boolean;
  // 予定再起動準備中（30分前〜予定時刻）
  scheduledRestartPreparing: {
    preparing: boolean;
    scheduleId: string | null;
    scheduledTime: string | null; // ISO 8601形式
    configFile: string | null;
  };
}
