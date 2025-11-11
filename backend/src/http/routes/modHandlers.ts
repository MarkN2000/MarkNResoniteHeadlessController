import { processManager } from '../../services/processManager.js';
import type { LogEntry } from '../../services/logBuffer.js';
import {
  logEntriesToString,
  parseWorldsOutput,
  parseStatusOutput,
  parseInviteOutput,
  parseAccessLevelOutput,
  parseRoleOutput,
  parseSessionUrlOutput
} from './modParsers.js';

/**
 * Mod APIアクションハンドラーの型定義
 */
export type ModActionHandler = (params: any) => Promise<any>;

/**
 * sessionlistアクション: セッション一覧を取得
 */
export const handleSessionList: ModActionHandler = async (_params) => {
  const logEntries = await processManager.executeCommand('worlds');
  const output = logEntriesToString(logEntries);
  const sessions = parseWorldsOutput(output);
  return sessions;
};

/**
 * focusアクション: 指定インデックスのセッションにフォーカスし、status結果を返す
 */
export const handleFocus: ModActionHandler = async (params) => {
  const indexRaw = params?.index ?? params?.focus ?? params?.id;
  if (indexRaw === undefined || indexRaw === null || String(indexRaw).trim() === '') {
    throw new Error('index is required');
  }
  const index = Number(indexRaw);
  if (!Number.isInteger(index) || index < 0) {
    throw new Error('index must be a non-negative integer');
  }

  // フォーカス実行
  await processManager.executeCommand(`focus ${index}`);

  // ステータス取得
  const statusLogs = await processManager.executeCommand('status', 4000);
  const statusOutput = logEntriesToString(statusLogs);
  const status = parseStatusOutput(statusOutput);
  return status;
};

/**
 * inviteアクション: ユーザーを招待
 */
export const handleInvite: ModActionHandler = async (params) => {
  const username = params?.username;
  if (!username || typeof username !== 'string' || username.trim() === '') {
    throw new Error('username is required');
  }

  const logEntries = await processManager.executeCommand(`invite "${username}"`, 5000);
  const output = logEntriesToString(logEntries);
  const result = parseInviteOutput(output);

  if (!result.success) {
    throw new Error(result.message);
  }

  return result;
};

/**
 * setaccesslevelアクション: セッションのアクセスレベルを変更
 */
export const handleSetAccessLevel: ModActionHandler = async (params) => {
  const accessLevel = params?.accessLevel;
  if (!accessLevel || typeof accessLevel !== 'string' || accessLevel.trim() === '') {
    throw new Error('accessLevel is required');
  }

  // 有効なアクセスレベルのチェック（オプション）
  const validLevels = ['Private', 'LAN', 'Friends', 'Anyone'];
  const normalizedLevel = accessLevel.trim();
  if (!validLevels.includes(normalizedLevel)) {
    throw new Error(`Invalid accessLevel. Must be one of: ${validLevels.join(', ')}`);
  }

  const logEntries = await processManager.executeCommand(`accesslevel ${normalizedLevel}`, 5000);
  const output = logEntriesToString(logEntries);
  const result = parseAccessLevelOutput(output);

  if (!result.success) {
    throw new Error(result.message);
  }

  return result;
};

/**
 * setroleアクション: ユーザーの権限を変更
 */
export const handleSetRole: ModActionHandler = async (params) => {
  const username = params?.username;
  const role = params?.role;

  if (!username || typeof username !== 'string' || username.trim() === '') {
    throw new Error('username is required');
  }
  if (!role || typeof role !== 'string' || role.trim() === '') {
    throw new Error('role is required');
  }

  // 有効なロールのチェック
  const validRoles = ['Admin', 'Builder', 'Moderator', 'Guest', 'Spectator'];
  const normalizedRole = role.trim();
  if (!validRoles.includes(normalizedRole)) {
    throw new Error(`Invalid role. Must be one of: ${validRoles.join(', ')}`);
  }

  const logEntries = await processManager.executeCommand(`role "${username}" "${normalizedRole}"`, 5000);
  const output = logEntriesToString(logEntries);
  const result = parseRoleOutput(output);

  if (!result.success) {
    throw new Error(result.message);
  }

  return result;
};

/**
 * startworldアクション: セッションを開始
 */
export const handleStartWorld: ModActionHandler = async (params) => {
  const url = params?.url;
  if (!url || typeof url !== 'string' || url.trim() === '') {
    throw new Error('url is required');
  }

  // URL形式の簡易チェック
  if (!url.match(/^res[-\w]*:\/\//i)) {
    throw new Error('Invalid URL format. Must start with res://, resrec://, res-steam://, etc.');
  }

  // プロンプト検出用の条件（">" で終わる行を検出）
  const promptDetector = (entry: LogEntry) => {
    const trimmed = entry.message.trim();
    return trimmed.endsWith('>') && trimmed.length > 1;
  };

  // ワールド開始コマンド実行（タイムアウトは長めに設定）
  const startLogs = await processManager.executeCommand(`startworldurl "${url}"`, 30000, {
    stopWhen: promptDetector,
    settleDurationMs: 500 // プロンプトが安定するまで少し待つ
  });

  const startOutput = logEntriesToString(startLogs);

  // ワールド名を抽出（プロンプトから取得、例: "地下貯水施設>"）
  const worldNameMatch = startOutput.match(/([^>]+)>$/);
  const worldName = worldNameMatch && worldNameMatch[1] ? worldNameMatch[1].trim() : undefined;

  // "World running..." が含まれているか確認
  if (!startOutput.toLowerCase().includes('world running')) {
    throw new Error('Failed to start world. World may not have loaded properly.');
  }

  // セッションURLを取得
  const sessionUrlLogs = await processManager.executeCommand('sessionURL', 5000);
  const sessionUrlOutput = logEntriesToString(sessionUrlLogs);
  const sessionUrlResult = parseSessionUrlOutput(sessionUrlOutput);

  if (!sessionUrlResult.sessionUrl) {
    throw new Error('Failed to get session URL');
  }

  return {
    success: true,
    sessionUrl: sessionUrlResult.sessionUrl,
    sessionId: sessionUrlResult.sessionId,
    worldName
  };
};

/**
 * アクションハンドラーのマップ
 */
export const actionHandlers: Record<string, ModActionHandler> = {
  sessionlist: handleSessionList,
  focus: handleFocus,
  invite: handleInvite,
  setaccesslevel: handleSetAccessLevel,
  setrole: handleSetRole,
  startworld: handleStartWorld
};

