const API_BASE = import.meta.env.VITE_BACKEND_URL || 'http://localhost:8080/api';

// 認証トークンの管理
let authToken: string | null = null;

// ブラウザ環境でのみlocalStorageを使用
const getStoredToken = (): string | null => {
  if (typeof window !== 'undefined' && window.localStorage) {
    return localStorage.getItem('authToken');
  }
  return null;
};

const setStoredToken = (token: string | null) => {
  if (typeof window !== 'undefined' && window.localStorage) {
    if (token) {
      localStorage.setItem('authToken', token);
    } else {
      localStorage.removeItem('authToken');
    }
  }
};

// 初期化時にlocalStorageからトークンを読み込み
if (typeof window !== 'undefined') {
  authToken = getStoredToken();
}

export const setAuthToken = (token: string | null) => {
  authToken = token;
  setStoredToken(token);
};

export const getAuthToken = () => authToken;

export interface HeadlessStatus {
  running: boolean;
  pid?: number;
  configPath?: string;
  startedAt?: string;
  exitCode?: number | null;
  signal?: string | null;
  userName?: string;
  userId?: string;
}

export interface LogEntry {
  id: number;
  timestamp: string;
  level: 'stdout' | 'stderr';
  message: string;
}

export interface ConfigEntry {
  path: string;
  name: string;
}

async function request(path: string, init?: RequestInit) {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  };

  // 認証トークンがある場合は追加
  if (authToken) {
    headers['Authorization'] = `Bearer ${authToken}`;
  }

  const res = await fetch(`${API_BASE}${path}`, {
    headers,
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
export const generateConfig = (name: string, username: string, password: string, configData?: any, overwrite?: boolean) =>
  request('/server/configs/generate', {
    method: 'POST',
    body: JSON.stringify({ name, username, password, configData, overwrite: Boolean(overwrite) })
  }) as Promise<{ ok: boolean; path: string; name: string }>;
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

export interface FriendRequestsData {
  raw: string;
  data: string[];
}

export interface RuntimeWorldEntry {
  name: string;
  sessionId: string;
  currentUsers?: number;
  presentUsers?: number;
  maxUsers?: number;
  accessLevel?: string;
  hiddenFromListing?: boolean;
  focusTarget: string;
  focused: boolean;
  raw: string;
}

export interface RuntimeWorldsData {
  raw: string;
  data: RuntimeWorldEntry[];
  focusedSessionId?: string | null;
  focusedSessionName?: string | null;
  focusedFocusTarget?: string | null;
}

export const getRuntimeStatus = () => request('/server/runtime/status') as Promise<RuntimeStatusData>;
export const getRuntimeUsers = () => request('/server/runtime/users') as Promise<RuntimeUsersData>;
export const getFriendRequests = () => request('/server/runtime/friend-requests') as Promise<FriendRequestsData>;
export const getRuntimeWorlds = () => request('/server/runtime/worlds') as Promise<RuntimeWorldsData>;
export const postFocusWorld = (sessionId: string) =>
  request('/server/runtime/worlds/focus', {
    method: 'POST',
    body: JSON.stringify({ sessionId })
  }) as Promise<{ raw: string }>;

export interface FocusRefreshResponse {
  worlds: RuntimeWorldsData;
  status: RuntimeStatusData;
  users: RuntimeUsersData;
}

export const postFocusWorldRefresh = (sessionId: string) =>
  request('/server/runtime/worlds/focus-refresh', {
    method: 'POST',
    body: JSON.stringify({ sessionId })
  }) as Promise<FocusRefreshResponse>;

export const postCommand = (command: string) =>
  request('/server/runtime/command', {
    method: 'POST',
    body: JSON.stringify({ command })
  }) as Promise<{ raw: string }>;

// World Search API
export interface WorldSearchItem {
  name: string;
  imageUrl: string | null;
  recordId: string;
  resoniteUrl: string;
}
export interface WorldSearchResponse {
  items: WorldSearchItem[];
}
export const getWorldSearch = (term: string) => request(`/server/world-search?term=${encodeURIComponent(term)}`) as Promise<WorldSearchResponse>;

// 認証関連API
export const login = (password: string) => 
  request('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ password })
  }) as Promise<{ token: string; message: string }>;

export const verifyAuth = () => 
  request('/auth/verify') as Promise<{ authenticated: boolean; user: any }>;

export const logout = () => 
  request('/auth/logout', { method: 'POST' }) as Promise<{ message: string }>;
