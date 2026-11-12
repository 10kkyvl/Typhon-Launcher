import { describe, expect, it } from 'vitest';
import buttonSource from '../lib/components/Button.svelte?raw';
import noticeSource from '../lib/components/Notice.svelte?raw';
import textLinkSource from '../lib/components/TextLink.svelte?raw';
import { cssColor, ruleBody, styleOf } from '../lib/testing/css';
import { goSources } from '../lib/testing/sources';
import { luminance, rgb } from '../lib/theme/accent';
import { CONTRAST_CHECKS, CONTRAST_FALLBACKS, CONTRAST_TOKENS, DERIVED_COLORS, contrastReport, type RGBA } from '../lib/theme/contrastReport';
import {
  BACKDROP_LOOK_COLORS,
  WASH_MARGIN,
  WASH_TEXT,
  backdropLook,
  lookPixels,
  washLimit,
  type BackdropLook,
  type WashLimit,
} from '../lib/backdrop/pixels';

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

// Button.svelte's danger button as it is written: the guard measures what the component draws.
const BUTTON = styleOf(buttonSource, 'Button.svelte');
const DANGER = ruleBody(BUTTON, '.danger');
const DANGER_HOVER = ruleBody(BUTTON, ".danger:hover:not(:disabled, [aria-disabled='true'])");

function over(top: RGBA, below: RGB): RGB {
  return [0, 1, 2].map((i) => top[i] * top[3] + below[i] * (1 - top[3])) as RGB;
}

const PAGE = ['--bg', '--bg-sidebar', '--surface', '--surface-2', '--surface-3', '--surface-4'];
const LOW = ['--bg', '--surface', '--surface-2'];
// Buttons, the card's Play and every other accent fill draw `var(--accent-on, var(--accent-on-default))`;
// --accent-on is set only by a personal accent, so on a theme's own accent the label is the fixed default
// (shell.css; control-tokens.test.ts holds it equal to this fallback).
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
  // A focused field with a bad value draws its ring in --danger (global.css).
  { fg: '--danger', on: PAGE, min: 3 },
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

// Every colour the frontend paints straight on the page, which the wash now is: the text levels, links
// and indicators. Status text sits on a surface (status-surface.test.ts): on the bare page a status
// colour is a mark, a dot or a badge at 3:1, and on the island it is still text.
const ON_WASH: { fg: string; min: number; island?: number }[] = [
  ...ON_ISLAND,
  { fg: '--success', min: 3, island: 4.5 },
  { fg: '--warning', min: 3, island: 4.5 },
  { fg: '--danger', min: 3, island: 4.5 },
];

// Which presets the bake leaves without a wash, and why; the others must draw one and keep it readable.
const NO_WASH: Record<string, string> = {
  light: 'its faint text keeps the wash so light that the art mid tones land under MIN_SPREAD grey levels apart',
  contrast: 'its dim share is 1, nothing of the art would show',
};

// The luminance the wash is held to where one is drawn: the band --text-3 alone allows at its own target,
// under it on a dark page and over it on a light one. No other colour on the page narrows it.
function faintBand(tokens: Record<string, string>) {
  const bg = paint(tokens, '--bg', BLACK);
  const faint = paint(tokens, '--text-3', bg);
  const target = WASH_TEXT.find((t) => t.name === '--text-3');
  expect(target, '--text-3 in WASH_TEXT').toBeDefined();
  return washLimit([{ color: faint, ratio: (target as { min: number }).min + WASH_MARGIN }], luminance(faint) > luminance(bg));
}

