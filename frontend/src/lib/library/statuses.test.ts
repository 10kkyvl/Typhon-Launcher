import { describe, expect, it } from 'vitest';
import { STATUS_FILTERS, compareByCompleted, compareByStatus, matchesStatus } from './statuses';

describe('matchesStatus', () => {
  it('lets every game through on all', () => {
    expect(matchesStatus('', 'all')).toBe(true);
    expect(matchesStatus('dropped', 'all')).toBe(true);
    expect(matchesStatus(undefined, 'all')).toBe(true);
  });

  it('matches the exact status', () => {
    expect(matchesStatus('mastered', 'mastered')).toBe(true);
    expect(matchesStatus('completed', 'mastered')).toBe(false);
  });

  it('treats an empty or unknown status as not started', () => {
    expect(matchesStatus('', 'none')).toBe(true);
    expect(matchesStatus(undefined, 'none')).toBe(true);
    expect(matchesStatus('something', 'none')).toBe(true);
    expect(matchesStatus('playing', 'none')).toBe(false);
  });

  it('offers every status exactly once, not started last', () => {
    expect(STATUS_FILTERS).toEqual(['all', 'playing', 'backlog', 'paused', 'completed', 'mastered', 'dropped', 'none']);
  });
});

describe('compareByStatus', () => {
  it('orders playing, backlog, paused, completed, mastered, dropped, not started', () => {
    const games = [
      { title: 'a', status: '' },
      { title: 'b', status: 'dropped' },
      { title: 'c', status: 'mastered' },
      { title: 'd', status: 'completed' },
      { title: 'e', status: 'paused' },
      { title: 'f', status: 'backlog' },
      { title: 'g', status: 'playing' },
    ];
    expect(games.toSorted(compareByStatus).map((game) => game.status)).toEqual([
      'playing',
      'backlog',
      'paused',
      'completed',
      'mastered',
      'dropped',
      '',
    ]);
  });

  it('sorts by title inside one status', () => {
    const games = [
      { title: 'Яблоко', status: 'playing' },
      { title: 'Арбуз', status: 'playing' },
      { title: 'Банан', status: 'playing' },
    ];
    expect(games.toSorted(compareByStatus).map((game) => game.title)).toEqual(['Арбуз', 'Банан', 'Яблоко']);
  });

  it('puts a missing status together with not started', () => {
    const games = [{ title: 'b' }, { title: 'a', status: '' }, { title: 'c', status: 'dropped' }];
    expect(games.toSorted(compareByStatus).map((game) => game.title)).toEqual(['c', 'a', 'b']);
  });
});

describe('compareByCompleted', () => {
  it('puts the newest completion first and undated games last', () => {
    const games = [
      { title: 'none' },
      { title: 'old', completedAt: '2024-01-05T10:00:00Z' },
      { title: 'new', completedAt: '2026-10-19T21:30:00Z' },
      { title: 'null', completedAt: null },
      { title: 'mid', completedAt: '2025-06-01T10:00:00Z' },
    ];
    expect(games.toSorted(compareByCompleted).map((game) => game.title)).toEqual(['new', 'mid', 'old', 'none', 'null']);
  });

  it('sorts equal dates by title', () => {
    const games = [
      { title: 'b', completedAt: '2026-01-01T00:00:00Z' },
      { title: 'a', completedAt: '2026-01-01T00:00:00Z' },
    ];
    expect(games.toSorted(compareByCompleted).map((game) => game.title)).toEqual(['a', 'b']);
  });

  it('treats an unparsable date as missing', () => {
    const games = [
      { title: 'bad', completedAt: 'not a date' },
      { title: 'good', completedAt: '2026-01-01T00:00:00Z' },
    ];
    expect(games.toSorted(compareByCompleted).map((game) => game.title)).toEqual(['good', 'bad']);
  });
});
