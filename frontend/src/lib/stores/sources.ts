import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  addSource as addSourceRequest,
  listSources,
  refreshAllSources,
  refreshSource,
  removeSource as removeSourceRequest,
  setSourceEnabled,
  type Source,
} from '../services/sources';
import { errorMessage } from '../utils/errors';
import { toast } from './toasts';

export { errorMessage };

export const sources = writable<Source[]>([]);
export const refreshingAll = writable(false);

interface SourceErrorEvent {
  sourceId: string;
  name: string;
  message: string;
}

interface ReleaseBatchEvent {
  sourceId: string;
  count: number;
}

function upsert(item: Source) {
  sources.update((list) => {
    const index = list.findIndex((s) => s.id === item.id);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

async function reload() {
  sources.set(await listSources());
}

export async function initSources() {
  await reload();
  if (!inWails) return;

  Events.On('source:updated', (event) => {
    const item = event.data as Source;
    if (!item.name) {
      sources.update((list) => list.filter((s) => s.id !== item.id));
      return;
    }
    upsert(item);
  });
  Events.On('source:error', (event) => {
    const { name, message } = event.data as SourceErrorEvent;
    toast(`Ошибка источника «${name}»: ${message}`, 'danger');
  });
  Events.On('release:added', (event) => {
    const { count } = event.data as ReleaseBatchEvent;
    if (count > 0) toast(`Новых записей: ${count}`);
  });
}

export async function refresh(id: string) {
  try {
    const summary = await refreshSource(id);
    await reload();
    toast(`Записей: ${summary.entries}, новых: ${summary.added}, на проверку: ${summary.review}`, 'success');
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export async function refreshAll() {
  refreshingAll.set(true);
  try {
    await refreshAllSources();
    await reload();
  } catch (err) {
    toast(errorMessage(err), 'danger');
  } finally {
    refreshingAll.set(false);
  }
}

export async function toggle(id: string, enabled: boolean) {
  try {
    await setSourceEnabled(id, enabled);
    await reload();
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export async function remove(id: string) {
  try {
    await removeSourceRequest(id);
    sources.update((list) => list.filter((s) => s.id !== id));
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export async function add(url: string) {
  const source = await addSourceRequest(url);
  await reload();
  return source;
}
