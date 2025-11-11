import type { LogEntry } from '../../services/logBuffer.js';

/**
 * コマンド出力を文字列に変換（ログエントリから）
 */
export const logEntriesToString = (entries: LogEntry[]): string => {
  return entries
    .map(entry => entry.message)
    .map(message => message.replace(/\r/g, ''))
    .map(message => message.trimEnd())
    .filter(message => {
      const trimmed = message.trim();
      if (!trimmed) return false;
      if (trimmed.startsWith('>')) return false; // コマンドエコーを除外
      return true;
    })
    .join('\n')
    .trim();
};

/**
 * worldsコマンドの出力をパース
 * 例: [0] セッション１                          Users: 2	Present: 1	AccessLevel: LAN	MaxUsers: 16
 */
export interface ParsedSession {
  index: number;
  name: string;
  users: number;
  present: number;
  accessLevel: string;
  maxUsers: number;
}

export const parseWorldsOutput = (output: string): ParsedSession[] => {
  const lines = output.split(/\r?\n/);
  const sessions: ParsedSession[] = [];

  // [0] セッション名    Users: 2	Present: 1	AccessLevel: LAN	MaxUsers: 16
  const regex = /^\[(?<index>\d+)\]\s+(?<name>.+?)\s+Users:\s+(?<users>\d+)[\s\t]+Present:\s+(?<present>\d+)[\s\t]+AccessLevel:\s+(?<access>\S+)[\s\t]+MaxUsers:\s+(?<max>\d+)/i;

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;

    const match = trimmed.match(regex);
    if (match?.groups) {
      sessions.push({
        index: Number(match.groups.index),
        name: match.groups.name.trim(),
        users: Number(match.groups.users),
        present: Number(match.groups.present),
        accessLevel: match.groups.access,
        maxUsers: Number(match.groups.max)
      });
    }
  }

  return sessions;
};

