import { derived, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  listInstallations,
  type Installation,
  type InstallStatus,
  type InstallType,
} from '../services/install';
import { toast } from './toasts';

export const installations = writable<Installation[]>([]);

export const installationsByDownload = derived(installations, ($items) => {
  const map = new Map<string, Installation>();
  for (const item of $items) {
    const current = map.get(item.downloadId);
    if (!current || Date.parse(item.startedAt) >= Date.parse(current.startedAt)) {
      map.set(item.downloadId, item);
    }
  }
  return map;
});

export const installStatusLabels: Record<InstallStatus, string> = {
  pending: 'В очереди',
  preparing: 'Подготовка',
  installing: 'Установка',
  extracting: 'Распаковка',
  verifying: 'Проверка',
  waiting_for_user: 'Требуется действие',
  completed: 'Установлено',
  failed: 'Ошибка',
  cancelled: 'Отменено',
  interrupted: 'Прервано',
};

export const installTypeLabels: Record<InstallType, string> = {
  portable: 'Портативная игра',
  archive_zip: 'ZIP-архив',
  archive_7z: '7z-архив',
  archive_rar: 'RAR-архив',
  exe_installer: 'Установщик (EXE)',
  msi_installer: 'Установщик (MSI)',
  unknown: 'Неизвестный пакет',
};

const activeStatuses: InstallStatus[] = ['pending', 'preparing', 'installing', 'extracting', 'verifying'];

export function installActive(status: InstallStatus) {
  return activeStatuses.includes(status);
}

export function upsertInstallation(item: Installation) {
  installations.update((list) => {
    const index = list.findIndex((i) => i.id === item.id);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

export async function initInstalls() {
  installations.set(await listInstallations());
  if (!inWails) return;

  Events.On('install:started', (event) => {
    upsertInstallation(event.data as Installation);
  });
  Events.On('install:updated', (event) => {
    upsertInstallation(event.data as Installation);
  });
  Events.On('install:completed', (event) => {
    const item = event.data as Installation;
    upsertInstallation(item);
    toast(`Игра «${item.name}» установлена`, 'success');
  });
  Events.On('install:failed', (event) => {
    const item = event.data as Installation;
    upsertInstallation(item);
    toast(`Ошибка установки «${item.name}»: ${item.error}`, 'danger');
  });
  Events.On('install:cancelled', (event) => {
    upsertInstallation(event.data as Installation);
  });
  Events.On('install:removed', (event) => {
    const { id } = event.data as { id: string };
    installations.update((list) => list.filter((i) => i.id !== id));
  });
}
