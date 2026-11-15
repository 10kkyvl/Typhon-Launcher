import { describe, expect, it } from 'vitest';
import { METER_GAP, METER_HAIRLINE, METER_MORE, METER_OTHER, METER_TONES, METER_WIDTH, gameTone, meter, sizeWords, type MeterSegment } from './diskMeter';

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

  // The sizes disagree with the total: which is wrong is not the meter's to know, so every part is
  // drawn against the others over the whole bar, and no free space is drawn or said.
  it('draws an overflow in proportion over the whole bar, every part visible, and says no free space', () => {
    const m = meter([seg('a', 800), seg('b', 400), seg('c', 0), seg('d', 200)], 1000);
    expect(m.overflow).toBe(true);
    expect(m.drawn.map((p) => [p.key, p.share])).toEqual([
      ['a', 800 / 1400],
      ['b', 400 / 1400],
      ['d', 200 / 1400],
    ]);
    expect(m.drawn.reduce((sum, p) => sum + p.share, 0)).toBeCloseTo(1, 12);
    expect(m.free).toBeNull();
    expect(m.parts.map((p) => [p.key, p.size])).toEqual([
      ['a', { kind: 'exact', bytes: 800 }],
      ['b', { kind: 'exact', bytes: 400 }],
      ['c', { kind: 'exact', bytes: 0 }],
      ['d', { kind: 'exact', bytes: 200 }],
    ]);
  });

  it('says no free space for an overflow even when the caller measured one', () => {
    const m = meter([seg('a', 1200)], 1000, 10);
    expect(m.overflow).toBe(true);
    expect(m.free).toBeNull();
    expect(m.drawn.map((p) => p.share)).toEqual([1]);
  });

  it('takes segments and the measured free space over the total for an overflow', () => {
    const m = meter([seg('a', 600), seg('b', 200)], 1000, 500);
    expect(m.overflow).toBe(true);
    expect(m.free).toBeNull();
    expect(m.drawn.map((p) => [p.key, p.share])).toEqual([
      ['a', 0.75],
      ['b', 0.25],
    ]);
  });

  it('is no overflow when the parts and the free space fill the total exactly', () => {
    const m = meter([seg('a', 600)], 1000, 400);
    expect(m.overflow).toBe(false);
    expect(m.free).toEqual({ kind: 'exact', bytes: 400 });
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

// The rule that keeps a bar truthful and inside its track, at the narrowest width it is laid out for:
// with k parts drawn, the room the parts share is the width less k − 1 gaps; a part drawn on its own
// takes at least a hairline of that room, so it is drawn at its size, not puffed up to the hairline.
// Only the smallest one drawn may be a hairline (a part far under a pixel, or the fold); the rest fold
// into one "N more" part, and the k hairlines and k − 1 gaps fit in the width.
describe('folding what cannot be drawn truthfully', () => {
  const room = (width: number, k: number) => width - (k - 1) * METER_GAP;

  // Other files are never folded in with the segments, so they too may be a hairline.
  function holds(m: ReturnType<typeof meter>, width: number) {
    const k = m.drawn.length;
    expect(k * METER_HAIRLINE + (k - 1) * METER_GAP, 'the hairlines and the gaps fit in the width').toBeLessThanOrEqual(width + 1e-9);
    const atSize = m.drawn
      .filter((p) => p.kind !== 'other')
      .sort((a, b) => a.share - b.share)
      .slice(1);
    for (const part of atSize) expect(part.share * room(width, k), `${part.key} is drawn at its size`).toBeGreaterThanOrEqual(METER_HAIRLINE - 1e-9);
  }

  it('lays a compact bar out for a narrower track than the full one', () => {
    expect(METER_WIDTH).toEqual({ full: 32, compact: 16 });
  });

  it('folds sixty equal games in a compact bar into twenty and one "40 more" part', () => {
    const games = Array.from({ length: 60 }, (_, i) => seg(`g${i}`, 10, { tone: gameTone(i) }));
    const m = meter(games, 600, undefined, METER_WIDTH.compact);
    expect(m.drawn.length).toBe(21);
    expect(m.drawn.slice(0, 20).map((p) => p.key)).toEqual(games.slice(0, 20).map((g) => g.key));
    const more = m.drawn[20];
    expect([more.kind, more.key, more.tone, more.size, more.folded?.length]).toEqual(['more', METER_MORE, 'more', { kind: 'exact', bytes: 400 }, 40]);
    expect(more.folded).toEqual(games.slice(20).map((g) => g.key));
    expect(m.parts.map((p) => p.key)).toEqual([...games.slice(0, 20).map((g) => g.key), METER_MORE]);
    holds(m, METER_WIDTH.compact);
  });

  it('draws the same sixty games each on its own in the full bar', () => {
    const games = Array.from({ length: 60 }, (_, i) => seg(`g${i}`, 10));
    const m = meter(games, 600, undefined, METER_WIDTH.full);
    expect(m.drawn.length).toBe(60);
    expect(m.parts.some((p) => p.kind === 'more')).toBe(false);
    holds(m, METER_WIDTH.full);
  });

  it('folds the smallest, keeps the caller order of the rest, and puts the fold before other files', () => {
    const m = meter([seg('a', 5), seg('big', 9000), seg('b', 3), seg('c', 7), seg('d', 3000)], 100_000, 87_000, METER_WIDTH.compact);
    expect(m.drawn.map((p) => p.key)).toEqual(['big', 'd', METER_MORE, METER_OTHER]);
    expect(m.parts.find((p) => p.kind === 'more')?.folded).toEqual(['a', 'b', 'c']);
    holds(m, METER_WIDTH.compact);
  });

  it('leaves one part far under a pixel on its own, as a hairline', () => {
    const m = meter([seg('big', 999_999_999_999), seg('tiny', 1)], 1_000_000_000_000, undefined, METER_WIDTH.compact);
    expect(m.drawn.map((p) => p.key)).toEqual(['big', 'tiny']);
  });

  it('says a fold holding a lower bound as a lower bound', () => {
    const m = meter([seg('big', 1000), seg('a', 1, { sizeUnknown: true }), seg('b', 1)], 1002, undefined, METER_WIDTH.compact);
    expect(m.drawn.map((p) => [p.key, p.size])).toEqual([
      ['big', { kind: 'exact', bytes: 1000 }],
      [METER_MORE, { kind: 'atLeast', bytes: 2 }],
    ]);
  });

  it('does not fold the parts it does not draw', () => {
    const m = meter([seg('big', 1000), seg('zero', 0), seg('none', 0, { sizeUnknown: true })], 2000, undefined, METER_WIDTH.compact);
    expect(m.parts.map((p) => p.key)).toEqual(['big', 'zero', 'none']);
  });

  // Many mixes of sizes, counts and widths: the rule holds for each.
  it('keeps every bar inside its track and in proportion', () => {
    let seed = 7;
    const next = () => (seed = (seed * 48271) % 2147483647) / 2147483647;
    for (let run = 0; run < 300; run += 1) {
      const count = 1 + Math.floor(next() * 120);
      const segments = Array.from({ length: count }, (_, i) => seg(`s${i}`, Math.floor(next() ** 4 * 1e6)));
      const width = next() < 0.5 ? METER_WIDTH.compact : METER_WIDTH.full;
      const sum = segments.reduce((total, s) => total + s.bytes, 0);
      const m = meter(segments, sum + Math.floor(next() * sum), undefined, width);
      holds(m, width);
      const listed = m.parts.reduce((total, p) => total + (p.size.kind === 'unknown' ? 0 : p.size.bytes), 0);
      expect(listed, 'the legend still adds up to the segments').toBe(sum);
    }
  });
});

// The track stands for the free space: space that is neither a segment nor measured free must not be
// drawn as free, so the meter lists it as a part of its own.
describe('space no segment accounts for', () => {
  const kinds = (m: ReturnType<typeof meter>) => m.parts.map((p) => [p.kind, p.key, p.size, p.share]);

  it('is drawn and listed as other files when the free space is measured', () => {
    const m = meter([seg('games', 600)], 1000, 300);
    expect(kinds(m)).toEqual([
      ['segment', 'games', { kind: 'exact', bytes: 600 }, 0.6],
      ['other', METER_OTHER, { kind: 'exact', bytes: 100 }, 0.1],
    ]);
    expect(m.parts[1].tone).toBe('other');
    expect(m.drawn.map((p) => p.key)).toEqual(['games', METER_OTHER]);
    expect(m.free).toEqual({ kind: 'exact', bytes: 300 });
  });

  it('is of unknown size when a segment is only a lower bound, and still drawn: it is not free', () => {
    const m = meter([seg('games', 600, { sizeUnknown: true })], 1000, 300);
    expect(kinds(m)[1]).toEqual(['other', METER_OTHER, { kind: 'unknown' }, 0.1]);
  });

  it('is not added when the segments and the free space fill the total', () => {
    expect(meter([seg('games', 600), seg('rest', 100, { tone: 'other' })], 1000, 300).parts.map((p) => p.kind)).toEqual(['segment', 'segment']);
  });

  it('is not added where the free space is derived, not measured, or the total is unknown', () => {
    expect(meter([seg('games', 600)], 1000).parts.map((p) => p.kind)).toEqual(['segment']);
    expect(meter([seg('games', 600)], 1000, null).parts.map((p) => p.kind)).toEqual(['segment']);
    expect(meter([seg('games', 600)], 0, 300).parts.map((p) => p.kind)).toEqual(['segment']);
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
    expect(METER_TONES).toEqual(['game', 'game-2', 'game-3', 'data', 'reclaim', 'other', 'more']);
  });

  it('give games their steps in turn', () => {
    expect([0, 1, 2, 3, 4, -1].map(gameTone)).toEqual(['game', 'game-2', 'game-3', 'game', 'game-2', 'game-3']);
  });
});
