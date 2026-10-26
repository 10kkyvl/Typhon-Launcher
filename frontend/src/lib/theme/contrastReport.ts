import { luminance } from './accent';

export type RGBA = [number, number, number, number];

export interface ContrastCheck {
  id: string;
  fg: string;
  backgrounds: readonly string[];
  min: number;
}

// WCAG 2.2: 4.5:1 for body text, 3:1 for large or secondary text and for
// controls that carry meaning. The report warns, it never blocks a save:
// some authored themes go low on contrast on purpose.
export const CONTRAST_CHECKS: readonly ContrastCheck[] = [
  { id: 'text', fg: '--text', backgrounds: ['--bg', '--bg-sidebar', '--surface', '--surface-2', '--surface-3'], min: 4.5 },
  { id: 'text2', fg: '--text-2', backgrounds: ['--bg', '--bg-sidebar', '--surface', '--surface-2'], min: 4.5 },
  { id: 'text3', fg: '--text-3', backgrounds: ['--bg', '--surface', '--surface-2'], min: 3 },
  { id: 'button', fg: '--accent-on', backgrounds: ['--accent', '--accent-hover'], min: 4.5 },
  { id: 'links', fg: '--accent-text', backgrounds: ['--accent-subtle', '--surface', '--bg'], min: 4.5 },
  { id: 'accent', fg: '--accent', backgrounds: ['--bg', '--surface', '--surface-2'], min: 3 },
  { id: 'danger', fg: '--danger', backgrounds: ['--bg', '--surface'], min: 3 },
  { id: 'success', fg: '--success', backgrounds: ['--bg', '--surface'], min: 3 },
  { id: 'warning', fg: '--warning', backgrounds: ['--bg', '--surface'], min: 3 },
];

export const CONTRAST_TOKENS: readonly string[] = [...new Set(CONTRAST_CHECKS.flatMap((c) => [c.fg, ...c.backgrounds]))];

// Not theme tokens: values the window falls back to where it reads them.
// --accent-on comes from a personal accent; without one, buttons draw white.
export const CONTRAST_FALLBACKS: Readonly<Record<string, string>> = { '--accent-on': '#fff' };

export interface ContrastRow {
  id: string;
  min: number;
  // ratio and against describe the worst background; null when a colour of
  // the pair could not be read, so the row says "unknown" rather than pass.
  ratio: number | null;
  against: string | null;
  ok: boolean | null;
}

function over(top: RGBA, below: RGBA): RGBA {
  const a = top[3];
  return [0, 1, 2].map((i) => top[i] * a + below[i] * (1 - a)).concat(1) as RGBA;
}

function ratio(a: RGBA, b: RGBA): number {
  const x = luminance([a[0], a[1], a[2]]);
  const y = luminance([b[0], b[1], b[2]]);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

// Surfaces may be translucent, so each one is laid over --bg, and the
// foreground over that, the way the window paints them.
export function contrastReport(colors: Record<string, RGBA | undefined>): ContrastRow[] {
  const base = colors['--bg'];
  const opaque = (name: string): RGBA | undefined => {
    const c = colors[name];
    if (!c) return undefined;
    if (c[3] >= 1 || name === '--bg') return c;
    return base ? over(c, base) : undefined;
  };
  return CONTRAST_CHECKS.map((check) => {
    const fg = colors[check.fg];
    let worst: { ratio: number; against: string } | null = null;
    let unknown = !fg;
    for (const name of check.backgrounds) {
      const bg = opaque(name);
      if (!bg || !fg) {
        unknown = true;
        continue;
      }
      const r = ratio(over(fg, bg), bg);
      if (!worst || r < worst.ratio) worst = { ratio: r, against: name };
    }
    if (!worst) return { id: check.id, min: check.min, ratio: null, against: null, ok: null };
    const ok = worst.ratio >= check.min ? (unknown ? null : true) : false;
    return { id: check.id, min: check.min, ratio: worst.ratio, against: worst.against, ok };
  });
}

export function formatRatio(value: number, locale: string): string {
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1, minimumFractionDigits: 1 }).format(Math.floor(value * 10) / 10)}:1`;
}
