import { luminance } from '../theme/accent';
import type { RGBA } from '../theme/contrastReport';

/**
 * A game's own colour, read from its art and clamped into the theme (the 2.0 prototype's
 * `identity/artTone.ts`, decision В12): Play is filled with it, its label is white or ink, whichever
 * reads, the game page's tab marker takes its accent, and a progress bar and a speed graph draw its mark.
 * Every clamp is computed against the theme's own colours, so the high-contrast preset keeps the game's
 * colour at its own contrast.
 * Pure maths: `stores/gameTone.ts` reads the pictures and hands the result to the UI.
 */

export type RGB = [number, number, number];

export interface ToneTheme {
  /** Page background, #rrggbb. */
  bg: string;
  /** Primary text, #rrggbb. */
  text: string;
  /** The panel marks are drawn on (--surface-2), #rrggbb; without it the mark is held on --bg alone. */
  panel?: string;
  /** The progress track's tint (--hover-strong), laid over --bg and over the panel; alpha 0–1. */
  track?: RGBA;
}

export interface GameTone {
  /** Fill of the game's own controls (Play): >= 3:1 against --bg. */
  fill: string;
  /** Label on `fill`: white or ink, whichever reads; >= 4.5:1 on `fill`. */
  onFill: string;
  /** The game's colour as a mark on the page (a tab marker): >= 4.5:1 on --bg. */
  accent: string;
  /** A graphic in the game's colour (a progress fill, a graph's line): >= 3:1 on every ground of `markGrounds`. */
  mark: string;
}

const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
const hex2 = (n: number) => Math.round(Math.min(255, Math.max(0, n))).toString(16).padStart(2, '0');
export const toHex = ([r, g, b]: RGB) => `#${hex2(r)}${hex2(g)}${hex2(b)}`;

export function fromHex(hex: string): RGB {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
  if (!m) throw new Error(`not a #rrggbb colour: ${hex}`);
  return [0, 2, 4].map((i) => parseInt(m[1].slice(i, i + 2), 16)) as RGB;
}

/** WCAG contrast ratio of two colours. */
export function contrast(a: RGB, b: RGB): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

export function rgbToHsl([r, g, b]: RGB): [number, number, number] {
  const [R, G, B] = [r / 255, g / 255, b / 255];
  const max = Math.max(R, G, B);
  const min = Math.min(R, G, B);
  const l = (max + min) / 2;
  if (max === min) return [0, 0, l];
  const d = max - min;
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
  const h = max === R ? (G - B) / d + (G < B ? 6 : 0) : max === G ? (B - R) / d + 2 : (R - G) / d + 4;
  return [h * 60, s, l];
}

export function hslToRgb(h: number, s: number, l: number): RGB {
  const k = (n: number) => (n + h / 30) % 12;
  const a = s * Math.min(l, 1 - l);
  const f = (n: number) => l - a * Math.max(-1, Math.min(k(n) - 3, Math.min(9 - k(n), 1)));
  return [f(0) * 255, f(8) * 255, f(4) * 255];
}

const HUE_BINS = 24;

/**
 * The art's own colour from RGBA pixels: the most vivid hue family, not the mean of all of them. Pixels
 * vote into 15° hue bins, weighted by saturation squared and by how far they are from black and white,
 * so a dark sky does not win over the one coloured sun; the answer is the weighted mean of the winning
 * bin and its two neighbours. A plain mean of a gold emblem on teal fog is an olive that is in neither.
 * Null when the picture has no colour (flat grey, transparent).
 */
