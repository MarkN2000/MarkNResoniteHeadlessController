import { EventEmitter } from 'node:events';
import type { SystemMetrics } from './systemMetrics.js';

const CHECK_INTERVAL_MS = 60 * 1000; // 1分ごとにチェック

/**
 * 高負荷監視サービス
 * SystemMetricsCollector のメトリクスイベントを利用して負荷を判定する
 */
export class HighLoadWatcher extends EventEmitter {
  private enabled = false;
  private cpuThreshold = 80;
  private memoryThreshold = 80;
  private durationMinutes = 10;
  private checkTimer: NodeJS.Timeout | null = null;
  private highLoadStartTime: Date | null = null;
  private disabledUntil: Date | null = null;
  private latestMetrics: SystemMetrics | null = null;

  constructor() {
    super();
  }

  /**
   * メトリクスを更新（SystemMetricsCollector の metrics イベントから呼ばれる）
   */
  updateMetrics(metrics: SystemMetrics): void {
    this.latestMetrics = metrics;
  }

  /**
   * 監視を開始
   */
  start(
    enabled: boolean,
    cpuThreshold: number,
    memoryThreshold: number,
    durationMinutes: number,
    disabledUntil: string | null
  ): void {
    this.enabled = enabled;
    this.cpuThreshold = cpuThreshold;
    this.memoryThreshold = memoryThreshold;
    this.durationMinutes = durationMinutes;

    if (disabledUntil) {
      this.disabledUntil = new Date(disabledUntil);
    } else {
      this.disabledUntil = null;
    }

    if (!this.enabled) {
      this.stop();
      return;
    }

    // 既存のタイマーをクリア
    if (this.checkTimer) {
      clearInterval(this.checkTimer);
    }

    // 定期チェックを開始（メトリクスは systemMetricsCollector から受け取る）
    this.checkTimer = setInterval(() => {
      this.checkLoad();
    }, CHECK_INTERVAL_MS);

    // 即座に1回チェック
    this.checkLoad();

    console.log(`[HighLoadWatcher] Started monitoring (CPU: ${cpuThreshold}%, Memory: ${memoryThreshold}%, Duration: ${durationMinutes}min)`);
  }

  /**
   * 監視を停止
   */
  stop(): void {
    if (this.checkTimer) {
      clearInterval(this.checkTimer);
      this.checkTimer = null;
    }
    this.highLoadStartTime = null;
    console.log('[HighLoadWatcher] Stopped');
  }

  /**
   * 負荷をチェック（systemMetricsCollector から受信したメトリクスを使用）
   */
  private checkLoad(): void {
    try {
      // 無効化期間中の場合はスキップ
      if (this.disabledUntil && new Date() < this.disabledUntil) {
        const remaining = Math.ceil((this.disabledUntil.getTime() - Date.now()) / 60000);
        if (this.highLoadStartTime) {
          console.log(`[HighLoadWatcher] Disabled (${remaining} min remaining), resetting high load timer`);
          this.highLoadStartTime = null;
        }
        return;
      }

      // メトリクスが未取得の場合はスキップ
      if (!this.latestMetrics) {
        console.log('[HighLoadWatcher] No metrics available yet, skipping check');
        return;
      }

      const cpuPercent = this.latestMetrics.cpu.usage;
      const memPercent = this.latestMetrics.memory.usage;

      console.log(`[HighLoadWatcher] CPU: ${cpuPercent.toFixed(1)}%, Memory: ${memPercent.toFixed(1)}%`);

      // 閾値を超えているかチェック
      const isHighLoad = cpuPercent >= this.cpuThreshold || memPercent >= this.memoryThreshold;

      if (isHighLoad) {
        if (!this.highLoadStartTime) {
          // 高負荷状態の開始
          this.highLoadStartTime = new Date();
          console.log(`[HighLoadWatcher] High load detected, monitoring for ${this.durationMinutes} minutes`);
        } else {
          // 高負荷状態が継続中
          const elapsedMinutes = (Date.now() - this.highLoadStartTime.getTime()) / 60000;

          if (elapsedMinutes >= this.durationMinutes) {
            // 指定時間以上高負荷が継続した場合
            console.log(`[HighLoadWatcher] High load continued for ${this.durationMinutes} minutes, triggering restart`);
            this.emit('trigger');

            // タイマーをリセット（再度発動を防ぐ）
            this.highLoadStartTime = null;
          } else {
            console.log(`[HighLoadWatcher] High load continues (${elapsedMinutes.toFixed(1)}/${this.durationMinutes} min)`);
          }
        }
      } else {
        // 負荷が正常範囲に戻った場合
        if (this.highLoadStartTime) {
          console.log('[HighLoadWatcher] Load returned to normal, resetting timer');
          this.highLoadStartTime = null;
        }
      }

    } catch (error) {
      console.error('[HighLoadWatcher] Failed to check load:', error);
    }
  }

  /**
   * クリーンアップ
   */
  cleanup(): void {
    this.stop();
    this.removeAllListeners();
  }
}
