// MRHC バックエンドの単一HTTP API（/api/v1）クライアント。
// Cookieセッションを使うため credentials:'include'。
// レスポンス封筒は {ok:true, data} / {ok:false, error:{code,message}}。

export type State = "stopped" | "starting" | "running" | "stopping";

export interface Status {
  state: State;
  pid?: number;
  config?: string;
  startedAt?: string;
  ready: boolean;
}

export interface LogLine {
  seq: number;
  time: string;
  kind: string; // out | err | sys | cmd
  text: string;
}

// worlds 一覧の1要素（internal/headless.World）。
export interface World {
  index: number;
  name: string;
  users: number;
  present: number;
  accessLevel: string;
  maxUsers: number;
}

// headless config 一覧の1要素（internal/hlconfig.Summary）。
export interface ConfigSummary {
  name: string;
  comment: string;
  worldCount: number;
}

// フォーカス中セッションの詳細（internal/headless.WorldStatus）。
export interface WorldStatus {
  name: string;
  sessionId: string;
  currentUsers: number;
  presentUsers: number;
  maxUsers: number;
  uptime: string;
  accessLevel: string;
  hiddenFromListing: boolean;
  mobileFriendly: boolean;
  description: string;
  tags: string[];
  users: string[];
  resoniteLink: string;
}

// セッション内ユーザー1人（internal/headless.UserInfo）。
export interface UserInfo {
  name: string;
  id: string;
  role: string;
  present: boolean;
  pingMs: number;
  fps: number;
  silenced: boolean;
}

// listbans の1件（internal/headless.BanEntry）。
export interface BanEntry {
  index: number;
  username: string;
  userId: string;
  machineIds: string[];
}

// Resonite 公開API のユーザー検索結果（internal/resonite.User）。iconUrl は https 正規化済（空可）。
export interface ResoniteUser {
  id: string;
  username: string;
  iconUrl: string;
}

// UI ドロップダウン用の enum 候補（値の権威は Resonite・サーバーは値検証しない）。docs §2.4。
export const ACCESS_LEVELS = [
  "Private",
  "LAN",
  "Contacts",
  "ContactsPlus",
  "RegisteredUsers",
  "Anyone",
] as const;
export const ROLES = ["Admin", "Builder", "Moderator", "Guest", "Spectator"] as const;

// 新規セッションのテンプレート候補（v1 踏襲。値の権威は Resonite・他名が使えるかは要実機採取）。
export const WORLD_TEMPLATES = ["Grid", "Platform", "Blank"] as const;

const API = "/api/v1";

