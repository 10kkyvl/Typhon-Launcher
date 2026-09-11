import { rgb } from './accent';

export interface HSV { h: number; s: number; v: number }
export const clampUnit = (n: number) => Math.max(0, Math.min(1, n));

export function hexToHsv(color: string, previousHue = 0): HSV {
  const [r, g, b] = rgb(color).map(c => c / 255);
  const max = Math.max(r, g, b), min = Math.min(r, g, b), delta = max - min;
  let h = previousHue;
  if (delta > 0) {
    h = 60 * (max === r ? (g - b) / delta : max === g ? (b - r) / delta + 2 : (r - g) / delta + 4);
    h = (h + 360) % 360;
  }
  return { h, s: max === 0 ? 0 : delta / max, v: max };
}

export function hsvToHex({ h, s, v }: HSV): string {
  const hue = ((h % 360) + 360) % 360 / 60;
  const chroma = clampUnit(v) * clampUnit(s);
  const x = chroma * (1 - Math.abs(hue % 2 - 1));
  const m = clampUnit(v) - chroma;
  const channels = hue < 1 ? [chroma, x, 0] : hue < 2 ? [x, chroma, 0]
    : hue < 3 ? [0, chroma, x] : hue < 4 ? [0, x, chroma]
    : hue < 5 ? [x, 0, chroma] : [chroma, 0, x];
  return '#' + channels.map(c => Math.round((c + m) * 255).toString(16).padStart(2, '0')).join('').toUpperCase();
}
