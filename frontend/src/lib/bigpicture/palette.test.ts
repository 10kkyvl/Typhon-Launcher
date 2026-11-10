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
  const shown = personal || theme.tokens['--accent'];
  return { '--accent-on': '#fff', ...root, ...bigPictureVars({ shown, theme, personal }) };
}

const seeded = (shown: string) => bigPictureVars({ shown, theme: null, personal: '' });
const family = (vars: Record<string, string>) => Object.fromEntries(group('accent').map((name) => [name, vars[name]]));

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
    const vars = seeded('#123456');
    for (const name of [...neutral, ...group('status')]) expect(vars[name], name).toBe(dark.tokens[name]);
  });

  it('sets every accent token, so no light theme value reaches its dark surfaces', () => {
    for (const seed of ['#123456', dark.tokens['--accent'], 'var(--surface)', '']) {
      const vars = seeded(seed);
      for (const name of [...group('accent'), '--accent-on']) expect(vars[name], `${seed} ${name}`).toMatch(/^(#|rgba\()/);
    }
  });

  it('keeps the dark preset accent family for the dark accent and for a seed it cannot read', () => {
    for (const seed of [dark.tokens['--accent'], dark.tokens['--accent'].toUpperCase(), 'var(--surface)', 'oklch(60% 0.2 250)', '']) {
      const vars = seeded(seed);
      for (const name of group('accent')) expect(vars[name], `${seed} ${name}`).toBe(dark.tokens[name]);
    }
  });

  it('keeps the hue of any other accent', () => {
    const pink = accentPalette('#e45d87', 'dark', bigPicturePalette)['--accent'];
    expect(seeded('#E45D87')['--accent']).toBe(pink);
    expect(seeded('rgb(228, 93, 135)')['--accent']).toBe(pink);
  });

  const contrastPreset = presets.find((p) => p.id === 'contrast') as Preset;
  const ownFamily = (tokens: Record<string, string>) => ({ ...family(tokens), '--accent-on': '#ffffff' });
  const darkFork = (accent: Record<string, string>) => ({ ...dark, id: 'fork', tokens: { ...dark.tokens, ...accent } });
  const rose = {
    '--accent': '#a3264f',
    '--accent-hover': '#8f2146',
    '--accent-subtle': 'rgba(255, 111, 163, 0.16)',
    '--accent-text': '#ff9cc0',
    '--accent-ring': 'rgba(255, 111, 163, 0.8)',
    '--accent-bright': '#ff6fa3',
  };

  it('keeps the own accent family of a dark theme that reads on its surfaces', () => {
    const vars = bigPictureVars({ shown: contrastPreset.tokens['--accent'], theme: contrastPreset, personal: '' });
    expect({ ...family(vars), '--accent-on': vars['--accent-on'] }).toEqual(ownFamily(contrastPreset.tokens));
    expect(contrast(vars['--accent'], vars['--accent-on'])).toBeGreaterThanOrEqual(4.8);
    const fork = darkFork(rose);
    expect(family(bigPictureVars({ shown: rose['--accent'], theme: fork, personal: '' }))).toEqual(family(fork.tokens));
  });

  it('derives the family when the theme is light, pale, incomplete or under a personal accent', () => {
    const pale = darkFork({ ...rose, '--accent': '#ffb3cf', '--accent-hover': '#ffc2d8' });
    const incomplete = darkFork(rose);
    delete incomplete.tokens['--accent-bright'];
    const blush = presets.find((p) => p.id === 'blush') as Preset;
    const light = presets.find((p) => p.id === 'light') as Preset;
    const derived = (seed: string) => family(accentPalette(seed, 'dark', bigPicturePalette));
    expect(family(bigPictureVars({ shown: '#ffb3cf', theme: pale, personal: '' }))).toEqual(derived('#ffb3cf'));
    expect(family(bigPictureVars({ shown: rose['--accent'], theme: incomplete, personal: '' }))).toEqual(derived(rose['--accent']));
    expect(family(bigPictureVars({ shown: blush.tokens['--accent'], theme: blush, personal: '' }))).toEqual(derived(blush.tokens['--accent']));
    expect(family(bigPictureVars({ shown: light.tokens['--accent'], theme: light, personal: '' }))).toEqual(family(dark.tokens));
    expect(family(bigPictureVars({ shown: '#E45D87', theme: contrastPreset, personal: '#E45D87' }))).toEqual(derived('#e45d87'));
  });

  for (const theme of presets) {
    for (const personal of ['', '#E45D87', '#F2E6B8', '#14202C', '#4F6BFF', '#000000', '#FFFFFF']) {
      it(`${theme.id}${personal ? ` with personal ${personal}` : ''}: every pair Big Picture draws is readable`, () => {
        expect(unreadable(insideBigPicture(theme, personal))).toEqual([]);
      });
    }
  }

  it('serialises every token into the inline style', () => {
    const source = { shown: '#E45D87', theme: null, personal: '' };
    const vars = bigPictureVars(source);
    const style = bigPictureStyle(source);
    for (const [name, value] of Object.entries(vars)) expect(style).toContain(`${name}: ${value}`);
  });
});

describe('labels on the Big Picture accent fill', () => {
  type Fs = { readFileSync(path: URL, encoding: 'utf8'): string; readdirSync(path: URL, options: { recursive: true }): string[] };
  const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
  const SRC = new URL('../../', import.meta.url);
  const files = [
    ...fs.readdirSync(new URL('lib/bigpicture/', SRC), { recursive: true }).map((p) => `lib/bigpicture/${p.replaceAll('\\', '/')}`),
    'lib/components/BigPictureShell.svelte',
  ].filter((p) => /\.(svelte|css)$/.test(p));

  interface Rule { where: string; selector: string; body: string }
  const rules: Rule[] = files.flatMap((path) => {
    const text = fs.readFileSync(new URL(path, SRC), 'utf8');
    return [...text.matchAll(/([^{};]+)\{([^{}]*)\}/g)].map((m) => ({
      where: `${path}:${text.slice(0, (m.index ?? 0) + m[1].length - m[1].trimStart().length).split('\n').length}`,
      selector: m[1].trim().replace(/\s+/g, ' '),
      body: m[2],
    }));
  });
  // A rule that only re-colours the fill for a state (`:hover`, `:active`) keeps the label of its base rule.
  const base = (selector: string) => selector.replace(/:(?:hover|active|focus-visible|not\([^)]*\))/g, '');
  const onFill = rules.filter((r) => /(?:^|;)\s*background(?:-color)?\s*:\s*var\(--accent(?:-hover)?\)\s*(?:;|$)/.test(r.body));
  const label = (r: Rule) => /(?:^|;)\s*color\s*:\s*([^;]+)/.exec(r.body)?.[1].trim();

  it('finds the fills to check', () => {
    expect(onFill.length).toBeGreaterThan(4);
  });

  // --accent-on is pinned inside Big Picture to the colour that reads on its fill; a literal white
  // does not once the fill is re-derived for a pale accent.
  it('draws every label and glyph on the accent fill in --accent-on', () => {
    const colourOf = (r: Rule): string | undefined => {
      const own = label(r);
      if (own !== undefined || base(r.selector) === r.selector) return own;
      const stateless = rules.find((b) => b.selector === base(r.selector) && label(b) !== undefined);
      return stateless ? label(stateless) : undefined;
    };
    const bad = onFill.filter((r) => !/^var\(--accent-on\b/.test(colourOf(r) ?? ''));
    expect(bad.map((r) => `${r.where} ${r.selector} { color: ${colourOf(r) ?? '(none)'} }`)).toEqual([]);
  });
});
