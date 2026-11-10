import { describe, it, expect } from 'vitest';
import { accentPalette, accentPresets, contrast, validAccent } from './accent';
import { goSources } from '../testing/sources';

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

it('composites alpha surfaces over a light theme instead of treating their black channels as opaque', () => {
  const p = accentPalette('#388BFF', 'light', { '--bg': '#ffffff', '--surface': '#ffffff', '--surface-3': 'rgba(0, 0, 0, 0.05)' });
  expect(p['--accent-text']).not.toBe('#000000');
  expect(contrast(p['--accent-text'], '#f2f2f2')).toBeGreaterThanOrEqual(4.5);
});
it('resolves CSS token references and uses the theme background for unsupported color forms', () => {
  const p = accentPalette('#388BFF', 'light', { '--bg': '#ffffff', '--surface': '#f8f9fb', '--surface-3': 'var(--surface)', '--surface-4': 'color(display-p3 1 1 1)' });
  expect(p['--accent-text']).not.toBe('#000000');
  expect(contrast(p['--accent-text'], '#f8f9fb')).toBeGreaterThanOrEqual(4.5);
});

describe('personal accent on the built-in presets', () => {
  const source = goSources['internal/theme/presets.go'] ?? '';
  const presets = [...source.matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)].map((m) => ({
    id: /\bID:\s*"([^"]+)"/.exec(m[1])?.[1] ?? '',
    base: /\bBase:\s*Base(\w+)/.exec(m[1])?.[1].toLowerCase() ?? '',
    tokens: Object.fromEntries([...m[1].matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((t) => [t[1], t[2]])),
  }));
  const surfaces = ['--bg', '--bg-sidebar', '--surface', '--surface-2', '--surface-3', '--surface-4'];

  it('reads the four presets', () => {
    expect(presets.map((p) => p.id)).toEqual(['dark', 'light', 'contrast', 'blush']);
  });

  for (const seed of ['#14202C', '#F2E6B8', '#4F6BFF', '#000000', '#FFFFFF', '#E45D87']) {
    it(`${seed}: indicators stand out and labels read on every preset`, () => {
      for (const preset of presets) {
        const p = accentPalette(seed, preset.base, preset.tokens);
        expect(p['--accent-bright'], `${preset.id} --accent-bright`).toMatch(/^#[0-9a-f]{6}$/);
        for (const name of surfaces) {
          expect(contrast(p['--accent-bright'], preset.tokens[name]), `${preset.id} bright on ${name}`).toBeGreaterThanOrEqual(3);
        }
        expect(contrast(p['--accent'], p['--accent-on']), `${preset.id} label`).toBeGreaterThanOrEqual(4.5);
        expect(contrast(p['--accent-hover'], p['--accent-on']), `${preset.id} hover label`).toBeGreaterThanOrEqual(4.5);
      }
    });
  }
});
