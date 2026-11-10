import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import Overlay from '../../overlay/Overlay.svelte';
import { bigPictureVars } from '../bigpicture/palette';
import { pinnedSizes } from './pinnedSizes';
import { tokenGroups } from './tokens';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string; readdirSync(path: URL, options: { recursive: true }): string[] };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const SRC = new URL('../../', import.meta.url);
const tree = (dir: string) => fs.readdirSync(new URL(dir, SRC), { recursive: true }).map((p) => `${dir}${p.replaceAll('\\', '/')}`);

// Big Picture and the in-game overlay keep the sizes the main window had before it took the redesign's scale.
const before: Record<string, string> = {
  '--radius-xs': '0.44rem',
  '--space-1': '0.44rem',
  '--space-2': '0.88rem',
  '--space-3': '1.32rem',
  '--space-4': '1.76rem',
  '--space-5': '2.2rem',
  '--space-6': '2.64rem',
  '--space-8': '3.52rem',
  '--space-10': '4.4rem',
  '--space-12': '5.28rem',
  '--page-x': '3.96rem',
  '--page-max': '255.2rem',
  '--prose-max': '83.6rem',
  '--sidebar-w': '25.52rem',
  '--island-gap': '0.88rem',
  '--topbar-h': '6.16rem',
  '--font-xs': '1.43rem',
  '--font-sm': '1.54rem',
  '--font-md': '1.65rem',
  '--font-lg': '1.87rem',
  '--font-xl': '2.09rem',
  '--font-title': '3.52rem',
  '--font-hero': 'clamp(3.96rem, 2.6vw, 4.84rem)',
  '--tracking-title': '-0.024em',
  '--tracking-heading': '-0.012em',
  '--control-sm': '3.52rem',
  '--control-md': '4.18rem',
  '--control-lg': '4.84rem',
};

const sizes = tokenGroups.find((g) => g.id === 'sizes')?.tokens ?? [];
const sharedRadii = new Set(['--radius-sm', '--radius-md', '--radius-lg', '--radius-xl', '--radius-control', '--cut']);

describe('sizes pinned in Big Picture and the overlay', () => {
  it('cover every size token their own sources read', () => {
    const files = [...tree('lib/bigpicture/'), 'lib/components/BigPictureShell.svelte', ...tree('overlay/')].filter(
      (p) => /\.(svelte|css|ts)$/.test(p) && !p.endsWith('.test.ts'),
    );
    const read = new Map<string, string>();
    for (const path of files) {
      const text = fs.readFileSync(new URL(path, SRC), 'utf8');
      for (const m of text.matchAll(/var\((--(?:font-|space-|control-|page-|prose-|sidebar-|topbar-|tracking-|radius-xs)[\w-]*)/g)) {
        if (sizes.includes(m[1])) read.set(m[1], path);
      }
    }
    expect(read.size).toBeGreaterThan(5);
    expect([...read].filter(([name]) => !(name in pinnedSizes)).map(([name, path]) => `${name} (${path})`)).toEqual([]);
  });

  it('cover every size token outside the shared radii, for the shared components inside them', () => {
    expect(Object.keys(pinnedSizes).sort()).toEqual(sizes.filter((name) => !sharedRadii.has(name)).sort());
  });

  it('hold the values from before the redesign scale', () => {
    expect(pinnedSizes).toEqual(before);
  });

  it('are painted inline on the Big Picture shell whatever the theme', () => {
    const vars = bigPictureVars({ shown: '#E45D87', theme: { base: 'light', tokens: { '--font-md': '3rem', '--space-2': '9rem' } }, personal: '' });
    for (const [name, value] of Object.entries(before)) expect(vars[name], name).toBe(value);
  });

  it('are set on the overlay root, which takes its text size from them', () => {
    const body = render(Overlay, { props: {} } as never).body;
    const root = /<div class="overlay[^"]*"[^>]*>/.exec(body)?.[0] ?? '';
    for (const [name, value] of Object.entries(before)) expect(root, name).toContain(`${name}: ${value}`);
    const source = fs.readFileSync(new URL('overlay/Overlay.svelte', SRC), 'utf8');
    const rule = /\.overlay\s*\{([^}]*)\}/.exec(source)?.[1] ?? '';
    expect(rule).toMatch(/font-size:\s*var\(--font-md\)/);
  });
});
