import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import { Kind } from '../../../bindings/typhon/internal/history';
import type { GameArt } from '../services/metadata';
import { historyArt, historyGames } from './historyGame';

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

describe('historyArt', () => {
  const art: Record<string, GameArt> = { 'cat-a': { cover: 'https://art/a.jpg', hero: '', heroIsPoster: false } };
  const own = { id: 'lib-a', cover: 'file:///a.jpg', canonicalGameId: 'cat-a' };

  it("takes a library game's catalog id, the art store's cover first, its own cover next", () => {
    expect(historyArt({ kind: Kind.KindInstalled, gameId: 'lib-a' }, own, art)).toEqual({ key: 'cat-a', cover: 'https://art/a.jpg', seed: 'cat-a' });
    expect(historyArt({ kind: Kind.KindInstalled, gameId: 'lib-a' }, own, {})).toEqual({ key: 'cat-a', cover: 'file:///a.jpg', seed: 'cat-a' });
  });

  it('seeds a library game of no catalog id by its library id', () => {
    expect(historyArt({ kind: Kind.KindInstalled, gameId: 'lib-x' }, { id: 'lib-x', cover: '' }, art)).toEqual({ key: '', cover: '', seed: 'lib-x' });
  });

  it('asks by the record id only where Go recorded a catalog id', () => {
    expect(historyArt({ kind: Kind.KindDownloaded, gameId: 'cat-a' }, undefined, art).key).toBe('cat-a');
    expect(historyArt({ kind: Kind.KindInstallFailed, gameId: 'cat-a' }, undefined, art).cover).toBe('https://art/a.jpg');
    expect(historyArt({ kind: Kind.KindUpdated, gameId: 'cat-u' }, undefined, art).key).toBe('cat-u');
    expect(historyArt({ kind: Kind.KindRolledBack, gameId: 'cat-r' }, undefined, art).key).toBe('cat-r');
    expect(historyArt({ kind: Kind.KindRemoved, gameId: 'lib-gone' }, undefined, art)).toEqual({ key: '', cover: '', seed: 'lib-gone' });
    expect(historyArt({ kind: Kind.KindMoved, gameId: undefined }, undefined, art)).toEqual({ key: '', cover: '', seed: '' });
  });
});
