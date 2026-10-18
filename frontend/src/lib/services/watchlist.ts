import { Service as WatchlistService } from '../../../bindings/typhon/internal/watchlist';
import type { Alert, Entry, Status } from '../../../bindings/typhon/internal/watchlist';
import { inWails } from './backend';

export type { Alert, Entry, Status };

const unavailable = () => new Error('unavailable in browser');

const emptyStatus: Status = { degraded: false, message: '' };

export async function listWatched(): Promise<Entry[]> {
  if (!inWails) return [];
  return ((await WatchlistService.List()) ?? []) as unknown as Entry[];
}

export async function listAlerts(): Promise<Alert[]> {
  if (!inWails) return [];
  return ((await WatchlistService.Alerts()) ?? []) as unknown as Alert[];
}

export async function getWatchlistStatus(): Promise<Status> {
  if (!inWails) return emptyStatus;
  return (await WatchlistService.StatusOf()) as unknown as Status;
}

export async function watchGame(gameId: string): Promise<Entry> {
  if (!inWails) throw unavailable();
  return (await WatchlistService.Watch(gameId)) as unknown as Entry;
}

export async function unwatchGame(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await WatchlistService.Unwatch(gameId);
}

export async function dismissWatchAlert(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await WatchlistService.DismissAlert(id);
}

export async function dismissWatchAlerts(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await WatchlistService.DismissAlerts(gameId);
}
