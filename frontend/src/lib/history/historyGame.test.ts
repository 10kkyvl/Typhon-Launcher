import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import { historyGames } from './historyGame';

const game = (id: string, canonicalGameId?: string): LibraryGame =>
  ({ id, title: id, canonicalGameId }) as unknown as LibraryGame;

describe('historyGames', () => {
  const lookup = historyGames([game('lib-a', 'cat-a'), game('lib-b'), game('lib-a2', 'cat-a'), game('cat-b')]);

  it('finds a game by its library id', () => {
    expect(lookup('lib-b')?.id).toBe('lib-b');
  });

  it('finds a game by its catalog id, the first of the library when two share it', () => {
    expect(lookup('cat-a')?.id).toBe('lib-a');
  });

  it('prefers a library id over a catalog id', () => {
    expect(historyGames([game('lib-x', 'same'), game('same')])('same')?.id).toBe('same');
  });

  it('finds nothing for a game the library does not hold, or for no id', () => {
    expect(lookup('gone')).toBeUndefined();
    expect(lookup(undefined)).toBeUndefined();
    expect(lookup('')).toBeUndefined();
  });
});
