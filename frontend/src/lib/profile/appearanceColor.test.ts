import { describe, expect, it } from 'vitest';
import { contrast } from '../theme/accent';
import { DARK_INK, inkFor, LIGHT_INK, luminanceOf, nameGradientEnd, pureSurfaces } from './appearanceColor';

describe('relative luminance', () => {
  it('matches the WCAG endpoints and a known mid value', () => {
    expect(luminanceOf('#000000')).toBeCloseTo(0, 5);
    expect(luminanceOf('#ffffff')).toBeCloseTo(1, 5);
    expect(luminanceOf('#808080')).toBeCloseTo(0.2159, 3);
  });
});

describe('canvas ink', () => {
  const cases: [string, typeof LIGHT_INK][] = [
    ['#000000', LIGHT_INK],
    ['#142235', LIGHT_INK],
    ['#7a3030', LIGHT_INK],
    ['#e8e8f0', DARK_INK],
    ['#ffffff', DARK_INK],
    ['#fff7b0', DARK_INK],
    ['#b8e8ff', DARK_INK],
  ];
  it.each(cases)('picks the readable text for %s', (background, ink) => {
    expect(inkFor(background)).toBe(ink);
  });
  it('reads at least as well as the alternative on every grey step', () => {
    for (let v = 0; v <= 255; v += 5) {
      const bg = '#' + v.toString(16).padStart(2, '0').repeat(3);
      const chosen = inkFor(bg);
      const other = chosen === DARK_INK ? LIGHT_INK : DARK_INK;
      expect(contrast(bg, chosen.text)).toBeGreaterThanOrEqual(contrast(bg, other.text));
    }
  });
  it('keeps the primary text above 4.5 on clearly light and clearly dark backgrounds', () => {
    for (const bg of ['#000000', '#101820', '#2a1030', '#ffffff', '#f4eedd', '#cfe8ff']) {
      expect(contrast(bg, inkFor(bg).text)).toBeGreaterThanOrEqual(4.5);
    }
  });
});

describe('pure custom surfaces', () => {
  it('returns the plain middle of the gradient without darkening', () => {
    const { background, ink } = pureSurfaces('#ffffff', '#ffffff');
    expect(background).toBe('#ffffff');
    expect(ink).toBe(DARK_INK);
  });
  it('averages the two colours', () => {
    expect(pureSurfaces('#000000', '#ffffff').background).toBe('#808080');
  });
  it('lifts the card off a black background instead of leaving it invisible', () => {
    const { background, surface } = pureSurfaces('#000000', '#000000');
    expect(background).toBe('#000000');
    expect(surface).not.toBe(background);
  });
  it('keeps the card text readable against the surface for light and dark pairs', () => {
    for (const [from, to] of [['#ffffff', '#f3e6c8'], ['#fff700', '#ffffff'], ['#101820', '#203040'], ['#000000', '#000000']]) {
      const { surface, ink } = pureSurfaces(from, to);
      expect(contrast(surface, ink.text)).toBeGreaterThanOrEqual(4.5);
    }
  });
});

describe('name gradient end', () => {
  it('lightens a dark colour and darkens a light one', () => {
    expect(luminanceOf(nameGradientEnd('#401020'))).toBeGreaterThan(luminanceOf('#401020'));
    expect(luminanceOf(nameGradientEnd('#ffffff'))).toBeLessThan(luminanceOf('#ffffff'));
  });
  it('never returns the start colour', () => {
    for (const color of ['#000000', '#ffffff', '#67d8ef', '#ff0000']) {
      expect(nameGradientEnd(color)).not.toBe(color);
    }
  });
});
