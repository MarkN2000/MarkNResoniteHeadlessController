// MRHC バックエンドの単一HTTP API（/api/v1）クライアント。
// Cookieセッションを使うため credentials:'include'。
// レスポンス封筒は {ok:true, data} / {ok:false, error:{code,message}}。

export type State = "stopped" | "starting" | "running" | "stopping";

// Resonite アカウントのログイン状態（起動ログから検出・headless.LoginState のミラー）。
export type ResoniteLoginState = "anonymous" | "loggedIn" | "failed";

export interface Status {
  state: State;
  pid?: number;
  config?: string;
  startedAt?: string;
  ready: boolean;
  loginState?: ResoniteLoginState; // anonymous|loggedIn|failed
  loginUserId?: string; // 例 "U-xxxx"（loggedIn 時のみ・U- 付き）
  fault?: string; // 起動できない致命要因（"duplicate_instance" 等・自動復帰しない）
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

// go.resonite.com のワールド検索結果（internal/resonite.World）。resoniteUrl をそのまま
// startWorldURL に渡して起動する。thumbnailUrl は https 絶対URL（空可）。
export interface WorldResult {
  name: string;
  ownerId: string; // U-xxx または G-xxx
  recordId: string; // R-xxx
  resoniteUrl: string; // resrec:///<owner>/<record>
  thumbnailUrl: string;
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
// init を渡せば POST/DELETE 等でも更新後 data を取り出せる（お気に入り add/remove で利用）。
async function getData<T>(path: string, init?: RequestInit): Promise<T | null> {
  try {
    const res = await req(path, init);
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
// runtimePrepare=true は「.NET ランタイムを設置してから起動する」非同期受付
// （進捗は steam SSE・結果はコンソールの sys ログ）。
export async function start(
  config: string,
): Promise<{ ok: boolean; status: number; error?: string; code?: string; runtimePrepare?: boolean }> {
  const res = await req("/start", { method: "POST", body: JSON.stringify({ config }) });
  if (res.ok) {
    let runtimePrepare: boolean | undefined;
    try {
      const j = await res.json();
      if (j?.data?.runtimePrepare === true) runtimePrepare = true;
    } catch {
      /* ignore */
    }
    return { ok: true, status: res.status, runtimePrepare };
  }
  let error: string | undefined;
  let code: string | undefined;
  try {
    const j = await res.json();
    error = j?.error?.message;
    code = j?.error?.code;
  } catch {
    /* ignore */
  }
  return { ok: false, status: res.status, error, code };
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

// go.resonite.com の公開ワールドを検索（HTML スクレイピング・上位24件）。
// 失敗（不達・構造変化）は getData が null→[] に吸収＝「該当なし」表示になる。
export async function searchResoniteWorlds(q: string): Promise<WorldResult[]> {
  return (await getData<WorldResult[]>(`/resonite/worlds?q=${encodeURIComponent(q)}`)) ?? [];
}

// --- ワールドお気に入り（favorites.json・サーバー保存。add/remove は更新後一覧を返す） ---

// 一覧取得（追加順）。失敗時は空配列。
export async function getFavorites(): Promise<WorldResult[]> {
  return (await getData<WorldResult[]>("/favorites")) ?? [];
}
// 追加（冪等）。更新後一覧を返す。失敗時は null（呼び出し側はローカル状態を維持）。
export async function addFavorite(w: WorldResult): Promise<WorldResult[] | null> {
  return getData<WorldResult[]>("/favorites", { method: "POST", body: JSON.stringify(w) });
}
// 削除（recordId 指定）。更新後一覧を返す。失敗時は null。
export async function removeFavorite(recordId: string): Promise<WorldResult[] | null> {
  return getData<WorldResult[]>(`/favorites/${encodeURIComponent(recordId)}`, { method: "DELETE" });
}

// --- Resonite ログ閲覧（{InstallDir}/Headless/Logs・読み取り専用） ---

// ログファイル一覧の1要素（internal/server.logFileInfo）。modTime は RFC3339。
export interface LogFileInfo {
  name: string;
  size: number;
  modTime: string;
}
export async function getLogFiles(): Promise<LogFileInfo[]> {
  return (await getData<LogFileInfo[]>("/logs")) ?? [];
}

// ログ本文。truncated=true なら末尾10MiBのみ（先頭省略）。
export interface LogContent {
  name: string;
  size: number;
  truncated: boolean;
  content: string;
}
export async function getLogContent(name: string): Promise<LogContent | null> {
  return getData<LogContent>(`/logs/${encodeURIComponent(name)}`);
}

// --- 自己更新（MRHC 自身の入れ替え・docs/design/self-update.md） ---

export interface UpdateInfo {
  current: string; // 実行中の版（dev 等もありうる）
  latest: string; // 最新リリースタグ
  updateAvailable: boolean; // 実行中の版より新しいリリースがあるか
  currentIsRelease: boolean; // 適用可能なリリースビルドか
  staged?: string; // 適用済み・再起動待ちの版
  goos: string; // "windows" | "linux"（再起動手順の出し分け）
  // GitHub への確認に失敗（current/staged/goos のローカル情報のみ有効）。
  // checkError はその errCode（no_release / update_failed 等）。
  checkFailed?: boolean;
  checkError?: string;
}

// 更新チェック。GitHub への問い合わせはこの呼び出し時のみ（常時ポーリングはしない）。失敗時 null。
export async function checkUpdate(): Promise<UpdateInfo | null> {
  return getData<UpdateInfo>("/update/check");
}

// 本体DLの進捗（downloaded/total バイト。total は不明なら -1）。
export interface UpdateProgress {
  downloaded: number;
  total: number;
}

export interface ApplyResult {
  ok: boolean;
  staged?: string;
  error?: string;
  code?: string;
}

// SSE の1イベント（event 名 + パース済み data）を取り出す。ping 行（":..."）は無視。
function parseSSEEvent(chunk: string): { event: string; data: unknown } | null {
  let event = "message";
  let data = "";
  for (const line of chunk.split("\n")) {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) data += line.slice(5).trim();
  }
  if (!data) return null;
  try {
    return { event, data: JSON.parse(data) };
  } catch {
    return null;
  }
}

// 更新の適用（DL→検証→入れ替え・数秒〜十数秒）。進捗を SSE でストリーミングし onProgress で通知する。
// 成功で staged（次回起動からの版）を返す。失敗理由は code で出し分ける（up_to_date/update_busy/
// no_release/not_release_build/exe_dir_not_writable/update_failed・通信不通は network）。
// http.Flusher 非対応のフォールバック（従来 JSON 応答）も受理する。
export async function applyUpdate(onProgress?: (p: UpdateProgress) => void): Promise<ApplyResult> {
  let res: Response;
  try {
    res = await req("/update/apply", { method: "POST" });
  } catch {
    return { ok: false, code: "network" };
  }

  // フォールバック: ストリーミングでない応答は JSON 封筒として読む。
  if (!(res.headers.get("Content-Type") ?? "").includes("text/event-stream") || !res.body) {
    if (res.ok) {
      const j = await res.json().catch(() => null);
      return { ok: true, staged: (j?.data as { staged?: string } | undefined)?.staged };
    }
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
  }

  // SSE をストリーム読み。最終イベント（update-result / update-error）で結果が確定する。
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  let result: ApplyResult | null = null;
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let idx: number;
      while ((idx = buf.indexOf("\n\n")) >= 0) {
        const ev = parseSSEEvent(buf.slice(0, idx));
        buf = buf.slice(idx + 2);
        if (!ev) continue;
        if (ev.event === "update-progress" && onProgress) {
          onProgress(ev.data as UpdateProgress);
        } else if (ev.event === "update-result") {
          result = { ok: true, staged: (ev.data as { staged?: string }).staged };
        } else if (ev.event === "update-error") {
          const d = ev.data as { code?: string; message?: string };
          result = { ok: false, code: d.code, error: d.message };
        }
      }
    }
  } catch {
    if (!result) return { ok: false, code: "network" };
  }
  return result ?? { ok: false, code: "update_failed" };
}

// MRHC プロセスの再起動依頼（自己更新後の「今すぐ再起動」）。応答後にサーバーは graceful 終了し、
// 新バイナリで自分自身を起動し直す。
export function restartApp(): Promise<WriteResult> {
  return write("POST", "/restart");
}

// サーバーが応答可能か（再起動からの復帰検出用）。HTTP 応答が返れば true（401 等のステータスも
// 「起動している」とみなす＝復帰検出には十分）、通信不通なら false。
export async function pingAlive(): Promise<boolean> {
  try {
    await fetch(API + "/status", { credentials: "include", cache: "no-store" });
    return true;
  } catch {
    return false;
  }
}

// --- write 操作（方針A: 成功は {executed:true}・封筒を解いて ok/error を返す）---

// 成功 = {ok:true, data?}。失敗 = {ok:false, error?, code?}。
//   code は backend の error.code（not_ready/timeout/process_gone/exec_failed/bad_request 等）。
//   通信不通など backend に届かない失敗は code:"network"。トーストの出し分けは code を権威にする。
//   data は成功封筒の data（applyUpdate 等、結果値が要る呼び出しだけが読む）。
export interface WriteResult {
  ok: boolean;
  error?: string;
  code?: string;
  data?: unknown;
}

// POST/PUT/DELETE の封筒を解いて WriteResult を返す共通実行（方針A）。
async function write(method: string, path: string, body?: unknown): Promise<WriteResult> {
  try {
    const res = await req(path, { method, body: body !== undefined ? JSON.stringify(body) : undefined });
    if (res.ok) {
      const j = await res.json().catch(() => null);
      return { ok: true, data: j?.data };
    }
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
// ID 指定 BAN（全セッションから・検索結果の userId を使う・R1）。unban と対称。
export const banByID = (userId: string) => post(`/bans/banByID`, { userId });
export const sendFriendRequest = (user: string) => post(`/friends/add`, { user });
export const removeFriend = (user: string) => post(`/friends/remove`, { user });

// 招待（フォーカス中セッションへ・focus 必要）
export const inviteUser = (idx: number, user: string) => post(`/sessions/${idx}/invite`, { user });

// セッション内コンテンツ操作（focus idx・R14）。
//   spawn   → spawn "<url>" <active> <persistent>（アイテムをワールド root に生成）
//   impulse → dynamicimpulsestring "<tag>" "<value>"（scene root へ impulse・tag 必須/value 任意）
export const spawnItem = (idx: number, url: string, active: boolean, persistent: boolean) =>
  post(`/sessions/${idx}/spawn`, { url, active, persistent });
export const sendImpulse = (idx: number, tag: string, value: string) =>
  post(`/sessions/${idx}/impulse`, { tag, value });

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
// 保存（上書き）。body は config 全文 map（未知フィールド含め丸ごと送る）。
// from 指定（≠name）は保存リネーム＝from の内容を name で保存し from を削除（マスク解決も from 側）。
export const saveConfig = (name: string, body: Record<string, unknown>, from?: string) =>
  write(
    "PUT",
    `/headless-configs/${encodeURIComponent(name)}${from ? `?from=${encodeURIComponent(from)}` : ""}`,
    body,
  );
// 新規＝テンプレから即時作成（名前はサーバーが採番: new-config, new-config2, …）。data={name}。
export const createConfig = () => post("/headless-configs");
// 複製＝サーバー側バイトコピー（{元名}-copy, -copy2, …。password も写る）。data={name}。
export const duplicateConfig = (name: string) => post(`/headless-configs/${encodeURIComponent(name)}/duplicate`);
// 削除。
export const deleteConfig = (name: string) => write("DELETE", `/headless-configs/${encodeURIComponent(name)}`);

// --- 設定タブ（7-5）: 中央 Resonite アカウント / アプリ設定 / 管理パスワード変更 ---

// 中央 Resonite アカウントの状態（password は返さず hasPassword のみ）。internal/server/configs.go。
export interface CredentialsInfo {
  username: string;
  hasPassword: boolean;
  userId: string; // username から解決した Resonite UserID（U-xxx・空=未解決・R12）
}
export async function getCredentials(): Promise<CredentialsInfo | null> {
  return getData<CredentialsInfo>("/headless-credentials");
}
// 保存（password 空=既存保持・username のみ更新）。
export const putCredentials = (username: string, password: string) =>
  write("PUT", "/headless-credentials", { username, password });

// 新規 config 雛形に入れる dataFolder/cacheFolder の既定値（{dataDir}/headless-data 等・絶対パス）。
// backend の EnsureDefault（default.json 生成）と同じ導出＝単一情報源 hlconfig.DefaultFolders（UI改善⑤）。
export interface ConfigDefaults {
  dataFolder: string;
  cacheFolder: string;
}
export async function getConfigDefaults(): Promise<ConfigDefaults | null> {
  return getData<ConfigDefaults>("/headless-config-defaults");
}

// アプリ設定（秘密・encoding を含まない公開サブセット）。internal/server/settings.go。
export interface AppSettings {
  port: number;
  headlessConfigDir: string;
}
export async function getAppSettings(): Promise<AppSettings | null> {
  return getData<AppSettings>("/app-settings");
}
export const putAppSettings = (s: AppSettings) => write("PUT", "/app-settings", s);

// 管理パスワード変更（成功時 backend が新Cookieを再発行＝このブラウザは継続・他端末は失効）。
export const changePassword = (currentPassword: string, newPassword: string) =>
  post("/password", { currentPassword, newPassword });

// --- キャッシュ管理（既定 {dataDir}/headless-cache・internal/server/cache.go）---

// 停止時の自動キャッシュ削除設定（既定 OFF・30日）。
export interface CacheConfig {
  enabled: boolean;
  maxAgeDays: number;
}
export async function getCacheConfig(): Promise<CacheConfig | null> {
  return getData<CacheConfig>("/cache/config");
}
export const putCacheConfig = (c: CacheConfig) => write("PUT", "/cache/config", c);

// キャッシュのパスと合計サイズ（サイズ集計は走査するため「サイズを計算」ボタン押下時のみ呼ぶ）。
export interface CacheInfo {
  path: string;
  sizeBytes: number;
  exists: boolean;
}
export async function getCacheInfo(): Promise<CacheInfo | null> {
  return getData<CacheInfo>("/cache/info");
}

// 全キャッシュ削除（停止中のみ・backend が State!=stopped を 409）。
export const clearCache = () => post("/cache/clear");

// --- Steam（DepotDownloader）: Resonite の入手/更新（P9-B）---
// 秘密（password/branchCode）は返さず hasXxx のみ。internal/server/steam.go。
export interface SteamConfig {
  username: string;
  installDir: string;
  hasPassword: boolean;
  hasBranchCode: boolean;
}
export async function getSteamConfig(): Promise<SteamConfig | null> {
  return getData<SteamConfig>("/steam/config");
}
// 保存（password/branchCode 空=既存維持）。
export const putSteamConfig = (body: {
  username: string;
  password: string;
  branchCode: string;
  installDir: string;
}) => write("PUT", "/steam/config", body);

// 更新の進行状態（internal/steam.Status・SSE steam-status / GET /steam/status と同形）。
// 主取得は SSE（steam-status）だが、SSE は満杯時に終端 result を取りこぼし得る（pubsub は非ブロッキング）。
// running 中はこの GET でも状態を突き合わせ、終端（success/failed）の取りこぼしで UI が固着しないようにする（H1）。
export interface SteamStatus {
  state: "idle" | "running" | "success" | "failed";
  runKind?: "update" | "runtime"; // run の種別（runtime=.NET ランタイム単独設置。表示の出し分け用）
  percent: number;
  phase?: string;
  file?: string;
  startedAt?: string;
  finishedAt?: string;
  lastError?: string; // エラー原文（ja・未知 errorCode のフォールバック）
  errorCode?: string; // エラー分類コード（フロントが locale 変換する）
  errorDetail?: string; // 見出し（errorCode）を除いた診断詳細（HTTP 状態・exit 等）
}
export async function getSteamStatus(): Promise<SteamStatus | null> {
  return getData<SteamStatus>("/steam/status");
}
// 入手/更新を非同期開始（停止中のみ・稼働中は 409）。進捗は SSE /steam/events。
export const steamDownload = () => post("/steam/download");
// 進行中の更新を中止。
export const steamCancel = () => post("/steam/cancel");

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
  quietWaitMin: number; // 告知前に静かに待つ（分）
  announceWaitMin: number; // 告知後に待つ（分）。締切 = quiet + announce（2区間モデル・R9）
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
  updateOnScheduledRestart: boolean; // 予定再起動時に Resonite を更新（P9-B・Steam未設定なら no-op）
}

// restart-status の応答（internal/server.restartStatus）。
export interface RestartStatus {
  running: boolean;
  uptimeSeconds: number;
  crashRecoveryEnabled: boolean;
  inProgress: boolean;
  phase: string; // idle | preparing | waiting | announcing | updating | restarting
  restartTriggerType?: string; // manual | scheduled（進行中のみ）
  restartConfigName?: string; // 進行中の対象 config
  deadlineAt: string | null; // ② 待機の締切（RFC3339）
  lastStartAt: string | null; // 最終起動（RFC3339・手動起動/再起動/予定/crash 共通）
  lastStartTrigger?: string; // manual | scheduled | crash
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

// マシン全体のシステム使用率（スケジュールタブの「システム使用率」カード）。
// supported=false は非対応OS/未サンプル。diskTotalBytes=0 はディスク取得不可（UI は「—」）。
export interface SystemMetrics {
  supported: boolean;
  cpuPercent: number;
  memUsedBytes: number;
  memTotalBytes: number;
  memPercent: number;
  diskFreeBytes: number;
  diskTotalBytes: number;
}
export async function getSystemMetrics(): Promise<SystemMetrics | null> {
  return getData<SystemMetrics>("/system/metrics");
}

// 手動「通常再起動」を受付（configName 空=前回 config）。
export const triggerRestart = (configName?: string) =>
  post("/restart/trigger", { configName: configName ?? "" });
// 進行中の再起動を中止（①②③のみ）。通常停止の中止にも共用。
export const cancelRestart = () => post("/restart/cancel");
// 通常停止（事前アクション→2分→停止・R7）。orchestrator 統一フローを終端=停止で流用。
export const gracefulStop = () => post("/stop/graceful");
