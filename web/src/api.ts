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

async function post(path: string, body?: unknown): Promise<WriteResult> {
  try {
    const res = await req(path, { method: "POST", body: body ? JSON.stringify(body) : undefined });
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
