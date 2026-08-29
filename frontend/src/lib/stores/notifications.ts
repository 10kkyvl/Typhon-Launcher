import { derived, get, writable } from 'svelte/store';
import { active } from './downloads';
import { historyRecent } from './history';
import { installActive, installations } from './install';
import { mergeNotifications } from '../notifications/merge';
import { navigate, type RouteName } from './router';
import { selfUpdateStatus } from './selfupdate';
import type { SelfUpdateStatus } from '../services/selfupdate';
import { sources } from './sources';
import { updates } from './updates';

function pct(value: number) {
  return `${Math.round(value * 100)}%`;
}

export interface Notification {
  id: string;
  title: string;
  text: string;
  route: RouteName;
  refId?: string;
  terminal: boolean;
}

const READ_STORAGE_KEY = 'typhon.notifications.read';

function loadRead(): Set<string> {
  try {
    const raw = localStorage.getItem(READ_STORAGE_KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    return new Set(Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === 'string') : []);
  } catch {
    return new Set();
  }
}

function persistRead(ids: Set<string>) {
  try {
    localStorage.setItem(READ_STORAGE_KEY, JSON.stringify([...ids]));
  } catch {
    // localStorage недоступен (приватный режим, тесты) — прочитанное не переживёт перезапуск
  }
}

const readIds = writable<Set<string>>(loadRead());

// Уведомления об ошибках держат id сущности, а сообщение меняется от попытки к
// попытке. Без отпечатка текста прочитанная ошибка навсегда прячет следующую,
// другую по смыслу ошибку той же сущности.
function fingerprint(text: string): string {
  let h = 5381;
  for (let i = 0; i < text.length; i++) h = ((h * 33) ^ text.charCodeAt(i)) >>> 0;
  return h.toString(36);
}

function launcherUpdateNotification(status: SelfUpdateStatus): Notification | null {
  const version = status.availableVersion ?? status.currentVersion;
  if (status.error || status.state === 'failed') {
    const text = status.error || 'Не удалось проверить обновления';
    return {
      id: `launcher-update-error:${version}:${fingerprint(text)}`,
      title: 'Обновление лаунчера',
      text,
      route: 'settings',
      terminal: false,
    };
  }
  if (status.state === 'ready' && status.availableVersion) {
    return {
      id: `launcher-update-ready:${status.availableVersion}`,
      title: 'Обновление лаунчера',
      text: `Версия ${status.availableVersion} готова к установке`,
      route: 'settings',
      terminal: false,
    };
  }
  if (status.state === 'available' && status.availableVersion) {
    return {
      id: `launcher-update:${status.availableVersion}`,
      title: 'Обновление лаунчера',
      text: `Доступна версия ${status.availableVersion}`,
      route: 'settings',
      terminal: false,
    };
  }
  return null;
}

const allNotifications = derived(
  [active, installations, updates, sources, selfUpdateStatus, historyRecent],
  ([$active, $installations, $updates, $sources, $selfUpdateStatus, $historyRecent]): Notification[] => {
    const items: Notification[] = [];

    const launcherUpdate = launcherUpdateNotification($selfUpdateStatus);
    if (launcherUpdate) items.push(launcherUpdate);

    for (const source of $sources) {
      if (!source.lastError) continue;
      items.push({
        id: `source:${source.id}:${fingerprint(source.lastError)}`,
        title: source.name,
        text: source.lastError,
        route: 'sources',
        terminal: false,
      });
    }

    for (const update of $updates) {
      if (update.error) {
        items.push({
          id: `update-error:${update.gameId}:${fingerprint(update.error)}`,
          title: update.title,
          text: `Обновление не удалось: ${update.error}`,
          route: 'installed',
          refId: update.gameId,
          terminal: true,
        });
        continue;
      }
      if (update.state === 'update_downloading' || update.state === 'updating') {
        items.push({
          id: `update:${update.gameId}`,
          title: update.title,
          text: `Обновление идёт — ${pct(update.progress)}`,
          route: 'installed',
          terminal: false,
        });
        continue;
      }
      if (update.availability.available) {
        items.push({
          id: `update-available:${update.gameId}`,
          title: update.title,
          text: 'Доступно обновление',
          route: 'installed',
          terminal: false,
        });
      }
    }

    for (const install of $installations) {
      if (!installActive(install.status)) continue;
      items.push({
        id: `install:${install.id}`,
        title: install.name,
        text: `Установка — ${pct(install.progress)}`,
        route: 'downloads',
        terminal: false,
      });
    }

    for (const download of $active) {
      items.push({
        id: `download:${download.id}`,
        title: download.name,
        text: `Загрузка — ${pct(download.progress)}`,
        route: 'downloads',
        terminal: false,
      });
    }

    return mergeNotifications(items, $historyRecent);
  },
);

allNotifications.subscribe((items) => {
  const liveIds = new Set(items.map((item) => item.id));
  const current = get(readIds);
  const stale = [...current].filter((id) => !liveIds.has(id));
  if (stale.length === 0) return;
  const next = new Set(current);
  for (const id of stale) next.delete(id);
  readIds.set(next);
  persistRead(next);
});

export const notifications = derived(
  [allNotifications, readIds],
  ([$allNotifications, $readIds]): Notification[] => $allNotifications.filter((item) => !$readIds.has(item.id)),
);

export function markAllRead() {
  const current = get(allNotifications);
  const next = new Set(get(readIds));
  for (const item of current) next.add(item.id);
  readIds.set(next);
  persistRead(next);
}

export function openHistory() {
  navigate('history');
}