function washLook(tokens: Record<string, string>) {
  const colors = Object.fromEntries(BACKDROP_LOOK_COLORS.map((name) => [name, rgba(tokens[name] ?? name)]));
  return backdropLook(colors, shellNumber('--ambient-saturate'), Number(tokens['--ambient-dim']));
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
    for (const pair of ON_WASH) {
      const direct = ratio(paint(tokens, pair.fg, wash), wash);
      if (direct < pair.min) out.add(`${pair.fg} on the wash: ${direct.toFixed(2)} < ${pair.min}`);
      const on = ratio(paint(tokens, pair.fg, island), island);
      const min = pair.island ?? pair.min;
      if (on < min) out.add(`${pair.fg} on --island-fill over the wash: ${on.toFixed(2)} < ${min}`);
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

// The muted text link (a breadcrumb, a path) stands on any page surface and on the backdrop wash, and
// turns to --text on hover: both colours must be ones measured there at 4.5:1 in every preset. --text-3
// is measured too, but holds the line by hundredths (4.51 on light surfaces, 4.57 on the dark wash);
// --text-2 keeps 5.87 and 6.22 at its worst.
describe('the muted text link is measured where it stands', () => {
  const link = styleOf(textLinkSource, 'TextLink.svelte');
  const token = (value: string) => /^var\((--[\w-]+)\)$/.exec(value)?.[1];
  const onEveryPage = (fg: string | undefined) => PAIRS.some((pair) => pair.fg === fg && pair.min >= 4.5 && PAGE.every((s) => pair.on.includes(s)));
  const onTheWash = (fg: string | undefined) => ON_WASH.some((pair) => pair.fg === fg && pair.min >= 4.5);

  it('at rest and hovered', () => {
    const rest = token(ruleBody(link, '.text-link.muted').color);
    const hover = token(ruleBody(link, '.text-link:hover:not(:disabled)').color);
    expect(rest).toBe('--text-2');
    for (const fg of [rest, hover]) {
      expect(onEveryPage(fg), `${fg} on every page surface`).toBe(true);
      expect(onTheWash(fg), `${fg} on the wash`).toBe(true);
    }
  });
});

// A text field's edge is what shows where it is (WCAG 1.4.11): 3:1 against whatever it sits on, every
// page surface and the toned plate of a notice, at rest and hovered. The browser paints a border over
// the field's own fill, so the edge is opaque: what is measured is what is drawn, whatever lies under.
describe('a text field shows where it is', () => {
  const global = fs.readFileSync(new URL('./global.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
  const shell = fs.readFileSync(new URL('./shell.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
  const notice = styleOf(noticeSource, 'Notice.svelte');
  const fixed = Object.fromEntries([...shell.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
  const rule = (selector: string) => {
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return new RegExp(`(?:^|\\})\\s*${escaped}\\s*\\{([^}]*)\\}`).exec(global)?.[1] ?? '';
  };
  const edgeOf = (body: string) => {
    const color = /border-color:\s*([^;]+);/.exec(body)?.[1] ?? /border:\s*\S+\s+solid\s+([^;]+);/.exec(body)?.[1];
    expect(color, body).toBeTypeOf('string');
    return (color as string).trim();
  };
  // A fixed token named by the rule, resolved once so the measure sees the themable tokens under it.
  const resolve = (value: string) => {
    const name = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
    return name && name in fixed ? fixed[name] : value;
  };
  // The notice's plate in each of its tones, as Notice.svelte writes them.
  const plate = ruleBody(notice, '.notice').background;
  const tones = [
    ruleBody(notice, '.notice')['--notice-tone'],
    ...['success', 'warning', 'danger'].map((tone) => ruleBody(notice, `.notice[data-tone='${tone}']`)['--notice-tone']),
  ];
  const STATES = [
    ['at rest', '.input'],
    ['hovered', '.input:hover:not(:disabled)'],
  ] as const;

  it('reads the plates and the edges it measures', () => {
    expect(plate).toMatch(/var\(--notice-tone\)/);
    expect(tones).toEqual(['var(--accent-bright)', 'var(--success)', 'var(--warning)', 'var(--danger)']);
    for (const [, selector] of STATES) expect(edgeOf(rule(selector)), selector).toBeTypeOf('string');
  });

  for (const id of ['dark', 'light', 'contrast', 'blush']) {
    it(`${id}: an opaque edge at 3:1 on every surface and notice plate, at rest and hovered`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      const tokens = (preset as Preset).tokens;
      const bg = paint(tokens, '--bg', BLACK);
      const grounds: [string, RGB][] = [
        ...PAGE.map((surface): [string, RGB] => [surface, surface === '--bg' ? bg : paint(tokens, surface, bg)]),
        ...tones.map((tone): [string, RGB] => [`notice ${tone}`, over(cssColor(plate.replace('var(--notice-tone)', tone), tokens), bg)]),
      ];
      const out: string[] = [];
      for (const [state, selector] of STATES) {
        const drawn = cssColor(resolve(edgeOf(rule(selector))), tokens);
        if (drawn[3] < 1) out.push(`${state}: translucent (${drawn[3]})`);
        for (const [ground, under] of grounds) {
          const r = ratio(over(drawn, under), under);
          if (r < 3) out.push(`${state} on ${ground}: ${r.toFixed(2)} < 3`);
        }
      }
      expect(out).toEqual([]);
    });
  }
});

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
    // The fill and the label Button.svelte writes for `.danger`, at rest and hovered, measured on every
    // surface the button can stand on.
    it(`${id} keeps the danger button's label readable at rest and hovered`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      const tokens = (preset as Preset).tokens;
      const bg = paint(tokens, '--bg', BLACK);
      const out: string[] = [];
      for (const surface of PAGE) {
        const ground = surface === '--bg' ? bg : paint(tokens, surface, bg);
        for (const [state, rule] of [['at rest', DANGER], ['hovered', DANGER_HOVER]] as const) {
          const fill = over(cssColor(rule.background, tokens), ground);
          const r = ratio(over(cssColor(rule.color, tokens), fill), fill);
          if (r < 4.5) out.push(`${state} over ${surface}: ${r.toFixed(2)}`);
        }
      }
      expect(out).toEqual([]);
    });

    it(`${id} keeps indicators and the focus ring at 3:1 on every surface`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      expect(failures((preset as Preset).tokens, INDICATORS)).toEqual([]);
    });

    // What shows a control's state without text must stand out at 3:1 (WCAG 1.4.11): the knob of an
    // OFF switch on its track (Toggle.svelte: --text-2 on --hover-strong over the surface it sits on).
    it(`${id} shows an OFF switch on every surface`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      const tokens = (preset as Preset).tokens;
      const toggle = fs.readFileSync(new URL('../lib/components/Toggle.svelte', import.meta.url), 'utf8');
      expect(toggle).toMatch(/\.track \{[^}]*background: var\(--hover-strong\);/);
      expect(toggle).toMatch(/\.knob \{[^}]*background: var\(--text-2\);/);

      const out: string[] = [];
      const bg = paint(tokens, '--bg', BLACK);
      for (const surface of PAGE) {
        const under = surface === '--bg' ? bg : paint(tokens, surface, bg);
        const track = paint(tokens, '--hover-strong', under);
        const knob = ratio(paint(tokens, '--text-2', track), track);
        if (knob < 3) out.push(`OFF knob on its track over ${surface}: ${knob.toFixed(2)} < 3`);
      }
      expect(out).toEqual([]);
    });

    // The scrollbar thumb shows where the page is (global.css: --text-3 mixed with what is under it).
    it(`${id} shows the scrollbar thumb on every surface`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      const tokens = (preset as Preset).tokens;
      const css = fs.readFileSync(new URL('./global.css', import.meta.url), 'utf8');
      const share = /::-webkit-scrollbar-thumb \{[^}]*background: color-mix\(in srgb, var\(--text-3\) (\d+)%, transparent\);/.exec(css)?.[1];
      expect(share, 'the scrollbar thumb is --text-3 mixed with transparent').toBeTypeOf('string');
      const part = Number(share) / 100;

      const out: string[] = [];
      const bg = paint(tokens, '--bg', BLACK);
      for (const surface of PAGE) {
        const under = surface === '--bg' ? bg : paint(tokens, surface, bg);
        const ink = paint(tokens, '--text-3', under);
        const thumb = ink.map((channel, i) => channel * part + under[i] * (1 - part)) as RGB;
        const seen = ratio(thumb, under);
        if (seen < 3) out.push(`scrollbar thumb on ${surface}: ${seen.toFixed(2)} < 3`);
      }
      expect(out).toEqual([]);
    });

    it(`${id} keeps text and markers readable on the sidebar island over any backdrop`, () => {
      const preset = builtIns().find((p) => p.id === id);
      expect(preset, id).toBeDefined();
      expect(islandFailures((preset as Preset).tokens)).toEqual([]);
    });

    if (id === 'dark') {
      it('the bake limits the wash for every colour that sits on it, at its own target', () => {
        const limited = new Map(WASH_TEXT.map((t) => [t.name, t.min]));
        expect(ON_WASH.filter((pair) => limited.get(pair.fg) !== pair.min).map((pair) => pair.fg)).toEqual([]);
      });
    }

    const why = NO_WASH[id];
    if (why) {
      it(`${id} draws no backdrop wash: ${why}`, () => {
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

      it(`${id} holds the wash to the luminance its faint text allows`, () => {
        const preset = builtIns().find((p) => p.id === id);
        expect(preset, id).toBeDefined();
        const tokens = (preset as Preset).tokens;
        const band = faintBand(tokens);
        expect(band, id).not.toBeNull();
        const limit = washLook(tokens)?.limit;
        expect(Object.keys(limit ?? {})).toEqual(Object.keys(band as WashLimit));
        for (const side of ['max', 'min'] as const) {
          if (band?.[side] !== undefined) expect(limit?.[side], side).toBeCloseTo(band[side] as number, 6);
        }
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
      const colors = Object.fromEntries(
        CONTRAST_TOKENS.map((name) => [name, name in DERIVED_COLORS ? cssColor(DERIVED_COLORS[name], tokens) : rgba(tokens[name] ?? name)]),
      );
      expect(contrastReport(colors).filter((row) => row.ok !== true)).toEqual([]);
    });
  }

  // A theme of the user's own is checked only by the Settings panel: it must measure the same pairs.
  it('the readability panel measures the danger button Button.svelte draws, at rest and hovered', () => {
    const name = (value: string) => /^var\((--[\w-]+)\)$/.exec(value)?.[1] ?? value;
    const check = (id: string) => CONTRAST_CHECKS.find((c) => c.id === id);
    const rest = check('dangerButton');
    expect(rest).toBeDefined();
    expect(DERIVED_COLORS[rest?.fg ?? '']).toBe(DANGER.color);
    expect(rest?.backgrounds).toEqual([name(DANGER.background)]);
    const hover = check('dangerButtonHover');
    expect(hover).toBeDefined();
    expect(hover?.fg).toBe(name(DANGER_HOVER.color));
    expect(hover?.backgrounds).toEqual([name(DANGER_HOVER.background)]);
    for (const c of [rest, hover]) expect(c?.min).toBe(4.5);
  });

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
