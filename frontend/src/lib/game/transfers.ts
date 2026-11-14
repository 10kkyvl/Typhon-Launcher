import type { Download, DownloadStatus } from '../services/downloads';
import type { Installation } from '../services/install';
import { installActive } from '../stores/install';
import type { TerminalDownloadStatus } from './view';

/**
 * Which downloads and installs belong to the game a page shows, read the same way by the desktop game
 * page and Big Picture's. A download belongs to the game when its origin names one of the game's
 * catalog ids or its library card; an install, when it installs one of those downloads or that card.
 * Every list is newest first, so "the game's download" is the one the player started last.
 */
export interface GameOwner {
  /** Catalog ids the page knows the game by; empty values are skipped. */
  gameIds: readonly (string | null | undefined)[];
  /** The game's library card, when it has one. */
  libraryId?: string;
}

const activeStatuses: readonly DownloadStatus[] = ['queued', 'metadata', 'downloading', 'paused', 'verifying'];

/** A download that has not finished yet: running, waiting in the queue or paused. */
export function downloadActive(status: DownloadStatus): boolean {
  return activeStatuses.includes(status);
}

export function ownsDownload(item: Download, owner: GameOwner): boolean {
  const origin = item.origin ?? {};
  if (origin.gameId && owner.gameIds.some((id) => id && id === origin.gameId)) return true;
  return Boolean(owner.libraryId && origin.libraryId === owner.libraryId);
}

function newest<T>(list: readonly T[], at: (item: T) => string): T[] {
  const time = (item: T) => {
    const value = Date.parse(at(item));
    return Number.isFinite(value) ? value : 0;
  };
  return list.toSorted((a, b) => time(b) - time(a));
}

/** The game's downloads, newest first. */
export function gameDownloads(list: readonly Download[], owner: GameOwner): Download[] {
  return newest(list.filter((item) => ownsDownload(item, owner)), (item) => item.addedAt);
}

/** The newest of the game's downloads that has not finished. */
export function activeDownload(owned: readonly Download[]): Download | undefined {
  return owned.find((item) => downloadActive(item.status));
}

/** The newest of the game's downloads that completed or failed. */
export function terminalDownload(owned: readonly Download[]): (Download & { status: TerminalDownloadStatus }) | undefined {
  return owned.find(
    (item): item is Download & { status: TerminalDownloadStatus } => item.status === 'completed' || item.status === 'failed',
  );
}

/** The game's installs, newest first: those of its downloads and those of its library card. */
export function gameInstalls(list: readonly Installation[], owned: readonly Download[], libraryId?: string): Installation[] {
  const ids = new Set(owned.map((item) => item.id));
  return newest(
    list.filter((item) => ids.has(item.downloadId) || Boolean(libraryId && item.gameId === libraryId)),
    (item) => item.startedAt,
  );
}

/** The newest of the game's installs that is still running. */
export function activeInstall(installs: readonly Installation[]): Installation | undefined {
  return installs.find((item) => installActive(item.status));
}
