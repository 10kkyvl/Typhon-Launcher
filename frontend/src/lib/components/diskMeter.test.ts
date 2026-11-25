import { describe, expect, it } from 'vitest';
import { METER_GAP, METER_HAIRLINE, METER_MORE, METER_OTHER, METER_PAD, METER_REST, METER_TONES, METER_WIDTH, gameTone, meter, sizeWords, sumMeter, type MeterSegment } from './diskMeter';

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

  // Free space is what the player acts on before installing: a measured one is said even when the
  // numbers disagree, and the parts are drawn against the segments and it, so it shows as track.
  it('says and draws a measured free space in an overflow of the segments alone', () => {
    const m = meter([seg('a', 1200)], 1000, 10);
    expect(m.overflow).toBe(true);
    expect(m.free).toEqual({ kind: 'exact', bytes: 10 });
    expect(m.drawn.map((p) => p.share)).toEqual([1200 / 1210]);
  });

  it('takes the segments and the measured free space over the total for an overflow, and says the free space', () => {
    const m = meter([seg('a', 600), seg('b', 200)], 1000, 500);
    expect(m.overflow).toBe(true);
    expect(m.free).toEqual({ kind: 'exact', bytes: 500 });
    expect(m.drawn.map((p) => [p.key, p.share])).toEqual([
      ['a', 600 / 1300],
      ['b', 200 / 1300],
    ]);
  });

  it('says no free space in an overflow when it is derived or could not be measured', () => {
    for (const free of [undefined, null, Number.NaN]) {
      const m = meter([seg('a', 600), seg('b', 600)], 1000, free);
      expect(m.overflow).toBe(true);
      expect(m.free).toBeNull();
      expect(m.drawn.map((p) => p.share)).toEqual([0.5, 0.5]);
    }
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
      [METER_REST, { kind: 'atMost', bytes: 60 }],
    ]);
    expect(m.drawn.map((p) => p.key)).toEqual(['bound', METER_REST]);
    expect(numbers(m).every(Number.isFinite)).toBe(true);
  });

  it('does not derive free space from sizes it does not know', () => {
    expect(meter([seg('bound', 40, { sizeUnknown: true })], 100).free).toBeNull();
    expect(meter([seg('none', 0, { sizeUnknown: true })], 100).free).toBeNull();
  });

  // Where the segments leave no space, or nothing is drawn against an unknown total, no part holds
  // the free space: it is said on its own, as unknown.
  it('says a free space the caller could not measure is unknown where no part holds it', () => {
    for (const free of [null, Number.NaN, -1]) {
      expect(meter([seg('a', 100)], 100, free).free).toEqual({ kind: 'unknown' });
      expect(meter([seg('a', 10)], 0, free).free).toEqual({ kind: 'unknown' });
    }
  });

  it('gives segments that share a key ids of their own, and keeps the key a pick reports', () => {
    const m = meter([seg('a', 1), seg('a', 2), seg('a#1', 3), seg('a', 4)], 100);
    expect(m.parts.map((p) => [p.id, p.key])).toEqual([
      ['a', 'a'],
      ['a#1', 'a'],
      ['a#1#1', 'a#1'],
      ['a#2', 'a'],
    ]);
  });

  it('has nothing to draw with no segments, and all of the total is free', () => {
    const m = meter([], 100);
    expect(m.drawn).toEqual([]);
    expect(m.free).toEqual({ kind: 'exact', bytes: 100 });
  });
});

