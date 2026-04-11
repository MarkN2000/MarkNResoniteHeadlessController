import { readable, writable } from 'svelte/store';
import type { HeadlessStatus, LogEntry, ConfigEntry, SystemMetrics } from '$lib/api';
import { connectServerSocket } from '$lib/socket';
import type {
  SteamUpdateState,
  SteamUpdateProgress,
  SteamUpdateSnapshot,
  SteamUpdateCheckResult
} from '$lib/socket';

const statusStore = writable<HeadlessStatus>({ running: false });
const logsStore = writable<LogEntry[]>([]);
const configsStore = writable<ConfigEntry[]>([]);
const metricsStore = writable<SystemMetrics | null>(null);

// SteamCMDアップデート進捗用のストア
const steamUpdateStateStore = writable<SteamUpdateState | null>(null);
const steamUpdateProgressStore = writable<SteamUpdateProgress | null>(null);
const steamUpdateLogLinesStore = writable<string[]>([]);

// Resonite 最新バージョン確認結果のストア
// バックエンドが定期的に SteamCMD で収集した結果を保持し、
// UI はこれを見て「新バージョンあり」の赤ドットバッジを出す
export const steamUpdateCheckStore = writable<SteamUpdateCheckResult | null>(null);

const STEAM_UPDATE_LOG_LIMIT = 500;

const appendSteamUpdateLog = (text: string): void => {
  // チャンクで届く可能性があるため行単位に分割してから追加
  const lines = text.split(/\r?\n/).filter(line => line.length > 0);
  if (lines.length === 0) return;
  steamUpdateLogLinesStore.update(current => {
    const next = current.concat(lines);
    if (next.length > STEAM_UPDATE_LOG_LIMIT) {
      return next.slice(next.length - STEAM_UPDATE_LOG_LIMIT);
    }
    return next;
  });
};

/**
 * バックエンドから送られた現在のアップデートスナップショットでストアを置換する
 * 用途: WS新規接続時 / 再接続時に、サーバー側に保持されている最新の状態へ追従する
 *
 * appendSteamUpdateLog と違って「置換」であることに注意:
 * 再接続時はクライアント側のログがズレている可能性があるため、サーバー側の情報を信頼する
 */
const applySteamUpdateSnapshot = (snapshot: SteamUpdateSnapshot): void => {
  steamUpdateStateStore.set(snapshot.state);
  steamUpdateProgressStore.set(snapshot.progress);
  // ログは上限内に収めて置換
  const logs = snapshot.recentLogs ?? [];
  steamUpdateLogLinesStore.set(
    logs.length > STEAM_UPDATE_LOG_LIMIT
      ? logs.slice(logs.length - STEAM_UPDATE_LOG_LIMIT)
      : logs.slice()
  );
};

let initialized = false;

export const createServerStores = () => {
  if (!initialized) {
    initialized = true;
    connectServerSocket({
      status: status => statusStore.set(status),
      logs: entries => logsStore.set(entries),
      log: entry => logsStore.update(current => {
        const next = [...current, entry];
        if (next.length > 1000) {
          next.shift();
        }
        return next;
      }),
      metrics: metrics => metricsStore.set(metrics),
      updateLog: text => appendSteamUpdateLog(text),
      updateStatus: state => steamUpdateStateStore.set(state),
      updateProgress: progress => steamUpdateProgressStore.set(progress),
      updateSnapshot: snapshot => applySteamUpdateSnapshot(snapshot),
      updateCheckResult: result => steamUpdateCheckStore.set(result),
      updateCheckSnapshot: result => steamUpdateCheckStore.set(result)
    });
  }

  return {
    status: readable<HeadlessStatus>({ running: false }, set => statusStore.subscribe(set)),
    logs: readable<LogEntry[]>([], set => logsStore.subscribe(set)),
    configs: readable<ConfigEntry[]>([], set => configsStore.subscribe(set)),
    metrics: readable<SystemMetrics | null>(null, set => metricsStore.subscribe(set)),
    steamUpdateState: readable<SteamUpdateState | null>(null, set =>
      steamUpdateStateStore.subscribe(set)
    ),
    steamUpdateProgress: readable<SteamUpdateProgress | null>(null, set =>
      steamUpdateProgressStore.subscribe(set)
    ),
    steamUpdateLogLines: readable<string[]>([], set => steamUpdateLogLinesStore.subscribe(set)),
    setStatus: (value: HeadlessStatus) => statusStore.set(value),
    setLogs: (entries: LogEntry[]) => logsStore.set(entries),
    appendLog: (entry: LogEntry) =>
      logsStore.update(current => {
        const next = [...current, entry];
        if (next.length > 1000) {
          next.shift();
        }
        return next;
      }),
    clearLogs: () => logsStore.set([]),
    setConfigs: (items: ConfigEntry[]) => configsStore.set(items),
    resetSteamUpdate: () => {
      steamUpdateStateStore.set(null);
      steamUpdateProgressStore.set(null);
      steamUpdateLogLinesStore.set([]);
    },
    setSteamUpdateState: (state: SteamUpdateState | null) => steamUpdateStateStore.set(state)
  };
};
