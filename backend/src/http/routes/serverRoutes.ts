import path from 'node:path';
import { Router } from 'express';
import { processManager } from '../../services/processManager.js';

export const serverRoutes = Router();

serverRoutes.get('/status', (_req, res) => {
  res.json(processManager.getStatus());
});

serverRoutes.get('/logs', (req, res) => {
  const limit = req.query.limit ? Number(req.query.limit) : undefined;
  res.json(processManager.getLogs(limit));
});

serverRoutes.get('/configs', (_req, res) => {
  const configs = processManager.listConfigs().map(filePath => ({
    path: filePath,
    name: path.basename(filePath)
  }));
  res.json(configs);
});

serverRoutes.post('/start', (req, res, next) => {
  try {
    const { configPath } = req.body ?? {};
    processManager.start(configPath);
    res.json({ ok: true });
  } catch (error) {
    next(error);
  }
});

serverRoutes.post('/stop', async (_req, res, next) => {
  try {
    await processManager.stop();
    res.json({ ok: true });
  } catch (error) {
    next(error);
  }
});

const runCommand = async (command: string) => {
  const logs = await processManager.executeCommand(command, 4000);
  return logs
    .map(entry => entry.message)
    .map(message => message.replace(/\r/g, ''))
    .map(message => message.trimEnd())
    .filter(message => {
      const trimmed = message.trim();
      if (!trimmed) return false;
      if (trimmed.startsWith('>')) return false;
      if (!trimmed.includes(':') && trimmed.endsWith('>')) return false;
      return true;
    })
    .join('\n')
    .trim();
};

