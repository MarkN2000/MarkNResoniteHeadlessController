import { processManager } from '../../services/processManager.js';
import { logEntriesToString, parseWorldsOutput, parseStatusOutput } from './modParsers.js';

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
 * アクションハンドラーのマップ
 */
export const actionHandlers: Record<string, ModActionHandler> = {
  sessionlist: handleSessionList,
  focus: handleFocus
};

