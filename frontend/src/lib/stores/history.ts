import { derived, get, writable } from 'svelte/store';
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

const REASONS: { [code: string]: MessageKey } = {
  'history.clear_failed': 'history.clear_failed',
};

export function historyErrorText(err: unknown, fallback: string = msg('errLibrary.historyErrFallback')): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : fallback;
}

export const history = writable<Record[]>([]);
/** Until the first list arrives, an empty `history` is not an empty journal. */
export const historyLoaded = writable(false);
/** Why the first list could not be read; null once a list has arrived. */
export const historyLoadError = writable<unknown>(null);
export const historyStatus = writable<Status>({ degraded: false, message: '' });

export const historyRecent = derived(history, ($history) => $history.slice(0, 20));

function sortDesc(records: Record[]): Record[] {
  return [...records].sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
}

function received(records: Record[]) {
  history.set(sortDesc(records));
  historyLoaded.set(true);
  historyLoadError.set(null);
}

// Every history event Go sent: a list asked for before one of them may predate it.
let heard = 0;

/** Reads the list; answers whether a list is there. A failure before any list is kept as state. */
async function readList(read: () => Promise<Record[]>): Promise<boolean> {
  try {
    received(await read());
  } catch (err) {
    if (!get(historyLoaded)) historyLoadError.set(err);
    return false;
  }
  return true;
}

// The journal's health is a note beside the list: a status that cannot be read leaves the one shown,
// and the list stands without it.
async function readStatus(read: () => Promise<Status>): Promise<void> {
  try {
    historyStatus.set(await read());
  } catch (err) {
    console.error('history status', err);
  }
}

/**
 * Reads the list and the status again, as the page's Retry after a failed first read does; answers
 * whether a list is there. An event that arrives meanwhile gets the list asked once more; a whole list
 * it brought counts even when that second read fails.
 */
export async function reloadHistory(): Promise<boolean> {
  await readList(async () => {
    const before = heard;
    const list = await listHistory();
    return heard === before ? list : listHistory();
  });
  const ok = get(historyLoaded);
  if (ok) await readStatus(getHistoryStatus);
  return ok;
}

/** Subscribes first, then reads the list; never rejects: a failed first read is `historyLoadError`. */
export async function initHistory(): Promise<void> {
  const first = listenFirst();
  if (!inWails) {
    await readList(listHistory);
    await readStatus(getHistoryStatus);
    return;
  }
  const on: typeof first.on = (name, handler) =>
    first.on(name, (event) => {
      heard += 1;
      handler(event);
    });

  on('history:recorded', (event) => {
    const record = event.data as Record;
    history.update((list) => sortDesc([record, ...list.filter((r) => r.id !== record.id)]));
    historyStatus.set({ degraded: false, message: '' });
  });
  on('history:updated', (event) => {
    received(event.data as Record[]);
    historyStatus.set({ degraded: false, message: '' });
  });
  on('history:degraded', (event) => {
    historyStatus.set(event.data as Status);
  });
  await readList(() => first.load(listHistory));
  await readStatus(() => first.load(getHistoryStatus));
}

/** Asks Go to clear the journal; a refusal is the caller's to say (the confirmation shows it). */
export async function clearHistory(): Promise<void> {
  await clearHistoryRequest();
  history.set([]);
}
