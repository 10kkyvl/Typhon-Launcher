import { isFinished } from '../game/status';
import type { LibraryGame } from '../services/library';
import type { GameCard } from '../services/social';

export const COMPLETED_LIMIT = 12;

export interface CompletedYear<T> {
  year: number;
  total: number;
  games: T[];
}

export function completedThisYear(games: readonly LibraryGame[], now: Date): CompletedYear<LibraryGame> {
  const year = now.getUTCFullYear();
  const done = games
    .filter((game) => !game.hidden && isFinished(game.status) && finishedIn(game.completedAt, year) !== null)
    .map((game) => ({ game, at: finishedIn(game.completedAt, year) as number }))
    .toSorted((a, b) => b.at - a.at || a.game.title.localeCompare(b.game.title, 'ru'));
  return { year, total: done.length, games: done.slice(0, COMPLETED_LIMIT).map((item) => item.game) };
}

function finishedIn(completedAt: string | null | undefined, year: number): number | null {
  if (!completedAt) return null;
  const at = new Date(completedAt);
  return !Number.isNaN(at.getTime()) && at.getUTCFullYear() === year ? at.getTime() : null;
}

export function parseCompletedData(data: unknown): CompletedYear<GameCard> | null {
  if (typeof data !== 'object' || data === null) return null;
  const { year, total, games, error } = data as { year?: unknown; total?: unknown; games?: unknown; error?: unknown };
  if (error) return null;
  if (typeof year !== 'number' || !Number.isInteger(year) || year <= 0) return null;
  if (typeof total !== 'number' || !Number.isInteger(total) || total < 0) return null;
  const list: unknown = games ?? (total === 0 ? [] : null);
  if (!Array.isArray(list)) return null;
  const valid = list.filter(
    (game): game is GameCard =>
      typeof game === 'object' && game !== null && typeof (game as GameCard).igdbId === 'number' && typeof (game as GameCard).title === 'string',
  );
  if (valid.length !== list.length || list.length > total) return null;
  return { year, total, games: valid };
}
