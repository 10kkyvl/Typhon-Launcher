import type { LibraryGame } from '../services/library';

export function recentLocalGames(games: readonly LibraryGame[]): LibraryGame[] {
  return games
    .toSorted((a, b) => {
      const played = (value: string | null) => {
        const time = value ? Date.parse(value) : NaN;
        return Number.isFinite(time) ? time : 0;
      };
      return played(b.lastPlayed) - played(a.lastPlayed) || a.title.localeCompare(b.title);
    });
}
