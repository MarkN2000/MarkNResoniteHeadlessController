// ============================================================================
// SteamCMD アップデート関連の型定義
// （backend / frontend の双方から参照される共通型）
// ============================================================================

/**
 * Resoniteアップデート時の状態
 */
export type SteamUpdateState =
  | 'starting'
  | 'authenticating'
  | 'connecting'
  | 'downloading'
  | 'verifying'
  | 'finalizing'
  | 'completed'
  | 'failed'
  | 'guard-required'
  | 'timeout';

/**
 * SteamCMDの出力からパースした進捗情報
 */
export interface SteamUpdateProgress {
  percent: number;        // 0..100
  state: SteamUpdateState;
  downloaded?: number;    // bytes
  total?: number;         // bytes
  raw?: string;           // 元の行（デバッグ用）
}

/**
 * WS再接続時に送信する現在の進捗スナップショット
 *
 * バックエンドが「いま何が起きているか」を保持し、
 * 後から接続してきた / 再接続してきたソケットへ初期同期するために使う。
 */
export interface SteamUpdateSnapshot {
  state: SteamUpdateState | null;
  progress: SteamUpdateProgress | null;
  isActive: boolean;       // 進行中かどうか（モーダル表示制御に使う）
  recentLogs: string[];    // 直近のログ行（リングバッファ）
}

/**
 * Resonite Headless の最新バージョン確認結果
 *
 * バックエンドが定期的に SteamCMD の `+app_info_print` を叩いて
 * Steam 側の最新 buildid を取得し、ローカルの `appmanifest_<appid>.acf` と比較した結果。
 * フロントはこれを見てボタンに赤ドットバッジを表示する。
 */
export interface SteamUpdateCheckResult {
  branch: string;                    // チェック対象のブランチ名（例: "headless"）
  installedBuildId: string | null;   // ローカルにインストール済みの buildid（未インストールなら null）
  latestBuildId: string | null;      // Steam 側の最新 buildid（取得失敗なら null）
  latestTimeUpdated: number | null;  // 最新ビルドの Unix 秒タイムスタンプ（Steam 側）
  updateAvailable: boolean;          // 比較結果: 更新が必要かどうか
  checkedAt: string;                 // チェック実行時刻（ISO 8601）
  error: string | null;              // このチェック実行で発生したエラー（成功時 null）
}

// ============================================================================
// 自動再起動設定の型定義
// ============================================================================

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
  // 予定ごとの待機制御設定（指定時のみ適用）
  waitControl?: {
    forceRestartTimeout: number; // 分
    actionTiming: number; // 分前
  };
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
