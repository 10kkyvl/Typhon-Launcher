import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import { createEntryCache } from '../library/entries';
import type { LibraryFilter as DesktopFilter } from '../library/filters';
import { queryLibrary, type LibraryQuery } from '../library/query';
import { buildShelves, filterLibrary, resolveSelection, type LibraryFilter, type LibrarySort } from './library';

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

describe('Big Picture library and the desktop library', () => {
  // Installed and removed, hidden, favourites, a collection by tag, Latin and Cyrillic titles, equal titles.
  const games = [
    game('hades', { title: 'Hades', lastPlayed: '2026-11-10T10:00:00Z', playtimeSeconds: 900, tags: ['rogue'], favorite: true, installedAt: '2026-01-01T00:00:00Z' }),
    game('witcher', { title: 'Ведьмак 3', lastPlayed: '2026-10-01T10:00:00Z', playtimeSeconds: 5000, installedAt: '2026-02-01T00:00:00Z' }),
    game('arma', { title: 'Arma', uninstalled: true, lastPlayed: '2026-11-13T10:00:00Z', tags: ['rogue'], favorite: true }),
    game('secret', { title: 'Secret Hades', hidden: true, tags: ['rogue'] }),
    game('twin-b', { title: 'Twin' }),
    game('twin-a', { title: 'Twin' }),
    game('yabloko', { title: 'Яблоко', canonicalGameId: 'c-y' }),
  ];
  const context = { collections: [{ id: 'k', name: 'Rogue', tags: ['rogue'], createdAt: '', updatedAt: '' }], genres: {} };
  const entries = createEntryCache().library(games, [], {});
  const cases: Array<[LibraryFilter, DesktopFilter, LibraryQuery['collection']]> = [
    ['all', 'all', undefined],
    ['installed', 'installed', undefined],
    ['uninstalled', 'uninstalled', undefined],
    ['favorites', 'favorites', undefined],
    ['hidden', 'hidden', undefined],
    ['collection:k', 'all', context.collections[0]],
  ];
  const sorts: LibrarySort[] = ['title', 'recent', 'playtime', 'added'];
  const table = ['en', 'ru'].flatMap((locale) =>
    cases.flatMap(([bp, desktop, collection]) =>
      sorts.flatMap((sort) => ['', 'HA', 'ве', 'ＴＷＩＮ'].map((search) => ({ locale, bp, desktop, collection, sort, search }))),
    ),
  );

  it.each(table)('give the same games in the same order: $locale $bp $sort "$search"', ({ locale, bp, desktop, collection, sort, search }) => {
    const desktopIds = queryLibrary(entries, [], { filter: desktop, source: 'all', status: 'all', collection, search, sort }, { genres: {}, locale }).map((entry) => entry.id);
    expect(filterLibrary(games, search, bp, sort, context, locale).map((item) => item.id)).toEqual(desktopIds);
  });

  it.each([
    ['en', ['arma', 'hades', 'twin-a', 'twin-b', 'witcher', 'yabloko']],
    ['ru', ['witcher', 'yabloko', 'arma', 'hades', 'twin-a', 'twin-b']],
  ])('order titles in the interface language with an id tie-break: %s', (locale, expected) => {
    expect(filterLibrary(games, '', 'all', 'title', context, locale).map((item) => item.id)).toEqual(expected);
  });

  it('answer the library games themselves', () => {
    expect(filterLibrary(games, '', 'all', 'title', context, 'en').every((item) => games.includes(item))).toBe(true);
  });
});
