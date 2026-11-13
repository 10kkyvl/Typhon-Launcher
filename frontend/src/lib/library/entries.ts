import type { Download } from '../services/downloads';
import type { LibraryGame } from '../services/library';
import type { GameArt } from '../services/metadata';
import { emulationPlatform } from './emulation';
import { canPlay } from './launch';
import { gameSource, type GameSource } from './source';

/** What the catalog says of a game a download belongs to; `null` in a title map: the catalog does not know the id. */
export interface CatalogTitle {
  title: string;
  releaseYear?: number;
  developer?: string;
}

export interface WatchedItem {
  gameId: string;
  title: string;
}

/**
 * One line of the library as every screen lists it: a library game, a download that is not installed
 * yet (keyed by its catalog id), or a watched game. Art is not part of an entry: it arrives on its own
 * and is looked up when drawn (`entryCover`, `entryHeroArt`), so a cover arriving rebuilds nothing.
 */
export interface LibraryEntry {
  readonly id: string;
  readonly kind: 'game' | 'download' | 'watched';
  readonly title: string;
  readonly installed: boolean;
  readonly playable: boolean;
  readonly hidden: boolean;
  readonly favorite: boolean;
  readonly playtimeSeconds: number;
  readonly sizeBytes: number;
  readonly lastPlayed: string | null;
  readonly installedAt: string;
  /** The catalog id art and genres are kept under. */
  readonly catalogId?: string;
  readonly tags?: string[];
  readonly status?: string;
  readonly completedAt?: string | null;
  readonly source: GameSource | null;
  readonly platform?: string;
  readonly game?: LibraryGame;
  readonly download?: Pick<Download, 'name' | 'status' | 'total'>;
  readonly catalog?: CatalogTitle;
}

export function gameEntry(game: LibraryGame): LibraryEntry {
  return {
    id: game.id,
    kind: 'game',
    title: game.title,
    installed: !game.uninstalled,
    playable: canPlay(game),
    hidden: Boolean(game.hidden),
    favorite: Boolean(game.favorite),
    playtimeSeconds: game.playtimeSeconds,
    sizeBytes: game.sizeBytes,
    lastPlayed: game.lastPlayed,
    installedAt: game.installedAt,
    catalogId: game.canonicalGameId || undefined,
    tags: game.tags,
    status: game.status,
    completedAt: game.completedAt,
    source: gameSource(game),
    platform: emulationPlatform(game),
    game,
  };
}

export function downloadEntry(gameId: string, item: Download, catalog: CatalogTitle | null | undefined): LibraryEntry {
  return {
    id: gameId,
    kind: 'download',
    title: catalog?.title || item.name,
    installed: false,
    playable: false,
    hidden: false,
    favorite: false,
    playtimeSeconds: 0,
    sizeBytes: item.total,
    lastPlayed: null,
    installedAt: '',
    catalogId: gameId,
    source: 'typhon',
    download: { name: item.name, status: item.status, total: item.total },
    catalog: catalog ?? undefined,
  };
}

export function watchedEntry(item: WatchedItem): LibraryEntry {
  return {
    id: item.gameId,
    kind: 'watched',
    title: item.title,
    installed: false,
    playable: false,
    hidden: false,
    favorite: false,
    playtimeSeconds: 0,
    sizeBytes: 0,
    lastPlayed: null,
    installedAt: '',
    catalogId: item.gameId,
    source: null,
  };
}

function addedAt(item: Download): number {
  const time = Date.parse(item.addedAt);
  return Number.isNaN(time) ? 0 : time;
}

/** The newest download of each catalog game the library does not hold yet. */
export function downloadsByGame(downloads: readonly Download[], games: readonly LibraryGame[]): Map<string, Download> {
  const owned = new Set<string>();
  for (const game of games) if (game.canonicalGameId) owned.add(game.canonicalGameId);
  const map = new Map<string, Download>();
  for (const item of downloads) {
    const gameId = item.origin?.gameId;
    if (!gameId || owned.has(gameId)) continue;
    const current = map.get(gameId);
    if (!current || addedAt(item) >= addedAt(current)) map.set(gameId, item);
  }
  return map;
}

export function entryCover(entry: LibraryEntry, art: Readonly<Record<string, GameArt>>): string {
  const own = entry.catalogId ? art[entry.catalogId] : undefined;
  return own?.cover || entry.game?.cover || '';
}

export function entryHeroArt(entry: LibraryEntry, art: Readonly<Record<string, GameArt>>): string {
  return (entry.catalogId ? art[entry.catalogId]?.hero : undefined) ?? '';
}

function sameList<T>(next: T[], previous: T[]): T[] {
  return next.length === previous.length && next.every((item, i) => item === previous[i]) ? previous : next;
}

/**
 * Builds the entries and keeps them: an input that did not change gives back the same entry object,
 * and a list whose entries all stayed gives back the same array, so a download progress tick (new
 * download objects, same status and size) re-sorts nothing downstream.
 */
export function createEntryCache() {
  const games = new WeakMap<LibraryGame, LibraryEntry>();
  let downloads = new Map<string, LibraryEntry>();
  let watched = new Map<string, LibraryEntry>();
  let lastLibrary: LibraryEntry[] = [];
  let lastWatched: LibraryEntry[] = [];

  function ofGame(game: LibraryGame): LibraryEntry {
    let entry = games.get(game);
    if (!entry) {
      entry = gameEntry(game);
      games.set(game, entry);
    }
    return entry;
  }

  function ofDownload(next: Map<string, LibraryEntry>, gameId: string, item: Download, catalog: CatalogTitle | null | undefined) {
    const before = downloads.get(gameId);
    const fresh =
      before &&
      before.download?.name === item.name &&
      before.download.status === item.status &&
      before.download.total === item.total &&
      before.catalog === (catalog ?? undefined)
        ? before
        : downloadEntry(gameId, item, catalog);
    next.set(gameId, fresh);
    return fresh;
  }

  return {
    library(
      libraryGames: readonly LibraryGame[],
      allDownloads: readonly Download[],
      titles: Readonly<Record<string, CatalogTitle | null>>,
    ): LibraryEntry[] {
      const next = new Map<string, LibraryEntry>();
      const list = libraryGames.map(ofGame);
      for (const [gameId, item] of downloadsByGame(allDownloads, libraryGames)) {
        list.push(ofDownload(next, gameId, item, titles[gameId]));
      }
      downloads = next;
      lastLibrary = sameList(list, lastLibrary);
      return lastLibrary;
    },
    watched(items: readonly WatchedItem[]): LibraryEntry[] {
      const next = new Map<string, LibraryEntry>();
      const list = items.map((item) => {
        const before = watched.get(item.gameId);
        const entry = before && before.title === item.title ? before : watchedEntry(item);
        next.set(item.gameId, entry);
        return entry;
      });
      watched = next;
      lastWatched = sameList(list, lastWatched);
      return lastWatched;
    },
  };
}
