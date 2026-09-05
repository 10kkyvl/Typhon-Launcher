import { derived, get, writable } from 'svelte/store';
import { active } from './downloads';
import { friendRequestNotification } from '../social/view';
import { historyRecent } from './history';
import { msg } from '../i18n';
import { incomingCount, incomingPeak } from './social';
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
    const text = status.error || msg('state.notifUpdateCheckFailed');
    return {
      id: `launcher-update-error:${version}:${fingerprint(text)}`,
      title: msg('state.notifLauncherUpdateTitle'),
      text,
      route: 'settings',
      terminal: false,
    };
  }
  if (status.state === 'ready' && status.availableVersion) {
    return {
      id: `launcher-update-ready:${status.availableVersion}`,
      title: msg('state.notifLauncherUpdateTitle'),
      text: msg('state.notifLauncherUpdateReady', { version: status.availableVersion }),
      route: 'settings',
      terminal: false,
    };
  }
  if (status.state === 'available' && status.availableVersion) {
    return {
      id: `launcher-update:${status.availableVersion}`,
      title: msg('state.notifLauncherUpdateTitle'),
      text: msg('state.notifLauncherUpdateAvailable', { version: status.availableVersion }),
      route: 'settings',
      terminal: false,
    };
  }
  return null;
}

const allNotifications = derived(
  [active, installations, updates, sources, selfUpdateStatus, historyRecent, incomingCount, incomingPeak],
  ([
    $active,
    $installations,
    $updates,
    $sources,
    $selfUpdateStatus,
    $historyRecent,
    $incomingCount,
    $incomingPeak,
  ]): Notification[] => {
    const items: Notification[] = [];

    const launcherUpdate = launcherUpdateNotification($selfUpdateStatus);
    if (launcherUpdate) items.push(launcherUpdate);

    const friendRequests = friendRequestNotification($incomingCount, $incomingPeak);
    if (friendRequests) items.push(friendRequests);

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
          text: msg('state.notifUpdateFailed', { error: update.error }),
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
          text: msg('state.notifUpdateInProgress', { percent: pct(update.progress) }),
          route: 'installed',
          terminal: false,
        });
        continue;
      }
      if (update.availability.available) {
        items.push({
          id: `update-available:${update.gameId}`,
          title: update.title,
          text: msg('state.notifUpdateAvailable'),
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
        text: msg('state.notifInstallProgress', { percent: pct(install.progress) }),
        route: 'downloads',
        terminal: false,
      });
    }

    for (const download of $active) {
      items.push({
        id: `download:${download.id}`,
        title: download.name,
        text: msg('state.notifDownloadProgress', { percent: pct(download.progress) }),
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
