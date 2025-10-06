import path from 'node:path';
import { Router } from 'express';
import * as cheerio from 'cheerio';
import { processManager } from '../../services/processManager.js';
import type { ExecuteCommandOptions } from '../../services/processManager.js';

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

const PROMPT_DETECTOR = (entry: { message: string }) => {
  const trimmed = entry.message.trim();
  if (!trimmed) return false;
  if (trimmed === '>') return true;
  return trimmed.endsWith('>') && !trimmed.startsWith('>');
};

const createPromptAfterDataDetector = (pattern: RegExp) => {
  let sawData = false;
  return (entry: { message: string }) => {
    const trimmed = entry.message.trim();
    if (!trimmed) return false;
    if (!sawData && pattern.test(trimmed)) {
      sawData = true;
    }
    if (!sawData) return false;
    if (trimmed === '>') return true;
    return trimmed.endsWith('>') && !trimmed.startsWith('>');
  };
};

const STATUS_DATA_REGEX = /^(Name|SessionID|Current Users|Present Users|Max Users|Access Level|Hidden from listing|Mobile Friendly|Description|Tags|Users):/i;
const USERS_DATA_REGEX = /^.+\s+ID:\s+/i;
const WORLDS_DATA_REGEX = /^(\[\d+\]\s+.+\s+Users:\s+\d+|Name:\s+)/i;

interface RunCommandOptions {
  timeoutMs?: number;
  stopWhen?: ExecuteCommandOptions['stopWhen'];
  stopWhenPrompt?: boolean;
  settleDurationMs?: number;
}

const runCommand = async (command: string, options: RunCommandOptions = {}) => {
  const execOptions: ExecuteCommandOptions = {};
  let hasOption = false;

  if (options.stopWhen) {
    execOptions.stopWhen = options.stopWhen;
    hasOption = true;
  } else if (options.stopWhenPrompt) {
    execOptions.stopWhen = PROMPT_DETECTOR;
    hasOption = true;
  }

  if (options.settleDurationMs !== undefined) {
    execOptions.settleDurationMs = options.settleDurationMs;
    hasOption = true;
  }

  const logs = await processManager.executeCommand(
    command,
    options.timeoutMs ?? 4000,
    hasOption ? execOptions : undefined
  );
  return logs
    .map(entry => entry.message)
    .map(message => message.replace(/\r/g, ''))
    .map(message => message.trimEnd())
    .filter(message => {
      const trimmed = message.trim();
      if (!trimmed) return false;
      if (trimmed.startsWith('>')) return false;
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
        focused: false
      });
    }
  }

  if (sessions.length > 0) {
    const result = finalizeWorldsFocus(sessions, focusedName);
    return result;
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
      focused: false
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

  return finalizeWorldsFocus(sessions, focusedName);
};

const finalizeWorldsFocus = (
  sessions: Array<{
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
  }>,
  focusedName: string | null
) => {
  let focusedSessionId: string | null = null;
  let focusedFocusTarget: string | null = null;

  if (focusedName) {
    for (const session of sessions) {
      if (
        session.name === focusedName ||
        session.sessionId === focusedName ||
        session.focusTarget === focusedName
      ) {
        session.focused = true;
        focusedSessionId = session.sessionId;
        focusedFocusTarget = session.focusTarget;
      } else {
        session.focused = false;
      }
    }
  }

  return {
    sessions,
    focusedSessionId,
    focusedSessionName: focusedName,
    focusedFocusTarget
  };
};

const parseFriendRequestsOutput = (output: string) =>
  output
    .split(/\n+/)
    .map(line => line.trim())
    .filter(line => line && !line.endsWith('>'));

serverRoutes.get('/runtime/status', async (_req, res, next) => {
  try {
    const output = await runCommand('status', {
      stopWhen: createPromptAfterDataDetector(STATUS_DATA_REGEX),
      settleDurationMs: 120,
      timeoutMs: 2000
    });
    res.json({ raw: output, data: parseStatusOutput(output) });
  } catch (error) {
    next(error);
  }
});

serverRoutes.get('/runtime/users', async (_req, res, next) => {
  try {
    const output = await runCommand('users', {
      stopWhen: createPromptAfterDataDetector(USERS_DATA_REGEX),
      settleDurationMs: 120,
      timeoutMs: 2000
    });
    res.json({ raw: output, data: parseUsersOutput(output) });
  } catch (error) {
    next(error);
  }
});

