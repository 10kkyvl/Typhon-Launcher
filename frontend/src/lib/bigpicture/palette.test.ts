import { describe, expect, it } from 'vitest';
import { tokenGroups } from '../theme/tokens';
import { accentPalette, contrast, rgb } from '../theme/accent';
import { bigPicturePalette, bigPictureStyle, bigPictureVars } from './palette';
import { goSources } from '../testing/sources';

const group = (...ids: string[]) => tokenGroups.filter((g) => ids.includes(g.id)).flatMap((g) => [...g.tokens]);
const neutral = group('background', 'surfaces', 'text');

interface Preset {
  id: string;
  base: string;
  tokens: Record<string, string>;
}

const presets: Preset[] = [...(goSources['internal/theme/presets.go'] ?? '').matchAll(/^var preset\w+ = Theme\{([\s\S]*?)^\}/gm)].map((m) => ({
  id: /\bID:\s*"([^"]+)"/.exec(m[1])?.[1] ?? '',
  base: /\bBase:\s*Base(\w+)/.exec(m[1])?.[1].toLowerCase() ?? '',
  tokens: Object.fromEntries([...m[1].matchAll(/"(--[\w-]+)":\s*"([^"]*)"/g)].map((t) => [t[1], t[2]])),
}));
const dark = presets.find((p) => p.id === 'dark') as Preset;

const SURFACES = ['--bg', '--bg-sidebar', '--surface', '--surface-2', '--surface-3', '--surface-4'];
const LOW = ['--bg', '--surface', '--surface-2'];

// What a Big Picture element sees: the theme's values from the root (a personal accent painted over
// them), then the shell's own inline values; components draw the label on a fill as `--accent-on, #fff`.
function insideBigPicture(theme: Preset, personal: string): Record<string, string> {
  const root = { ...theme.tokens, ...accentPalette(personal, theme.base, theme.tokens) };
  const seed = personal || theme.tokens['--accent'];
  return { '--accent-on': '#fff', ...root, ...bigPictureVars(seed) };
}

const toHex = (c: number[]) => '#' + c.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');

function unreadable(vars: Record<string, string>): string[] {
  const bg = rgb(vars['--bg'], [0, 0, 0]);
  const out: string[] = [];
  const pair = (fg: string, on: string, min: number) => {
    const under = on === '--bg' ? bg : rgb(vars[on], bg);
    const r = contrast(toHex(rgb(vars[fg], under)), toHex(under));
    if (r < min) out.push(`${fg} on ${on}: ${r.toFixed(2)} < ${min}`);
  };
  for (const s of SURFACES) for (const fg of ['--text', '--text-2', '--text-3']) pair(fg, s, 4.5);
  for (const s of LOW) for (const fg of ['--danger', '--success', '--warning', '--accent-text']) pair(fg, s, 4.5);
  for (const s of SURFACES) pair('--accent-bright', s, 3);
  for (const s of SURFACES) pair('--accent-ring', s, 3);
  for (const fill of ['--accent', '--accent-hover']) pair('--accent-on', fill, 4.5);
  return out;
}

describe('bigPicturePalette', () => {
  it('reads the built-in presets', () => {
    expect(presets.map((p) => p.id)).toEqual(['dark', 'light', 'contrast', 'blush']);
  });

  it('covers every neutral theme token', () => {
    expect(Object.keys(bigPicturePalette).sort()).toEqual([...neutral].sort());
  });

  it('takes its neutrals and status colours from the dark preset', () => {
    const vars = bigPictureVars('#123456');
    for (const name of [...neutral, ...group('status')]) expect(vars[name], name).toBe(dark.tokens[name]);
  });

  it('sets every accent token, so no light theme value reaches its dark surfaces', () => {
    for (const seed of ['#123456', dark.tokens['--accent'], 'var(--surface)', '']) {
      const vars = bigPictureVars(seed);
      for (const name of [...group('accent'), '--accent-on']) expect(vars[name], `${seed} ${name}`).toMatch(/^(#|rgba\()/);
    }
  });

  it('keeps the dark preset accent family for the dark accent and for a seed it cannot read', () => {
    for (const seed of [dark.tokens['--accent'], dark.tokens['--accent'].toUpperCase(), 'var(--surface)', 'oklch(60% 0.2 250)', '']) {
      const vars = bigPictureVars(seed);
      for (const name of group('accent')) expect(vars[name], `${seed} ${name}`).toBe(dark.tokens[name]);
    }
  });

  it('keeps the hue of any other accent', () => {
    const pink = accentPalette('#e45d87', 'dark', bigPicturePalette)['--accent'];
    expect(bigPictureVars('#E45D87')['--accent']).toBe(pink);
    expect(bigPictureVars('rgb(228, 93, 135)')['--accent']).toBe(pink);
  });

  for (const theme of presets) {
    for (const personal of ['', '#E45D87', '#F2E6B8', '#14202C', '#4F6BFF', '#000000', '#FFFFFF']) {
      it(`${theme.id}${personal ? ` with personal ${personal}` : ''}: every pair Big Picture draws is readable`, () => {
        expect(unreadable(insideBigPicture(theme, personal))).toEqual([]);
      });
    }
  }

  it('serialises every token into the inline style', () => {
    const vars = bigPictureVars('#E45D87');
    const style = bigPictureStyle('#E45D87');
    for (const [name, value] of Object.entries(vars)) expect(style).toContain(`${name}: ${value}`);
  });
});