export function dominant(data: ArrayLike<number>): RGB | null {
  const w = new Float64Array(HUE_BINS);
  const r = new Float64Array(HUE_BINS);
  const g = new Float64Array(HUE_BINS);
  const b = new Float64Array(HUE_BINS);
  let total = 0;
  for (let i = 0; i + 3 < data.length; i += 4) {
    if (data[i + 3] < 128) continue;
    const px: RGB = [data[i], data[i + 1], data[i + 2]];
    const [h, s, l] = rgbToHsl(px);
    const weight = s * s * (1 - Math.abs(2 * l - 1)) ** 2;
    if (weight <= 0) continue;
    const bin = Math.min(HUE_BINS - 1, Math.floor((h / 360) * HUE_BINS));
    w[bin] += weight;
    r[bin] += px[0] * weight;
    g[bin] += px[1] * weight;
    b[bin] += px[2] * weight;
    total += weight;
  }
  if (total < 0.02) return null;
  const at = (i: number) => (i + HUE_BINS) % HUE_BINS;
  let best = 0;
  let bestScore = -1;
  for (let i = 0; i < HUE_BINS; i += 1) {
    const score = w[i] + 0.5 * (w[at(i - 1)] + w[at(i + 1)]);
    if (score > bestScore) {
      bestScore = score;
      best = i;
    }
  }
  const bins = [at(best - 1), best, at(best + 1)];
  const sum = bins.reduce((acc, i) => acc + w[i], 0);
  const out: RGB = [bins.reduce((acc, i) => acc + r[i], 0) / sum, bins.reduce((acc, i) => acc + g[i], 0) / sum, bins.reduce((acc, i) => acc + b[i], 0) / sum];
  return rgbToHsl(out)[1] < 0.04 ? null : out;
}

/** The colour's hue with its lightness moved until it holds 4.5:1 on --bg. */
export function clampTone(color: RGB, theme: ToneTheme): { accent: string } {
  const bg = fromHex(theme.bg);
  const text = fromHex(theme.text);
  const [h, s] = rgbToHsl(color);
  const dark = luminance(bg) < luminance(text);
  const sat = clamp01(Math.max(0.35, Math.min(s, 0.75)));
  let l = dark ? 0.62 : 0.34;
  let accent = hslToRgb(h, sat, l);
  while (contrast(fromHex(toHex(accent)), bg) < 4.5 && l > 0.02 && l < 0.98) {
    l += dark ? 0.02 : -0.02;
    accent = hslToRgb(h, sat, l);
  }
  return { accent: toHex(accent) };
}

const LIGHT_LABEL: RGB = [255, 255, 255];
const DARK_LABEL: RGB = [14, 15, 17];

/**
 * The game's colour as a control fill. Saturation is held in 0.38–0.86 and lightness searched outward
 * from a mid value until the fill keeps 3:1 against the page (a visible control) and one of the two
 * labels keeps 4.5:1 on it (AA text). Every hue has such a lightness, so the search lands; the label is
 * whichever of white and ink reads better.
 */
export function clampFill(color: RGB, theme: ToneTheme): { fill: string; onFill: string } {
  const bg = fromHex(theme.bg);
  const [h, s] = rgbToHsl(color);
  const sat = clamp01(Math.max(0.38, Math.min(s, 0.86)));
  const dark = luminance(bg) < 0.2;
  // Checked on the rounded hex the UI will paint, with a hair of margin.
  const ok = (raw: RGB) => {
    const rgb = fromHex(toHex(raw));
    return contrast(rgb, bg) >= 3.05 && Math.max(contrast(rgb, LIGHT_LABEL), contrast(rgb, DARK_LABEL)) >= 4.6;
  };
  const start = dark ? 0.5 : 0.42;
  let best: RGB | null = null;
  for (let step = 0; step <= 50 && !best; step += 1) {
    for (const l of [start + step * 0.01, start - step * 0.01]) {
      if (l <= 0 || l >= 1) continue;
      const rgb = hslToRgb(h, sat, l);
      if (ok(rgb)) {
        best = rgb;
        break;
      }
    }
  }
  const fill = fromHex(toHex(best ?? (dark ? [220, 222, 226] : [40, 42, 46])));
  const onFill = contrast(fill, LIGHT_LABEL) >= contrast(fill, DARK_LABEL) ? LIGHT_LABEL : DARK_LABEL;
  return { fill: toHex(fill), onFill: toHex(onFill) };
}

/** What a mark is drawn on: the page, the panel, and the progress track over each, as the window blends them. */
export function markGrounds(theme: ToneTheme): RGB[] {
  const page = fromHex(theme.bg);
  const under = theme.panel ? [page, fromHex(theme.panel)] : [page];
  const track = theme.track;
  if (!track) return under;
  const a = clamp01(track[3]);
  const over = (below: RGB) => fromHex(toHex(below.map((v, i) => track[i] * a + v * (1 - a)) as RGB));
  return [...under, ...under.map(over)];
}

