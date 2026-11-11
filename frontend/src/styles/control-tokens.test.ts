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

// A toggle keeps its name and tells its state through aria-pressed. A button whose name flips with the
// state ("Watch" / "Watching") already says it: aria-pressed on it would announce the state twice and
// contradict the name ("Watching, pressed"), so it is drawn `lit` instead.
describe('a pressed button keeps its name', () => {
  const sources = fs
    .readdirSync(new URL('../', import.meta.url), { recursive: true })
    .map((path) => path.replaceAll('\\', '/'))
    .filter((path) => path.endsWith('.svelte'));

  function elements(text: string): { at: number; markup: string }[] {
    const found: { at: number; markup: string }[] = [];
    for (const m of text.matchAll(/<(IconButton|Button)\b/g)) {
      let depth = 0;
      let end = m.index;
      for (; end < text.length; end++) {
        const c = text[end];
        if (c === '{') depth++;
        else if (c === '}') depth--;
        else if (c === '>' && depth === 0) break;
      }
      const selfClosing = text[end - 1] === '/';
      const close = selfClosing ? end + 1 : text.indexOf(`</${m[1]}>`, end);
      found.push({ at: m.index, markup: text.slice(m.index, close < 0 ? end + 1 : close) });
    }
    return found;
  }

  const flips = (markup: string) =>
    /\?\s*msg\([^)]*\)\s*:\s*msg\(/.test(markup) || /\{#if [^}]*\}[\s\S]*?msg\([\s\S]*?\{:else\}[\s\S]*?msg\(/.test(markup);
  const pressed = (markup: string) => /(?<![\w-])pressed=/.test(markup);

  it('no button that is pressed changes its name with the state', () => {
    const found: string[] = [];
    for (const path of sources) {
      const text = read(path);
      for (const { at, markup } of elements(text)) {
        if (pressed(markup) && flips(markup)) found.push(`${path}:${text.slice(0, at).split('\n').length}`);
      }
    }
    expect(sources.length).toBeGreaterThan(200);
    expect(found).toEqual([]);
  });

  it('sees one when there is one, in a label or in the text', () => {
    const label = `<IconButton label={on ? msg('a.off') : msg('a.on')} pressed={on} onclick={() => (on = !on)}><X /></IconButton>`;
    const text = `<Button pressed={on} onclick={go}>{#if on}<BellRing />{msg('a.watching')}{:else}<Bell />{msg('a.watch')}{/if}</Button>`;
    const fixed = `<Button pressed={on} onclick={go}>{msg('a.mute')}</Button><IconButton label={msg('a.x')} lit={on} />`;
    expect(elements(label + text + fixed).map(({ markup }) => pressed(markup) && flips(markup))).toEqual([true, true, false, false]);
  });
});

describe('one spinner', () => {
  const sources = fs
    .readdirSync(new URL('../', import.meta.url), { recursive: true })
    .map((path) => path.replaceAll('\\', '/'))
    .filter((path) => path.endsWith('.svelte') || path.endsWith('.css'));
  // Big Picture and the overlay keep their own until their pass; the search field is rebuilt on its own branch.
  const LATER = /^lib\/bigpicture\/|^lib\/components\/BigPictureShell\.svelte$|^overlay\/|^lib\/components\/SearchInput\.svelte$/;
  const turning = (text: string) => [...text.matchAll(/@keyframes\s+([\w-]+)\s*\{\s*(?:from\s*\{[^}]*\}\s*)?to\s*\{\s*transform:\s*rotate\(360deg\);?\s*\}\s*\}/g)].map((m) => m[1]);

  // A turn that is not a "working" mark: the animated frame around an avatar.
  const DECORATION = ['lib/components/Avatar.svelte @keyframes frame-spin'];

  it('nothing but the Spinner turns a ring or an icon on its own keyframes', () => {
    const found = sources
      .filter((path) => path !== 'lib/components/Spinner.svelte' && !LATER.test(path))
      .flatMap((path) => turning(read(path)).map((name) => `${path} @keyframes ${name}`))
      .filter((use) => !DECORATION.includes(use));
    expect(turning(read('lib/components/Spinner.svelte'))).toEqual(['spinner-turn']);
    expect(found).toEqual([]);
  });

  it('sees a copied spinner when there is one', () => {
    expect(turning('@keyframes spin {\n  to {\n    transform: rotate(360deg);\n  }\n}')).toEqual(['spin']);
  });
});

// The theme editor's preview is a miniature drawn by hand, not the components themselves: it must
// take the same tokens for the same parts, or a theme author tunes a button that does not exist.
describe('the theme preview draws the button the app draws', () => {
  const rule = (css: string, selector: string) => {
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const body = new RegExp(`(?:^|\\})\\s*${escaped}\\s*\\{([^}]*)\\}`, 'm').exec(css)?.[1];
    expect(body, selector).toBeTypeOf('string');
    return Object.fromEntries(declarations(`{${body}`).map((d) => [d.prop, d.value]));
  };
  const button = style('lib/components/Button.svelte');
  const preview = style('routes/settings/ThemePreview.svelte');

  it('same radius and weight', () => {
    expect(rule(preview, '.btn')['border-radius']).toBe(rule(button, '.btn')['border-radius']);
    expect(rule(preview, '.btn')['font-weight']).toBe(rule(button, '.btn')['font-weight']);
  });

  it('same fill and label for the primary button, at rest and hovered', () => {
    expect(rule(preview, '.btn.primary')).toEqual(rule(button, '.primary'));
    expect(rule(preview, '.btn.primary.hover').background).toBe(rule(button, ".primary:hover:not(:disabled, [aria-disabled='true'])").background);
  });

  it('same surface, border and text for the secondary button', () => {
    const real = rule(button, '.secondary');
    const drawn = rule(preview, '.btn.secondary');
    expect(drawn.background).toBe(real.background);
    expect(drawn.color).toBe(real.color);
    expect(drawn.border).toBe(`1px solid ${real['border-color']}`);
  });
});

