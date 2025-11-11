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

export interface ParsedStatus {
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
}

export const parseStatusOutput = (output: string): ParsedStatus => {
  const lines = output
    .split(/\n+/)
    .map(line => line.trim())
    .filter(Boolean);

  const map: Record<string, string> = {};
  for (const line of lines) {
    const match = line.match(/^([^:]+):\s*(.*)$/);
    if (!match) continue;
    const [, rawKey, rawValue] = match;
    if (!rawKey || !rawValue) continue;
    map[rawKey.trim()] = rawValue.trim();
  }

  const parseNumber = (value?: string) => {
    if (!value) return undefined;
    const num = Number(value.replace(/[^0-9.-]/g, ''));
    return Number.isNaN(num) ? undefined : num;
  };

  const parseBoolean = (value?: string) => {
    if (!value) return undefined;
    if (value.toLowerCase() === 'true') return true;
    if (value.toLowerCase() === 'false') return false;
    return undefined;
  };

  const tags = map['Tags']
    ? map['Tags'].split(',').map(s => s.trim()).filter(Boolean)
    : [];

  const users = map['Users']
    ? map['Users'].split(',').map(s => s.trim()).filter(Boolean)
    : [];

  return {
    name: map['Name'],
    sessionId: map['SessionID'],
    currentUsers: parseNumber(map['Current Users']),
    presentUsers: parseNumber(map['Present Users']),
    maxUsers: parseNumber(map['Max Users']),
    uptime: map['Uptime'],
    accessLevel: map['Access Level'],
    hiddenFromListing: parseBoolean(map['Hidden from listing']),
    mobileFriendly: parseBoolean(map['Mobile Friendly']),
    description: map['Description'] ?? '',
    tags,
    users
  };
};

/**
 * inviteコマンドの出力をパース
 * 成功時: "Invite sent!"
 */
export const parseInviteOutput = (output: string): { success: boolean; message: string } => {
  const lowerOutput = output.toLowerCase();
  if (lowerOutput.includes('invite sent!')) {
    return { success: true, message: 'Invite sent!' };
  }
  return { success: false, message: output.trim() || 'Failed to send invite' };
};

/**
 * accesslevelコマンドの出力をパース
 * 成功時: "World セッション１ now has access level Private"
 */
export const parseAccessLevelOutput = (output: string): { success: boolean; message: string; accessLevel?: string } => {
  const match = output.match(/now has access level\s+(\S+)/i);
  if (match) {
    return {
      success: true,
      message: output.trim(),
      accessLevel: match[1]
    };
  }
  return { success: false, message: output.trim() || 'Failed to change access level' };
};

/**
 * roleコマンドの出力をパース
 * 成功時: "MarkN now has role Admin!"
 */
export const parseRoleOutput = (output: string): { success: boolean; message: string; username?: string; role?: string } => {
  const match = output.match(/(\S+)\s+now has role\s+(\S+)/i);
  if (match) {
    return {
      success: true,
      message: output.trim(),
      username: match[1],
      role: match[2].replace(/[!.]/g, '') // "Admin!" -> "Admin"
    };
  }
  return { success: false, message: output.trim() || 'Failed to change role' };
};

/**
 * sessionURLコマンドの出力をパース
 * 例: res-steam://76561198384468054/0/S-aac647b6-241c-47ae-b5d7-29365df96c24
 */
export const parseSessionUrlOutput = (output: string): { sessionUrl?: string; sessionId?: string } => {
  // res-steam://, resrec://, ressession:// などのURLを検出
  const urlMatch = output.match(/(res[-\w]*:\/\/[^\s]+)/i);
  if (urlMatch) {
    const sessionUrl = urlMatch[1];
    // SessionIDを抽出 (S-で始まるID)
    const sessionIdMatch = sessionUrl.match(/S-([a-f0-9-]+)/i);
    const sessionId = sessionIdMatch ? `S-${sessionIdMatch[1]}` : undefined;
    return { sessionUrl, sessionId };
  }
  return {};
};

