import { describe, expect, it } from 'vitest';
import type { ActivityDay, GameRef } from '../services/profile';
import { weekSummary } from './week';

const now = new Date(2026, 8, 3, 15, 0, 0);

function game(id: string, title: string): GameRef {
  return { id, title, cover: '', playtimeSeconds: 0, status: '' };
}

function day(date: string, entries: [GameRef, number][]): ActivityDay {
  return { date, entries: entries.map(([g, seconds]) => ({ game: g, seconds })) };
}

describe('weekSummary', () => {
  it('always returns seven days ending today', () => {
    const week = weekSummary([], now);
    expect(week.days).toHaveLength(7);
    expect(week.days[0].date).toBe('2026-08-28');
    expect(week.days[6].date).toBe('2026-09-03');
    expect(week.days[6].today).toBe(true);
    expect(week.days.filter((d) => d.today)).toHaveLength(1);
    expect(week.days[6].at).toEqual(new Date(2026, 8, 3));
    expect(week.days[0].at).toEqual(new Date(2026, 7, 28));
  });

  it('sums every game of a day into that day', () => {
    const week = weekSummary(
      [day('2026-09-03', [[game('a', 'Celeste'), 1800], [game('b', 'Hades'), 600]])],
      now,
    );
    expect(week.days[6].seconds).toBe(2400);
    expect(week.totalSeconds).toBe(2400);
    expect(week.bestSeconds).toBe(2400);
  });

  it('ignores days outside the window', () => {
    const week = weekSummary(
      [
        day('2026-09-03', [[game('a', 'Celeste'), 60]]),
        day('2026-08-27', [[game('b', 'Hades'), 7200]]),
      ],
      now,
    );
    expect(week.totalSeconds).toBe(60);
    expect(week.games.map((g) => g.game.id)).toEqual(['a']);
  });

  it('adds up one game across days and ranks by time', () => {
    const week = weekSummary(
      [
        day('2026-09-03', [[game('a', 'Celeste'), 600]]),
        day('2026-09-02', [[game('a', 'Celeste'), 600], [game('b', 'Hades'), 900]]),
      ],
      now,
    );
    expect(week.games).toEqual([
      { game: game('a', 'Celeste'), seconds: 1200 },
      { game: game('b', 'Hades'), seconds: 900 },
    ]);
    expect(week.totalSeconds).toBe(2100);
    expect(week.bestSeconds).toBe(1500);
  });

  it('breaks equal times by title', () => {
    const week = weekSummary(
      [day('2026-09-01', [[game('b', 'Ведьмак'), 600], [game('a', 'Атомик'), 600]])],
      now,
    );
    expect(week.games.map((g) => g.game.title)).toEqual(['Атомик', 'Ведьмак']);
  });

  it('keeps only the top games', () => {
    const week = weekSummary(
      [
        day('2026-09-03', [
          [game('a', 'A'), 400],
          [game('b', 'B'), 300],
          [game('c', 'C'), 200],
          [game('d', 'D'), 100],
        ]),
      ],
      now,
    );
    expect(week.games.map((g) => g.game.id)).toEqual(['a', 'b', 'c']);
  });

  it('crosses a month boundary', () => {
    const week = weekSummary([day('2026-08-31', [[game('a', 'Celeste'), 300]])], new Date(2026, 8, 2, 9, 0, 0));
    expect(week.days[0].date).toBe('2026-08-27');
    expect(week.days.find((d) => d.date === '2026-08-31')?.seconds).toBe(300);
  });

  it('drops non-positive entries', () => {
    const week = weekSummary([day('2026-09-03', [[game('a', 'Celeste'), 0]])], now);
    expect(week.totalSeconds).toBe(0);
    expect(week.games).toEqual([]);
  });

  it('is empty without activity', () => {
    const week = weekSummary([], now);
    expect(week.totalSeconds).toBe(0);
    expect(week.bestSeconds).toBe(0);
    expect(week.games).toEqual([]);
  });
});
