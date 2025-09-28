const API_BASE = import.meta.env.VITE_BACKEND_URL || 'http://localhost:8080/api';

export interface HeadlessStatus {
  running: boolean;
  pid?: number;
  configPath?: string;
  startedAt?: string;
  exitCode?: number | null;
  signal?: string | null;
}

export interface LogEntry {
  id: number;
  timestamp: string;
  level: 'stdout' | 'stderr';
  message: string;
}

export type { ConfigEntry };

async function request(path: string, init?: RequestInit) {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: {
      'Content-Type': 'application/json'
    },
    ...init
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? res.statusText);
  }
  return res.json();
}

export const getStatus = () => request('/server/status') as Promise<HeadlessStatus>;
export const getLogs = (limit?: number) => request(`/server/logs${limit ? `?limit=${limit}` : ''}`) as Promise<LogEntry[]>;
export const getConfigs = () => request('/server/configs') as Promise<ConfigEntry[]>;
export const startServer = (configPath?: string) =>
  request('/server/start', {
    method: 'POST',
    body: JSON.stringify({ configPath })
  });
export const stopServer = () =>
  request('/server/stop', {
    method: 'POST'
  });

export interface RuntimeStatusData {
  raw: string;
  data: {
    name?: string;
    sessionId?: string;
    currentUsers?: number;
    presentUsers?: number;
    maxUsers?: number;
    uptime?: string;
    accessLevel?: string;
    hiddenFromListing?: boolean;
    mobileFriendly?: boolean;
    description?: string;
    tags: string[];
    users: string[];
  };
}

export interface RuntimeUserEntry {
  name: string;
  id: string;
  role: string;
  present: boolean;
  pingMs: number;
  fps: number;
  silenced: boolean;
}

export interface RuntimeUsersData {
  raw: string;
  data: RuntimeUserEntry[];
}

export const getRuntimeStatus = () => request('/server/runtime/status') as Promise<RuntimeStatusData>;
export const getRuntimeUsers = () => request('/server/runtime/users') as Promise<RuntimeUsersData>;
