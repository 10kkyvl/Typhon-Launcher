import { get } from 'svelte/store';
import { locale } from '../i18n/locale';
import type { LibraryCollection, LibraryGame } from '../services/library';
import { collectionShelfId, matchesCollection } from '../library/collections';
import { gameEntry, type LibraryEntry } from '../library/entries';
import { queryLibrary, type LibrarySort as ModelSort } from '../library/query';

export type ShelfID = 'recent' | 'favorites' | 'installed' | `collection:${string}`;

export interface CollectionContext {
  collections: readonly LibraryCollection[];
  genres: Readonly<Record<string, string[]>>;
}

const noCollections: CollectionContext = { collections: [], genres: {} };

function inCollection(game: LibraryGame, collection: LibraryCollection, genres: CollectionContext['genres']) {
  return matchesCollection(game, game.canonicalGameId ? genres[game.canonicalGameId] : undefined, collection);
}
export interface Shelf {
  id: ShelfID;
  games: LibraryGame[];
}

function timestamp(value: string | null | undefined): number {
  const time = value ? Date.parse(value) : NaN;
  return Number.isFinite(time) ? time : 0;
}

export function buildShelves(games: LibraryGame[], context: CollectionContext = noCollections): Shelf[] {
  const installed = games.filter((game) => !game.uninstalled && !game.hidden);
  const alphabetical = (a: LibraryGame, b: LibraryGame) => a.title.localeCompare(b.title) || a.id.localeCompare(b.id);
  const shelves: Shelf[] = [
    { id: 'recent', games: installed.filter((game) => timestamp(game.lastPlayed) > 0)
      .toSorted((a, b) => timestamp(b.lastPlayed) - timestamp(a.lastPlayed) || alphabetical(a, b)).slice(0, 12) },
    { id: 'favorites', games: installed.filter((game) => game.favorite)
      .toSorted((a, b) => timestamp(b.favoriteAt) - timestamp(a.favoriteAt) || alphabetical(a, b)) },
    ...context.collections.map((collection): Shelf => ({
      id: collectionShelfId(collection.id),
      games: installed.filter((game) => inCollection(game, collection, context.genres)).toSorted(alphabetical),
    })),
    { id: 'installed', games: installed.toSorted(alphabetical) },
  ];
  return shelves.filter((shelf) => shelf.games.length > 0);
}

export function resolveSelection(shelves: Shelf[], id: string): LibraryGame | undefined {
  return shelves.flatMap((shelf) => shelf.games).find((game) => game.id === id) ?? shelves[0]?.games[0];
}

export type LibraryFilter = 'all' | 'installed' | 'favorites' | 'uninstalled' | 'hidden' | `collection:${string}`;
export type LibrarySort = Extract<ModelSort, 'title' | 'recent' | 'playtime' | 'added'>;

const entries = new WeakMap<LibraryGame, LibraryEntry>();

function entryOf(game: LibraryGame): LibraryEntry {
  let entry = entries.get(game);
  if (!entry) {
    entry = gameEntry(game);
    entries.set(game, entry);
  }
  return entry;
}

/** The desktop library's query over the library games, with Big Picture's filter names. */
export function filterLibrary(games: LibraryGame[], query: string, filter: LibraryFilter, sort: LibrarySort,
  context: CollectionContext = noCollections, language: string = get(locale)): LibraryGame[] {
  const shelf = filter.startsWith('collection:');
  const collection = shelf ? context.collections.find((c) => collectionShelfId(c.id) === filter) : undefined;
  if (shelf && !collection) return [];
  const listed = queryLibrary(games.map(entryOf), [], {
    filter: shelf ? 'all' : (filter as Exclude<LibraryFilter, `collection:${string}`>),
    source: 'all', status: 'all', collection, search: query, sort,
  }, { genres: context.genres, locale: language });
  return listed.map((entry) => entry.game).filter((game): game is LibraryGame => game !== undefined);
}