serverRoutes.get('/runtime/worlds', async (_req, res, next) => {
  try {
    const output = await runCommand('worlds', {
      stopWhen: createPromptAfterDataDetector(WORLDS_DATA_REGEX),
      settleDurationMs: 120,
      timeoutMs: 2000
    });
    const parsed = parseWorldsOutput(output);
    res.json({
      raw: output,
      data: parsed.sessions,
      focusedSessionId: parsed.focusedSessionId,
      focusedSessionName: parsed.focusedSessionName,
      focusedFocusTarget: parsed.focusedFocusTarget
    });
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

    const output = await runCommand(`focus ${sessionId}`, { stopWhenPrompt: true, settleDurationMs: 120, timeoutMs: 2000 });
    res.json({ raw: output });
  } catch (error) {
    next(error);
  }
});

serverRoutes.post('/runtime/worlds/focus-refresh', async (req, res, next) => {
  try {
    const sessionId = typeof req.body?.sessionId === 'string' ? req.body.sessionId.trim() : '';
    if (!sessionId) {
      return res.status(400).json({ error: 'sessionId is required' });
    }
    if (!/^[A-Za-z0-9:_\-\.]+$/.test(sessionId)) {
      return res.status(400).json({ error: 'Invalid sessionId format' });
    }

    await runCommand(`focus ${sessionId}`, { stopWhenPrompt: true, settleDurationMs: 120, timeoutMs: 2000 });

    const worlds = await runCommand('worlds', {
      stopWhen: createPromptAfterDataDetector(WORLDS_DATA_REGEX),
      settleDurationMs: 120,
      timeoutMs: 2000
    });
    const status = await runCommand('status', {
      stopWhen: createPromptAfterDataDetector(STATUS_DATA_REGEX),
      settleDurationMs: 120,
      timeoutMs: 2000
    });
    const users = await runCommand('users', {
      stopWhen: createPromptAfterDataDetector(USERS_DATA_REGEX),
      settleDurationMs: 120,
      timeoutMs: 2000
    });

    const parsedWorlds = parseWorldsOutput(worlds);

    res.json({
      worlds: {
        raw: worlds,
        data: parsedWorlds.sessions,
        focusedSessionId: parsedWorlds.focusedSessionId,
        focusedSessionName: parsedWorlds.focusedSessionName,
        focusedFocusTarget: parsedWorlds.focusedFocusTarget
      },
      status: { raw: status, data: parseStatusOutput(status) },
      users: { raw: users, data: parseUsersOutput(users) }
    });
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

// World search via go.resonite.com HTML scraping
serverRoutes.get('/world-search', async (req, res, next) => {
  try {
    const term = typeof req.query.term === 'string' ? req.query.term.trim() : '';
    if (!term) return res.status(400).json({ error: 'term is required' });

    const url = `https://go.resonite.com/world?term=${encodeURIComponent(term)}`;
    const resp = await fetch(url, { headers: { 'User-Agent': 'MRHC/1.0' } });
    if (!resp.ok) {
      return res.status(502).json({ error: `upstream error ${resp.status}` });
    }
    const html = await resp.text();
    const $ = cheerio.load(html);

    const origin = 'https://go.resonite.com';
    const absolutize = (u?: string | null) => {
      if (!u) return null;
      if (/^https?:\/\//i.test(u)) return u;
      if (u.startsWith('//')) return `https:${u}`;
      if (u.startsWith('/')) return origin + u;
      return u;
    };

    const items: Array<{ name: string; imageUrl: string | null; recordId: string; resoniteUrl: string }> = [];
    $('ol.listing li a.listing-item').each((_i, el) => {
      const anchor = $(el);
      const name = anchor.find('h2.listing-item__heading span').first().text().trim();
      const href = anchor.attr('href') || '';
      // Try to find image in common places (img/src, img/srcset, data-src, background-image)
      let imageUrl: string | null = null;
      const img = anchor.find('img').first();
      if (img && img.attr('src')) imageUrl = img.attr('src')!;
      if (!imageUrl && img && img.attr('data-src')) imageUrl = img.attr('data-src')!;
      if (!imageUrl && img && img.attr('srcset')) {
        const srcset = img.attr('srcset')!;
        const first = srcset.split(',')[0]?.trim().split(' ')[0];
        if (first) imageUrl = first;
      }
      if (!imageUrl) {
        const styled = anchor.find('[style*="background-image"]').first();
        const style = styled.attr('style') || '';
        const m = style.match(/background-image:\s*url\(("|')?(?<u>[^)"']+)("|')?\)/i);
        if (m && m.groups && m.groups.u) imageUrl = m.groups.u;
      }
      imageUrl = absolutize(imageUrl);

      const match = href.match(/\/R-([A-Za-z0-9_-]+)/);
      if (!name || !match) return;
      const recordId = `R-${match[1]}`;
      const resoniteUrl = `resonite:///world/${recordId}`;
      items.push({ name, imageUrl, recordId, resoniteUrl });
    });

    res.json({ items });
  } catch (error) {
    next(error);
  }
});
