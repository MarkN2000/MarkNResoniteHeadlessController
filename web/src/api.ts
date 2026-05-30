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
