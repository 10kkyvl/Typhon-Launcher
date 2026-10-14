import type { LibraryCollection, LibraryGame } from '../services/library';
import { collectionShelfId, matchesCollection } from '../library/collections';

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
export type LibrarySort = 'title' | 'recent' | 'playtime' | 'added';
export function filterLibrary(games: LibraryGame[], query: string, filter: LibraryFilter, sort: LibrarySort,
  context: CollectionContext = noCollections): LibraryGame[] {
  const collection = filter.startsWith('collection:') ? context.collections.find((c) => collectionShelfId(c.id) === filter) : undefined;
  const needle = query.trim().normalize('NFKC').toLocaleLowerCase();
  const filtered = games.filter((game) => (!needle || game.title.normalize('NFKC').toLocaleLowerCase().includes(needle)) &&
    (filter === 'hidden' ? Boolean(game.hidden) : !game.hidden &&
      (filter === 'all' || (filter === 'installed' && !game.uninstalled) ||
      (filter === 'uninstalled' && game.uninstalled) || (filter === 'favorites' && game.favorite) ||
      (collection !== undefined && inCollection(game, collection, context.genres)))));
  return filtered.toSorted((a, b) => {
    const difference = sort === 'recent' ? timestamp(b.lastPlayed) - timestamp(a.lastPlayed)
      : sort === 'playtime' ? b.playtimeSeconds - a.playtimeSeconds
      : sort === 'added' ? timestamp(b.installedAt) - timestamp(a.installedAt) : 0;
    return difference || a.title.localeCompare(b.title) || a.id.localeCompare(b.id);
  });
}
