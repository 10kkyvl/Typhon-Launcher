// Personal accents are local settings, never part of an exported theme.
export const accentPresets = ['#6673F2', '#388BFF', '#16A6B6', '#36A66A', '#E4B836', '#ED873D', '#E45D87', '#AB70E5'];
export const validAccent = (value: string): boolean => /^#[0-9a-f]{6}$/i.test(value);
type RGB = [number, number, number];
// Resolve translucent surfaces against the theme background before measuring contrast.
// Unknown CSS forms inherit that background instead of becoming an unrelated dark surface.
export function rgb(value: string, background: RGB = [17, 23, 31]): RGB {
  const text = value.trim().toLowerCase();
  let color: RGB;
  let alpha = 1;
  const hexMatch = /^#([\da-f]{3,4}|[\da-f]{6}|[\da-f]{8})$/.exec(text);
  if (hexMatch) {
    let digits = hexMatch[1];
    if (digits.length <= 4) digits = [...digits].map(c => c + c).join('');
    color = [0, 2, 4].map(i => parseInt(digits.slice(i, i + 2), 16)) as RGB;
    if (digits.length === 8) alpha = parseInt(digits.slice(6), 16) / 255;
  } else {
    if (text === 'transparent') return [...background];
    const match = /^rgba?\(\s*([^()]+)\s*\)$/.exec(text);
    if (!match) return [...background];
    const parts = match[1].trim().split(/[\s,/]+/);
    if (parts.length < 3 || parts.length > 4 || parts.some(p => !/^[+-]?(?:\d+\.?\d*|\.\d+)%?$/.test(p))) return [...background];
    color = parts.slice(0, 3).map(p => Math.max(0, Math.min(255, parseFloat(p) * (p.endsWith('%') ? 2.55 : 1)))) as RGB;
    if (parts[3]) alpha = Math.max(0, Math.min(1, parseFloat(parts[3]) / (parts[3].endsWith('%') ? 100 : 1)));
  }
  return color.map((channel, i) => channel * alpha + background[i] * (1 - alpha)) as RGB;
}

const hex = (c: RGB) => '#' + c.map(v => Math.round(v).toString(16).padStart(2, '0')).join('');
const mix = (a: RGB, b: RGB, t: number): RGB => a.map((v, i) => v + (b[i] - v) * t) as RGB;
function luminance(c: RGB): number {
  return c.map(v => { const n = v / 255; return n <= .04045 ? n / 12.92 : ((n + .055) / 1.055) ** 2.4; }).reduce((n, v, i) => n + v * [.2126, .7152, .0722][i], 0);
}
export function contrast(a: string, b: string): number {
  const x = luminance(rgb(a)), y = luminance(rgb(b));
  return (Math.max(x, y) + .05) / (Math.min(x, y) + .05);
}
function readable(seed: RGB, surfaces: string[], ratio: number, light: boolean): RGB {
  const end: RGB = light ? [0, 0, 0] : [255, 255, 255];
  for (let i = 0; i <= 100; i++) {
    const candidate = mix(seed, end, i / 100);
    if (surfaces.every(bg => contrast(hex(candidate), bg) >= ratio)) return candidate;
  }
  return end;
}
export function accentPalette(color: string, base: string, tokens: Record<string, string | undefined> = {}): Record<string, string> {
  if (!validAccent(color)) return {};
  const light = base === 'light';
  const fallback: RGB = light ? [255, 255, 255] : [36, 44, 56];
  const resolve = (value: string, seen = new Set<string>()): string => value.replace(/var\(\s*(--[\w-]+)\s*(?:,\s*([^()]+))?\)/g, (_, name: string, alternative: string) => {
    if (seen.has(name)) return alternative || '';
    return resolve(tokens[name] || alternative || '', new Set([...seen, name]));
  });
  const background = rgb(resolve(tokens['--bg'] || ''), fallback);
  const backgrounds = ['--bg', '--surface', '--surface-2', '--surface-3', '--surface-4', '--bg-sidebar'].map(k => hex(rgb(resolve(tokens[k] || ''), background)));
  const seed = rgb(color);
  const main = readable(seed, backgrounds, 3, light);
  const foreground = contrast(hex(main), '#ffffff') >= contrast(hex(main), '#000000') ? '#ffffff' : '#000000';
  // Keep the same foreground on hover, and retain the control's boundary contrast.
  const toward: RGB = light ? [0, 0, 0] : [255, 255, 255];
  let hover = main;
  for (let step = 12; step > 0; step--) {
    const candidate = mix(main, toward, step / 100);
    const value = hex(candidate);
    if (contrast(value, foreground) >= 4.5 && backgrounds.every(bg => contrast(value, bg) >= 3)) {
      hover = candidate;
      break;
    }
  }
  const subtle = hex(mix(rgb(backgrounds[1]), main, .14));
  return {
    '--accent': hex(main), '--accent-hover': hex(hover), '--accent-on': foreground,
    '--accent-subtle': subtle,
    '--accent-text': hex(readable(seed, [...backgrounds, subtle], 4.5, light)),
    '--accent-ring': hex(readable(seed, backgrounds, 3, light)),
  };
}
