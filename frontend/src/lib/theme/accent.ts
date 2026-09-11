// Personal accents are local settings, never part of an exported theme.
export const accentPresets = ['#6673F2', '#388BFF', '#16A6B6', '#36A66A', '#E4B836', '#ED873D', '#E45D87', '#AB70E5'];
export const validAccent = (value: string): boolean => /^#[0-9a-f]{6}$/i.test(value);
type RGB = [number, number, number];
export function rgb(value: string): RGB {
  const hex = value.trim().replace(/^#([\da-f])([\da-f])([\da-f])$/i, '#$1$1$2$2$3$3');
  if (validAccent(hex)) return [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)) as RGB;
  const channels = value.match(/^rgba?\(\s*([\d.]+)[, ]+([\d.]+)[, ]+([\d.]+)/);
  return channels ? channels.slice(1, 4).map(Number) as RGB : [17, 23, 31];
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
  const backgrounds = ['--bg', '--surface', '--surface-2', '--surface-3', '--surface-4', '--bg-sidebar'].map(k => tokens[k] || (light ? '#ffffff' : '#242c38'));
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
  const subtle = hex(mix(rgb(tokens['--surface'] || (light ? '#ffffff' : '#11171f')), main, .14));
  return {
    '--accent': hex(main), '--accent-hover': hex(hover), '--accent-on': foreground,
    '--accent-subtle': subtle,
    '--accent-text': hex(readable(seed, [...backgrounds, subtle], 4.5, light)),
    '--accent-ring': hex(readable(seed, backgrounds, 3, light)),
  };
}
