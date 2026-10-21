import { contrast, luminance, rgb } from '../theme/accent';

type RGB = [number, number, number];

export const PROFILE_TEXT_3 = '#99a5b5';
const READABLE = 4.6;

export function toHex(color: RGB): string {
  return '#' + color.map((v) => Math.round(Math.max(0, Math.min(255, v))).toString(16).padStart(2, '0')).join('');
}

export function mixHex(a: string, b: string, t: number): string {
  const from = rgb(a), to = rgb(b);
  return toHex(from.map((v, i) => v + (to[i] - v) * t) as RGB);
}

export function darkenUntil(color: string, against: string, ratio: number): string {
  for (let step = 0; step <= 100; step++) {
    const candidate = mixHex(color, '#000000', step / 100);
    if (contrast(candidate, against) >= ratio) return candidate;
  }
  return '#000000';
}

export function deriveSurfaces(from: string, to: string): { background: string; surface: string } {
  const middle = mixHex(from, to, 0.5);
  const background = darkenUntil(mixHex(middle, '#000000', 0.62), PROFILE_TEXT_3, READABLE);
  const surface = darkenUntil(mixHex(background, middle, 0.22), PROFILE_TEXT_3, READABLE);
  return { background, surface };
}

export interface CanvasInk { text: string; text2: string; text3: string; border: string; borderStrong: string }

export const LIGHT_INK: CanvasInk = {
  text: '#f1f4f8', text2: '#bec7d2', text3: PROFILE_TEXT_3,
  border: 'rgba(200, 220, 240, .12)', borderStrong: 'rgba(200, 220, 240, .23)',
};
export const DARK_INK: CanvasInk = {
  text: '#121821', text2: '#2b3542', text3: '#4b5665',
  border: 'rgba(18, 24, 33, .14)', borderStrong: 'rgba(18, 24, 33, .26)',
};

export function luminanceOf(color: string): number {
  return luminance(rgb(color));
}

export function inkFor(background: string): CanvasInk {
  return contrast(background, DARK_INK.text) > contrast(background, LIGHT_INK.text) ? DARK_INK : LIGHT_INK;
}

export function pureSurfaces(from: string, to: string): { background: string; surface: string; ink: CanvasInk } {
  const background = mixHex(from, to, 0.5);
  const ink = inkFor(background);
  const towards = ink === DARK_INK || luminanceOf(background) < 0.02 ? '#ffffff' : '#000000';
  return { background, surface: mixHex(background, towards, ink === DARK_INK ? 0.35 : 0.08), ink };
}

export function nameGradientEnd(color: string): string {
  return luminanceOf(color) > 0.5 ? mixHex(color, '#000000', 0.45) : mixHex(color, '#ffffff', 0.5);
}
