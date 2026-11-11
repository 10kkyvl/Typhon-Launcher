import { luminance } from '../theme/accent';
import type { RGBA } from '../theme/contrastReport';

/**
 * Pixel work for the window backdrop (ShellBackdrop). A game's art is reduced to a BACKDROP_W x
 * BACKDROP_H picture once, softened, desaturated and dimmed in those few thousand pixels; the screen
 * stretches it. The maths runs on flat RGBA arrays, so it is tested without a canvas.
 */

export const BACKDROP_W = 64;
export const BACKDROP_H = 36;

type RGB = [number, number, number];
export type Pixels = Uint8ClampedArray | number[];

export interface WashLimit {
  /** Highest relative luminance a pixel may reach (light text on a dark base). */
  max?: number;
  /** Lowest relative luminance a pixel may reach (dark text on a light base). */
  min?: number;
}

export interface BackdropLook {
  /** 1 keeps the art's colours, 0 makes it grey. */
  saturate: number;
  /** The page colour (`--bg`): every pixel is pulled toward it, and the art is laid over it. */
  page: RGB;
  /** How far every pixel is pulled toward the page, 0..1 (`--ambient-dim`). */
  dim: number;
  limit: WashLimit;
}

/**
 * Every colour the frontend paints as text or a marker straight on the page, which the wash is, and the
 * WCAG contrast it keeps there. The bake adds WASH_MARGIN for the rounding of the 8-bit picture.
 * The status colours are on the page only as marks, dots and badges: their text sits on a surface
 * (styles/status-surface.test.ts), so they keep the 3:1 of a marker and do not narrow the band.
 */
export const WASH_TEXT: readonly { name: string; min: number }[] = [
  { name: '--text', min: 4.5 },
  { name: '--text-2', min: 4.5 },
  { name: '--text-3', min: 4.5 },
  { name: '--accent-text', min: 4.5 },
  { name: '--accent-bright', min: 3 },
  { name: '--success', min: 3 },
  { name: '--warning', min: 3 },
  { name: '--danger', min: 3 },
];
const WASH_MARGIN = 0.1;
/** Fewer grey levels than this between the art's mid tones is a flat tint, not art: no wash is drawn. */
export const MIN_SPREAD = 8;

export const BACKDROP_LOOK_COLORS: readonly string[] = ['--bg', ...WASH_TEXT.map((t) => t.name)];

const over = (c: RGBA, below: RGB): RGB => [0, 1, 2].map((i) => c[i] * c[3] + below[i] * (1 - c[3])) as RGB;

const isLight = (bg: RGB) => {
  const l = luminance(bg);
  return 1.05 / (l + 0.05) < (l + 0.05) / 0.05;
};

/** The luminance band every text keeps its ratio over; null when no band does. */
export function washLimit(texts: readonly { color: RGB; ratio: number }[], dark: boolean): WashLimit | null {
  if (dark) {
    const max = Math.min(...texts.map((t) => (luminance(t.color) + 0.05) / t.ratio - 0.05));
    return max >= 0 ? { max } : null;
  }
  const min = Math.max(...texts.map((t) => t.ratio * (luminance(t.color) + 0.05) - 0.05));
  return min <= 1 ? { min } : null;
}

/**
 * The look for the theme on screen, from its resolved colours, the shell's saturation and the theme's
 * dim share. Null means no wash: a colour or the share could not be read, the share is 1 (nothing of
 * the art would show), no luminance keeps the text readable, the readable band leaves out the page
 * colour itself, or the art's mid tones would land fewer than MIN_SPREAD grey levels apart.
 */
