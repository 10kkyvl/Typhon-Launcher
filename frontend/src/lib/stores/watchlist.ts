import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  dismissWatchAlert,
  dismissWatchAlerts,
  getWatchlistStatus,
  listAlerts,
  listWatched,
  unwatchGame,
  watchGame,
  type Alert,
  type Entry,
  type Status,
} from '../services/watchlist';
import { msg } from '../i18n';
import { libraryErrorText } from '../library/errors';
import { toast } from './toasts';

export const watchlist = writable<Entry[]>([]);
export const watchAlerts = writable<Alert[]>([]);
export const watchAlertsLoaded = writable(false);
export const watchlistStatus = writable<Status>({ degraded: false, message: '' });

export const watchedIds = derived(watchlist, ($watchlist) => new Set($watchlist.map((entry) => entry.gameId)));

function errorText(err: unknown): string {
  return libraryErrorText(err, msg('errLibrary.watchlistErrFallback'));
}

function mergeAlerts(current: Alert[], fresh: Alert[]): Alert[] {
  const known = new Set(current.map((alert) => alert.id));
  return [...current, ...fresh.filter((alert) => !known.has(alert.id))];
}

export async function initWatchlist() {
  try {
    watchlist.set(await listWatched());
    watchAlerts.set(await listAlerts());
    watchAlertsLoaded.set(true);
    watchlistStatus.set(await getWatchlistStatus());
  } catch (err) {
    toast(errorText(err), 'danger');
  }
  if (!inWails) return;

  Events.On('watchlist:alert', (event) => {
    const fresh = event.data as Alert[];
    watchAlerts.update((list) => mergeAlerts(list, fresh));
    for (const alert of fresh) toast(msg('games.watchAlertToast', { title: alert.title }));
  });
  Events.On('watchlist:updated', (event) => {
    watchlist.set(event.data as Entry[]);
    watchlistStatus.set({ degraded: false, message: '' });
  });
  Events.On('watchlist:degraded', (event) => {
    const status = event.data as Status;
    watchlistStatus.set(status);
    if (status.degraded) toast(msg('games.watchDegraded', { message: status.message }), 'danger');
  });
}

export async function toggleWatch(gameId: string) {
  const watched = get(watchedIds).has(gameId);
  try {
    if (watched) {
      await unwatchGame(gameId);
      watchlist.update((list) => list.filter((entry) => entry.gameId !== gameId));
      watchAlerts.update((list) => list.filter((alert) => alert.gameId !== gameId));
    } else {
      const entry = await watchGame(gameId);
      watchlist.update((list) => (list.some((e) => e.gameId === entry.gameId) ? list : [...list, entry]));
    }
  } catch (err) {
    toast(errorText(err), 'danger');
  }
}

export async function dismissAlert(id: string) {
  try {
    await dismissWatchAlert(id);
    watchAlerts.update((list) => list.filter((alert) => alert.id !== id));
  } catch (err) {
    toast(errorText(err), 'danger');
  }
}

export async function dismissAlerts(gameId: string) {
  try {
    await dismissWatchAlerts(gameId);
    watchAlerts.update((list) => list.filter((alert) => alert.gameId !== gameId));
  } catch (err) {
    toast(errorText(err), 'danger');
  }
}