const parseStatusOutput = (output: string) => {
  const lines = output
    .split(/\n+/)
    .map(line => line.trim())
    .filter(Boolean);

  const map: Record<string, string> = {};
  for (const line of lines) {
    const match = line.match(/^([^:]+):\s*(.*)$/);
    if (!match) continue;
    const [, rawKey, rawValue] = match;
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
    ? map['Tags']
        .split(',')
        .map(tag => tag.trim())
        .filter(Boolean)
    : [];

  const users = map['Users']
    ? map['Users']
        .split(',')
        .map(user => user.trim())
        .filter(Boolean)
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

const usersLineRegex = /^(?<name>\S+)\s+ID:\s+(?<id>\S+)\s+Role:\s+(?<role>\S+)\s+Present:\s+(?<present>True|False)\s+Ping:\s+(?<ping>[0-9.]+)\s+ms\s+FPS:\s+(?<fps>[0-9.]+)\s+Silenced:\s+(?<silenced>True|False)$/i;

const parseUsersOutput = (output: string) => {
  const lines = output
    .split(/\n+/)
    .map(line => line.trim())
    .filter(line => line && !line.endsWith('>'));

  return lines
    .map(line => {
      const match = line.match(usersLineRegex);
      if (!match || !match.groups) return null;
      return {
        name: match.groups.name,
        id: match.groups.id,
        role: match.groups.role,
        present: match.groups.present.toLowerCase() === 'true',
        pingMs: Number(match.groups.ping),
        fps: Number(match.groups.fps),
        silenced: match.groups.silenced.toLowerCase() === 'true'
      };
    })
    .filter((entry): entry is NonNullable<typeof entry> => Boolean(entry));
};

const parseWorldsOutput = (output: string) => {
  const rawLines = output.split(/\r?\n/);
  const focusPrompt = [...rawLines]
    .reverse()
    .map(line => line.trim())
    .find(line => line.endsWith('>'));
  const focusedName = focusPrompt ? focusPrompt.slice(0, -1).trim() : null;

  const trimmedLines = rawLines
    .map(line => line.trim())
    .filter(line => line && !line.endsWith('>'));

  const sessions: Array<{
    name: string;
    sessionId: string;
    currentUsers?: number;
    presentUsers?: number;
    maxUsers?: number;
    accessLevel?: string;
    hiddenFromListing?: boolean;
    focusTarget: string;
    raw: string;
    focused: boolean;
  }> = [];

  const indexLineRegex = /^\[(?<index>\d+)\]\s+(?<name>.+?)\s+Users:\s+(?<users>\d+)\s+Present:\s+(?<present>\d+)\s+AccessLevel:\s+(?<access>\S+)\s+MaxUsers:\s+(?<max>\d+)/i;

  for (const line of trimmedLines) {
    const match = line.match(indexLineRegex);
    if (match?.groups) {
      const name = match.groups.name.trim();
      const focusTarget = match.groups.index;
      sessions.push({
        name,
        sessionId: name,
        currentUsers: Number(match.groups.users),
        presentUsers: Number(match.groups.present),
        maxUsers: Number(match.groups.max),
        accessLevel: match.groups.access,
        hiddenFromListing: undefined,
        focusTarget,
        raw: line,
        focused: focusedName ? focusedName === name : false
      });
    }
  }

  if (sessions.length > 0) {
    return sessions;
  }

  const toNumber = (value?: string) => {
    if (!value) return undefined;
    const parsed = Number(value.replace(/[^0-9.-]/g, ''));
    return Number.isNaN(parsed) ? undefined : parsed;
  };

  const toBoolean = (value?: string) => {
    if (!value) return undefined;
    if (value.toLowerCase() === 'true') return true;
    if (value.toLowerCase() === 'false') return false;
    return undefined;
  };

  let current: {
    name?: string;
    sessionId?: string;
    currentUsers?: number;
    presentUsers?: number;
    maxUsers?: number;
    accessLevel?: string;
    hiddenFromListing?: boolean;
    raw: string[];
  } | null = null;

  const commit = () => {
    if (!current) return;
    const sessionId = current.sessionId ?? current.name;
    if (!sessionId) {
      current = null;
      return;
    }
    const name = current.name ?? sessionId;
    sessions.push({
      name,
      sessionId,
      currentUsers: current.currentUsers,
      presentUsers: current.presentUsers,
      maxUsers: current.maxUsers,
      accessLevel: current.accessLevel,
      hiddenFromListing: current.hiddenFromListing,
      focusTarget: sessionId,
      raw: current.raw.join('\n'),
      focused: focusedName ? focusedName === name || focusedName === sessionId : false
    });
    current = null;
  };

  for (const line of trimmedLines) {
    const kv = line.match(/^([^:]+):\s*(.*)$/);
    if (!kv) {
      if (current) current.raw.push(line);
      continue;
    }

    const [, rawKey, rawValue] = kv;
    const key = rawKey.trim().toLowerCase();
    const value = rawValue.trim();

    if (!current) {
      current = { raw: [] };
    }

    if ((key === 'sessionid' || key === 'session id') && current.sessionId) {
      commit();
      current = { raw: [], sessionId: value };
    }

    current.raw.push(line);

    switch (key) {
      case 'name':
        current.name = value;
        break;
      case 'sessionid':
      case 'session id':
        current.sessionId = value;
        break;
      case 'current users':
        current.currentUsers = toNumber(value);
        break;
      case 'present users':
        current.presentUsers = toNumber(value);
        break;
      case 'max users':
        current.maxUsers = toNumber(value);
        break;
      case 'access level':
        current.accessLevel = value;
        break;
      case 'hidden from listing':
      case 'hidden':
        current.hiddenFromListing = toBoolean(value);
        break;
      default:
        break;
    }
  }

  commit();

  return sessions;
};

const parseFriendRequestsOutput = (output: string) =>
  output
    .split(/\n+/)
    .map(line => line.trim())
    .filter(line => line && !line.endsWith('>'));

serverRoutes.get('/runtime/status', async (_req, res, next) => {
  try {
    const output = await runCommand('status');
    res.json({ raw: output, data: parseStatusOutput(output) });
  } catch (error) {
    next(error);
  }
});

serverRoutes.get('/runtime/users', async (_req, res, next) => {
  try {
    const output = await runCommand('users');
    res.json({ raw: output, data: parseUsersOutput(output) });
  } catch (error) {
    next(error);
  }
});

serverRoutes.get('/runtime/worlds', async (_req, res, next) => {
  try {
    const output = await runCommand('worlds');
    res.json({ raw: output, data: parseWorldsOutput(output) });
  } catch (error) {
    next(error);
  }
});

serverRoutes.get('/runtime/friend-requests', async (_req, res, next) => {
  try {
    const output = await runCommand('friendrequests');
    res.json({ raw: output, data: parseFriendRequestsOutput(output) });
  } catch (error) {
    next(error);
  }
});

serverRoutes.post('/runtime/worlds/focus', async (req, res, next) => {
  try {
    const sessionId = typeof req.body?.sessionId === 'string' ? req.body.sessionId.trim() : '';
    if (!sessionId) {
      return res.status(400).json({ error: 'sessionId is required' });
    }
    if (!/^[A-Za-z0-9:_\-\.]+$/.test(sessionId)) {
      return res.status(400).json({ error: 'Invalid sessionId format' });
    }

    const output = await runCommand(`focus ${sessionId}`);
    res.json({ raw: output });
  } catch (error) {
    next(error);
  }
});

serverRoutes.post('/runtime/command', async (req, res, next) => {
  try {
    const { command } = req.body ?? {};
    const trimmed = typeof command === 'string' ? command.trim() : '';
    if (!trimmed) {
      return res.status(400).json({ error: 'command is required' });
    }
    const output = await runCommand(trimmed);
    res.json({ raw: output });
  } catch (error) {
    next(error);
  }
});
