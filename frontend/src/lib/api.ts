// 開発環境では直接バックエンドに接続、本番環境では相対パスを使用
const getApiBase = () => {
  // 環境変数が設定されている場合はそれを使用（開発環境）
  if (import.meta.env.VITE_BACKEND_URL) {
    return import.meta.env.VITE_BACKEND_URL;
  }
  
  // ブラウザ環境の場合、相対パスを使用（本番環境）
  if (typeof window !== 'undefined') {
    return '/api';
  }
  
  // フォールバック（SSR等）
  return 'http://localhost:8080/api';
};

const API_BASE = getApiBase();

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

export interface SystemMetrics {
  cpu: {
    usage: number;
  };
  memory: {
    total: number;
    used: number;
    usage: number;
  };
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
export const generateConfig = (name: string, configData?: any, overwrite?: boolean) =>
  request('/server/configs/generate', {
    method: 'POST',
    body: JSON.stringify({ name, configData, overwrite: Boolean(overwrite) })
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

export interface BanEntry {
  index: number;
  username: string;
  userId: string;
  machineIds: string;
}
export interface BansData {
  raw: string;
  data: BanEntry[];
}
export const getBans = () => request('/server/runtime/bans') as Promise<BansData>;

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

// セキュリティ関連API
export const getSecurityConfig = () => 
  request('/security/config') as Promise<{ allowedCidrs: string[] }>;

export const getClientInfo = () => 
  request('/security/client-info') as Promise<{ 
    clientIp: string; 
    isAllowed: boolean; 
    userAgent: string;
    forwardedFor?: string;
    realIp?: string;
  }>;

// Resonite ユーザー情報取得API
export interface ResoniteUserFull {
  id: string;
  username: string | null;
  profile: {
    iconUrl: string | null;
    displayBadges: string[];
    description: string | null;
  };
  registrationTime: string | null;
  isPatreonSupporter: boolean;
  tags: string[];
}

export const getResoniteUserFull = (identifier: string) =>
  request(`/server/resonite-user-full/${encodeURIComponent(identifier)}`) as Promise<ResoniteUserFull>;

export interface ResoniteUsersSearchResponse {
  users: ResoniteUserFull[];
  count: number;
}

export const searchResoniteUsers = (username: string) =>
  request(`/server/resonite-users-search?name=${encodeURIComponent(username)}`) as Promise<ResoniteUsersSearchResponse>;

export interface ResoniteUserIdResponse {
  userid: string;
}

export const getResoniteUserId = (username: string) =>
  request(`/server/resonite-user/${encodeURIComponent(username)}`) as Promise<ResoniteUserIdResponse>;

// ============================================================
// 自動再起動機能のAPI
// ============================================================

export interface ScheduledRestartEntry {
  id: string;
  enabled: boolean;
  type: 'once' | 'weekly' | 'daily';
  specificDate?: {
    year: number;
    month: number;
    day: number;
    hour: number;
    minute: number;
  };
  weeklyDay?: number;
  weeklyTime?: {
    hour: number;
    minute: number;
  };
  dailyTime?: {
    hour: number;
    minute: number;
  };
  configFile: string;
}

export interface RestartConfig {
  triggers: {
    scheduled: {
      enabled: boolean;
      schedules: ScheduledRestartEntry[];
    };
    highLoad: {
      enabled: boolean;
      cpuThreshold: number;
      memoryThreshold: number;
      durationMinutes: number;
    };
    userZero: {
      enabled: boolean;
      minUptimeMinutes: number;
    };
  };
  preRestartActions: {
    waitControl: {
      waitForZeroUsers: number;
      forceRestartTimeout: number;
      actionTiming: number;
    };
    chatMessage: {
      enabled: boolean;
      message: string;
    };
    itemSpawn: {
      enabled: boolean;
      itemType: string;
      itemUrl: string;
      message: string;
    };
    sessionChanges: {
      setPrivate: boolean;
      setMaxUserToOne: boolean;
      changeSessionName: {
        enabled: boolean;
        newName: string;
      };
    };
  };
  failsafe: {
    retryCount: number;
    retryIntervalSeconds: number;
  };
}

export interface RestartStatus {
  nextScheduledRestart: {
    scheduleId: string | null;
    datetime: string | null;
    configFile: string | null;
  };
  currentUptime: number;
  lastRestart: {
    timestamp: string | null;
    trigger: 'scheduled' | 'highLoad' | 'userZero' | 'manual' | 'forced' | null;
    scheduleId?: string;
  };
  highLoadTriggerDisabledUntil: string | null;
  restartInProgress: boolean;
  waitingForUsers: boolean;
  scheduledRestartPreparing: {
    preparing: boolean;
    scheduleId: string | null;
    scheduledTime: string | null;
    configFile: string | null;
  };
}

/**
 * 再起動設定を取得
 */
export const getRestartConfig = () =>
  request('/restart/config') as Promise<RestartConfig>;

/**
 * 再起動設定を保存
 */
export const saveRestartConfig = (config: RestartConfig) =>
  request('/restart/config', {
    method: 'POST',
    body: JSON.stringify(config)
  }) as Promise<{ success: boolean; message: string }>;

/**
 * 再起動ステータスを取得
 */
export const getRestartStatus = () =>
  request('/restart/status') as Promise<RestartStatus>;

/**
 * 手動で再起動をトリガー
 */
export const triggerRestart = (type: 'manual' | 'forced') =>
  request('/restart/trigger', {
    method: 'POST',
    body: JSON.stringify({ type })
  }) as Promise<{ success: boolean; message: string }>;

/**
 * 再起動設定をデフォルトにリセット
 */
export const resetRestartConfig = () =>
  request('/restart/config/reset', {
    method: 'POST'
  }) as Promise<{ success: boolean; message: string; config: RestartConfig }>;

// HeadlessCredentials取得API
export interface HeadlessCredentialsResponse {
  username: string;
}

export const getHeadlessCredentials = () =>
  request('/server/headless-credentials') as Promise<HeadlessCredentialsResponse>;