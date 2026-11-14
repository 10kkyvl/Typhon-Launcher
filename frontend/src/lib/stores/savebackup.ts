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

/** A saves action that runs: its kind, the backup it is about (restore, delete), and who started it. */
export interface SavesWork {
  kind: 'create' | 'restore' | 'delete' | 'sync' | 'pull' | 'delete-remote';
  snapshot?: string;
  by: symbol;
}

/** What runs for one game's saves, by lane: one backup action and one cloud action at a time. */
export interface SavesInFlight {
  backup?: SavesWork;
  cloud?: SavesWork;
}

export type SavesLane = keyof SavesInFlight;

const flights = writable<ReadonlyMap<string, SavesInFlight>>(new Map());

/**
 * The saves actions that run, by game. They outlive the view that started them (the dialog closed, the
 * game page's tab left): a view mounted meanwhile shows them busy and starts no second one.
 */
export const savesInFlight: Readable<ReadonlyMap<string, SavesInFlight>> = { subscribe: flights.subscribe };

function setWork(gameId: string, lane: SavesLane, work: SavesWork | undefined) {
  flights.update((map) => {
    const next = new Map(map);
    const entry = { ...next.get(gameId), [lane]: work };
    if (entry.backup || entry.cloud) next.set(gameId, entry);
    else next.delete(gameId);
    return next;
  });
}

/**
 * Runs `action` for `gameId` in `lane` unless an action already runs there; answers whether it ran.
 * The lane is free again on every outcome, and a rejection reaches the caller.
 */
export async function runSavesWork(gameId: string, lane: SavesLane, work: SavesWork, action: () => Promise<void>): Promise<boolean> {
  if (get(flights).get(gameId)?.[lane]) return false;
  setWork(gameId, lane, work);
  try {
    await action();
  } finally {
    setWork(gameId, lane, undefined);
  }
  return true;
}

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
