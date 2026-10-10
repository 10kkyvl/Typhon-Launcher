import { describe, expect, it } from 'vitest';
import { CENTER, arcPath, emblemOf, seedOf, type FingerprintInput } from './fingerprint';
import { pickTarget } from './reorder';

const rpg: FingerprintInput = {
  genres: [
    { name: 'Role-playing (RPG)', share: 0.6 },
    { name: 'Adventure', share: 0.25 },
    { name: 'Strategy', share: 0.1 },
  ],
  hours: 412,
  games: 38,
  completed: 12,
};

const shooter: FingerprintInput = {
  genres: [
    { name: 'Shooter', share: 0.7 },
    { name: 'Indie', share: 0.2 },
  ],
  hours: 90,
  games: 14,
  completed: 3,
};

describe('emblemOf', () => {
  it('draws the same emblem for the same data', () => {
    expect(emblemOf(rpg)).toEqual(emblemOf(structuredClone(rpg)));
    expect(JSON.stringify(emblemOf(rpg))).toBe(JSON.stringify(emblemOf(rpg)));
  });

  it('draws visibly different emblems for different libraries', () => {
    const a = emblemOf(rpg);
    const b = emblemOf(shooter);

    expect(a.seed).not.toBe(b.seed);
    expect(a.rings.map((ring) => ring.arc)).not.toEqual(b.rings.map((ring) => ring.arc));
    expect(a.rotation).not.toBe(b.rotation);
  });

  it('changes when only the hours change', () => {
    expect(emblemOf({ ...rpg, hours: 413 }).seed).not.toBe(emblemOf(rpg).seed);
  });

  it('gives each top genre one ring whose arc follows its share', () => {
    const emblem = emblemOf(rpg);

    expect(emblem.rings).toHaveLength(3);
    expect(emblem.rings.map((ring) => ring.sweep)).toEqual([216, 90, 36]);
  });

  it('keeps at most five rings and nests them inside the frame', () => {
    const many: FingerprintInput = {
      ...rpg,
      genres: Array.from({ length: 8 }, (_, i) => ({ name: `G${i}`, share: 0.1 })),
    };

    const emblem = emblemOf(many);
    const radii = emblem.rings.map((ring) => ring.r);

    expect(emblem.rings).toHaveLength(5);
    expect(radii).toEqual([...radii].sort((x, y) => y - x));
    expect(Math.max(...emblem.rings.map((ring) => ring.r + ring.width / 2))).toBeLessThanOrEqual(CENTER);
    expect(Math.min(...radii) - emblem.rings[0].width / 2).toBeGreaterThan(emblem.core);
  });

  it('makes the rings thicker for more hours and the core bigger for more games', () => {
    const small = emblemOf({ ...rpg, hours: 2, games: 1 });
    const large = emblemOf({ ...rpg, hours: 5000, games: 400 });

    expect(large.rings[0].width).toBeGreaterThan(small.rings[0].width);
    expect(large.core).toBeGreaterThan(small.core);
  });

  it('places one satellite per completed game, up to twelve', () => {
    expect(emblemOf({ ...rpg, completed: 0 }).satellites).toHaveLength(0);
    expect(emblemOf({ ...rpg, completed: 5 }).satellites).toHaveLength(5);
    expect(emblemOf({ ...rpg, completed: 40 }).satellites).toHaveLength(12);
  });

  it('never lets a tiny share vanish or a whole share close the circle', () => {
    const sweeps = emblemOf({ ...rpg, genres: [{ name: 'A', share: 0.001 }, { name: 'B', share: 1 }] }).rings.map((ring) => ring.sweep);

    expect(sweeps).toEqual([10, 359]);
  });

  it('still draws a quiet emblem for an empty library', () => {
    const emblem = emblemOf({ genres: [], hours: 0, games: 0, completed: 0 });

    expect(emblem.rings).toHaveLength(1);
    expect(emblem.rings[0].arc).toBe('');
    expect(emblem.satellites).toEqual([]);
    expect(emblem.core).toBeGreaterThan(0);
  });

  it.each([Number.NaN, -5, Number.POSITIVE_INFINITY])('survives %s in the counters', (value) => {
    const emblem = emblemOf({ ...rpg, hours: value, games: value, completed: value });

    expect(emblem.rings.every((ring) => Number.isFinite(ring.r) && Number.isFinite(ring.width))).toBe(true);
    expect(Number.isFinite(emblem.core)).toBe(true);
  });
});

describe('seedOf', () => {
  it('is a stable unsigned 32-bit number', () => {
    expect(seedOf(rpg)).toBe(seedOf(rpg));
    expect(seedOf(rpg)).toBeGreaterThanOrEqual(0);
    expect(seedOf(rpg)).toBeLessThan(2 ** 32);
  });

  it('depends on the order and the names of the genres', () => {
    expect(seedOf({ ...rpg, genres: [...rpg.genres].reverse() })).not.toBe(seedOf(rpg));
    expect(seedOf({ ...rpg, genres: [{ name: 'Other', share: 0.6 }, ...rpg.genres.slice(1)] })).not.toBe(seedOf(rpg));
  });
});

describe('arcPath', () => {
  it('starts at the top for a zero start angle', () => {
    expect(arcPath(40, 0, 90)).toBe('M 50 10 A 40 40 0 0 1 90 50');
  });

  it('sets the large-arc flag past half a circle', () => {
    expect(arcPath(40, 0, 180)).toContain('0 0 1');
    expect(arcPath(40, 0, 200)).toContain('0 1 1');
  });
});

describe('pickTarget', () => {
  const boxes = [
    { index: 0, left: 0, top: 0, right: 100, bottom: 50 },
    { index: 1, left: 110, top: 0, right: 210, bottom: 50 },
    { index: 4, left: 0, top: 60, right: 210, bottom: 110 },
  ];

  it('picks the box under the pointer', () => {
    expect(pickTarget(150, 20, boxes)).toBe(1);
    expect(pickTarget(10, 100, boxes)).toBe(4);
  });

  it('picks the nearest box in a gap or outside', () => {
    expect(pickTarget(105, 20, boxes)).toBe(0);
    expect(pickTarget(500, 20, boxes)).toBe(1);
    expect(pickTarget(100, 500, boxes)).toBe(4);
  });

  it('picks nothing without boxes', () => {
    expect(pickTarget(1, 1, [])).toBeNull();
  });
});
