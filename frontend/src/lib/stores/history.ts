import { derived, writable } from 'svelte/store';
import { listenFirst } from './listenFirst';
import { inWails } from '../services/backend';
import {
  clearHistory as clearHistoryRequest,
  getHistoryStatus,
  listHistory,
  type Record,
  type Status,
} from '../services/history';
import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';
import { toast } from './toasts';

const REASONS: { [code: string]: MessageKey } = {
  'history.clear_failed': 'history.clear_failed',
};

export function historyErrorText(err: unknown, fallback: string = msg('errLibrary.historyErrFallback')): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : fallback;
}

export const history = writable<Record[]>([]);
export const historyStatus = writable<Status>({ degraded: false, message: '' });

export const historyRecent = derived(history, ($history) => $history.slice(0, 20));

function sortDesc(records: Record[]): Record[] {
  return [...records].sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
}

export async function initHistory() {
  const { on, load } = listenFirst();
  if (!inWails) {
    history.set(sortDesc(await listHistory()));
    historyStatus.set(await getHistoryStatus());
    return;
  }

  on('history:recorded', (event) => {
    const record = event.data as Record;
    history.update((list) => sortDesc([record, ...list.filter((r) => r.id !== record.id)]));
    historyStatus.set({ degraded: false, message: '' });
  });
  on('history:updated', (event) => {
    history.set(sortDesc(event.data as Record[]));
    historyStatus.set({ degraded: false, message: '' });
  });
  on('history:degraded', (event) => {
    historyStatus.set(event.data as Status);
  });
  history.set(sortDesc(await load(listHistory)));
  historyStatus.set(await load(getHistoryStatus));
}

export async function clearHistory() {
  try {
    await clearHistoryRequest();
    history.set([]);
  } catch (err) {
    toast(historyErrorText(err), 'danger');
  }
}
