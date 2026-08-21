import { derived, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  cancelDownload as cancelRequest,
  forceStartDownload,
  listDownloads,
  moveDownloadDown,
  moveDownloadUp,
  pauseDownload,
  removeDownload,
  resumeDownload,
  type Download,
  type DownloadStatus,
} from '../services/downloads';
import { errorMessage } from '../utils/errors';
import { toast } from './toasts';

export { errorMessage };

export const downloads = writable<Download[]>([]);

export const downloadsById = derived(downloads, ($downloads) => {
  const map = new Map<string, Download>();
  for (const d of $downloads) map.set(d.id, d);
  return map;
});

const activeStatuses: DownloadStatus[] = ['downloading', 'metadata', 'verifying', 'paused'];

export const active = derived(downloads, ($downloads) =>
  $downloads.filter((d) => activeStatuses.includes(d.status)),
);

export const queue = derived(downloads, ($downloads) => $downloads.filter((d) => d.status === 'queued'));

export const completed = derived(downloads, ($downloads) =>
  $downloads
    .filter((d) => d.status === 'completed')
    .slice()
    .sort((a, b) => Date.parse(b.completedAt ?? b.addedAt) - Date.parse(a.completedAt ?? a.addedAt)),
);

export const stats = derived(downloads, ($downloads) => ({
  downSpeed: $downloads.reduce((sum, d) => sum + d.downloadSpeed, 0),
  upSpeed: $downloads.reduce((sum, d) => sum + d.uploadSpeed, 0),
  activeCount: $downloads.filter((d) => activeStatuses.includes(d.status)).length,
  queuedCount: $downloads.filter((d) => d.status === 'queued').length,
}));

export const statusLabels: Record<DownloadStatus, string> = {
  queued: 'В очереди',
  metadata: 'Получение метаданных',
  downloading: 'Загрузка',
  paused: 'Пауза',
  verifying: 'Проверка файлов',
  completed: 'Завершено',
  failed: 'Ошибка',
};

function upsert(item: Download) {
  downloads.update((list) => {
    const index = list.findIndex((d) => d.id === item.id);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

async function refresh() {
  downloads.set(await listDownloads());
}

export async function initDownloads() {
  await refresh();
  if (!inWails) return;

  Events.On('download:added', (event) => {
    const item = event.data as Download;
    upsert(item);
    toast(`Загрузка «${item.name}» добавлена`);
  });
  Events.On('download:updated', (event) => {
    upsert(event.data as Download);
  });
  Events.On('download:completed', (event) => {
    const item = event.data as Download;
    upsert(item);
    toast(`«${item.name}» загружена`, 'success');
  });
  Events.On('download:failed', (event) => {
    const item = event.data as Download;
    upsert(item);
    toast(`Ошибка загрузки «${item.name}»: ${item.error}`, 'danger');
  });
  Events.On('download:removed', (event) => {
    const { id } = event.data as { id: string };
    downloads.update((list) => list.filter((d) => d.id !== id));
  });
}

async function run(action: () => Promise<void>) {
  try {
    await action();
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export function pause(id: string) {
  return run(() => pauseDownload(id));
}

export function resume(id: string) {
  return run(() => resumeDownload(id));
}

export function cancel(id: string) {
  return run(() => cancelRequest(id));
}

export function remove(id: string) {
  return run(() => removeDownload(id));
}

export function forceStart(id: string) {
  return run(() => forceStartDownload(id));
}

export function moveUp(id: string) {
  return run(async () => {
    await moveDownloadUp(id);
    await refresh();
  });
}

export function moveDown(id: string) {
  return run(async () => {
    await moveDownloadDown(id);
    await refresh();
  });
}
