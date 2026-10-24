import { describe, expect, it, vi } from 'vitest';
import type { LibraryCollection, LibraryGame } from '../services/library';
import { defaultFilters, isStale, pickCandidates, pickRandom, runSpin, spinDelay, spinFrames, type PickFilters } from './randomPick';

const NOW = Date.parse('2026-10-24T12:00:00Z');
const DAY = 24 * 60 * 60 * 1000;

function game(id: string, extra: Partial<LibraryGame> = {}): LibraryGame {
  return { id, title: id, lastPlayed: null, executable: `C:/games/${id}.exe`, ...extra } as LibraryGame;
}

const filters = (extra: Partial<PickFilters> = {}): PickFilters => ({ ...defaultFilters, ...extra });
const ids = (list: LibraryGame[]) => list.map((item) => item.id);

describe('pickCandidates', () => {
  it('returns nothing for an empty library', () => {
    expect(pickCandidates([], filters(), {}, [], NOW)).toEqual([]);
  });

  it('returns nothing when every game is hidden', () => {
    const games = [game('a', { hidden: true }), game('b', { hidden: true })];
    for (const source of ['installed', 'all', 'backlog'] as const) {
      expect(pickCandidates(games, filters({ source }), {}, [], NOW)).toEqual([]);
    }
  });

  it('installed skips games that cannot be launched', () => {
    const games = [game('a'), game('b', { executable: '' })];
    expect(ids(pickCandidates(games, filters(), {}, [], NOW))).toEqual(['a']);
  });

  it('installed skips uninstalled games, all keeps them', () => {
    const games = [game('a'), game('b', { uninstalled: true })];
    expect(ids(pickCandidates(games, filters(), {}, [], NOW))).toEqual(['a']);
    expect(ids(pickCandidates(games, filters({ source: 'all' }), {}, [], NOW))).toEqual(['a', 'b']);
  });

  it('backlog keeps only games with the backlog status', () => {
    const games = [game('a', { status: 'backlog' }), game('b', { status: 'playing' }), game('c')];
    expect(ids(pickCandidates(games, filters({ source: 'backlog' }), {}, [], NOW))).toEqual(['a']);
  });

  it('stale keeps never played and games older than 30 days', () => {
    const games = [
      game('never'),
      game('old', { lastPlayed: new Date(NOW - 31 * DAY).toISOString() }),
      game('fresh', { lastPlayed: new Date(NOW - 5 * DAY).toISOString() }),
      game('edge', { lastPlayed: new Date(NOW - 30 * DAY).toISOString() }),
    ];
    expect(ids(pickCandidates(games, filters({ stale: true }), {}, [], NOW))).toEqual(['never', 'old']);
  });

  it('collection uses tags and catalog genres', () => {
    const collection = { id: 'c1', name: 'x', tags: ['coop'], genres: ['RPG'] } as LibraryCollection;
    const games = [
      game('a', { tags: ['coop'], canonicalGameId: 'ga' }),
      game('b', { tags: ['coop'], canonicalGameId: 'gb' }),
      game('c', { canonicalGameId: 'ga' }),
    ];
    const genres = { ga: ['RPG'], gb: ['Racing'] };
    expect(ids(pickCandidates(games, filters({ source: 'collection', collectionId: 'c1' }), genres, [collection], NOW))).toEqual(['a']);
  });

  it('collection source with a missing collection yields nothing', () => {
    expect(pickCandidates([game('a')], filters({ source: 'collection', collectionId: 'gone' }), {}, [], NOW)).toEqual([]);
  });

  it('genre filter uses the catalog genres of the canonical game', () => {
    const games = [game('a', { canonicalGameId: 'ga' }), game('b', { canonicalGameId: 'gb' }), game('c')];
    const genres = { ga: ['RPG', 'Action'], gb: ['Racing'] };
    expect(ids(pickCandidates(games, filters({ genre: 'RPG' }), genres, [], NOW))).toEqual(['a']);
  });
});

describe('isStale', () => {
  it('treats missing and unparsable dates as never played', () => {
    expect(isStale(null, NOW)).toBe(true);
    expect(isStale('garbage', NOW)).toBe(true);
  });
});

describe('pickRandom', () => {
  it('returns null for an empty list', () => {
    expect(pickRandom([], null, () => 0.5)).toBeNull();
  });

  it('allows a repeat when there is a single option', () => {
    const only = game('a');
    expect(pickRandom([only], 'a', () => 0.9)).toBe(only);
  });

  it('excludes the previous pick when there are several options', () => {
    const list = [game('a'), game('b'), game('c')];
    for (const roll of [0, 0.34, 0.5, 0.99]) {
      expect(pickRandom(list, 'a', () => roll)?.id).not.toBe('a');
    }
    expect(pickRandom(list, 'a', () => 0)?.id).toBe('b');
    expect(pickRandom(list, 'a', () => 0.99)?.id).toBe('c');
  });

  it('survives an rng returning 1', () => {
    const list = [game('a'), game('b')];
    expect(pickRandom(list, null, () => 1)?.id).toBe('b');
  });
});

describe('spinFrames', () => {
  it('ends on the final game and never repeats a frame back to back', () => {
    const list = [game('a'), game('b'), game('c'), game('d')];
    let seed = 0.13;
    const rng = () => (seed = (seed * 9301 + 0.49297) % 1);
    const frames = spinFrames(list, list[2], rng, 10);
    expect(frames).toHaveLength(10);
    expect(frames[frames.length - 1].id).toBe('c');
    for (let i = 1; i < frames.length; i++) expect(frames[i].id).not.toBe(frames[i - 1].id);
  });

  it('is just the final game when it is the only candidate', () => {
    const only = game('a');
    expect(spinFrames([only], only, () => 0.5)).toEqual([only]);
  });
});

describe('spinDelay', () => {
  it('slows down towards the end', () => {
    expect(spinDelay(0)).toBeLessThan(spinDelay(9));
    expect(spinDelay(9)).toBeLessThan(spinDelay(17));
  });
});

describe('runSpin', () => {
  it('plays every frame in order, then reports the last one, and can be cancelled', () => {
    vi.useFakeTimers();
    try {
      const list = [game('a'), game('b'), game('c')];
      const seen: string[] = [];
      const done: string[] = [];
      runSpin(list, (item) => seen.push(item.id), (item) => done.push(item.id));
      vi.runAllTimers();
      expect(seen).toEqual(['a', 'b']);
      expect(done).toEqual(['c']);

      const cancelled: string[] = [];
      const cancel = runSpin(list, (item) => cancelled.push(item.id), () => cancelled.push('done'));
      cancel();
      vi.runAllTimers();
      expect(cancelled).toEqual(['a']);
    } finally {
      vi.useRealTimers();
    }
  });
});
