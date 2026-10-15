import { describe, expect, it } from 'vitest';
import type { Stats } from '../services/playlog';
import { gameTitle, summarize } from './summary';

function stats(patch: Partial<Stats>): Stats {
  return {
    period: 'week',
    from: '2026-10-09',
    to: '2026-10-15',
    totalSeconds: 0,
    sessions: 0,
    days: [],
    top: [],
    longest: null,
    ...patch,
  };
}

describe('summarize', () => {
  it('marks a period without play as empty', () => {
    const summary = summarize(stats({ top: null }));
    expect(summary.empty).toBe(true);
    expect(summary.top).toEqual([]);
    expect(summary.longest).toBeNull();
  });

  it('computes each top game share of the total', () => {
    const summary = summarize(
      stats({
        totalSeconds: 4000,
        sessions: 3,
        top: [
          { gameId: 'a', title: 'A', seconds: 3000, sessions: 2 },
          { gameId: 'b', title: 'B', seconds: 1000, sessions: 1 },
          { gameId: 'c', title: 'C', seconds: 0, sessions: 0 },
        ],
      }),
    );
    expect(summary.empty).toBe(false);
    expect(summary.top.map((item) => [item.gameId, item.share])).toEqual([
      ['a', 0.75],
      ['b', 0.25],
    ]);
  });

  it('parses the date of the longest session and drops a zero one', () => {
    const longest = { gameId: 'a', title: 'A', seconds: 7200, date: '2026-10-12' };
    const summary = summarize(stats({ totalSeconds: 7200, sessions: 1, longest }));
    expect(summary.longest?.seconds).toBe(7200);
    expect(summary.longest?.at?.getDate()).toBe(12);
    expect(summarize(stats({ longest: { ...longest, seconds: 0 } })).longest).toBeNull();
  });
});

describe('gameTitle', () => {
  it('prefers the recorded title, then the library one, then the deleted label', () => {
    expect(gameTitle('Doom', 'Library', 'Deleted')).toBe('Doom');
    expect(gameTitle('  ', 'Library', 'Deleted')).toBe('Library');
    expect(gameTitle('', undefined, 'Deleted')).toBe('Deleted');
  });
});
