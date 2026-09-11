import { describe, it, expect } from 'vitest';
import { accentPalette, accentPresets, contrast, validAccent } from './accent';

describe('personal accent palette', () => {
  for (const base of ['dark', 'light', 'contrast']) {
    for (const color of [...accentPresets, '#FFFFFF', '#000000', '#FFFF00']) {
      it(`${base} / ${color}: readable controls, links and focus`, () => {
        const surfaces = base === 'light' ? ['#eef1f4', '#f8f9fb', '#ffffff', '#e9ecf1', '#dfe3ea'] : base === 'contrast' ? ['#000000', '#080808', '#111111', '#1c1c1c'] : ['#0a0f15', '#11171f', '#141c25', '#1c242f', '#242c38'];
        const tokens = Object.fromEntries(['--bg', '--surface', '--surface-2', '--surface-3', '--surface-4'].map((key, i) => [key, surfaces[i % surfaces.length]]));
        const p = accentPalette(color, base, tokens);
        expect(contrast(p['--accent'], p['--accent-on'])).toBeGreaterThanOrEqual(4.5);
        expect(contrast(p['--accent-hover'], p['--accent-on'])).toBeGreaterThanOrEqual(4.5);
        for (const surface of [...surfaces, p['--accent-subtle']]) expect(contrast(p['--accent-text'], surface)).toBeGreaterThanOrEqual(4.5);
        for (const surface of surfaces) {
          expect(contrast(p['--accent-ring'], surface)).toBeGreaterThanOrEqual(3);
          expect(contrast(p['--accent-hover'], surface)).toBeGreaterThanOrEqual(3);
        }
        expect(Object.keys(p).every(k => k.startsWith('--accent'))).toBe(true);
      });
    }
  }
  it('rejects partial, shorthand, alpha and injected colors', () => {
    for (const color of ['', '#', '#fff', '#GGGGGG', '#12345678', '#123456;', 'red']) {
      expect(validAccent(color)).toBe(false);
      expect(accentPalette(color, 'dark')).toEqual({});
    }
  });
});
