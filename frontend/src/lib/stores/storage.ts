import { get, writable } from 'svelte/store';
import { getStorageInfo, type StorageInfo } from '../services/system';
import { installedGames } from './library';
import { activeMoves } from './relocate';
import { settings } from './settings';

export const storageInfo = writable<StorageInfo | null>(null);
export const storageFailed = writable(false);

let seq = 0;

export async function refreshStorage() {
  const mine = ++seq;
  if (!get(settings)?.libraryPath) {
    storageInfo.set(null);
    storageFailed.set(false);
    return;
  }
  try {
    const info = await getStorageInfo();
    if (mine !== seq) return;
    storageInfo.set(info);
    storageFailed.set(false);
  } catch {
    if (mine !== seq) return;
    storageInfo.set(null);
    storageFailed.set(true);
  }
}

/**
 * For the app, once: reads the numbers again when the library folder Go measures changes, whatever
 * screen is shown (Disk usage reads them without Installed mounted). The first settings are the
 * folder already read at start; another setting changing reads nothing. Answers the disposer.
 */
export function refreshStorageOnFolderChange(): () => void {
  let path: string | undefined;
  return settings.subscribe((value) => {
    if (!value) return;
    if (path !== undefined && value.libraryPath !== path) void refreshStorage();
    path = value.libraryPath;
  });
}

/**
 * Keeps the numbers true while a screen shows them: reads them now, then again when the library
 * folder changes (the one Go measures), when an installed game comes, goes, moves or is measured,
 * and when any move ends. A game that only changes otherwise (played, renamed) reads nothing. Answers
 * the disposer.
 */
export function followStorage(): () => void {
  let path: string | undefined;
  let games: string | undefined;
  let moving = new Set<string>();
  const offs = [
    settings.subscribe((value) => {
      const next = value?.libraryPath ?? '';
      if (path !== undefined && next !== path) void refreshStorage();
      path = next;
    }),
    installedGames.subscribe((list) => {
      const next = list.map((game) => [game.id, game.installDir, game.sizeBytes, game.sizeUnknown ? 1 : 0].join('\t')).join('\n');
      if (games !== undefined && next !== games) void refreshStorage();
      games = next;
    }),
    activeMoves.subscribe((jobs) => {
      const next = new Set(jobs.map((job) => job.id));
      if ([...moving].some((id) => !next.has(id))) void refreshStorage();
      moving = next;
    }),
  ];
  void refreshStorage();
  return () => {
    for (const off of offs) off();
  };
}
