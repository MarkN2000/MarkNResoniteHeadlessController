import { processManager } from '../../services/processManager.js';
import { logEntriesToString, parseWorldsOutput, type ParsedSession } from './modParsers.js';

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
 * アクションハンドラーのマップ
 */
export const actionHandlers: Record<string, ModActionHandler> = {
  sessionlist: handleSessionList
};

