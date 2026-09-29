import { get, writable } from 'svelte/store';
import { backupsEnabled, onBackupEvent } from '../services/savebackup';
import { saveBackupCodeText } from '../savebackup/messages';
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
    if (event.kind === 'manual') return;
    const title = get(libraryGames).find((g) => g.id === event.gameId)?.title ?? '';
    if (event.status === 'failed') {
      const reason = saveBackupCodeText(event.code, msg('savebackup.fallback'));
      toast(msg('saves.autoFailed', { title, reason }), 'danger');
      return;
    }
    if (event.status === 'created' && event.error) {
      const reason = saveBackupCodeText(event.code, msg('savebackup.fallback'));
      toast(msg('saves.autoRotationFailed', { title, reason }));
    }
  });
}
