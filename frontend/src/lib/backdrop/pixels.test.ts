import { describe, expect, it } from 'vitest';
import { luminance } from '../theme/accent';
import type { RGBA } from '../theme/contrastReport';
import { BACKDROP_LOOK_COLORS, WASH_TEXT, backdropLook, drawArt, lookPixels, mixPixels, washLimit, type BackdropLook } from './pixels';

type RGB = [number, number, number];
const px = (r: number, g: number, b: number, a = 255) => [r, g, b, a];

function ratio(a: RGB, b: RGB): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

const DARK: Record<string, RGBA> = {
  '--bg': [14, 15, 17, 1],
  '--text': [237, 238, 240, 1],
  '--text-2': [182, 185, 191, 1],
  '--text-3': [147, 151, 158, 1],
  '--accent-text': [139, 233, 247, 1],
  '--accent-bright': [44, 207, 232, 1],
  '--success': [85, 207, 141, 1],
  '--warning': [224, 168, 90, 1],
  '--danger': [239, 112, 112, 1],
};

const BLUSH: Record<string, RGBA> = {
  '--bg': [236, 217, 220, 1],
  '--text': [31, 26, 28, 1],
  '--text-2': [69, 55, 59, 1],
  '--text-3': [90, 74, 79, 1],
  '--accent-text': [105, 65, 78, 1],
  '--accent-bright': [95, 74, 77, 1],
  '--success': [22, 95, 56, 1],
  '--warning': [122, 76, 6, 1],
  '--danger': [168, 32, 41, 1],
};

// Every corner of the colour cube, a grey ramp and a noisy picture: the art can be anything.
function anyArt(): Uint8ClampedArray {
  const data: number[] = [];
  for (const r of [0, 255]) for (const g of [0, 255]) for (const b of [0, 255]) data.push(...px(r, g, b));
  for (let v = 0; v < 256; v += 5) data.push(...px(v, v, v));
  for (let i = 0; i < 4096 - 60; i++) data.push((i * 97) % 256, (i * 57) % 256, (i * 193) % 256, 255);
  return Uint8ClampedArray.from(data);
}

function worstRatio(look: BackdropLook, text: RGB): number {
  const data = anyArt();
  lookPixels(data, 64, 64, look);
  let worst = Infinity;
  for (let i = 0; i < data.length; i += 4) worst = Math.min(worst, ratio(text, [data[i], data[i + 1], data[i + 2]]));
  return worst;
}

const rgbOf = (c: RGBA): RGB => [c[0], c[1], c[2]];

describe('the look of the backdrop', () => {
  it('asks for the page and every text colour that sits on the wash', () => {
    expect([...BACKDROP_LOOK_COLORS].sort()).toEqual(Object.keys(DARK).sort());
  });

  it('keeps the wash dark under light text and light under dark text', () => {
    expect(backdropLook(DARK, 0.75, 0.6)?.limit).toEqual({ max: expect.any(Number) });
    expect(backdropLook(BLUSH, 0.75, 0.74)?.limit).toEqual({ min: expect.any(Number) });
  });

  it('limits luminance by the faintest of the text colours', () => {
    const look = backdropLook(DARK, 0.75, 0.6);
    const text3 = (luminance(rgbOf(DARK['--text-3'])) + 0.05) / 4.6 - 0.05;
    expect(look?.limit.max).toBeCloseTo(text3, 6);
    const dimmer = backdropLook({ ...DARK, '--accent-text': [130, 130, 130, 1] }, 0.75, 0.6);
    expect(dimmer?.limit.max).toBeLessThan(text3);
  });

  it('measures translucent text as painted over the page', () => {
    const half = backdropLook({ ...DARK, '--text-3': [255, 255, 255, 0.5] }, 0.75, 0.6);
    const shown: RGB = [255 * 0.5 + 14 * 0.5, 255 * 0.5 + 15 * 0.5, 255 * 0.5 + 17 * 0.5];
    const bound = (c: RGB, r: number) => (luminance(c) + 0.05) / r - 0.05;
    const expected = Math.min(
      ...WASH_TEXT.map((t) => bound(t.name === '--text-3' ? shown : rgbOf(DARK[t.name]), t.min + 0.1)),
    );
    expect(half?.limit.max).toBeCloseTo(expected, 6);
  });

  it('draws nothing when a colour cannot be read', () => {
    for (const name of BACKDROP_LOOK_COLORS) {
      const colors: Record<string, RGBA | undefined> = { ...DARK, [name]: undefined };
      expect(backdropLook(colors, 0.75, 0.6), name).toBeNull();
    }
  });

  it('dims toward the page colour itself, by the share the theme sets', () => {
    const look = backdropLook({ ...DARK, '--bg': [6, 17, 28, 1] }, 0.75, 0.6);
    expect(look?.page).toEqual([6, 17, 28]);
    expect(look?.dim).toBe(0.6);
  });

  it('draws nothing when the share is 1: no art would be left in it', () => {
    expect(backdropLook(DARK, 0.75, 1)).toBeNull();
  });

  it('draws nothing for a share it cannot read or outside 0..1', () => {
    for (const dim of [Number.NaN, -0.1, 1.5]) expect(backdropLook(DARK, 0.75, dim), String(dim)).toBeNull();
    expect(backdropLook(DARK, 0.75, 0)?.dim).toBe(0);
  });

  it('draws nothing when no luminance keeps every text readable', () => {
    expect(washLimit([{ color: [20, 20, 20], ratio: 4.6 }], true)).toBeNull();
    expect(washLimit([{ color: [240, 240, 240], ratio: 4.6 }], false)).toBeNull();
    expect(backdropLook({ ...DARK, '--text-3': [30, 30, 30, 1] }, 0.75, 0.6)).toBeNull();
  });

  it('keeps the saturation inside 0..1 and falls back to the art as is for a value it cannot read', () => {
    expect(backdropLook(DARK, 3, 0.6)?.saturate).toBe(1);
    expect(backdropLook(DARK, -1, 0.6)?.saturate).toBe(0);
    expect(backdropLook(DARK, Number.NaN, 0.6)?.saturate).toBe(1);
  });
});