async function req(path: string, init?: RequestInit): Promise<Response> {
  return fetch(API + path, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

// 封筒から data を取り出す。失敗（HTTP エラー / ネットワーク不通）時は null。
async function getData<T>(path: string): Promise<T | null> {
  try {
    const res = await req(path);
    if (!res.ok) return null;
    const j = await res.json();
    return (j.data ?? null) as T | null;
  } catch {
    return null; // バックエンド未応答などは未認証扱いでログイン画面へ
  }
}

export async function login(password: string): Promise<{ ok: boolean; status: number }> {
  const res = await req("/login", { method: "POST", body: JSON.stringify({ password }) });
  return { ok: res.ok, status: res.status };
}

export async function logout(): Promise<void> {
  await req("/logout", { method: "POST" });
}

export async function getStatus(): Promise<Status | null> {
  return getData<Status>("/status");
}

// 起動はコンフィグ名必須（無config起動はワールドが公開になるため backend が 400）。
export async function start(config: string): Promise<{ ok: boolean; status: number; error?: string }> {
  const res = await req("/start", { method: "POST", body: JSON.stringify({ config }) });
  if (res.ok) return { ok: true, status: res.status };
  let error: string | undefined;
  try {
    const j = await res.json();
    error = j?.error?.message;
  } catch {
    /* ignore */
  }
  return { ok: false, status: res.status, error };
}

export async function stop(): Promise<void> {
  await req("/stop", { method: "POST" });
}

export async function sendCommand(cmd: string): Promise<void> {
  await req("/command", { method: "POST", body: JSON.stringify({ cmd }) });
}

export async function getSessions(): Promise<World[]> {
  return (await getData<World[]>("/sessions")) ?? [];
}

export async function getConfigs(): Promise<ConfigSummary[]> {
  return (await getData<ConfigSummary[]>("/headless-configs")) ?? [];
}

export async function getLastUsedConfig(): Promise<string> {
  const d = await getData<{ lastUsed: string }>("/headless-configs/last-used");
  return d?.lastUsed ?? "";
}

// --- セッション（フォーカス中 idx）の取得 ---

// status + users を1回の取得（ExecGroup(focus→status→users)）で返す（B1）。
// 個別取得が要れば backend の /sessions/{idx}/status・/users が使える（部分再取得は 7-7 で）。
export interface SessionDetail {
  status: WorldStatus;
  users: UserInfo[];
}
export async function getSessionDetail(idx: number): Promise<SessionDetail | null> {
  return getData<SessionDetail>(`/sessions/${idx}/detail`);
}

// --- フレンド / BAN（グローバル取得・focus 不要）---

// 受信中フレンドリクエストのユーザー名一覧（incoming pending のみ）。
export async function getFriendRequests(): Promise<string[]> {
  return (await getData<string[]>("/friendrequests")) ?? [];
}

// BAN 一覧（listbans）。
export async function getListBans(): Promise<BanEntry[]> {
  return (await getData<BanEntry[]>("/listbans")) ?? [];
}

// Resonite 公開API でユーザー検索（q が "U-" 始まりなら ID 検索・それ以外は名前検索）。
export async function searchResoniteUsers(q: string): Promise<ResoniteUser[]> {
  return (await getData<ResoniteUser[]>(`/resonite/users?q=${encodeURIComponent(q)}`)) ?? [];
}

// --- write 操作（方針A: 成功は {executed:true}・封筒を解いて ok/error を返す）---

// 成功 = {ok:true}。失敗 = {ok:false, error?, code?}。
//   code は backend の error.code（not_ready/timeout/process_gone/exec_failed/bad_request 等）。
//   通信不通など backend に届かない失敗は code:"network"。トーストの出し分けは code を権威にする。
export interface WriteResult {
  ok: boolean;
  error?: string;
  code?: string;
}

// POST/PUT/DELETE の封筒を解いて WriteResult を返す共通実行（方針A）。
async function write(method: string, path: string, body?: unknown): Promise<WriteResult> {
  try {
    const res = await req(path, { method, body: body !== undefined ? JSON.stringify(body) : undefined });
    if (res.ok) return { ok: true };
    let error: string | undefined;
    let code: string | undefined;
    try {
      const j = await res.json();
      error = j?.error?.message;
      code = j?.error?.code;
    } catch {
      /* ignore */
    }
    return { ok: false, error, code };
  } catch {
    return { ok: false, code: "network" };
  }
}

function post(path: string, body?: unknown): Promise<WriteResult> {
  return write("POST", path, body);
}

// セッション設定
export const setSessionName = (idx: number, name: string) => post(`/sessions/${idx}/name`, { name });
export const setAccessLevel = (idx: number, level: string) => post(`/sessions/${idx}/accesslevel`, { level });
export const setMaxUsers = (idx: number, maxUsers: number) => post(`/sessions/${idx}/maxusers`, { maxUsers });
export const setDescription = (idx: number, description: string) =>
  post(`/sessions/${idx}/description`, { description });
export const setHideFromListing = (idx: number, hide: boolean) =>
  post(`/sessions/${idx}/hidefromlisting`, { hide });

// セッションライフサイクル
export const saveSession = (idx: number) => post(`/sessions/${idx}/save`);
export const restartSession = (idx: number) => post(`/sessions/${idx}/restart`);
export const closeSession = (idx: number) => post(`/sessions/${idx}/close`);

// セッション内ユーザー操作
export const kickUser = (idx: number, user: string) => post(`/sessions/${idx}/kick`, { user });
export const banUser = (idx: number, user: string) => post(`/sessions/${idx}/ban`, { user });
export const silenceUser = (idx: number, user: string) => post(`/sessions/${idx}/silence`, { user });
export const unsilenceUser = (idx: number, user: string) => post(`/sessions/${idx}/unsilence`, { user });
export const respawnUser = (idx: number, user: string) => post(`/sessions/${idx}/respawn`, { user });
export const setUserRole = (idx: number, user: string, role: string) =>
  post(`/sessions/${idx}/role`, { user, role });
export const messageUser = (idx: number, user: string, message: string) =>
  post(`/sessions/${idx}/message`, { user, message });

// フレンド / BAN（グローバル・focus 不要）
export const acceptFriendRequest = (user: string) => post(`/friendrequests/accept`, { user });
export const unban = (userId: string) => post(`/bans/unban`, { userId });
export const sendFriendRequest = (user: string) => post(`/friends/add`, { user });
export const removeFriend = (user: string) => post(`/friends/remove`, { user });

// 招待（フォーカス中セッションへ・focus 必要）
export const inviteUser = (idx: number, user: string) => post(`/sessions/${idx}/invite`, { user });

// 新規セッション（稼働中に新ワールドを開始・focus 不要・backend timeout 60s）。
//   url      → startworldurl "<url>"      / template → startWorldTemplate "<name>"
// /start（プロセス起動）とは別物。結果は方針A で {executed:true}＝起動後に一覧を再取得して実状態を見せる。
export const startWorldURL = (url: string) => post(`/sessions/start`, { mode: "url", url });
export const startWorldTemplate = (template: string) => post(`/sessions/start`, { mode: "template", template });

// --- Headless Config CRUD（コンフィグタブ・§3.14・バックエンドは Pre-7b 実装済）---
// 全文取得（loginPassword は backend で "" マスク済）。不正JSON/未存在/通信不通は null。
export async function getConfig(name: string): Promise<Record<string, unknown> | null> {
  return getData<Record<string, unknown>>(`/headless-configs/${encodeURIComponent(name)}`);
}
// 保存（新規/上書き = upsert）。body は config 全文 map（未知フィールド含め丸ごと送る）。
export const saveConfig = (name: string, body: Record<string, unknown>) =>
  write("PUT", `/headless-configs/${encodeURIComponent(name)}`, body);
// 削除。
export const deleteConfig = (name: string) => write("DELETE", `/headless-configs/${encodeURIComponent(name)}`);

// --- 設定タブ（7-5）: 中央 Resonite アカウント / アプリ設定 / 管理パスワード変更 ---

// 中央 Resonite アカウントの状態（password は返さず hasPassword のみ）。internal/server/configs.go。
export interface CredentialsInfo {
  username: string;
  hasPassword: boolean;
}
export async function getCredentials(): Promise<CredentialsInfo | null> {
  return getData<CredentialsInfo>("/headless-credentials");
}
// 保存（password 空=既存保持・username のみ更新）。
export const putCredentials = (username: string, password: string) =>
  write("PUT", "/headless-credentials", { username, password });

// アプリ設定（秘密・encoding を含まない公開サブセット）。internal/server/settings.go。
export interface AppSettings {
  port: number;
  resoniteHeadlessPath: string;
  headlessConfigDir: string;
}
export async function getAppSettings(): Promise<AppSettings | null> {
  return getData<AppSettings>("/app-settings");
}
export const putAppSettings = (s: AppSettings) => write("PUT", "/app-settings", s);

// 管理パスワード変更（成功時 backend が新Cookieを再発行＝このブラウザは継続・他端末は失効）。
export const changePassword = (currentPassword: string, newPassword: string) =>
  post("/password", { currentPassword, newPassword });

// --- スケジュール（自動再起動）タブ（Phase 8・§3.16）---
// restart 設定は単一オブジェクト（config.Restart のミラー）。完全オブジェクトを PUT する（pointer 設計前提）。

// 告知アイテムのテンプレート（v1 main の登録 2 種を踏襲）。選択で itemUrl を設定。
// 受信タグは全テンプレ共通（v1 restartManager の固定値）＝下の ANNOUNCE_COMMON_TAG。
export const ANNOUNCE_TEMPLATES = [
  { label: "とらぞセッション閉店アナウンス", url: "resrec:///U-MarkN/R-ba48e002-7810-43b6-b12d-41e68863d5c4" },
  { label: "テキスト読み上げ", url: "resrec:///U-MarkN/R-47c7c916-1e47-470d-abae-9e7c22315743" },
] as const;
export const ANNOUNCE_COMMON_TAG = "MRHC.play";

export type RestartType = "once" | "weekly" | "daily";

export interface ScheduledRestart {
  id: string;
  enabled: boolean;
  type: RestartType;
  year?: number; // once のみ
  month?: number; // once のみ（1-12）
  day?: number; // once のみ（1-31）
  weekday: number; // weekly のみ（0=日..6=土）
  hour: number; // 0-23
  minute: number; // 0-59
  configName: string; // 空=前回config
}
export interface RestartWaitControl {
  forceRestartTimeoutMin: number;
  actionTimingMin: number;
}
export interface RestartAnnounce {
  enabled: boolean;
  itemUrl: string;
  impulseTag: string;
  message: string;
}
export interface RestartSessionChanges {
  setPrivate: boolean;
  setMaxUsersOne: boolean;
  renameEnabled: boolean;
  renameTo: string;
}
export interface RestartCrashRecovery {
  enabled: boolean;
  maxCrashes: number;
  windowMinutes: number;
}
export interface RestartConfig {
  scheduled: ScheduledRestart[];
  waitControl: RestartWaitControl;
  preActions: { announce: RestartAnnounce; sessionChanges: RestartSessionChanges };
  crashRecovery: RestartCrashRecovery;
}

// restart-status の応答（internal/server.restartStatus）。
export interface RestartStatus {
  running: boolean;
  uptimeSeconds: number;
  crashRecoveryEnabled: boolean;
  inProgress: boolean;
  phase: string; // idle | preparing | waiting | announcing | restarting
  restartTriggerType?: string; // manual | scheduled（進行中のみ）
  restartConfigName?: string; // 進行中の対象 config
  deadlineAt: string | null; // ② 待機の締切（RFC3339）
  lastRestartAt: string | null; // 最終再起動（RFC3339）
  lastRestartTrigger?: string; // manual | scheduled | crash
  nextScheduledAt: string | null; // 次回予定（RFC3339）
  nextScheduledConfigName: string;
  nextScheduledId: string;
  nextScheduledType: string; // once | weekly | daily
}

export async function getRestartConfig(): Promise<RestartConfig | null> {
  return getData<RestartConfig>("/restart-config");
}
export const putRestartConfig = (rc: RestartConfig) => write("PUT", "/restart-config", rc);

export async function getRestartStatus(): Promise<RestartStatus | null> {
  return getData<RestartStatus>("/restart-status");
}

// 手動「通常再起動」を受付（configName 空=前回 config）。
export const triggerRestart = (configName?: string) =>
  post("/restart/trigger", { configName: configName ?? "" });
// 進行中の再起動を中止（①②③のみ）。
export const cancelRestart = () => post("/restart/cancel");
