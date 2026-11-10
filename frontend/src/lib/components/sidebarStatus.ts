import type { CloudStatus } from '../services/cloudsaves';
import type { SelfUpdateProgress, SelfUpdateStatus } from '../services/selfupdate';
import { accountErrorText } from '../services/accountMessages';
import { msg } from '../i18n';
import { openCloudConflict } from '../stores/cloudsaves';
import { navigate } from '../stores/router';
import { requestDownload, retryFailed } from '../stores/selfupdate';
import { toast } from '../stores/toasts';
import { leaveGuest } from '../stores/user';

export type CloudRow =
  | { kind: 'none' | 'off' | 'signIn' | 'offline' | 'pending' | 'synced' }
  | { kind: 'account' | 'error'; code: string }
  | { kind: 'conflict'; gameId: string; title: string; count: number }
  | { kind: 'failed'; gameId: string; title: string };

export type UpdateRow =
  | { kind: 'none' }
  | { kind: 'version' | 'available' | 'ready' | 'failed'; version: string }
  | { kind: 'downloading'; version: string; percent: number };

// Priority when games differ: conflict, then a failed game, then a failed pass, then (online) pending,
// then synced. Only games the library still has are counted: the row could not open the others.
export function cloudRow(
  status: CloudStatus | null,
  who: { offline: boolean; guest: boolean },
  games: readonly { id: string; title: string }[],
): CloudRow {
  if (!status) return { kind: 'none' };
  if (!status.enabled) return { kind: 'off' };
  // A guest signs in from here; a signed-in user whose cloud has no token cannot fix it from the row.
  if (!status.signedIn) return who.guest ? { kind: 'signIn' } : { kind: 'account', code: status.error };
  const titles = new Map(games.map((g) => [g.id, g.title]));
  const known = status.games.filter((g) => titles.has(g.gameId));
  const conflicts = known.filter((g) => g.state === 'conflict');
  if (conflicts.length > 0) {
    const gameId = conflicts[0].gameId;
    return { kind: 'conflict', gameId, title: titles.get(gameId) ?? '', count: conflicts.length };
  }
  const failed = known.find((g) => g.state === 'failed');
  if (failed) return { kind: 'failed', gameId: failed.gameId, title: titles.get(failed.gameId) ?? '' };
  if (status.error) return { kind: 'error', code: status.error };
  // Without the server nothing syncs: neither "synced" nor "syncing" is true.
  if (who.offline) return { kind: 'offline' };
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

// The account menu's sign-in for a guest, with its error text.
export async function signInFromGuest(): Promise<void> {
  try {
    await leaveGuest();
  } catch (err) {
    toast(accountErrorText(err, msg('ui.signOutFailed')), 'danger');
  }
}

export async function openCloudRow(row: CloudRow): Promise<void> {
  switch (row.kind) {
    case 'off':
      navigate('settings', { tab: 'downloads' });
      return;
    case 'signIn':
      await signInFromGuest();
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