/** WCAG 1.4.11: a graphic needs 3:1 against what it is next to. */
const MARK_MIN = 3;

/**
 * The game's colour as a graphic on any ground of `markGrounds`: the fill's saturation, lightness searched
 * outward from the fill's own until the mark keeps 3:1 (with a hair of margin) against every ground at once.
 * Lightness 0 and 1 are black and white, so if any colour holds, the search lands; the hue fades towards
 * the end it needs. Where none holds (a light panel on a black page), the end that comes nearest wins,
 * --text's end on a tie.
 */
export function clampMark(color: RGB, theme: ToneTheme): { mark: string } {
  const grounds = markGrounds(theme);
  const worst = (rgb: RGB) => Math.min(...grounds.map((g) => contrast(rgb, g)));
  const [h, s] = rgbToHsl(color);
  const sat = clamp01(Math.max(0.38, Math.min(s, 0.86)));
  const dark = luminance(fromHex(theme.bg)) < 0.2;
  const start = dark ? 0.5 : 0.42;
  for (let step = 0; step <= 100; step += 1) {
    for (const l of [start + step * 0.01, start - step * 0.01]) {
      if (l < 0 || l > 1) continue;
      const rgb = fromHex(toHex(hslToRgb(h, sat, l)));
      if (worst(rgb) >= MARK_MIN + 0.05) return { mark: toHex(rgb) };
    }
  }
  const text = fromHex(theme.text);
  const ends: RGB[] = luminance(text) >= 0.5 ? [[255, 255, 255], [0, 0, 0]] : [[0, 0, 0], [255, 255, 255]];
  return { mark: toHex(worst(ends[1]) > worst(ends[0]) ? ends[1] : ends[0]) };
}

/** Everything the UI paints in a game's colour, clamped into the current theme. */
export function gameToneOf(color: RGB, theme: ToneTheme): GameTone {
  return { ...clampTone(color, theme), ...clampFill(color, theme), ...clampMark(color, theme) };
}

/** A shade of the fill that moves away from what is drawn on it, so hover and press never cost its contrast. */
function shifted(fill: string, amount: number, on: string): string {
  const lightOn = fromHex(on).every((v) => v > 128);
  const to: RGB = lightOn ? [0, 0, 0] : [255, 255, 255];
  return toHex(fromHex(fill).map((v, i) => v + (to[i] - v) * amount) as RGB);
}

const WHITE = '#ffffff';
/** Non-text contrast (WCAG 1.4.11): what a glyph needs on its ground. */
const GLYPH_MIN = 3;
/** How strongly a hovered Play's glow shows its fill: the theme's ring is its bright accent at 0.75. */
const GLOW_ALPHA = 0.75;

/**
 * The glyph of an icon-only control in the game's colour (the round Play on a card). A glyph needs 3:1,
 * not a label's 4.5:1, so it stays white on the mid tones where a label has to turn ink.
 */
function glyphOn(tone: GameTone): string {
  return contrast(fromHex(tone.fill), fromHex(WHITE)) >= GLYPH_MIN ? WHITE : tone.onFill;
}

/**
 * The custom properties a game's subtree is painted with (stores/gameTone.ts sets them inline with the
 * `game-tone` class; shell.css points Play at them). Nothing for no game: the theme's accent stays.
 */
export function toneVars(tone: GameTone | null | undefined): Record<string, string> {
  if (!tone) return {};
  const glyph = glyphOn(tone);
  const [r, g, b] = fromHex(tone.fill);
  return {
    '--game-fill': tone.fill,
    '--game-fill-hover': shifted(tone.fill, 0.1, tone.onFill),
    '--game-fill-active': shifted(tone.fill, 0.2, tone.onFill),
    '--game-on-fill': tone.onFill,
    '--game-glow': `rgba(${r}, ${g}, ${b}, ${GLOW_ALPHA})`,
    '--game-glyph': glyph,
    '--game-disc-hover': shifted(tone.fill, 0.1, glyph),
    '--game-disc-active': shifted(tone.fill, 0.2, glyph),
    '--game-accent': tone.accent,
  };
}
