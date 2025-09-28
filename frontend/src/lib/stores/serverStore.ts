import { readable, writable } from 'svelte/store';
import type { HeadlessStatus, LogEntry } from '$lib/api';
import { connectServerSocket } from '$lib/socket';

const statusStore = writable<HeadlessStatus>({ running: false });
const logsStore = writable<LogEntry[]>([]);
const configsStore = writable<string[]>([]);

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
      })
    });
  }

  return {
    status: readable<HeadlessStatus>({ running: false }, set => statusStore.subscribe(set)),
    logs: readable<LogEntry[]>([], set => logsStore.subscribe(set)),
    configs: readable<string[]>([], set => configsStore.subscribe(set)),
    setConfigs: (items: string[]) => configsStore.set(items)
  };
};
