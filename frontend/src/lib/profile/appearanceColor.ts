import { contrast, rgb } from '../theme/accent';

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
