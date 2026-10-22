import { derived, get, writable } from 'svelte/store';
import { friendRequestNotification } from '../social/view';
import { historyRecent } from './history';
import { locale, msg } from '../i18n';
import { sourceErrorText } from '../sources/sourceErrors';
import { updateErrorText } from '../updates/updateErrors';
import { statusReason } from '../services/selfupdateMessages';
import { incomingCount, incomingPeak } from './social';
import { installActive, installations } from './install';
import { mergeNotifications } from '../notifications/merge';
import { navigate, type RouteName } from './router';
import { selfUpdateStatus } from './selfupdate';
import type { SelfUpdateStatus } from '../services/selfupdate';
import { bytesSize, progressPercent } from '../utils/format';
import { installTotalUnknown } from '../install/progress';
import { sources } from './sources';
import { unreadReplies } from './support';
import { updates } from './updates';
import { dismissAlert, watchAlerts, watchAlertsLoaded } from './watchlist';

function pct(value: number) {
  return `${progressPercent(value)}%`;
}

export interface Notification {
  id: string;
  title: string;
  text: string;
  route: RouteName;
  params?: Record<string, string>;
  onOpen?: () => void;
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
    const text = statusReason(status) || msg('state.notifUpdateCheckFailed');
    return {
      id: `launcher-update-error:${version}:${fingerprint(status.error || status.state)}`,
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
  [
    installations,
    updates,
    sources,
    selfUpdateStatus,
    historyRecent,
    watchAlerts,
    incomingCount,
    incomingPeak,
    unreadReplies,
    locale,
  ],
  ([
    $installations,
    $updates,
    $sources,
    $selfUpdateStatus,
    $historyRecent,
    $watchAlerts,
    $incomingCount,
    $incomingPeak,
    $unreadReplies,
  ]): Notification[] => {
    const items: Notification[] = [];

    if ($unreadReplies > 0) {
      items.push({
        id: `support-unread:${$unreadReplies}`,
        title: msg('tickets.notifTitle'),
        text: msg('tickets.notifText', { count: $unreadReplies }),
        route: 'support',
        terminal: false,
      });
    }

    for (const alert of $watchAlerts) {
      items.push({
        id: `watch:${alert.id}`,
        title: msg('games.watchAlertTitle'),
        text: `${alert.title} — ${alert.releaseTitle}`,
        route: 'game',
        params: { id: alert.gameId },
        onOpen: () => void dismissAlert(alert.id),
        terminal: false,
      });
    }

    const launcherUpdate = launcherUpdateNotification($selfUpdateStatus);
    if (launcherUpdate) items.push(launcherUpdate);

    const friendRequests = friendRequestNotification($incomingCount, $incomingPeak);
    if (friendRequests) items.push(friendRequests);

    for (const source of $sources) {
      if (!source.lastError) continue;
      items.push({
        id: `source:${source.id}:${fingerprint(source.lastError)}`,
        title: source.name,
        text: sourceErrorText(source.lastError),
        route: 'sources',
        terminal: false,
      });
    }

    for (const update of $updates) {
      if (update.error) {
        items.push({
          id: `update-error:${update.gameId}:${fingerprint(update.error)}`,
          title: update.title,
          text: msg('state.notifUpdateFailed', { error: updateErrorText(update.error) }),
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
        text: msg('state.notifInstallProgress', {
          percent: installTotalUnknown(install) ? bytesSize(install.bytesDone) : pct(install.progress),
        }),
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
  const alertsPending = !get(watchAlertsLoaded);
  const stale = [...current].filter((id) => !liveIds.has(id) && !(alertsPending && id.startsWith('watch:')));
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
