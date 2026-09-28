import type { LibraryGame } from '../services/library';

export type ShelfID = 'recent' | 'favorites' | 'installed';
export interface Shelf {
  id: ShelfID;
  games: LibraryGame[];
}

function timestamp(value: string | null | undefined): number {
  const time = value ? Date.parse(value) : NaN;
  return Number.isFinite(time) ? time : 0;
}

export function buildShelves(games: LibraryGame[]): Shelf[] {
  const installed = games.filter((game) => !game.uninstalled);
  const alphabetical = (a: LibraryGame, b: LibraryGame) => a.title.localeCompare(b.title) || a.id.localeCompare(b.id);
  const shelves: Shelf[] = [
    { id: 'recent', games: installed.filter((game) => timestamp(game.lastPlayed) > 0)
      .toSorted((a, b) => timestamp(b.lastPlayed) - timestamp(a.lastPlayed) || alphabetical(a, b)).slice(0, 12) },
    { id: 'favorites', games: installed.filter((game) => game.favorite)
      .toSorted((a, b) => timestamp(b.favoriteAt) - timestamp(a.favoriteAt) || alphabetical(a, b)) },
    { id: 'installed', games: installed.toSorted(alphabetical) },
  ];
  return shelves.filter((shelf) => shelf.games.length > 0);
}

export function resolveSelection(shelves: Shelf[], id: string): LibraryGame | undefined {
  return shelves.flatMap((shelf) => shelf.games).find((game) => game.id === id) ?? shelves[0]?.games[0];
}

export type LibraryFilter = 'all' | 'installed' | 'favorites' | 'uninstalled';
export type LibrarySort = 'title' | 'recent' | 'playtime' | 'added';
export function filterLibrary(games: LibraryGame[], query: string, filter: LibraryFilter, sort: LibrarySort): LibraryGame[] {
  const needle = query.trim().normalize('NFKC').toLocaleLowerCase();
  const filtered = games.filter((game) => (!needle || game.title.normalize('NFKC').toLocaleLowerCase().includes(needle)) &&
    (filter === 'all' || (filter === 'installed' && !game.uninstalled) ||
      (filter === 'uninstalled' && game.uninstalled) || (filter === 'favorites' && game.favorite)));
  return filtered.toSorted((a, b) => {
    const difference = sort === 'recent' ? timestamp(b.lastPlayed) - timestamp(a.lastPlayed)
      : sort === 'playtime' ? b.playtimeSeconds - a.playtimeSeconds
      : sort === 'added' ? timestamp(b.installedAt) - timestamp(a.installedAt) : 0;
    return difference || a.title.localeCompare(b.title) || a.id.localeCompare(b.id);
  });
}
