export interface ServerStatus {
  running: boolean;
  profile: string | null;
  pid?: number;
}

export interface LogEntry {
  timestamp: string;
  level: 'info' | 'warn' | 'error';
  message: string;
}
