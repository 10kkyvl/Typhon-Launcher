import type { LibraryGame } from '../services/library';

export type HintKind = 'playing' | 'mastered';

type Candidate = Pick<LibraryGame, 'status' | 'playtimeSeconds'> | undefined;

export function hintKey(kind: HintKind, gameId: string): string {
  return `${kind}:${gameId}`;
}

export function shouldSuggestPlaying(game: Candidate, gameId: string, shown: ReadonlySet<string>): boolean {
  if (!game || game.status) return false;
  if (game.playtimeSeconds !== 0) return false;
  return !shown.has(hintKey('playing', gameId));
}

export function shouldSuggestMastered(
  game: Candidate,
  progress: { total: number; unlocked: number },
  gameId: string,
  shown: ReadonlySet<string>,
): boolean {
  if (!game || game.status === 'mastered') return false;
  if (progress.total <= 0 || progress.unlocked !== progress.total) return false;
  return !shown.has(hintKey('mastered', gameId));
}
