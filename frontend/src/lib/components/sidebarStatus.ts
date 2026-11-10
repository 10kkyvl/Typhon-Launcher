import type { CloudStatus } from '../services/cloudsaves';
import type { SelfUpdateProgress, SelfUpdateStatus } from '../services/selfupdate';
import { openCloudConflict } from '../stores/cloudsaves';
import { navigate } from '../stores/router';
import { requestDownload, retryFailed } from '../stores/selfupdate';

export type CloudRow =
  | { kind: 'none' | 'off' | 'signedOut' | 'offline' | 'pending' | 'synced' }
  | { kind: 'conflict'; gameId: string; title: string; count: number }
  | { kind: 'failed'; gameId: string; title: string };

export type UpdateRow =
  | { kind: 'none' }
  | { kind: 'version' | 'available' | 'ready' | 'failed'; version: string }
  | { kind: 'downloading'; version: string; percent: number };

// Priority when games differ: conflict, then failed, then (online) pending, then synced.
export function cloudRow(status: CloudStatus | null, offline: boolean, games: readonly { id: string; title: string }[]): CloudRow {
  if (!status) return { kind: 'none' };
  if (!status.enabled) return { kind: 'off' };
  if (!status.signedIn) return { kind: 'signedOut' };
  const title = (gameId: string) => games.find((g) => g.id === gameId)?.title ?? gameId;
  const conflicts = status.games.filter((g) => g.state === 'conflict');
  if (conflicts.length > 0) {
    const gameId = conflicts[0].gameId;
    return { kind: 'conflict', gameId, title: title(gameId), count: conflicts.length };
  }
  const failed = status.games.find((g) => g.state === 'failed');
  if (failed) return { kind: 'failed', gameId: failed.gameId, title: title(failed.gameId) };
  // Without the server nothing syncs: neither "synced" nor "syncing" is true.
  if (offline) return { kind: 'offline' };
  if (status.pending || status.games.some((g) => g.state === 'pending')) return { kind: 'pending' };
  return { kind: 'synced' };
}

// `view` is selfUpdateView. A failed check says nothing about an update, so the row keeps showing
// the version; only a failure with an update on offer is shown as failed.
export function updateRow(
  status: SelfUpdateStatus,
  view: string,
  downloading: boolean,
  progress: SelfUpdateProgress | null,
): UpdateRow {
  if (!status.currentVersion) return { kind: 'none' };
  const next = status.availableVersion;
  if (!next) return { kind: 'version', version: status.currentVersion };
  if (view === 'failed') return { kind: 'failed', version: next };
  if (downloading || view === 'downloading') {
    const total = progress?.totalBytes ?? status.totalBytes ?? 0;
    const done = progress?.downloadedBytes ?? status.downloadedBytes ?? 0;
    const percent = total > 0 ? Math.round(Math.min(100, Math.max(0, (done / total) * 100))) : 0;
    return { kind: 'downloading', version: next, percent };
  }
  if (view === 'ready') return { kind: 'ready', version: next };
  if (view === 'available') return { kind: 'available', version: next };
  return { kind: 'version', version: status.currentVersion };
}

export function openCloudRow(row: CloudRow): void {
  switch (row.kind) {
    case 'off':
    case 'signedOut':
      navigate('settings', { tab: 'downloads' });
      return;
    case 'conflict':
      openCloudConflict(row.gameId);
      return;
    case 'failed':
      navigate('game', { id: row.gameId });
      return;
  }
}

// The store actions report their own failures with a toast and never reject.
export async function runUpdateRow(row: UpdateRow, askRestart: () => void): Promise<void> {
  switch (row.kind) {
    case 'available':
      await requestDownload();
      return;
    case 'failed':
      await retryFailed();
      return;
    case 'ready':
      askRestart();
      return;
  }
}
