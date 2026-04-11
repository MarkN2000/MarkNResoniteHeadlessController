import { EventEmitter } from 'node:events';
import type {
  SteamUpdateProgress,
  SteamUpdateState,
  SteamUpdateSnapshot,
  SteamUpdateCheckResult
} from '../../../shared/src/index.js';

// 既存 import 経路の後方互換のため、shared から取り込んだ型を再エクスポート
export type { SteamUpdateSnapshot, SteamUpdateCheckResult };

/** サーバー側で保持するログの上限（フロントの 500 と独立して控えめに設定） */
const SNAPSHOT_LOG_LIMIT = 200;

/** isActive を false に落とす終了状態 */
const TERMINAL_STATES: ReadonlySet<SteamUpdateState> = new Set<SteamUpdateState>([
  'completed',
  'failed',
  'timeout'
]);

/**
 * SteamCMDアップデートのリアルタイムイベントを中継するシングルトンバス
 *
 * processManager と同じ単方向フロー:
 *   ルートハンドラ (SteamUpdateManager) -> このバス -> WS層 (Socket.IO namespace)
 *
 * 発火するイベント:
 *   'log'      (text: string)
 *   'status'   (state: SteamUpdateState)
 *   'progress' (progress: SteamUpdateProgress)
 *
 * 加えて、最新の状態を内部に保持し getSnapshot() で取得できる。
 * これは WS 再接続時の状態同期に使用される。
 */
class SteamUpdateBus extends EventEmitter {
  private state: SteamUpdateState | null = null;
  private progress: SteamUpdateProgress | null = null;
  private isActive = false;
  private recentLogs: string[] = [];
  /** Resonite 最新バージョン確認の直前結果（WS 再接続時のスナップショット用） */
  private lastCheckResult: SteamUpdateCheckResult | null = null;

  emitLog(text: string): void {
    // 受信時に行単位でリングバッファへ蓄積
    // （SteamCMD は行末改行を含むため split で複数行に分かれる場合がある）
    const lines = text.split(/\r?\n/).filter(line => line.length > 0);
    if (lines.length > 0) {
      this.recentLogs.push(...lines);
      if (this.recentLogs.length > SNAPSHOT_LOG_LIMIT) {
        this.recentLogs.splice(0, this.recentLogs.length - SNAPSHOT_LOG_LIMIT);
      }
    }
    this.emit('log', text);
  }

  emitStatus(state: SteamUpdateState): void {
    // 'starting' を新しい実行の開始シグナルとしてスナップショットをリセット
    // （前回終了したアップデートのログ・進捗が新しい実行に混ざるのを防ぐ）
    if (state === 'starting') {
      this.recentLogs = [];
      this.progress = null;
      this.isActive = true;
    }
    this.state = state;
    if (TERMINAL_STATES.has(state)) {
      this.isActive = false;
    }
    this.emit('status', state);
  }

  emitProgress(progress: SteamUpdateProgress): void {
    this.progress = progress;
    this.emit('progress', progress);
  }

  /**
   * WS新規接続時に「いま進行中の状態」を返す
   * state が null の場合はまだ一度もアップデートが実行されていないことを意味する
   */
  getSnapshot(): SteamUpdateSnapshot {
    return {
      state: this.state,
      progress: this.progress,
      isActive: this.isActive,
      // ミューテーション防止のためコピーを返す
      recentLogs: this.recentLogs.slice()
    };
  }

  /**
   * いま SteamCMD のアップデートが進行中かどうかを公開する。
   * SteamUpdateChecker が同時実行を避けるためにこのフラグを参照する。
   */
  isUpdateActive(): boolean {
    return this.isActive;
  }

  /**
   * バージョン確認 (`app_info_print`) の結果を受け取って中継する。
   * フロントは WS 経由で `'check-result'` を受け取り、バッジ表示を更新する。
   */
  emitCheckResult(result: SteamUpdateCheckResult): void {
    this.lastCheckResult = result;
    this.emit('check-result', result);
  }

  /**
   * WS 新規接続時に渡す「直前のバージョン確認結果」。
   * まだ一度もチェックが走っていない場合は null。
   */
  getCheckSnapshot(): SteamUpdateCheckResult | null {
    return this.lastCheckResult;
  }
}

export const steamUpdateBus = new SteamUpdateBus();
// 複数のWSクライアントが接続してもリスナー警告が出ないように上限を緩める
steamUpdateBus.setMaxListeners(50);
