import { describe, expect, it } from 'vitest';
import { METER_TONES, gameTone, meter, sizeWords, type MeterSegment } from './diskMeter';

const seg = (key: string, bytes: number, extra: Partial<MeterSegment> = {}): MeterSegment => ({ key, label: key, bytes, tone: 'game', ...extra });

// Every number the model carries, at any depth: none may be NaN or infinite, whatever came in.
function numbers(value: unknown): number[] {
  if (typeof value === 'number') return [value];
  if (Array.isArray(value)) return value.flatMap(numbers);
  if (value && typeof value === 'object') return Object.values(value).flatMap(numbers);
  return [];
}

describe('the disk meter model', () => {
  const unusable: [string, number | null | undefined][] = [
    ['zero', 0],
    ['undefined', undefined],
    ['null', null],
    ['negative', -1024],
    ['NaN', Number.NaN],
    ['infinite', Number.POSITIVE_INFINITY],
  ];

  for (const [name, total] of unusable) {
    it(`the total ${name} is unknown: nothing is drawn, no free space is made up, no NaN`, () => {
      const m = meter([seg('a', 30), seg('b', 20)], total);
      expect(m.total).toBeNull();
      expect(m.drawn).toEqual([]);
      expect(m.free).toBeNull();
      expect(m.overflow).toBe(false);
      expect(m.parts.map((p) => p.size)).toEqual([
        { kind: 'exact', bytes: 30 },
        { kind: 'exact', bytes: 20 },
      ]);
      expect(numbers(m).every(Number.isFinite)).toBe(true);
    });
  }

  it('an unknown total keeps the free space the caller measured', () => {
    expect(meter([seg('a', 30)], 0, 500).free).toEqual({ kind: 'exact', bytes: 500 });
  });

  it('draws each segment as its share of the total and derives the free space', () => {
    const m = meter([seg('a', 30), seg('b', 20)], 100);
    expect(m.total).toBe(100);
    expect(m.drawn.map((p) => [p.key, p.share])).toEqual([
      ['a', 0.3],
      ['b', 0.2],
    ]);
    expect(m.free).toEqual({ kind: 'exact', bytes: 50 });
    expect(m.overflow).toBe(false);
  });

  it('clamps segments that sum over the total and draws no free space for the overflow', () => {
    const m = meter([seg('a', 80), seg('b', 50), seg('c', 10)], 100);
    expect(m.overflow).toBe(true);
    expect(m.drawn.map((p) => [p.key, p.share])).toEqual([
      ['a', 0.8],
      ['b', 0.2],
    ]);
    expect(m.drawn.reduce((sum, p) => sum + p.share, 0)).toBe(1);
    expect(m.free).toEqual({ kind: 'exact', bytes: 0 });
    expect(m.parts.map((p) => p.key)).toEqual(['a', 'b', 'c']);
  });

  it('keeps the free space the caller measured when the segments overflow', () => {
    expect(meter([seg('a', 120)], 100, 10).free).toEqual({ kind: 'exact', bytes: 10 });
  });

  it('draws a segment far under one pixel, so the bar can show it as a hairline', () => {
    const m = meter([seg('big', 999_999_999_999), seg('tiny', 1)], 1_000_000_000_000);
    const tiny = m.drawn.find((p) => p.key === 'tiny');
    expect(tiny).toBeDefined();
    expect(tiny?.share).toBeGreaterThan(0);
    expect(tiny?.share).toBeLessThan(1e-9);
  });

  it('does not draw a segment of zero bytes, and lists it at zero', () => {
    const m = meter([seg('a', 0), seg('b', 10)], 100);
    expect(m.drawn.map((p) => p.key)).toEqual(['b']);
    expect(m.parts[0].size).toEqual({ kind: 'exact', bytes: 0 });
  });

  it('says a lower bound and an unknown size instead of drawing them as 0', () => {
    const m = meter([seg('bound', 40, { sizeUnknown: true }), seg('none', 0, { sizeUnknown: true }), seg('broken', Number.NaN), seg('negative', -5)], 100);
    expect(m.parts.map((p) => [p.key, p.size])).toEqual([
      ['bound', { kind: 'atLeast', bytes: 40 }],
      ['none', { kind: 'unknown' }],
      ['broken', { kind: 'unknown' }],
      ['negative', { kind: 'unknown' }],
    ]);
    expect(m.drawn.map((p) => p.key)).toEqual(['bound']);
    expect(numbers(m).every(Number.isFinite)).toBe(true);
  });

  it('does not derive free space from sizes it does not know', () => {
    expect(meter([seg('bound', 40, { sizeUnknown: true })], 100).free).toBeNull();
    expect(meter([seg('none', 0, { sizeUnknown: true })], 100).free).toBeNull();
  });

  it('says a free space the caller could not measure is unknown', () => {
    expect(meter([seg('a', 10)], 100, null).free).toEqual({ kind: 'unknown' });
    expect(meter([seg('a', 10)], 100, Number.NaN).free).toEqual({ kind: 'unknown' });
    expect(meter([seg('a', 10)], 100, -1).free).toEqual({ kind: 'unknown' });
  });

  it('has nothing to draw with no segments, and all of the total is free', () => {
    const m = meter([], 100);
    expect(m.drawn).toEqual([]);
    expect(m.free).toEqual({ kind: 'exact', bytes: 100 });
  });
});

describe('sizes in words', () => {
  const format = (bytes: number) => `${bytes} B`;
  const words = { atLeast: (size: string) => `at least ${size}`, unknown: 'size unknown' };

  it('writes an exact size, a lower bound and an unknown size', () => {
    expect(sizeWords({ kind: 'exact', bytes: 5 }, format, words)).toBe('5 B');
    expect(sizeWords({ kind: 'atLeast', bytes: 5 }, format, words)).toBe('at least 5 B');
    expect(sizeWords({ kind: 'unknown' }, format, words)).toBe('size unknown');
  });
});

describe('the tones', () => {
  it('are a small closed set', () => {
    expect(METER_TONES).toEqual(['game', 'game-2', 'game-3', 'data', 'reclaim', 'other']);
  });

  it('give games their steps in turn', () => {
    expect([0, 1, 2, 3, 4, -1].map(gameTone)).toEqual(['game', 'game-2', 'game-3', 'game', 'game-2', 'game-3']);
  });
});
