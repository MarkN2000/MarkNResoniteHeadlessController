// MRHC バックエンドの単一HTTP API（/api/v1）クライアント。
// Cookieセッションを使うため credentials:'include'。

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

const API = "/api/v1";

async function req(path: string, init?: RequestInit): Promise<Response> {
  return fetch(API + path, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

export async function login(password: string): Promise<{ ok: boolean; status: number }> {
  const res = await req("/login", { method: "POST", body: JSON.stringify({ password }) });
  return { ok: res.ok, status: res.status };
}

export async function logout(): Promise<void> {
  await req("/logout", { method: "POST" });
}

export async function getStatus(): Promise<Status | null> {
  const res = await req("/status");
  if (!res.ok) return null;
  const j = await res.json();
  return j.data as Status;
}

export async function start(): Promise<void> {
  await req("/start", { method: "POST", body: "{}" });
}

export async function stop(): Promise<void> {
  await req("/stop", { method: "POST" });
}

export async function sendCommand(cmd: string): Promise<void> {
  await req("/command", { method: "POST", body: JSON.stringify({ cmd }) });
}
