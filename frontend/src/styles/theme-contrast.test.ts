import { describe, expect, it } from 'vitest';
import { goSources } from '../lib/testing/sources';
import { luminance, rgb } from '../lib/theme/accent';
import { CONTRAST_FALLBACKS, CONTRAST_TOKENS, contrastReport, type RGBA } from '../lib/theme/contrastReport';
import { BACKDROP_LOOK_COLORS, backdropLook, lookPixels, type BackdropLook } from '../lib/backdrop/pixels';

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
  // An ON switch draws its knob in `--accent-on, var(--bg)` on the bright track; presets set no --accent-on.
  { fg: '--bg', on: ['--accent-bright'], min: 3 },
];

// The sidebar island is translucent and is to float over game art, so what sits on it is measured with
// the island painted over the page and over the two extremes art can bring, black and white.
const WHITE: RGB = [255, 255, 255];
const ON_ISLAND: { fg: string; min: number }[] = [
  { fg: '--text', min: 4.5 },
  { fg: '--text-2', min: 4.5 },
  { fg: '--text-3', min: 4.5 },
  { fg: '--accent-text', min: 4.5 },
  { fg: '--accent-bright', min: 3 },
];

function islandFailures(tokens: Record<string, string>): string[] {
  const out: string[] = [];
  const grounds: [string, RGB][] = [
    ['--bg', paint(tokens, '--bg', BLACK)],
    ['black', BLACK],
    ['white', WHITE],
  ];
  for (const [ground, below] of grounds) {
    const island = paint(tokens, '--island-fill', below);
    for (const pair of ON_ISLAND) {
      const r = ratio(paint(tokens, pair.fg, island), island);
      if (r < pair.min) out.push(`${pair.fg} on --island-fill over ${ground}: ${r.toFixed(2)} < ${pair.min}`);
    }
  }
  return out;
}

// The window backdrop is the game's art baked by the same code the window runs, with the theme's own
// colours and the shell's saturation. Every kind of art goes through it: the corners of the colour cube,
// a grey ramp and noise. The text levels must read on what comes out, and on the island over it.
function shellNumber(name: string): number {
  const css = fs.readFileSync(new URL('./shell.css', import.meta.url), 'utf8');
  const value = new RegExp(`${name}\\s*:\\s*([^;]+);`).exec(css)?.[1];
  expect(value, `${name} in shell.css`).toBeTypeOf('string');
  return Number(value);
}

function anyArt(): Uint8ClampedArray {
  const data: number[] = [];
  for (const r of [0, 255]) for (const g of [0, 255]) for (const b of [0, 255]) data.push(r, g, b, 255);
  for (let v = 0; v < 256; v += 5) data.push(v, v, v, 255);
  for (let i = data.length / 4; i < 64 * 64; i++) data.push((i * 97) % 256, (i * 57) % 256, (i * 193) % 256, 255);
  return Uint8ClampedArray.from(data);
}

function washLook(tokens: Record<string, string>) {
  const colors = Object.fromEntries(BACKDROP_LOOK_COLORS.map((name) => [name, rgba(tokens[name] ?? name)]));
  return backdropLook(colors, shellNumber('--ambient-saturate'));
}

function washFailures(tokens: Record<string, string>): string[] {
  const look = washLook(tokens);
  expect(look, 'the theme draws a wash').not.toBeNull();
  const limit = (look as BackdropLook).limit;
  const art = anyArt();
  lookPixels(art, 64, 64, look as BackdropLook);
  // The worst the bake allows is a pixel right at its luminance limit; it is measured alongside the art.
  const bound = limit.max ?? limit.min;
  expect(bound, 'luminance limit').toBeTypeOf('number');
  const at = bound as number;
  const grey = 255 * (at <= 0.0031308 ? at * 12.92 : 1.055 * at ** (1 / 2.4) - 0.055);
  const washes: RGB[] = [[grey, grey, grey]];
  for (let i = 0; i < art.length; i += 4) washes.push([art[i], art[i + 1], art[i + 2]]);
  const out = new Set<string>();
  for (const wash of washes) {
    const island = paint(tokens, '--island-fill', wash);
    for (const pair of ON_ISLAND) {
      const direct = ratio(paint(tokens, pair.fg, wash), wash);
      if (direct < pair.min) out.add(`${pair.fg} on the wash: ${direct.toFixed(2)} < ${pair.min}`);
      const on = ratio(paint(tokens, pair.fg, island), island);
      if (on < pair.min) out.add(`${pair.fg} on --island-fill over the wash: ${on.toFixed(2)} < ${pair.min}`);
    }
  }
  return [...out];
}

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

    it(`${id} keeps text and markers readable on the sidebar island over any backdrop`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      expect(islandFailures((preset as Preset).tokens)).toEqual([]);
    });

    if (id === 'contrast') {
      // High contrast keeps its plain black page: an opaque dim leaves nothing of the art to show.
      it('contrast draws no backdrop wash', () => {
        const preset = builtIns().find((p) => p.id === id);
        expect(preset, id).toBeDefined();
        expect(washLook((preset as Preset).tokens)).toBeNull();
      });
    } else {
      it(`${id} keeps text and markers readable on the backdrop wash over any art, and on the island over it`, () => {
        const preset = builtIns().find((p) => p.id === id);
        expect(preset, id).toBeDefined();
        expect(washFailures((preset as Preset).tokens)).toEqual([]);
      });
    }

    // --bg-sidebar rings the presence dot on the account avatar inside the island: it is the colour
    // the island shows over the page, or the ring reads as a halo.
    it(`${id} rings the avatar in the colour the island shows over the page`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      const tokens = (preset as Preset).tokens;
      const shown = paint(tokens, '--island-fill', paint(tokens, '--bg', BLACK));
      const ring = paint(tokens, '--bg-sidebar', BLACK);
      expect(ring.map((c, i) => Math.abs(c - shown[i])).every((d) => d <= 1), `${ring} vs ${shown.map(Math.round)}`).toBe(true);
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
