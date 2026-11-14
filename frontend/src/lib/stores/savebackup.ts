import { derived, get, writable, type Readable } from 'svelte/store';
import { isEmulated, type EmulatedGame } from '../library/emulation';
import { backupsEnabled, onBackupEvent } from '../services/savebackup';
import { backupNotice } from '../savebackup/messages';
import { msg } from '../i18n';
import { libraryGames } from './library';
import { settings } from './settings';
import { toast } from './toasts';

export interface SaveBackupTarget {
  gameId: string;
  title: string;
}

export const saveBackupTarget = writable<SaveBackupTarget | null>(null);
export const saveBackupsEnabled = writable(false);

export type SavesTabGame = EmulatedGame & { canonicalGameId?: string };

/**
 * Whether the game page gives a library game a Saves tab: not an emulated one (its saves live in the
 * emulator), and only when there is something to show: backups on, or a cloud copy the game can have
 * (cloud saves on and the game known to the catalog). With backups off the tab holds the cloud part
 * alone. The "Saves…" menu item is shown on its own condition (backups on).
 */
export const savesTab: Readable<(game: SavesTabGame | null | undefined) => boolean> = derived(
  [saveBackupsEnabled, settings],
  ([backups, current]) =>
    (game: SavesTabGame | null | undefined) =>
      Boolean(game) && !isEmulated(game) && (backups || (Boolean(current?.cloudSaves) && Boolean(game?.canonicalGameId))),
);

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
