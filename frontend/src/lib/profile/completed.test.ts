import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import { COMPLETED_LIMIT, completedThisYear, parseCompletedData } from './completed';

const now = new Date(2026, 9, 23, 12);

function game(id: string, patch: Partial<LibraryGame> = {}): LibraryGame {
  return { id, title: `Game ${id}`, status: 'completed', completedAt: '2026-05-10T12:00:00Z', ...patch } as LibraryGame;
}

describe('completedThisYear', () => {
  it('keeps completed and mastered games of the current year, newest first', () => {
    const result = completedThisYear(
      [
        game('old', { completedAt: '2026-02-01T12:00:00Z' }),
        game('new', { status: 'mastered', completedAt: '2026-09-01T12:00:00Z' }),
        game('mid', { completedAt: '2026-05-01T12:00:00Z' }),
      ],
      now,
    );

    expect(result.year).toBe(2026);
    expect(result.total).toBe(3);
    expect(result.games.map((item) => item.id)).toEqual(['new', 'mid', 'old']);
  });

  it('skips other years, other statuses, unknown dates and hidden games', () => {
    const result = completedThisYear(
      [
        game('last-year', { completedAt: '2025-12-20T12:00:00Z' }),
        game('playing', { status: 'playing' }),
        game('no-date', { completedAt: null }),
        game('bad-date', { completedAt: 'nonsense' }),
        game('hidden', { hidden: true }),
        game('ok'),
      ],
      now,
    );

    expect(result.games.map((item) => item.id)).toEqual(['ok']);
    expect(result.total).toBe(1);
  });

  it('counts every game but lists no more than twelve', () => {
    const many = Array.from({ length: 15 }, (_, index) => game(`g${index}`, { completedAt: `2026-03-${String(index + 1).padStart(2, '0')}T12:00:00Z` }));

    const result = completedThisYear(many, now);

    expect(result.total).toBe(15);
    expect(result.games).toHaveLength(COMPLETED_LIMIT);
    expect(result.games[0].id).toBe('g14');
  });

  it('returns an empty block for an empty library', () => {
    expect(completedThisYear([], now)).toEqual({ year: 2026, total: 0, games: [] });
  });
});

describe('parseCompletedData', () => {
  const card = { igdbId: 1, title: 'Game', coverUrl: 'c', heroUrl: 'h', status: 'mastered', completedAt: '2026-10-19T21:30:00Z' };

  it('reads the public block payload', () => {
    expect(parseCompletedData({ year: 2026, total: 15, games: [card] })).toEqual({ year: 2026, total: 15, games: [card] });
  });

  it('accepts an empty list as a null list only when nothing was completed', () => {
    expect(parseCompletedData({ year: 2026, total: 0, games: null })).toEqual({ year: 2026, total: 0, games: [] });
    expect(parseCompletedData({ year: 2026, total: 3, games: null })).toBeNull();
  });

  it.each([
    ['no data', undefined],
    ['null', null],
    ['an error from the server', { error: 'unavailable', year: 2026, total: 0, games: [] }],
    ['no year', { total: 1, games: [card] }],
    ['a fractional year', { year: 2026.5, total: 1, games: [card] }],
    ['a negative total', { year: 2026, total: -1, games: [] }],
    ['a total below the list', { year: 2026, total: 0, games: [card] }],
    ['a broken game', { year: 2026, total: 1, games: [{ title: 'No id' }] }],
  ])('rejects %s', (_name, data) => {
    expect(parseCompletedData(data)).toBeNull();
  });
});