// The rule that keeps a bar truthful and inside its track, at the narrowest width it is laid out for:
// the parts are laid out in the track's content box (the width less its padding on both sides); with
// k parts drawn, the room they share is that box less k − 1 gaps; a part drawn on its own takes at
// least a hairline of that room, so it is drawn at its size, not puffed up to the hairline. Only the
// smallest one drawn may be a hairline (a part far under a pixel, or the fold); the rest fold into one
// "N more" part, and the k hairlines and k − 1 gaps fit in the box.
describe('folding what cannot be drawn truthfully', () => {
  const box = (width: number) => width - 2 * METER_PAD;
  const room = (width: number, k: number) => box(width) - (k - 1) * METER_GAP;

  // Other files are never folded in with the segments, so they too may be a hairline. The fold never
  // takes every segment: when none is drawn at its size, the largest stays beside the fold, both
  // hairlines.
  function holds(m: ReturnType<typeof meter>, width: number) {
    const k = m.drawn.length;
    expect(k * METER_HAIRLINE + (k - 1) * METER_GAP, 'the hairlines and the gaps fit in the content box').toBeLessThanOrEqual(box(width) + 1e-9);
    const own = m.drawn.filter((p) => p.kind !== 'other' && p.kind !== 'rest');
    expect(own.length === 0 || own.some((p) => p.kind === 'segment'), 'a segment is drawn beside the fold').toBe(true);
    const lone = own.length === 2 && own[1].kind === 'more';
    const atSize = own.sort((a, b) => a.share - b.share).slice(lone ? 2 : 1);
    for (const part of atSize) expect(part.share * room(width, k), `${part.key} is drawn at its size`).toBeGreaterThanOrEqual(METER_HAIRLINE - 1e-9);
  }

  it('lays a compact bar out for a narrower track than the full one', () => {
    expect(METER_WIDTH).toEqual({ full: 32, compact: 16 });
  });

  // 16rem less 0.4rem of padding is 15.6rem; with 18 games and the fold drawn, 18 gaps leave 12rem, of
  // which each game's 1/60 is the 0.2rem hairline. Without the padding the arithmetic would keep 20.
  it('folds sixty equal games in a compact bar into eighteen and one "42 more" part', () => {
    const games = Array.from({ length: 60 }, (_, i) => seg(`g${i}`, 10, { tone: gameTone(i) }));
    const m = meter(games, 600, undefined, METER_WIDTH.compact);
    expect(m.drawn.length).toBe(19);
    expect(m.drawn.slice(0, 18).map((p) => p.key)).toEqual(games.slice(0, 18).map((g) => g.key));
    const more = m.drawn[18];
    expect([more.kind, more.key, more.tone, more.size, more.folded?.length]).toEqual(['more', METER_MORE, 'more', { kind: 'exact', bytes: 420 }, 42]);
    expect(more.folded?.map((p) => [p.key, p.label, p.size])).toEqual(games.slice(18).map((g) => [g.key, g.label, { kind: 'exact', bytes: 10 }]));
    expect(m.parts.map((p) => p.key)).toEqual([...games.slice(0, 18).map((g) => g.key), METER_MORE]);
    holds(m, METER_WIDTH.compact);
  });

  it('draws the same sixty games each on its own in the full bar', () => {
    const games = Array.from({ length: 60 }, (_, i) => seg(`g${i}`, 10));
    const m = meter(games, 600, undefined, METER_WIDTH.full);
    expect(m.drawn.length).toBe(60);
    expect(m.parts.some((p) => p.kind === 'more')).toBe(false);
    holds(m, METER_WIDTH.full);
  });

  // Other files take a slot and a gap of their own: 59 games of 1/60 of the drive and 5 of 600 for
  // other files leave room for 17 games beside the fold and other files (15.6rem less 18 gaps is
  // 12rem); counted without other files, the room would keep 18, a little under their size.
  it('counts other files among the parts the room is shared by', () => {
    const games = Array.from({ length: 59 }, (_, i) => seg(`g${i}`, 10));
    const m = meter(games, 600, 5, METER_WIDTH.compact);
    expect(m.drawn.map((p) => p.kind)).toEqual([...Array.from({ length: 17 }, () => 'segment'), 'more', 'other']);
    expect(m.parts.find((p) => p.kind === 'more')?.folded).toHaveLength(42);
    holds(m, METER_WIDTH.compact);
  });

  it('folds the smallest, keeps the caller order of the rest, and puts the fold before other files', () => {
    const m = meter([seg('a', 5), seg('big', 9000), seg('b', 3), seg('c', 7), seg('d', 3000)], 100_000, 87_000, METER_WIDTH.compact);
    expect(m.drawn.map((p) => p.key)).toEqual(['big', 'd', METER_MORE, METER_OTHER]);
    expect(m.parts.find((p) => p.kind === 'more')?.folded?.map((p) => p.key)).toEqual(['a', 'b', 'c']);
    holds(m, METER_WIDTH.compact);
  });

  // Three games of 1, 5 and 3 GB on a 2 TB drive: none takes a hairline of the bar. Folding all three
  // would name no game and leave "N more" first with nothing before it.
  it('never folds every segment: the largest stays, as a hairline, and only the rest fold', () => {
    const GB = 1024 ** 3;
    const m = meter([seg('celeste', 1 * GB), seg('hades', 5 * GB), seg('inside', 3 * GB)], 2048 * GB, 1948 * GB);
    expect(m.parts.map((p) => p.key)).toEqual(['hades', METER_MORE, METER_OTHER]);
    expect(m.drawn.map((p) => p.key)).toEqual(['hades', METER_MORE, METER_OTHER]);
    expect(m.parts[1].folded?.map((p) => p.key)).toEqual(['celeste', 'inside']);
    expect(m.parts[1].size).toEqual({ kind: 'exact', bytes: 4 * GB });
    holds(m, METER_WIDTH.full);
  });

  it('folds nothing when only two segments are too small: a fold of one does not exist', () => {
    const m = meter([seg('a', 1), seg('b', 2)], 1e9);
    expect(m.drawn.map((p) => p.key)).toEqual(['a', 'b']);
  });

  it('gives each folded segment an id of its own when keys repeat', () => {
    const m = meter([seg('big', 1e9), seg('x', 1), seg('x', 1), seg('y', 1)], 2e9, undefined, METER_WIDTH.compact);
    expect(m.parts.find((p) => p.kind === 'more')?.folded?.map((p) => [p.id, p.key])).toEqual([
      ['x', 'x'],
      ['x#1', 'x'],
      ['y', 'y'],
    ]);
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
    expect(m.parts.map((p) => p.key)).toEqual(['big', 'zero', 'none', METER_REST]);
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

  // A segment that is only a lower bound may take some of the gap: what is left for other files is at
  // most the gap. That space is still known not to be free, so it is drawn at the gap's width.
  it('is an upper bound beside a segment that is only a lower bound, and still drawn: it is not free', () => {
    const m = meter([seg('games', 600, { sizeUnknown: true })], 1000, 300);
    expect(kinds(m)[1]).toEqual(['other', METER_OTHER, { kind: 'atMost', bytes: 100 }, 0.1]);
    expect(m.drawn.map((p) => p.key)).toEqual(['games', METER_OTHER]);
  });

  it('is an upper bound beside a segment of unknown size', () => {
    const m = meter([seg('games', 600), seg('new', 0, { sizeUnknown: true })], 1000, 300);
    expect(kinds(m)[2]).toEqual(['other', METER_OTHER, { kind: 'atMost', bytes: 100 }, 0.1]);
  });

  it('is not added when the segments and the free space fill the total', () => {
    expect(meter([seg('games', 600), seg('rest', 100, { tone: 'other' })], 1000, 300).parts.map((p) => p.kind)).toEqual(['segment', 'segment']);
  });

  it('is not added where the free space is derived, not measured, or the total is unknown', () => {
    expect(meter([seg('games', 600)], 1000).parts.map((p) => p.kind)).toEqual(['segment']);
    expect(meter([seg('games', 600)], 1000, null).parts.map((p) => p.kind)).toEqual(['segment', 'rest']);
    expect(meter([seg('games', 600)], 0, 300).parts.map((p) => p.kind)).toEqual(['segment']);
  });
});

// The bare track means free space and nothing else. The space after the segments is drawn as track
// only when it is known to be free (measured, or derived from exact sizes); otherwise it is one part
// of its own, other files and free space together, and the free space is not said apart from it.
describe('space not known to be free', () => {
  const kinds = (m: ReturnType<typeof meter>) => m.parts.map((p) => [p.kind, p.key, p.tone, p.size, p.share]);

  it('is a part of its own when the free space could not be measured, exact beside exact sizes', () => {
    for (const free of [null, Number.NaN, -1]) {
      const m = meter([seg('games', 600)], 1000, free);
      expect(kinds(m)).toEqual([
        ['segment', 'games', 'game', { kind: 'exact', bytes: 600 }, 0.6],
        ['rest', METER_REST, null, { kind: 'exact', bytes: 400 }, 0.4],
      ]);
      expect(m.drawn.map((p) => p.key)).toEqual(['games', METER_REST]);
      expect(m.free).toBeNull();
    }
  });

  it('is an upper bound beside a size not fully measured when the free space could not be measured', () => {
    const m = meter([seg('games', 600, { sizeUnknown: true }), seg('new', 0, { sizeUnknown: true })], 1000, null);
    expect(kinds(m)[2]).toEqual(['rest', METER_REST, null, { kind: 'atMost', bytes: 400 }, 0.4]);
    expect(m.free).toBeNull();
  });

  it('is a part of its own, an upper bound, when free space cannot be derived beside a lower bound or an unknown size', () => {
    const bound = meter([seg('games', 600, { sizeUnknown: true })], 1000);
    expect(kinds(bound)[1]).toEqual(['rest', METER_REST, null, { kind: 'atMost', bytes: 400 }, 0.4]);
    expect(bound.free).toBeNull();
    const unknown = meter([seg('games', 600), seg('new', 0, { sizeUnknown: true })], 1000);
    expect(kinds(unknown)[2]).toEqual(['rest', METER_REST, null, { kind: 'atMost', bytes: 400 }, 0.4]);
    expect(unknown.drawn.map((p) => p.key)).toEqual(['games', METER_REST]);
  });

  it('is the track, said as free, when it is derived from exact sizes or measured', () => {
    expect(meter([seg('games', 600)], 1000).parts.map((p) => p.kind)).toEqual(['segment']);
    expect(meter([seg('games', 600)], 1000).free).toEqual({ kind: 'exact', bytes: 400 });
    expect(meter([seg('games', 600, { sizeUnknown: true })], 1000, 300).parts.map((p) => p.kind)).toEqual(['segment', 'other']);
  });

  it('is not added where the segments leave no space or overflow', () => {
    expect(meter([seg('games', 1000)], 1000, null).parts.map((p) => p.kind)).toEqual(['segment']);
    expect(meter([seg('games', 1000, { sizeUnknown: true })], 1000).parts.map((p) => p.kind)).toEqual(['segment']);
    const over = meter([seg('games', 1200, { sizeUnknown: true })], 1000, null);
    expect([over.overflow, over.parts.map((p) => p.kind), over.free]).toEqual([true, ['segment'], null]);
  });

  it('is placed after the fold and takes a slot of the room', () => {
    const games = Array.from({ length: 59 }, (_, i) => seg(`g${i}`, 10));
    const m = meter(games, 600, null, METER_WIDTH.compact);
    expect(m.drawn.map((p) => p.kind)).toEqual([...Array.from({ length: 17 }, () => 'segment'), 'more', 'rest']);
  });
});

// Every combination the screens can hand the meter: overflow or not × free measured, derived or not
// measurable × sizes exact, a lower bound or an unknown one × a fold or none × the compact or the full
// width. For each, the model must follow the rules, so that the words, the legend and the bar, all
// drawn from it, agree.
describe('the rules over every combination', () => {
  const GB = 1024 ** 3;
  const T = 1000 * GB;
  type Free = 'measured' | 'derived' | 'null';
  type Sizes = 'exact' | 'lowerBound' | 'unknown';

  function input(overflow: boolean, free: Free, sizes: Sizes, fold: boolean) {
    const segments = [seg('g1', 300 * GB), seg('g2', 200 * GB, { sizeUnknown: sizes === 'lowerBound' })];
    if (overflow) segments.push(seg('g3', 700 * GB));
    if (sizes === 'unknown') segments.push(seg('new', 0, { sizeUnknown: true }));
    if (fold) for (let i = 0; i < 60; i += 1) segments.push(seg(`t${i}`, GB / 100));
    const measured = overflow ? 250 * GB : 300 * GB;
    return { segments, free: free === 'measured' ? measured : free === 'null' ? null : undefined };
  }

  const combos: [boolean, Free, Sizes, boolean, number][] = [];
  for (const overflow of [false, true])
    for (const free of ['measured', 'derived', 'null'] as const)
      for (const sizes of ['exact', 'lowerBound', 'unknown'] as const)
        for (const fold of [false, true]) for (const width of [METER_WIDTH.compact, METER_WIDTH.full]) combos.push([overflow, free, sizes, fold, width]);

  for (const [overflow, free, sizes, fold, width] of combos) {
    it(`overflow ${overflow}, free ${free}, sizes ${sizes}, fold ${fold}, width ${width}`, () => {
      const given = input(overflow, free, sizes, fold);
      const m = meter(given.segments, T, given.free, width);
      const used = given.segments.reduce((sum, s) => sum + s.bytes, 0);
      const measured = typeof given.free === 'number' ? given.free : 0;
      const exact = sizes === 'exact';
      expect(m.overflow).toBe(overflow);

      // Drawn against the segments and the measured free space in an overflow, else against the total.
      const against = overflow ? used + measured : T;
      const gap = !overflow && free === 'measured' ? T - used - measured : 0;
      // The space after the segments when it is not known to be free: free not measured, or not
      // derivable beside a size not fully measured.
      const rest = !overflow && (free === 'null' || (free === 'derived' && !exact)) ? T - used : 0;
      const bytesOf = (p: (typeof m.parts)[number]) =>
        p.kind === 'other'
          ? gap
          : p.kind === 'rest'
            ? rest
            : p.kind === 'more'
              ? (p.folded ?? []).reduce((sum, f) => sum + (f.size.kind === 'unknown' ? 0 : f.size.bytes), 0)
              : (given.segments.find((s) => s.key === p.key)?.bytes ?? 0);
      for (const part of m.drawn) expect(part.share * against, part.key).toBeCloseTo(bytesOf(part), -3);
      const drawn = m.drawn.reduce((sum, p) => sum + p.share, 0);
      expect(drawn).toBeCloseTo(overflow ? used / against : (used + gap + rest) / T, 9);

      // The legend lists every segment once (folded or not), other files for the gap and the rest for
      // the space not known to be free.
      const listed = m.parts.flatMap((p) => (p.kind === 'more' ? (p.folded ?? []) : [p]));
      expect(listed.filter((p) => p.kind === 'segment').map((p) => p.key)).toEqual(given.segments.map((s) => s.key));
      const other = m.parts.find((p) => p.kind === 'other');
      expect(other?.size ?? null).toEqual(gap > 0 ? { kind: exact ? 'exact' : 'atMost', bytes: gap } : null);
      const restPart = m.parts.find((p) => p.kind === 'rest');
      expect(restPart?.size ?? null).toEqual(rest > 0 ? { kind: exact ? 'exact' : 'atMost', bytes: rest } : null);
      expect(m.parts.some((p) => p.kind === 'more')).toBe(fold);

      // Free space: a measured one is always said; a derived one only without overflow and with every
      // size exact; one that could not be measured is not said apart: the rest says it, and in an
      // overflow nothing does.
      const said = free === 'measured' ? { kind: 'exact', bytes: measured } : free === 'derived' && !overflow && exact ? { kind: 'exact', bytes: T - used } : null;
      expect(m.free).toEqual(said);

      // The bare track is free space and nothing else: without an overflow, what the drawn parts leave
      // of the bar is the free space the model says.
      if (!overflow) expect((1 - drawn) * T, 'the bare track is the free space said').toBeCloseTo(m.free?.kind === 'exact' ? m.free.bytes : 0, -3);
      expect(numbers(m).every(Number.isFinite)).toBe(true);
    });
  }
});

describe('sizes in words', () => {
  const format = (bytes: number) => `${bytes} B`;
  const words = { atLeast: (size: string) => `at least ${size}`, atMost: (size: string) => `at most ${size}`, unknown: 'size unknown' };

  it('writes an exact size, a lower bound, an upper bound and an unknown size', () => {
    expect(sizeWords({ kind: 'exact', bytes: 5 }, format, words)).toBe('5 B');
    expect(sizeWords({ kind: 'atLeast', bytes: 5 }, format, words)).toBe('at least 5 B');
    expect(sizeWords({ kind: 'atMost', bytes: 5 }, format, words)).toBe('at most 5 B');
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

// An owner's own files, not a disk: the launcher's files across every drive have no free space of
// their own, and their total is their sum.
describe('the disk meter model of an owner\'s own files', () => {
  const GB = 1024 ** 3;
  it('says no free space and adds no part of its own', () => {
    const model = sumMeter([
      { key: 'a', label: 'A', bytes: 30 * GB, tone: 'game' },
      { key: 'b', label: 'B', bytes: 10 * GB, tone: 'data' },
    ]);
    expect(model.total).toBe(40 * GB);
    expect(model.totalAtLeast).toBe(false);
    expect(model.free).toBeNull();
    expect(model.overflow).toBe(false);
    expect(model.parts.map((part) => part.kind)).toEqual(['segment', 'segment']);
    expect(model.parts.map((part) => part.share)).toEqual([0.75, 0.25]);
  });

  it('says the total as a lower bound beside a size not fully measured', () => {
    const model = sumMeter([
      { key: 'a', label: 'A', bytes: 30 * GB, tone: 'game', sizeUnknown: true },
      { key: 'b', label: 'B', bytes: 10 * GB, tone: 'data' },
    ]);
    expect(model.total).toBe(40 * GB);
    expect(model.totalAtLeast).toBe(true);
    expect(model.free).toBeNull();
    expect(model.parts.every((part) => part.kind === 'segment')).toBe(true);
  });

  it('is an unknown total when nothing is measured, and a disk\'s total is never a lower bound', () => {
    expect(sumMeter([{ key: 'a', label: 'A', bytes: 0, tone: 'game', sizeUnknown: true }]).total).toBeNull();
    expect(meter([{ key: 'a', label: 'A', bytes: 30 * GB, tone: 'game', sizeUnknown: true }], 100 * GB, 10 * GB).totalAtLeast).toBe(false);
  });
});

