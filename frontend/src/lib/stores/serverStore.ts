import { readable, writable } from 'svelte/store';
import type { HeadlessStatus, LogEntry, ConfigEntry, SystemMetrics } from '$lib/api';
import { connectServerSocket } from '$lib/socket';

const statusStore = writable<HeadlessStatus>({ running: false });
const logsStore = writable<LogEntry[]>([]);
const configsStore = writable<ConfigEntry[]>([]);
const metricsStore = writable<SystemMetrics | null>(null);

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
      metrics: metrics => metricsStore.set(metrics)
    });
  }

  return {
    status: readable<HeadlessStatus>({ running: false }, set => statusStore.subscribe(set)),
    logs: readable<LogEntry[]>([], set => logsStore.subscribe(set)),
    configs: readable<ConfigEntry[]>([], set => configsStore.subscribe(set)),
    metrics: readable<SystemMetrics | null>(null, set => metricsStore.subscribe(set)),
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
    setConfigs: (items: ConfigEntry[]) => configsStore.set(items)
  };
};
