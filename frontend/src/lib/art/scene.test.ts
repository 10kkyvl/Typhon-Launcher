import { describe, expect, it } from 'vitest';
import { artScene, artSpec } from './scene';

describe('the generated art of a game', () => {
  it('is the same every time for the same game', () => {
    expect(artSpec('g-hades')).toEqual(artSpec('g-hades'));
    expect(artScene('g-hades', 'cover')).toEqual(artScene('g-hades', 'cover'));
    expect(artScene('g-hades', 'wide')).toEqual(artScene('g-hades', 'wide'));
  });

  it('differs between games', () => {
    const ids = Array.from({ length: 24 }, (_, i) => `game-${i}`);
    const specs = new Set(ids.map((id) => JSON.stringify(artSpec(id))));
    const scenes = new Set(ids.map((id) => JSON.stringify(artScene(id, 'cover'))));
    expect(specs.size).toBe(ids.length);
    expect(scenes.size).toBe(ids.length);
  });

  it('draws a sky and at least one land, in the box of its shape', () => {
    const cover = artScene('g-celeste', 'cover');
    const wide = artScene('g-celeste', 'wide');
    expect(cover.width).toBeLessThan(cover.height);
    expect(wide.width).toBeGreaterThan(wide.height);
    for (const scene of [cover, wide]) {
      expect(scene.sky.length).toBeGreaterThanOrEqual(3);
      expect(scene.lands.length).toBeGreaterThanOrEqual(1);
      expect(scene.lands.every((land) => /^(M-?[\d.]+ -?[\d.]+(L-?[\d.]+ -?[\d.]+)+Z)+$/.test(land.d))).toBe(true);
    }
  });

  it('is data only: no picture to fetch, no link', () => {
    const text = JSON.stringify([artScene('g-hades', 'cover'), artScene('g-hades', 'wide')]);
    expect(text).not.toMatch(/https?:|url\(|data:|blob:/);
  });

  it('takes nothing from the theme: an empty seed still draws', () => {
    expect(artScene('', 'cover').lands.length).toBeGreaterThan(0);
  });
});
