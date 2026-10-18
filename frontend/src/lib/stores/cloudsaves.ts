import { get, writable } from 'svelte/store';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { inWails } from '../services/backend';
import { loadCloudStatus, onCloudEvent, type CloudEvent, type CloudStatus } from '../services/cloudsaves';
import { libraryGames } from './library';
import { toast } from './toasts';

export interface CloudConflictTarget {
  gameId: string;
  title: string;
}

export const cloudStatus = writable<CloudStatus | null>(null);
export const cloudConflictTarget = writable<CloudConflictTarget | null>(null);

export function openCloudConflict(gameId: string) {
  const game = get(libraryGames).find((g) => g.id === gameId);
  if (!game) return;
  cloudConflictTarget.set({ gameId, title: game.title });
}

export function closeCloudConflict() {
  cloudConflictTarget.set(null);
}

export async function refreshCloudStatus(): Promise<boolean> {
  if (!inWails) return false;
  try {
    cloudStatus.set(await loadCloudStatus());
    return true;
  } catch (err) {
    toast(cloudErrorText(err, msg('cloudsaves.statusFailed')), 'danger');
    return false;
  }
}

function applyEvent(event: CloudEvent) {
  const now = new Date().toISOString();
  cloudStatus.update((status) => {
    if (!status) return status;
    const next: CloudStatus = { ...status };
    if (event.quotaBytes > 0) {
      next.usedBytes = event.usedBytes;
      next.quotaBytes = event.quotaBytes;
    }
    if (!event.gameId) {
      next.pending = event.status === 'pending';
      return next;
    }
    const games = status.games.filter((g) => g.gameId !== event.gameId);
    const previous = status.games.find((g) => g.gameId === event.gameId);
    games.push({
      gameId: event.gameId,
      state: event.status,
      code: event.code,
      message: event.message,
      at: now,
      syncedAt: event.status === 'synced' ? now : (previous?.syncedAt ?? ''),
      conflict: event.status === 'conflict' ? previous?.conflict : undefined,
    });
    next.games = games;
    return next;
  });
}

export function initCloudSaves() {
  if (!inWails) return () => {};
  void refreshCloudStatus();
  return onCloudEvent((event) => {
    const wasConflict = get(cloudStatus)?.games.some((g) => g.gameId === event.gameId && g.state === 'conflict');
    applyEvent(event);
    if (event.status !== 'conflict' || !event.gameId) return;
    void refreshCloudStatus();
    if (!wasConflict) openCloudConflict(event.gameId);
  });
}
