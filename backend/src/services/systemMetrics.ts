import si from 'systeminformation';
import { EventEmitter } from 'node:events';

/**
 * システムメトリクス情報
 */
export interface SystemMetrics {
  cpu: {
    usage: number; // 全体のCPU使用率（0-100）
  };
  memory: {
    total: number; // 合計メモリ（バイト）
    used: number;  // 使用中メモリ（バイト）
    usage: number; // メモリ使用率（0-100）
  };
}

/**
 * システムメトリクスを定期的に収集するクラス
 */
export class SystemMetricsCollector extends EventEmitter {
  private intervalId?: NodeJS.Timeout;
  private currentMetrics: SystemMetrics | null = null;
  private readonly intervalMs: number;

  constructor(intervalMs = 2000) {
    super();
    this.intervalMs = intervalMs;
  }

  /**
   * メトリクス収集を開始
   */
  start(): void {
    if (this.intervalId) {
      console.warn('[SystemMetrics] Already started');
      return;
    }

    console.log(`[SystemMetrics] Starting metrics collection (interval: ${this.intervalMs}ms)`);
    
    // 即座に一度実行
    this.collectMetrics();
    
    // 定期的に実行
    this.intervalId = setInterval(() => {
      this.collectMetrics();
    }, this.intervalMs);
  }

  /**
   * メトリクス収集を停止
   */
  stop(): void {
    if (this.intervalId) {
      console.log('[SystemMetrics] Stopping metrics collection');
      clearInterval(this.intervalId);
      this.intervalId = undefined;
    }
  }

  /**
   * 現在のメトリクスを取得
   */
  getCurrentMetrics(): SystemMetrics | null {
    return this.currentMetrics;
  }

  /**
   * メトリクスを収集してイベントをemit
   */
  private async collectMetrics(): Promise<void> {
    try {
      // CPUの現在の負荷を取得（システム全体）
      const cpuLoad = await si.currentLoad();
      
      // メモリ情報を取得（システム全体）
      const memInfo = await si.mem();

      const metrics: SystemMetrics = {
        cpu: {
          usage: Math.round(cpuLoad.currentLoad * 100) / 100, // 小数点2桁まで
        },
        memory: {
          total: memInfo.total,
          used: memInfo.used,
          usage: Math.round((memInfo.used / memInfo.total) * 10000) / 100, // 小数点2桁まで
        },
      };

      this.currentMetrics = metrics;
      this.emit('metrics', metrics);
    } catch (error) {
      console.error('[SystemMetrics] Failed to collect metrics:', error);
    }
  }
}

// シングルトンインスタンス（5秒間隔で更新）
export const systemMetricsCollector = new SystemMetricsCollector(5000);