export function backdropLook(colors: Readonly<Record<string, RGBA | undefined>>, saturate: number, dim: number): BackdropLook | null {
  const bgValue = colors['--bg'];
  if (!bgValue || !(dim >= 0 && dim < 1)) return null;
  const bg = over(bgValue, [0, 0, 0]);
  const texts: { color: RGB; ratio: number }[] = [];
  for (const text of WASH_TEXT) {
    const value = colors[text.name];
    if (!value) return null;
    texts.push({ color: over(value, bg), ratio: text.min + WASH_MARGIN });
  }
  const limit = washLimit(texts, !isLight(bg));
  if (!limit) return null;
  // A band that leaves out the page colour itself would lift (or sink) every pixel past the page.
  const pageL = luminance(bg);
  if ((limit.max !== undefined && limit.max < pageL) || (limit.min !== undefined && limit.min > pageL)) return null;
  const look = { saturate: Number.isNaN(saturate) ? 1 : Math.min(1, Math.max(0, saturate)), page: bg, dim, limit };
  return washSpread(look) < MIN_SPREAD ? null : look;
}

/** Separable box blur, `passes` times, radius `r`, edges clamped. In place. */
function boxBlur(px: Pixels, w: number, h: number, r: number, passes: number) {
  const tmp = new Array<number>(Math.max(w, h));
  const run = (len: number, stride: number, base: number) => {
    for (let c = 0; c < 3; c++) {
      for (let i = 0; i < len; i++) {
        let sum = 0;
        for (let k = -r; k <= r; k++) sum += px[base + Math.min(len - 1, Math.max(0, i + k)) * stride + c];
        tmp[i] = sum / (2 * r + 1);
      }
      for (let i = 0; i < len; i++) px[base + i * stride + c] = tmp[i];
    }
  };
  for (let p = 0; p < passes; p++) {
    for (let y = 0; y < h; y++) run(w, 4, y * w * 4);
    for (let x = 0; x < w; x++) run(h, w * 4, x * 4);
  }
}

const toLinear = (v: number) => {
  const s = v / 255;
  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
};
const toSrgb = (v: number) => 255 * (v <= 0.0031308 ? v * 12.92 : 1.055 * v ** (1 / 2.4) - 0.055);

/** Scales a too-bright pixel down, or mixes a too-dark one toward white, keeping its hue. */
function clampLuminance(px: Pixels, i: number, limit: WashLimit) {
  const l = luminance([px[i], px[i + 1], px[i + 2]]);
  if (limit.max !== undefined && l > limit.max) {
    // Luminance is linear in linear-light channels: scale there and convert back.
    const k = limit.max / l;
    for (let c = 0; c < 3; c++) px[i + c] = toSrgb(toLinear(px[i + c]) * k);
  } else if (limit.min !== undefined && l < limit.min) {
    // The mix toward white raises luminance monotonically; a dozen halvings land within a grey level.
    let lo = 0;
    let hi = 1;
    for (let n = 0; n < 12; n++) {
      const t = (lo + hi) / 2;
      const m = luminance([px[i] + (255 - px[i]) * t, px[i + 1] + (255 - px[i + 1]) * t, px[i + 2] + (255 - px[i + 2]) * t]);
      if (m < limit.min) lo = t;
      else hi = t;
    }
    for (let c = 0; c < 3; c++) px[i + c] += (255 - px[i + c]) * hi;
  }
}

/** Softens, desaturates, dims and luminance-limits an opaque picture in place. */
export function lookPixels(px: Pixels, w: number, h: number, look: BackdropLook) {
  boxBlur(px, w, h, 2, 2);
  for (let i = 0; i + 3 < px.length; i += 4) lookPixel(px, i, look);
}

/** Desaturates one pixel, pulls it toward the page by the dim share and limits its luminance. */
function lookPixel(px: Pixels, i: number, look: BackdropLook) {
  const [dr, dg, db] = look.page;
  const da = look.dim;
  const grey = 0.2126 * px[i] + 0.7152 * px[i + 1] + 0.0722 * px[i + 2];
  const r = grey + (px[i] - grey) * look.saturate;
  const g = grey + (px[i + 1] - grey) * look.saturate;
  const b = grey + (px[i + 2] - grey) * look.saturate;
  px[i] = r + (dr - r) * da;
  px[i + 1] = g + (dg - g) * da;
  px[i + 2] = b + (db - b) * da;
  px[i + 3] = 255;
  clampLuminance(px, i, look.limit);
}

