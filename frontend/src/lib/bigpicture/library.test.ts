import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import { buildShelves, filterLibrary, resolveSelection } from './library';

function game(id: string, overrides: Partial<LibraryGame> = {}): LibraryGame {
  return { id, title: id, executable: `${id}.exe`, installDir: id, cover: '', version: '', sizeBytes: 0,
    lastPlayed: null, playtimeSeconds: 0, installedAt: '', ...overrides };
}

describe('Big Picture library', () => {
  it('searches saved games independently of installed shelves and combines filters', () => {
    const installed = game('installed', { title: 'Alpha World', favorite: true });
    const removed = game('removed', { title: 'ALPHA Two', favorite: true, uninstalled: true });
    const other = game('other', { title: 'Beta' });
    const games = [installed, removed, other];
    expect(filterLibrary(games, ' alpha ', 'all', 'title')).toEqual([removed, installed]);
    expect(filterLibrary(games, 'alpha', 'installed', 'title')).toEqual([installed]);
    expect(filterLibrary(games, '', 'uninstalled', 'title')).toEqual([removed]);
    expect(filterLibrary(games, 'beta', 'favorites', 'title')).toEqual([]);
    expect(games).toEqual([installed, removed, other]);
  });

  it('sorts by time with deterministic title fallback for missing/invalid dates', () => {
    const a = game('a', { title: 'Alpha', lastPlayed: 'invalid', playtimeSeconds: 1 });
    const z = game('z', { title: 'Zulu', lastPlayed: null, playtimeSeconds: 100 });
    const b = game('b', { title: 'Beta', lastPlayed: '2026-09-22', installedAt: '2026-09-23' });
    expect(filterLibrary([z, b, a], '', 'all', 'recent')).toEqual([b, a, z]);
    expect(filterLibrary([z, b, a], '', 'all', 'playtime')).toEqual([z, a, b]);
    expect(filterLibrary([z, b, a], '', 'all', 'added')).toEqual([b, a, z]);
  });
  it('only offers installed games, even when an uninstalled game is recent and favorite', () => {
    const games = [game('installed'), game('removed', { uninstalled: true, favorite: true, lastPlayed: '2026-09-22' })];
    expect(buildShelves(games).map((shelf) => [shelf.id, shelf.games.map((g) => g.id)]))
      .toEqual([['installed', ['installed']]]);
  });

  it('sorts recent/favorites independently and leaves the source store unchanged', () => {
    const games = [game('Zulu', { lastPlayed: '2026-09-22', favorite: true, favoriteAt: '2026-09-20' }),
      game('Alpha', { lastPlayed: '2026-09-21', favorite: true, favoriteAt: '2026-09-22' }),
      game('Beta', { lastPlayed: 'invalid' })];
    expect(buildShelves(games).map((shelf) => [shelf.id, shelf.games.map((g) => g.id)]))
      .toEqual([['recent', ['Zulu', 'Alpha']], ['favorites', ['Alpha', 'Zulu']], ['installed', ['Alpha', 'Beta', 'Zulu']]]);
    expect(games.map((g) => g.id)).toEqual(['Zulu', 'Alpha', 'Beta']);
  });

  it('limits the recent rail without dropping games from installed', () => {
    const games = Array.from({ length: 20 }, (_, index) => game(String(index), { lastPlayed: '2026-09-22' }));
    expect(buildShelves(games).map((shelf) => shelf.games.length)).toEqual([12, 20]);
  });

  it('keeps hidden games out of the shelves and every filter except hidden', () => {
    const shown = game('shown', { lastPlayed: '2026-09-22', favorite: true });
    const secret = game('secret', { hidden: true, lastPlayed: '2026-09-23', favorite: true });
    const secretRemoved = game('gone', { hidden: true, uninstalled: true });
    const games = [shown, secret, secretRemoved];
    expect(buildShelves(games).flatMap((shelf) => shelf.games.map((g) => g.id))).toEqual(['shown', 'shown', 'shown']);
    for (const filter of ['all', 'installed', 'favorites', 'uninstalled'] as const) {
      expect(filterLibrary(games, '', filter, 'title').map((g) => g.id)).not.toContain('secret');
      expect(filterLibrary(games, '', filter, 'title').map((g) => g.id)).not.toContain('gone');
    }
    expect(filterLibrary(games, '', 'hidden', 'title').map((g) => g.id)).toEqual(['gone', 'secret']);
    expect(filterLibrary(games, 'sec', 'hidden', 'title')).toEqual([secret]);
  });

  it('builds a shelf per collection from tags and catalog genres, without hidden games', () => {
    const a = game('a', { tags: ['cozy'], canonicalGameId: 'ca' });
    const b = game('b', { tags: ['cozy'], canonicalGameId: 'cb' });
    const c = game('c', { tags: ['cozy'], hidden: true, canonicalGameId: 'ca' });
    const context = {
      collections: [{ id: 'k1', name: 'Cozy RPG', tags: ['Cozy'], genres: ['rpg'], createdAt: '', updatedAt: '' }],
      genres: { ca: ['RPG'], cb: ['Indie'] },
    };
    const shelves = buildShelves([a, b, c], context);
    expect(shelves.map((shelf) => [shelf.id, shelf.games.map((g) => g.id)]))
      .toEqual([['collection:k1', ['a']], ['installed', ['a', 'b']]]);
    expect(filterLibrary([a, b, c], '', 'collection:k1', 'title', context)).toEqual([a]);
    expect(filterLibrary([a, b, c], '', 'collection:missing', 'title', context)).toEqual([]);
  });

  it('keeps selection across reordering and recovers if the selected game disappears', () => {
    const first = game('first', { lastPlayed: '2026-09-22' });
    const second = game('second');
    expect(resolveSelection(buildShelves([second, first]), 'second')).toBe(second);
    expect(resolveSelection(buildShelves([first]), 'second')).toBe(first);
    expect(resolveSelection([], 'second')).toBeUndefined();
  });
});
