import { describe, expect, it } from 'vitest';
import { goSources } from '../lib/testing/sources';
import { luminance, rgb } from '../lib/theme/accent';
import { CONTRAST_FALLBACKS, CONTRAST_TOKENS, contrastReport, type RGBA } from '../lib/theme/contrastReport';

/**
 * WCAG 2.2 AA for every built-in theme, measured on the preset values in internal/theme/presets.go:
 * the three text levels on every surface, accent and status text where components set them on the
 * page, and the label colour on the accent fill. tokens.css carries the dark preset as the stylesheet
 * default, so the two must not drift.
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

interface Preset {
  id: string;
  tokens: Record<string, string>;
}

function goPresets(): Map<string, Preset> {
  const source = goSources['internal/theme/presets.go'];
  expect(source).toBeTypeOf('string');
  const byVar = new Map<string, Preset>();
  for (const m of source.matchAll(/^var (preset\w+) = Theme\{([\s\S]*?)^\}/gm)) {
    const id = /\bID:\s*"([^"]+)"/.exec(m[2])?.[1];
    expect(id, m[1]).toBeTypeOf('string');
    const tokens = Object.fromEntries([...m[2].matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((t) => [t[1], t[2]]));
    byVar.set(m[1], { id: id as string, tokens });
  }
  return byVar;
}

function builtIns(): Preset[] {
  const source = goSources['internal/theme/presets.go'];
  const list = /^var presets = \[\]Theme\{([^}]*)\}/m.exec(source)?.[1];
  expect(list, 'var presets').toBeTypeOf('string');
  const byVar = goPresets();
  return (list as string).split(',').map((name) => name.trim()).filter(Boolean).map((name) => {
    const preset = byVar.get(name);
    expect(preset, name).toBeDefined();
    return preset as Preset;
  });
}

function rootBlock(): Record<string, string> {
  const css = fs.readFileSync(new URL('./tokens.css', import.meta.url), 'utf8');
  const start = css.indexOf(':root {');
  expect(start).toBeGreaterThanOrEqual(0);
  const body = css.slice(css.indexOf('{', start) + 1, css.indexOf('\n}', start));
  return Object.fromEntries([...body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
}

type RGB = [number, number, number];
const BLACK: RGB = [0, 0, 0];
const MEASURABLE = /^(?:#[0-9a-f]{3,8}|rgba?\([^()]*\))$/i;

// Each layer is painted over the one it sits on, so a translucent value is measured as the user sees it.
function paint(tokens: Record<string, string>, name: string, below: RGB): RGB {
  const value = tokens[name] ?? name;
  expect(value, `${name} is a colour the guard can measure`).toMatch(MEASURABLE);
  return rgb(value, below);
}

function ratio(a: RGB, b: RGB): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

const PAGE = ['--bg', '--bg-sidebar', '--surface', '--surface-2', '--surface-3', '--surface-4'];
const LOW = ['--bg', '--surface', '--surface-2'];
// Buttons, the card's Play and every other accent fill draw `var(--accent-on, #fff)`; --accent-on is
// set only by a personal accent, so on a theme's own accent the label is the fallback.
const ON_ACCENT = CONTRAST_FALLBACKS['--accent-on'];

const PAIRS: { fg: string; on: string[]; min: number }[] = [
  { fg: '--text', on: PAGE, min: 4.5 },
  { fg: '--text-2', on: PAGE, min: 4.5 },
  { fg: '--text-3', on: PAGE, min: 4.5 },
  { fg: '--accent-text', on: LOW, min: 4.5 },
  { fg: '--danger', on: LOW, min: 4.5 },
  { fg: '--success', on: LOW, min: 4.5 },
  { fg: '--warning', on: LOW, min: 4.5 },
  { fg: ON_ACCENT, on: ['--accent', '--accent-hover'], min: 4.5 },
];

function failures(tokens: Record<string, string>, pairs = PAIRS): string[] {
  const out: string[] = [];
  const bg = paint(tokens, '--bg', BLACK);
  for (const pair of pairs) {
    for (const surface of pair.on) {
      const under = surface === '--bg' ? bg : paint(tokens, surface, bg);
      const r = ratio(paint(tokens, pair.fg, under), under);
      if (r < pair.min) out.push(`${pair.fg} on ${surface}: ${r.toFixed(2)} < ${pair.min}`);
    }
  }
  return out;
}

// Indicators (progress, toggles, selection, markers) and the focus ring must stand out on every level;
// --accent-bright also reads as text on the low surfaces, where links and labels sit.
const INDICATORS: { fg: string; on: string[]; min: number }[] = [
  { fg: '--accent-bright', on: PAGE, min: 3 },
  { fg: '--accent-bright', on: LOW, min: 4.5 },
  { fg: '--accent-ring', on: PAGE, min: 3 },
];

function rgba(value: string): RGBA {
  expect(value).toMatch(MEASURABLE);
  const [r, g, b] = rgb(value, BLACK);
  const alpha = /^rgba\([^,]+,[^,]+,[^,]+,\s*([\d.]+)\s*\)$/.exec(value)?.[1];
  if (alpha === undefined) return [r, g, b, 1];
  const a = Number(alpha);
  return a === 0 ? [0, 0, 0, 0] : [r / a, g / a, b / a, a];
}

describe('built-in themes', () => {
  it('are dark, light, contrast and blush, in that order', () => {
    expect(builtIns().map((p) => p.id)).toEqual(['dark', 'light', 'contrast', 'blush']);
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps text readable on every surface (WCAG AA)`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      expect(failures((preset as Preset).tokens)).toEqual([]);
    });
  }

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id} keeps indicators and the focus ring at 3:1 on every surface`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      expect(failures((preset as Preset).tokens, INDICATORS)).toEqual([]);
    });

    it(`${id} shows no warning in the Settings readability panel`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      const tokens = { ...CONTRAST_FALLBACKS, ...(preset as Preset).tokens };
      const colors = Object.fromEntries(CONTRAST_TOKENS.map((name) => [name, rgba(tokens[name] ?? name)]));
      expect(contrastReport(colors).filter((row) => row.ok !== true)).toEqual([]);
    });
  }

  // High contrast keeps the accent itself an indicator (focus ring, toggles) on every level.
  it('contrast keeps its accent at 3:1 on every surface', () => {
    const preset = builtIns().find((p) => p.id === 'contrast');
    expect(preset).toBeDefined();
    expect(failures((preset as Preset).tokens, [{ fg: '--accent', on: PAGE, min: 3 }])).toEqual([]);
  });

  it('tokens.css :root is the dark preset, value for value', () => {
    const dark = builtIns().find((p) => p.id === 'dark');
    expect(dark).toBeDefined();
    expect(rootBlock()).toEqual((dark as Preset).tokens);
  });
});
