import { describe, expect, it } from 'vitest';
import type { DayTotal } from '../services/playlog';
import { barHeight, bestSeconds, chartBuckets, chartUnit, labelEvery, parseDay } from './buckets';

function range(from: string, count: number, seconds: (index: number) => number): DayTotal[] {
  const start = parseDay(from)!;
  return Array.from({ length: count }, (_, index) => {
    const date = new Date(start.getFullYear(), start.getMonth(), start.getDate() + index);
    const key = `${date.getFullYear()}-${`${date.getMonth() + 1}`.padStart(2, '0')}-${`${date.getDate()}`.padStart(2, '0')}`;
    return { date: key, seconds: seconds(index) };
  });
}

describe('chartBuckets', () => {
  it('keeps one bar per day for a week and a month', () => {
    const week = chartBuckets('week', range('2026-10-09', 7, (i) => i * 60));
    const month = chartBuckets('month', range('2026-09-16', 30, () => 60));
    expect(week).toHaveLength(7);
    expect(month).toHaveLength(30);
    expect(week[3]).toMatchObject({ key: '2026-10-12', seconds: 180, unit: 'day' });
  });

  it('folds a year of days into calendar months and keeps the total', () => {
    const days = range('2025-10-17', 365, () => 100);
    const buckets = chartBuckets('year', days);
    expect(buckets).toHaveLength(13);
    expect(buckets[0]).toMatchObject({ key: '2025-10', unit: 'month' });
    expect(buckets[0].at.getDate()).toBe(1);
    expect(buckets[0].seconds).toBe(15 * 100);
    expect(buckets[buckets.length - 1].key).toBe('2026-10');
    expect(buckets.reduce((sum, bucket) => sum + bucket.seconds, 0)).toBe(365 * 100);
  });

  it('skips days with an unreadable date and clamps negative seconds', () => {
    const buckets = chartBuckets('week', [
      { date: 'oops', seconds: 50 },
      { date: '2026-10-15', seconds: -5 },
    ]);
    expect(buckets).toHaveLength(1);
    expect(buckets[0].seconds).toBe(0);
  });

  it('returns nothing for no days', () => {
    expect(chartBuckets('year', [])).toEqual([]);
  });
});

describe('chart scale', () => {
  it('uses month buckets only for the year', () => {
    expect(chartUnit('year')).toBe('month');
    expect(chartUnit('month')).toBe('day');
    expect(chartUnit('week')).toBe('day');
  });

  it('finds the best bucket and scales bars against it', () => {
    const buckets = chartBuckets('week', range('2026-10-09', 7, (i) => (i === 2 ? 3600 : 0)));
    const best = bestSeconds(buckets);
    expect(best).toBe(3600);
    expect(barHeight(3600, best)).toBe(100);
    expect(barHeight(0, best)).toBe(0);
    expect(barHeight(10, best)).toBe(4);
    expect(barHeight(10, 0)).toBe(0);
  });

  it('labels every fifth day of a month and every bar otherwise', () => {
    expect(labelEvery('month')).toBe(5);
    expect(labelEvery('week')).toBe(1);
    expect(labelEvery('year')).toBe(1);
  });
});
