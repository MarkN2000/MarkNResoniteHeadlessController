import { EventEmitter } from 'node:events';

/**
 * ユーザー0監視サービス
 */
export class UserZeroWatcher extends EventEmitter {
  private enabled = false;
  private minUptimeMinutes = 240; // 最小稼働時間（分）
  private lastUserCount: number | null = null; // 前回のユーザー数
  private serverStartTime: Date | null = null; // サーバー起動時刻

  constructor() {
    super();
  }

  /**
   * 監視を有効化
   */
  enable(enabled: boolean, minUptimeMinutes: number): void {
    this.enabled = enabled;
    this.minUptimeMinutes = minUptimeMinutes;
    
    if (!this.enabled) {
      this.reset();
    }
    
    console.log(`[UserZeroWatcher] ${enabled ? 'Enabled' : 'Disabled'} (min uptime: ${minUptimeMinutes}min)`);
  }

  /**
   * サーバー起動時刻を設定
   */
  setServerStartTime(startTime: Date | null): void {
    this.serverStartTime = startTime;
    
    // サーバー起動時はユーザー数をリセット
    if (startTime) {
      this.lastUserCount = null;
      console.log('[UserZeroWatcher] Server started, user count reset');
    }
  }

  /**
   * ユーザー数を更新してチェック
   */
  checkUserCount(totalUsers: number): void {
    if (!this.enabled) {
      return;
    }
    
    // 最小稼働時間をチェック
    if (!this.canTrigger()) {
      return;
    }
    
    // 前回のユーザー数が記録されている場合のみチェック
    if (this.lastUserCount !== null) {
      // 複数人 → 0人 の変化を検出
      if (this.lastUserCount > 0 && totalUsers === 0) {
        console.log('[UserZeroWatcher] User count changed from multiple to zero, triggering restart');
        this.emit('trigger');
        
        // トリガー後はユーザー数をリセット（連続発動を防ぐ）
        this.lastUserCount = null;
        return;
      }
    }
    
    // ユーザー数を更新
    this.lastUserCount = totalUsers;
  }

  /**
   * トリガー可能か判定（最小稼働時間のチェック）
   */
  private canTrigger(): boolean {
    if (!this.serverStartTime) {
      return false;
    }
    
    const uptimeMinutes = (Date.now() - this.serverStartTime.getTime()) / 60000;
    
    if (uptimeMinutes < this.minUptimeMinutes) {
      return false;
    }
    
    return true;
  }

  /**
   * リセット
   */
  reset(): void {
    this.lastUserCount = null;
  }

  /**
   * クリーンアップ
   */
  cleanup(): void {
    this.reset();
    this.removeAllListeners();
  }
}

