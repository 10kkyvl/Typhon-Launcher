import { compile } from 'svelte/compiler';
import { describe, expect, it } from 'vitest';
import { cssColor, declarations, ruleBody, styleOf, topLevel } from '../lib/testing/css';
import { CONTRAST_FALLBACKS, type RGBA } from '../lib/theme/contrastReport';

/**
 * The base controls take every colour, radius, duration and layer from tokens: a theme then changes
 * them all at once, and none drifts from the others. No allow-list: a value a control needs and no
 * token holds becomes a token (styles/tokens.css for themes, styles/shell.css fixed).
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string; readdirSync(path: URL, options: { recursive: true }): string[] };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');

const CONTROLS = ['lib/components/Button.svelte', 'lib/components/IconButton.svelte', 'lib/components/Chip.svelte', 'lib/components/Spinner.svelte'];

const style = (path: string) => styleOf(read(path), path);

const COLOUR = /#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch)\(|\b(?:white|black)\b/i;
const RADIUS_OK = /^(?:0|50%|inherit|var\(--radius-[\w-]+\))$/;
const TIME = /(?<![\w.-])\d*\.?\d+m?s\b/;

// A box dimension, a gap or a stroke written as a length. Transforms and shadows are motion and
// elevation, not sizes. The one exception is the in-game overlay: it keeps the controls it had before
// 2.0 (data-controls="1x"), and those rules pin the old values on purpose.
const SIZED = /^(?:(?:min-|max-)?(?:width|height)|margin(?:-[\w-]+)?|padding(?:-[\w-]+)?|(?:row-|column-)?gap|font-size|inset|top|right|bottom|left|flex-basis|border(?:-(?:top|right|bottom|left))?(?:-width)?|outline(?:-width|-offset)?)$/;
const LENGTH = /(?<![\w.-])-?\d*\.?\d+(?:rem|px|em|vh|vw|ch)\b/;
const OVERLAY = /^:global\(\[data-controls='1x'\]\)/;

function sizes(source: string, css: string): string[] {
  const icons = [...source.replace(/<style>[\s\S]*<\/style>/, '').matchAll(/\bsize="([^"]+)"/g)]
    .filter((m) => LENGTH.test(m[1]))
    .map((m) => `size="${m[1]}"`);
  const rules = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => ({ selector: m[1].trim(), body: m[2] }));
  const boxes = rules
    .filter((rule) => !OVERLAY.test(rule.selector))
    .flatMap((rule) => declarations(`{${rule.body}`).map((d) => ({ ...d, selector: rule.selector })))
    .filter((d) => SIZED.test(d.prop) && LENGTH.test(d.value))
    .map((d) => `${d.selector} ${d.prop}: ${d.value}`);
  return [...icons, ...boxes];
}

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

    it(`${path} writes no size`, () => {
      expect(sizes(read(path), css)).toEqual([]);
    });
  }

  it('sees a size in a box, a stroke or an icon, and lets the overlay keep the sizes it is pinned to', () => {
    const source = `<X size="1.2rem" /><Y size="sm" /><style>
      .a { width: 2.4rem; gap: var(--space-1); margin-left: 1px; }
      .b { border: 2px solid var(--hover-strong); padding: 0 var(--space-3); font-size: 1.3rem; }
      .c { transform: scale(0.9); border-radius: var(--radius-sm); }
      :global([data-controls='1x']) .a { width: 3.6rem; padding: 0 1.5rem; }
      @media (prefers-reduced-motion: reduce) { .d { height: 2rem; } }
    </style>`;
    expect(sizes(source, styleOf(source, 'sample'))).toEqual([
      'size="1.2rem"',
      '.a width: 2.4rem',
      '.a margin-left: 1px',
      '.b border: 2px solid var(--hover-strong)',
      '.b font-size: 1.3rem',
      '.d height: 2rem',
    ]);
  });

  // The launcher setting and the system request both turn motion off; a press or a hover then changes
  // colour only. Each rule comes after the one that moves the control, at the same specificity or more.
  it.each([
    ['lib/components/Button.svelte', ".btn:is(:hover, :active):not(:disabled, [aria-disabled='true'])", ".btn:active:not(:disabled, [aria-disabled='true'])"],
    ['lib/components/IconButton.svelte', '.icon-btn:active:not(:disabled)', '.icon-btn:active:not(:disabled)'],
    ['lib/components/Chip.svelte', 'button.chip:active', 'button.chip:active'],
  ])('%s moves nothing when animations are off', (path, still, moving) => {
    const css = style(path);
    const escaped = still.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const reduced = new RegExp(`@media \\(prefers-reduced-motion: reduce\\) \\{\\s*${escaped} \\{\\s*transform: none;\\s*\\}\\s*\\}`).exec(css);
    expect(reduced, `${path} under reduced motion`).not.toBeNull();
    expect(ruleBody(css, `:global(.no-anim) ${still}`)).toEqual({ transform: 'none' });
    const moves = css.indexOf(`${moving} {`);
    expect(moves).toBeGreaterThan(-1);
    expect((reduced as RegExpExecArray).index).toBeGreaterThan(moves);
    expect(css.indexOf(`:global(.no-anim) ${still} {`)).toBeGreaterThan(moves);
  });

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

  // Svelte gives every selector of a component its scope class, except what is written to be global,
  // in whichever form. The compiled CSS is read, not the source, so no form of :global slips by.
  const SCOPE = 'guard-scope';
  const scoped = new RegExp(`\\.${SCOPE}(?![\\w-])`);

  function compounds(selector: string): string[] {
    const out: string[] = [];
    let depth = 0;
    let part = '';
    for (const c of selector) {
      if (c === '(') depth++;
      else if (c === ')') depth--;
      if (depth === 0 && /[\s>+~]/.test(c)) {
        if (part) out.push(part);
        part = '';
      } else part += c;
    }
    return part ? [...out, part] : out;
  }

  // The base classes a component's styles reach without its own scope.
  function reachIns(component: string, filename = 'Sample.svelte'): string[] {
    if (!component.includes('<style')) return [];
    const css = compile(component, { filename, css: 'external', cssHash: () => SCOPE }).css?.code ?? '';
    const found: string[] = [];
    for (const m of css.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{};]+)\{/g)) {
      const prelude = m[1].trim();
      if (prelude.startsWith('@') || /^(?:from|to|[\d.]+%)\b/.test(prelude)) continue;
      for (const selector of topLevel(prelude)) {
        for (const compound of compounds(selector)) {
          if (scoped.test(compound)) continue;
          for (const c of compound.matchAll(/\.(btn|icon-btn|chip)(?![\w-])/g)) found.push(c[1]);
        }
      }
    }
    return found;
  }

  it('no component reaches into Button, IconButton or Chip through :global', () => {
    const found: string[] = [];
    for (const path of sources) {
      const text = read(path);
      for (const name of reachIns(text, path)) {
        if (path !== `lib/components/${OWNERS[name]}.svelte`) found.push(`${path} .${name}`);
      }
    }
    expect(sources.length).toBeGreaterThan(200);
    expect(found).toEqual([]);
  });

  // Every form Svelte 5 accepts for a selector outside the component's scope.
  it.each([
    ['a :global() selector', '.lead :global(.btn) { min-width: 18rem; }', ['btn']],
    ['one inside a nested rule', '.cta { :global(.icon-btn:hover) { color: var(--text); } }', ['icon-btn']],
    ['a :global block', ':global { .chip { color: var(--text); } }', ['chip']],
    ['a :global block under a scoped selector', '.lead :global { .btn { padding: 0; } }', ['btn']],
    ['a :global block nested in a rule', '.lead { :global { .btn, .icon-btn:hover { padding: 0; } } }', ['btn', 'icon-btn']],
    ['a bare :global before the class', '.lead :global .btn { padding: 0; }', ['btn']],
    ['a :global block in a media query', '@media (width > 1px) { :global { .chip { padding: 0; } } }', ['chip']],
    ['none: its own class of the same name', '.btn { padding: 0; }', []],
  ])('sees %s', (_, css, names) => {
    const markup = '<div class="lead cta"><span class="btn"></span></div>';
    expect(reachIns(`${markup}<style>${css}</style>`)).toEqual(names);
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

function darkPalette(): Record<string, string> {
  const css = read('styles/tokens.css');
  const body = css.slice(css.indexOf('{', css.indexOf(':root {')) + 1, css.indexOf('\n}', css.indexOf(':root {')));
  return Object.fromEntries([...body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
}

// The overlay keeps the danger button it had before 2.0 (Button.svelte, data-controls="1x"), whose
// border was the literal rgba(217, 105, 105, 0.3), 0.5 hovered. A literal ignored the theme; the
// tokens that replace it must draw the same border in the dark palette, to the rounding of a pixel.
describe('the overlay keeps its danger button', () => {
  const css = style('lib/components/Button.svelte');
  const dark = darkPalette();
  const near = (got: RGBA, want: RGBA) =>
    got.slice(0, 3).every((c, i) => Math.abs(c - want[i]) <= 0.5) && Math.abs(got[3] - want[3]) < 0.001;

  it('draws its border in the colour it had, at rest and hovered', () => {
    const rest = cssColor(ruleBody(css, ":global([data-controls='1x']) .danger")['border-color'], dark);
    const hover = cssColor(ruleBody(css, ":global([data-controls='1x']) .danger:hover:not(:disabled, [aria-disabled='true'])")['border-color'], dark);
    expect(near(rest, [217, 105, 105, 0.3]), `rest ${rest.map((c) => c.toFixed(2))}`).toBe(true);
    expect(near(hover, [217, 105, 105, 0.5]), `hover ${hover.map((c) => c.toFixed(2))}`).toBe(true);
  });

  it('computes a mix the way the browser does', () => {
    const tokens = { '--a': '#ff0000', '--b': '#0000ff', '--t': 'rgba(0, 0, 255, 0.5)' };
    expect(cssColor('color-mix(in srgb, var(--a) 25%, var(--b))', tokens)).toEqual([63.75, 0, 191.25, 1]);
    expect(cssColor('color-mix(in srgb, var(--a) 30%, transparent)', tokens)).toEqual([255, 0, 0, 0.3]);
    expect(cssColor('color-mix(in srgb, var(--a), var(--t))', tokens).map((c) => +c.toFixed(3))).toEqual([170, 0, 85, 0.75]);
  });
});

// The theme editor's preview is a miniature drawn by hand, not the components themselves: it must
// take the same tokens for the same parts, or a theme author tunes a button that does not exist.
describe('the theme preview draws the button the app draws', () => {
  const rule = ruleBody;
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