describe('backdrop pixels', () => {
  it('mixes two pictures linearly and ends on each of them', () => {
    const from = px(0, 0, 0);
    const to = px(200, 100, 50);
    const out = [0, 0, 0, 0];
    mixPixels(from, to, 0, out);
    expect(out).toEqual([0, 0, 0, 255]);
    mixPixels(from, to, 1, out);
    expect(out).toEqual([200, 100, 50, 255]);
    mixPixels(from, to, 0.5, out);
    expect(out.map(Math.round)).toEqual([100, 50, 25, 255]);
  });

  it('fading in from no art keeps the colour (no pass through black) and fading out ends empty', () => {
    const none = px(0, 0, 0, 0);
    const art = px(200, 100, 50);
    const out = [0, 0, 0, 0];
    mixPixels(none, art, 0.25, out);
    expect(out.slice(0, 3).map(Math.round)).toEqual([200, 100, 50]);
    expect(Math.round(out[3])).toBe(64);
    mixPixels(art, none, 1, out);
    expect(out).toEqual([0, 0, 0, 0]);
  });

  it('keeps every text and status colour readable over any art on a dark base', () => {
    const look = backdropLook(DARK, 0.75, 0.6) as BackdropLook;
    for (const t of WASH_TEXT) expect(worstRatio(look, rgbOf(DARK[t.name])), t.name).toBeGreaterThanOrEqual(t.min);
  });

  it('keeps every text and status colour readable over any art on a light base', () => {
    const look = backdropLook(BLUSH, 0.7, 0.74) as BackdropLook;
    for (const t of WASH_TEXT) expect(worstRatio(look, rgbOf(BLUSH[t.name])), t.name).toBeGreaterThanOrEqual(t.min);
  });

  it('pulls white art down to the limit on a dark base and black art up to it on a light one', () => {
    const dark = backdropLook(DARK, 0.75, 0.6) as BackdropLook;
    const white = Uint8ClampedArray.from(px(255, 255, 255));
    lookPixels(white, 1, 1, dark);
    expect(luminance([white[0], white[1], white[2]])).toBeCloseTo(dark.limit.max as number, 3);
    const light = backdropLook(BLUSH, 0.7, 0.74) as BackdropLook;
    const black = Uint8ClampedArray.from(px(0, 0, 0));
    lookPixels(black, 1, 1, light);
    expect(luminance([black[0], black[1], black[2]])).toBeCloseTo(light.limit.min as number, 2);
  });

  it('pulls saturation down', () => {
    const data = px(250, 40, 40);
    lookPixels(data, 1, 1, { saturate: 0.5, page: [0, 0, 0], dim: 0, limit: {} });
    expect(data[0] - data[1]).toBeLessThan(250 - 40);
    const grey = px(250, 40, 40);
    lookPixels(grey, 1, 1, { saturate: 0, page: [0, 0, 0], dim: 0, limit: {} });
    expect(Math.round(grey[0])).toBe(Math.round(grey[1]));
  });

  it('pulls every pixel toward the page colour by the share', () => {
    const data = px(200, 200, 200);
    lookPixels(data, 1, 1, { saturate: 1, page: [0, 100, 0], dim: 0.5, limit: {} });
    expect(data.slice(0, 3).map(Math.round)).toEqual([100, 150, 100]);
    expect(data[3]).toBe(255);
  });

  it('softens a hard edge', () => {
    const data: number[] = [];
    for (let x = 0; x < 16; x++) data.push(...(x < 8 ? px(0, 0, 0) : px(255, 255, 255)));
    lookPixels(data, 16, 1, { saturate: 1, page: [0, 0, 0], dim: 0, limit: {} });
    expect(data[7 * 4]).toBeGreaterThan(0);
    expect(data[8 * 4]).toBeLessThan(255);
  });
});

describe('drawing the art into the picture', () => {
  it('lays the art over the page colour, so a transparent cover does not bake as black', () => {
    const calls: string[] = [];
    const ctx = {
      set fillStyle(value: string | CanvasGradient | CanvasPattern) {
        calls.push(`fill ${value}`);
      },
      fillRect: () => calls.push('fillRect'),
      drawImage: () => calls.push('drawImage'),
      imageSmoothingQuality: 'low' as ImageSmoothingQuality,
    };
    const look = backdropLook(DARK, 0.75, 0.6) as BackdropLook;
    drawArt(ctx, { naturalWidth: 200, naturalHeight: 100 } as HTMLImageElement, look);
    expect(calls).toEqual(['fill rgb(14, 15, 17)', 'fillRect', 'drawImage']);
  });
});