/** Art between a quarter and three quarters grey: the tones most game art lives in. */
const MID_TONES = [64, 192] as const;

/** How many grey levels apart the look puts the mid tones of the art: the room the art has to show. */
export function washSpread(look: BackdropLook): number {
  const [low, high] = MID_TONES.map((v) => {
    const px = [v, v, v, 255];
    lookPixel(px, 0, look);
    return toSrgb(luminance([px[0], px[1], px[2]]));
  });
  return Math.abs(high - low);
}

/**
 * Mixes two pictures at `t` (0 = from, 1 = to) into `out`. Alpha is a weight: an empty pixel (no art)
 * brings no colour, so a fade from nothing to a picture does not pass through black.
 */
export function mixPixels(from: Pixels, to: Pixels, t: number, out: Pixels) {
  for (let i = 0; i + 3 < out.length; i += 4) {
    const fa = (from[i + 3] / 255) * (1 - t);
    const ta = (to[i + 3] / 255) * t;
    const a = fa + ta;
    if (a <= 0) {
      out[i] = out[i + 1] = out[i + 2] = out[i + 3] = 0;
      continue;
    }
    out[i] = (from[i] * fa + to[i] * ta) / a;
    out[i + 1] = (from[i + 1] * fa + to[i + 1] * ta) / a;
    out[i + 2] = (from[i + 2] * fa + to[i + 2] * ta) / a;
    out[i + 3] = a * 255;
  }
}

type ArtContext = Pick<CanvasRenderingContext2D, 'fillStyle' | 'fillRect' | 'drawImage' | 'imageSmoothingQuality'>;

/**
 * Cover-fits the art into BACKDROP_W x BACKDROP_H over the page colour, so the transparent parts of a
 * cover or logo show the page, as they would on screen, instead of baking as black.
 */
export function drawArt(ctx: ArtContext, img: HTMLImageElement, look: BackdropLook) {
  const [r, g, b] = look.page.map(Math.round);
  ctx.fillStyle = `rgb(${r}, ${g}, ${b})`;
  ctx.fillRect(0, 0, BACKDROP_W, BACKDROP_H);
  const scale = Math.max(BACKDROP_W / img.naturalWidth, BACKDROP_H / img.naturalHeight);
  const dw = img.naturalWidth * scale;
  const dh = img.naturalHeight * scale;
  ctx.imageSmoothingQuality = 'high';
  ctx.drawImage(img, (BACKDROP_W - dw) / 2, (BACKDROP_H - dh) / 2, dw, dh);
}

let bakeCanvas: HTMLCanvasElement | undefined;

/**
 * The picture for a loaded image, drawn by `drawArt` and given the look. Null when there is nothing to
 * read: an empty image, no 2D context, or a canvas the image tainted.
 */
export function bakeImage(img: HTMLImageElement, look: BackdropLook): Uint8ClampedArray | null {
  if (!img.complete || img.naturalWidth === 0 || img.naturalHeight === 0) return null;
  bakeCanvas ??= document.createElement('canvas');
  bakeCanvas.width = BACKDROP_W;
  bakeCanvas.height = BACKDROP_H;
  const ctx = bakeCanvas.getContext('2d');
  if (!ctx) return null;
  drawArt(ctx, img, look);
  let data: ImageData;
  try {
    data = ctx.getImageData(0, 0, BACKDROP_W, BACKDROP_H);
  } catch (err) {
    // A cross-origin image without CORS taints the canvas; the game simply gets no wash.
    console.warn('backdrop: art pixels cannot be read', err);
    return null;
  }
  lookPixels(data.data, BACKDROP_W, BACKDROP_H, look);
  return data.data;
}

/** Loads an image whose pixels may be read; null when it fails to load. */
export function loadImage(url: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image();
    if (/^https?:/i.test(url)) img.crossOrigin = 'anonymous';
    img.decoding = 'async';
    img.onload = () => resolve(img);
    img.onerror = () => resolve(null);
    img.src = url;
  });
}
