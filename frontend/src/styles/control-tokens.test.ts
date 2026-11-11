import { describe, expect, it } from 'vitest';
import { CONTRAST_FALLBACKS } from '../lib/theme/contrastReport';

/**
 * The base controls take every colour, radius, duration and layer from tokens: a theme then changes
 * them all at once, and none drifts from the others. No allow-list: a value a control needs and no
 * token holds becomes a token (styles/tokens.css for themes, styles/shell.css fixed).
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string; readdirSync(path: URL, options: { recursive: true }): string[] };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');

const CONTROLS = ['lib/components/Button.svelte', 'lib/components/IconButton.svelte', 'lib/components/Chip.svelte', 'lib/components/Spinner.svelte'];

function style(path: string): string {
  const source = read(path);
  const block = /<style>([\s\S]*)<\/style>/.exec(source)?.[1];
  expect(block, `${path} has a style block`).toBeTypeOf('string');
  return (block as string).replace(/\/\*[\s\S]*?\*\//g, '');
}

function declarations(css: string): { prop: string; value: string }[] {
  return [...css.matchAll(/(?:^|[;{])\s*([-\w]+)\s*:\s*([^;{}]+)/g)].map((m) => ({ prop: m[1], value: m[2].trim() }));
}

const COLOUR = /#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch)\(|\b(?:white|black)\b/i;
const RADIUS_OK = /^(?:0|50%|inherit|var\(--radius-[\w-]+\))$/;
const TIME = /(?<![\w.-])\d*\.?\d+m?s\b/;

describe('base controls take their look from tokens', () => {
  for (const path of CONTROLS) {
    const css = style(path);
    const all = declarations(css);

    it(`${path} writes no colour`, () => {
      expect(all.filter((d) => COLOUR.test(d.value)).map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
    });

    it(`${path} writes no radius`, () => {
      const radii = all.filter((d) => /radius$/.test(d.prop));
      expect(radii.filter((d) => !d.value.split(/\s+/).every((part) => RADIUS_OK.test(part))).map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
    });

    it(`${path} writes no duration`, () => {
      const timed = all.filter((d) => /^(?:transition|animation)/.test(d.prop));
      expect(timed.filter((d) => TIME.test(d.value)).map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
    });

    it(`${path} sets no layer`, () => {
      expect(all.filter((d) => d.prop === 'z-index').map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
    });
  }

  it('finds the rules it checks', () => {
    for (const path of CONTROLS) expect(declarations(style(path)).length, path).toBeGreaterThan(5);
  });

  it('measures the label on an accent fill with the colour the controls fall back to', () => {
    const fixed = /--accent-on-default:\s*([^;]+);/.exec(read('styles/shell.css'))?.[1].trim();
    expect(fixed).toBe(CONTRAST_FALLBACKS['--accent-on']);
  });
});

describe('a base control is restyled by a prop, never from outside', () => {
  const OWNERS: Record<string, string> = { btn: 'Button', 'icon-btn': 'IconButton', chip: 'Chip' };
  const sources = fs
    .readdirSync(new URL('../', import.meta.url), { recursive: true })
    .map((path) => path.replaceAll('\\', '/'))
    .filter((path) => path.endsWith('.svelte'));

  it('no component reaches into Button, IconButton or Chip through :global', () => {
    const found: string[] = [];
    for (const path of sources) {
      const text = read(path);
      for (const m of text.matchAll(/:global\(\s*[^)]*?\.(btn|icon-btn|chip)(?![\w-])/g)) {
        if (path === `lib/components/${OWNERS[m[1]]}.svelte`) continue;
        found.push(`${path}:${text.slice(0, m.index).split('\n').length} .${m[1]}`);
      }
    }
    expect(sources.length).toBeGreaterThan(200);
    expect(found).toEqual([]);
  });

  it('sees a reach-in when there is one', () => {
    const sample = '.lead :global(.btn) { min-width: 18rem; } .cta :global(.icon-btn:hover) {}';
    expect([...sample.matchAll(/:global\(\s*[^)]*?\.(btn|icon-btn|chip)(?![\w-])/g)].map((m) => m[1])).toEqual(['btn', 'icon-btn']);
  });
});

