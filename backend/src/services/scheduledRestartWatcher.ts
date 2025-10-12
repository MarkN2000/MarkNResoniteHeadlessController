import { EventEmitter } from 'node:events';
import type { ScheduledRestartEntry } from '../../../shared/src/index.js';

const CHECK_INTERVAL_MS = 60 * 1000; // 1分ごとにチェック

/**
 * 予定再起動の監視サービス
 */
export class ScheduledRestartWatcher extends EventEmitter {
  private schedules: ScheduledRestartEntry[] = [];
  private enabled = false;
  private checkTimer: NodeJS.Timeout | null = null;
  private triggeredSpecificDates = new Set<string>(); // 既に発動した指定日時を記録
  private preparingSchedules = new Set<string>(); // 既に準備中のスケジュールを記録

  constructor() {
    super();
  }

  /**
   * 監視を開始
   */
  start(schedules: ScheduledRestartEntry[], enabled: boolean): void {
    this.schedules = schedules.filter(s => s.enabled);
    this.enabled = enabled;
    
    if (!this.enabled || this.schedules.length === 0) {
      this.stop();
      return;
    }
    
    // 既存のタイマーをクリア
    if (this.checkTimer) {
      clearInterval(this.checkTimer);
    }
    
    // 定期チェックを開始
    this.checkTimer = setInterval(() => {
      this.checkSchedules();
    }, CHECK_INTERVAL_MS);
    
    // 即座に1回チェック
    this.checkSchedules();
    
    console.log(`[ScheduledRestartWatcher] Started monitoring ${this.schedules.length} schedule(s)`);
  }

  /**
   * 監視を停止
   */
  stop(): void {
    if (this.checkTimer) {
      clearInterval(this.checkTimer);
      this.checkTimer = null;
    }
    console.log('[ScheduledRestartWatcher] Stopped');
  }

  /**
   * スケジュールをチェック
   */
  private checkSchedules(): void {
    const now = new Date();
    
    // 30分前チェック（準備開始）
    this.check30MinutesBefore(now);
    
    // トリガーチェック（予定時刻）
    for (const schedule of this.schedules) {
      if (this.shouldTrigger(schedule, now)) {
        console.log(`[ScheduledRestartWatcher] Triggering schedule: ${schedule.id}`);
        this.emit('trigger', schedule.id, schedule.configFile);
        
        // 準備中フラグをクリア
        this.preparingSchedules.delete(schedule.id);
        
        // 指定日時の場合は発動済みとしてマーク
        if (schedule.type === 'once') {
          const key = this.getSpecificDateKey(schedule);
          this.triggeredSpecificDates.add(key);
        }
      }
    }
  }

  /**
   * 30分前チェック（準備開始の通知）
   */
  private check30MinutesBefore(now: Date): void {
    const THIRTY_MINUTES_MS = 30 * 60 * 1000;
    
    for (const schedule of this.schedules) {
      const nextTime = this.getNextTriggerTime(schedule, now);
      if (!nextTime) {
        // 予定時刻がない場合は準備中フラグをクリア
        this.preparingSchedules.delete(schedule.id);
        continue;
      }
      
      const timeDiff = nextTime.getTime() - now.getTime();
      
      // 30分前〜予定時刻の間の場合
      if (timeDiff > 0 && timeDiff <= THIRTY_MINUTES_MS) {
        // 既に準備中の場合はスキップ
        if (this.preparingSchedules.has(schedule.id)) {
          continue;
        }
        
        // 準備中としてマーク
        this.preparingSchedules.add(schedule.id);
        
        // 準備開始イベントを発火
        this.emit('preparing', schedule.id, schedule.configFile, nextTime.toISOString());
        console.log(`[ScheduledRestartWatcher] Preparing for schedule: ${schedule.id} (scheduled at ${nextTime.toISOString()})`);
      } else if (timeDiff > THIRTY_MINUTES_MS) {
        // 30分前より前の場合は準備中フラグをクリア（リセット）
        this.preparingSchedules.delete(schedule.id);
      }
    }
  }

  /**
   * スケジュールをトリガーすべきか判定
   */
  private shouldTrigger(schedule: ScheduledRestartEntry, now: Date): boolean {
    if (!schedule.enabled) return false;
    
    switch (schedule.type) {
      case 'once':
        return this.shouldTriggerSpecific(schedule, now);
      case 'weekly':
        return this.shouldTriggerWeekly(schedule, now);
      case 'daily':
        return this.shouldTriggerDaily(schedule, now);
      default:
        return false;
    }
  }

  /**
   * 指定日時のスケジュールをトリガーすべきか判定
   */
  private shouldTriggerSpecific(schedule: ScheduledRestartEntry, now: Date): boolean {
    if (!schedule.specificDate) return false;
    
    const { year, month, day, hour, minute } = schedule.specificDate;
    
    // 既に発動済みの場合はスキップ
    const key = this.getSpecificDateKey(schedule);
    if (this.triggeredSpecificDates.has(key)) {
      return false;
    }
    
    // 指定日時と現在時刻を比較（分単位）
    const targetDate = new Date(year, month - 1, day, hour, minute, 0, 0);
    const currentMinute = new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours(), now.getMinutes(), 0, 0);
    
