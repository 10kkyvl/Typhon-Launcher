import { get, writable } from 'svelte/store';
import { backupsEnabled, onBackupEvent } from '../services/savebackup';
import { backupNotice } from '../savebackup/messages';
import { msg } from '../i18n';
import { libraryGames } from './library';
import { toast } from './toasts';

export interface SaveBackupTarget {
  gameId: string;
  title: string;
}

export const saveBackupTarget = writable<SaveBackupTarget | null>(null);
export const saveBackupsEnabled = writable(false);

export function openSaveBackups(gameId: string) {
  const game = get(libraryGames).find((g) => g.id === gameId);
  if (!game) return;
  saveBackupTarget.set({ gameId, title: game.title });
}

export function closeSaveBackups() {
  saveBackupTarget.set(null);
}

export function initSaveBackups() {
  backupsEnabled().then(
    (on) => saveBackupsEnabled.set(on),
    () => saveBackupsEnabled.set(false),
  );
  onBackupEvent((event) => {
    const title = get(libraryGames).find((g) => g.id === event.gameId)?.title ?? '';
    const notice = backupNotice(event, title);
    if (notice) toast(notice.text, notice.kind === 'info' ? undefined : notice.kind);
  });
}
