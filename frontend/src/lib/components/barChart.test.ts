import { describe, expect, it } from 'vitest';
import { barShare, chartMax, hourScale, isEmpty, secondsScale, ticks, type ChartBucket } from './barChart';

const bucket = (key: string, value: number, extra: Partial<ChartBucket> = {}): ChartBucket => ({ key, label: key, value, ...extra });

describe('hourScale', () => {
  it('picks a round hour scale', () => {
    expect(hourScale(0)).toEqual({ max: 1, step: 1 });
    expect(hourScale(2.5 * 3600)).toEqual({ max: 3, step: 1 });
    expect(hourScale(7 * 3600)).toEqual({ max: 8, step: 2 });
    expect(hourScale(11 * 3600)).toEqual({ max: 12, step: 4 });
  });

  it('keeps an exact round value as the maximum', () => {
    expect(hourScale(3600)).toEqual({ max: 1, step: 1 });
    expect(hourScale(4 * 3600)).toEqual({ max: 4, step: 1 });
    expect(hourScale(24 * 3600)).toEqual({ max: 24, step: 6 });
  });

  it('goes past a day in steps of four hours', () => {
    expect(hourScale(30 * 3600)).toEqual({ max: 32, step: 8 });
    expect(hourScale(100 * 3600)).toEqual({ max: 100, step: 25 });
  });

  it('reads a value that is not a time as no time', () => {
    for (const value of [Number.NaN, -3600, Number.POSITIVE_INFINITY]) expect(hourScale(value)).toEqual({ max: 1, step: 1 });
  });

  it('in seconds, for a chart whose values are seconds', () => {
    expect(secondsScale(7 * 3600)).toEqual({ max: 8 * 3600, step: 2 * 3600 });
  });
});

describe('the chart arithmetic', () => {
  it('lists the ticks of a scale from zero', () => {
    expect(ticks({ max: 8, step: 2 })).toEqual([0, 2, 4, 6, 8]);
    expect(ticks({ max: 12, step: 4 })).toEqual([0, 4, 8, 12]);
  });

  it('draws no tick for a scale that is not one', () => {
    expect(ticks({ max: 0, step: 1 })).toEqual([]);
    expect(ticks({ max: 8, step: 0 })).toEqual([]);
    expect(ticks({ max: Number.NaN, step: 1 })).toEqual([]);
    expect(ticks({ max: 1e9, step: 1 })).toEqual([]);
  });

  it('scales to the largest value, or to the scale given', () => {
    expect(chartMax([bucket('a', 3), bucket('b', 9)])).toBe(9);
    expect(chartMax([bucket('a', 3)], { max: 8, step: 2 })).toBe(8);
    expect(chartMax([bucket('a', 0)])).toBe(1);
    expect(chartMax([])).toBe(1);
  });

  it('gives each bar its share of the height, never below 0 or over 1', () => {
    expect(barShare(4, 8)).toBe(0.5);
    expect(barShare(0, 8)).toBe(0);
    expect(barShare(20, 8)).toBe(1);
    expect(barShare(-1, 8)).toBe(0);
    expect(barShare(Number.NaN, 8)).toBe(0);
    expect(barShare(4, 0)).toBe(0);
  });

  it('is empty with no buckets or nothing above zero', () => {
    expect(isEmpty([])).toBe(true);
    expect(isEmpty([bucket('a', 0), bucket('b', 0)])).toBe(true);
    expect(isEmpty([bucket('a', Number.NaN)])).toBe(true);
    expect(isEmpty([bucket('a', 0), bucket('b', 1)])).toBe(false);
  });
});