    // 指定日時が現在時刻と一致する場合にトリガー
    return targetDate.getTime() === currentMinute.getTime();
  }

  /**
   * 毎週のスケジュールをトリガーすべきか判定
   */
  private shouldTriggerWeekly(schedule: ScheduledRestartEntry, now: Date): boolean {
    if (schedule.weeklyDay === undefined || !schedule.weeklyTime) return false;
    
    const { weeklyDay, weeklyTime } = schedule;
    const { hour, minute } = weeklyTime;
    
    // 曜日と時刻をチェック
    const currentDay = now.getDay(); // 0=日曜, 1=月曜, ..., 6=土曜
    const currentHour = now.getHours();
    const currentMinute = now.getMinutes();
    
    return currentDay === weeklyDay && currentHour === hour && currentMinute === minute;
  }

  /**
   * 毎日のスケジュールをトリガーすべきか判定
   */
  private shouldTriggerDaily(schedule: ScheduledRestartEntry, now: Date): boolean {
    if (!schedule.dailyTime) return false;
    
    const { hour, minute } = schedule.dailyTime;
    const currentHour = now.getHours();
    const currentMinute = now.getMinutes();
    
    return currentHour === hour && currentMinute === minute;
  }

  /**
   * 指定日時スケジュールのキーを生成
   */
  private getSpecificDateKey(schedule: ScheduledRestartEntry): string {
    if (!schedule.specificDate) return '';
    const { year, month, day, hour, minute } = schedule.specificDate;
    return `${schedule.id}-${year}-${month}-${day}-${hour}-${minute}`;
  }

  /**
   * 次回の予定再起動を計算
   */
  getNextScheduledRestart(): { scheduleId: string; datetime: string; configFile: string } | null {
    if (!this.enabled || this.schedules.length === 0) {
      return null;
    }
    
    const now = new Date();
    let nearest: { scheduleId: string; datetime: Date; configFile: string } | null = null;
    
    for (const schedule of this.schedules) {
      const nextDatetime = this.getNextTriggerTime(schedule, now);
      if (nextDatetime) {
        if (!nearest || nextDatetime < nearest.datetime) {
          nearest = {
            scheduleId: schedule.id,
            datetime: nextDatetime,
            configFile: schedule.configFile
          };
        }
      }
    }
    
    if (nearest) {
      return {
        scheduleId: nearest.scheduleId,
        datetime: nearest.datetime.toISOString(),
        configFile: nearest.configFile
      };
    }
    
    return null;
  }

  /**
   * スケジュールの次回発動時刻を計算
   */
  private getNextTriggerTime(schedule: ScheduledRestartEntry, from: Date): Date | null {
    switch (schedule.type) {
      case 'once':
        return this.getNextSpecificTime(schedule, from);
      case 'weekly':
        return this.getNextWeeklyTime(schedule, from);
      case 'daily':
        return this.getNextDailyTime(schedule, from);
      default:
        return null;
    }
  }

  /**
   * 指定日時の次回発動時刻を計算
   */
  private getNextSpecificTime(schedule: ScheduledRestartEntry, from: Date): Date | null {
    if (!schedule.specificDate) return null;
    
    const { year, month, day, hour, minute } = schedule.specificDate;
    const targetDate = new Date(year, month - 1, day, hour, minute, 0, 0);
    
    // 既に過去の日時の場合はnull
    if (targetDate <= from) {
      return null;
    }
    
    // 既に発動済みの場合はnull
    const key = this.getSpecificDateKey(schedule);
    if (this.triggeredSpecificDates.has(key)) {
      return null;
    }
    
    return targetDate;
  }

  /**
   * 毎週の次回発動時刻を計算
   */
  private getNextWeeklyTime(schedule: ScheduledRestartEntry, from: Date): Date | null {
    if (schedule.weeklyDay === undefined || !schedule.weeklyTime) return null;
    
    const { weeklyDay, weeklyTime } = schedule;
    const { hour, minute } = weeklyTime;
    
    const currentDay = from.getDay();
    let daysUntilNext = (weeklyDay - currentDay + 7) % 7;
    
    // 今日が該当曜日で、時刻がまだ来ていない場合
    if (daysUntilNext === 0) {
      const currentHour = from.getHours();
      const currentMinute = from.getMinutes();
      
      if (currentHour < hour || (currentHour === hour && currentMinute < minute)) {
        // 今日の指定時刻
        const nextDate = new Date(from);
        nextDate.setHours(hour, minute, 0, 0);
        return nextDate;
      } else {
        // 次週の該当曜日
        daysUntilNext = 7;
      }
    }
    
    // 次回の該当曜日
    const nextDate = new Date(from);
    nextDate.setDate(from.getDate() + daysUntilNext);
    nextDate.setHours(hour, minute, 0, 0);
    return nextDate;
  }

  /**
   * 毎日の次回発動時刻を計算
   */
  private getNextDailyTime(schedule: ScheduledRestartEntry, from: Date): Date | null {
    if (!schedule.dailyTime) return null;
    
    const { hour, minute } = schedule.dailyTime;
    const currentHour = from.getHours();
    const currentMinute = from.getMinutes();
    
    const nextDate = new Date(from);
    nextDate.setHours(hour, minute, 0, 0);
    
    // 今日の指定時刻がまだ来ていない場合は今日、そうでなければ明日
    if (currentHour < hour || (currentHour === hour && currentMinute < minute)) {
      return nextDate;
    } else {
      nextDate.setDate(from.getDate() + 1);
      return nextDate;
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

