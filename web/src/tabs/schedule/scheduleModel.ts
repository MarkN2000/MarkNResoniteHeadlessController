// スケジュールタブの表示ヘルパ（純関数）。Phase 8・§3.16。
import type {
  RestartAnnounce,
  RestartCrashRecovery,
  RestartSessionChanges,
  RestartWaitControl,
  ScheduledRestart,
} from "../../api";

// 2桁ゼロ埋め（時刻表示）。
export const pad = (n: number): string => String(n).padStart(2, "0");

// 種別 → i18n キー。
export function typeKey(type: string): string {
  switch (type) {
    case "once":
      return "schedule.typeOnce";
    case "weekly":
      return "schedule.typeWeekly";
    default:
      return "schedule.typeDaily";
  }
}

// 曜日(0=日..6=土) → i18n キー。
export const WEEKDAY_KEYS = [
  "schedule.weekday0",
  "schedule.weekday1",
  "schedule.weekday2",
  "schedule.weekday3",
  "schedule.weekday4",
  "schedule.weekday5",
  "schedule.weekday6",
] as const;

// 予定の時刻を種別ごとに整形（weekly の曜日ラベルは呼び出し側が解決して渡す）。
export function formatScheduleTime(s: ScheduledRestart, weekdayLabel: string): string {
  const hm = `${pad(s.hour)}:${pad(s.minute)}`;
  if (s.type === "once") return `${s.year ?? 0}/${pad(s.month ?? 0)}/${pad(s.day ?? 0)} ${hm}`;
  if (s.type === "weekly") return `${weekdayLabel} ${hm}`;
  return hm; // daily
}

// once の年の下限（backend Validate と揃える＝restart.go の Year>=2000）。UI も同値で拘束する。
export const MIN_YEAR = 2000;

// once の年月日が実在する暦日か（2/30 等を弾く・JS Date 往復）。年下限も backend と揃える。
export function isValidOnceDate(y: number, m: number, d: number): boolean {
  if (!Number.isInteger(y) || !Number.isInteger(m) || !Number.isInteger(d)) return false;
  if (y < MIN_YEAR || m < 1 || m > 12 || d < 1 || d > 31) return false;
  const dt = new Date(y, m - 1, d);
  return dt.getFullYear() === y && dt.getMonth() === m - 1 && dt.getDate() === d;
}

// 予定の一意 id を生成。crypto.randomUUID はセキュアコンテキスト（HTTPS/localhost）限定のため、
// LAN/HTTP でも動くよう getRandomValues（insecure 可）→ Math.random の順でフォールバックする。
export function genId(): string {
  const c = globalThis.crypto;
  if (c?.randomUUID) return c.randomUUID();
  if (c?.getRandomValues) {
    const b = c.getRandomValues(new Uint8Array(16));
    return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  }
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`;
}

// 新規予定の雛形（毎日 05:00・有効・前回 config）。id はブラウザ生成。
export function defaultScheduled(): ScheduledRestart {
  return { id: genId(), enabled: true, type: "daily", weekday: 0, hour: 5, minute: 0, configName: "" };
}

// 各 restart 設定の既定値（backend config.DefaultRestart() をミラー＝「既定値に戻す」のリセット先）。
// 値を変えるときは internal/config/restart.go の DefaultRestart() と同期すること。
export function defaultWaitControl(): RestartWaitControl {
  return { forceRestartTimeoutMin: 60, actionTimingMin: 2 };
}
export function defaultCrashRecovery(): RestartCrashRecovery {
  return { enabled: true, maxCrashes: 3, windowMinutes: 10 };
}
export function defaultAnnounce(): RestartAnnounce {
  return { enabled: false, itemUrl: "", impulseTag: "", message: "まもなく再起動します" };
}
export function defaultSessionChanges(): RestartSessionChanges {
  return { setPrivate: false, setMaxUsersOne: true, renameEnabled: false, renameTo: "" };
}

// 稼働時間/残り時間を言語非依存の短い表記にする（例: "1d 2h" / "2h 34m" / "34m"）。
export function formatDuration(sec: number): string {
  if (sec <= 0) return "0m";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

// RFC3339 をロケール準拠の短い日時表記にする（不正値はそのまま返す）。
export function formatDateTime(rfc3339: string): string {
  const d = new Date(rfc3339);
  if (isNaN(d.getTime())) return rfc3339;
  return d.toLocaleString(undefined, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// 締切までの残り（現在時刻基準）。過ぎていれば "0m"。
export function formatRemaining(deadlineISO: string): string {
  const ms = new Date(deadlineISO).getTime() - Date.now();
  if (isNaN(ms) || ms <= 0) return "0m";
  return formatDuration(Math.floor(ms / 1000));
}

// 進行フェーズ → i18n キー。
export function phaseKey(phase: string): string {
  switch (phase) {
    case "preparing":
      return "schedule.phasePreparing";
    case "waiting":
      return "schedule.phaseWaiting";
    case "announcing":
      return "schedule.phaseAnnouncing";
    case "restarting":
      return "schedule.phaseRestarting";
    default:
      return "schedule.phaseIdle";
  }
}

// トリガー種別 → i18n キー。
export function triggerKey(trigger: string): string {
  switch (trigger) {
    case "manual":
      return "schedule.triggerManual";
    case "scheduled":
      return "schedule.triggerScheduled";
    case "crash":
      return "schedule.triggerCrash";
    default:
      return "schedule.triggerManual";
  }
}

// 進行中フェーズが「中止可能」か（①②③のみ・④restarting は不可）。§3.16(1)。
export function isCancellablePhase(phase: string): boolean {
  return phase === "preparing" || phase === "waiting" || phase === "announcing";
}
