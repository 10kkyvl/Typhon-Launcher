import { describe, expect, it } from 'vitest';

/**
 * One scale of window layers: the `--z-*` tokens in shell.css (fixed, not themable). A `z-index` in any
 * stylesheet of the frontend (Svelte `<style>` blocks, `.css` files, `style:z-index` and inline
 * `style="z-index: …"` in markup) is one of:
 *
 * - `var(--z-<name>)` with the token defined in shell.css;
 * - `auto`, `0`, `1` or `-1` for stacking inside a component that isolates itself: the same rule sets
 *   `isolation: isolate` or a `--z-*` z-index (its own stacking context), or so does a rule of the
 *   component's root element: the first tag of the Svelte markup outside snippet definitions when it is an
 *   HTML element (not a component such as `<Page>`), matched by a selector made of
 *   its first static class with no combinator (`.avatar.framed` counts for `<span class="avatar …">`).
 *
 * Nothing else: no other integer, no file exceptions. Script does not write z-index (`style.zIndex`,
 * `setProperty('z-index', …)`, `cssText`, a quoted `z-index:`) with one named exception: the theme
 * guard's host (ThemeGuard.ts), pinned inline with `!important` above every layer on purpose, because a
 * theme's CSS can redefine any custom property and so could hide it if it took its place from this scale.
 */

type Fs = {
  readFileSync(path: URL, encoding: 'utf8'): string;
  readdirSync(path: URL, options: { recursive: true }): string[];
};
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

const SRC = new URL('../', import.meta.url);
const SHELL = 'styles/shell.css';
const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');
const files = fs
  .readdirSync(SRC, { recursive: true })
  .map((path) => path.replaceAll('\\', '/'))
  .filter((path) => /\.(svelte|css)$/.test(path));

// Bottom to top. Big Picture's inner layers live inside its own stacking context (.big-picture).
const ORDER = [
  'backdrop',
  'sticky',
  'popover',
  'tooltip',
  'menu',
  'corner',
  'bigpicture',
  'chat',
  'gate',
  'dialog',
  'toast',
  'titlebar',
  'update',
];
const BIG_PICTURE_ORDER = ['bp-menu', 'bp-keyboard', 'bp-dialog'];

const LOCAL = new Set(['auto', '0', '1', '-1']);
const TOKEN = /^var\(--z-([a-z-]+)\)$/;
const RULE = /([^{}]*)\{([^{}]*)\}/g;
const Z = /(?<![-\w])z-index\s*:\s*([^;}]+)/g;
const ISOLATES = /(?<![-\w])isolation\s*:\s*isolate|(?<![-\w])z-index\s*:\s*var\(--z-/;

function tokens(): Map<string, number> {
  const scale = new Map<string, number>();
  for (const m of read(SHELL).matchAll(/--z-([a-z-]+)\s*:\s*(-?\d+)\s*;/g)) scale.set(m[1], Number(m[2]));
  return scale;
}

function styles(path: string, text: string): string {
  if (path.endsWith('.css')) return text;
  return [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
}

function markup(text: string): string {
  return text
    .replace(/<script[\s\S]*?<\/script>/g, '')
    .replace(/<style[\s\S]*?<\/style>/g, '')
    .replace(/<!--[\s\S]*?-->/g, '');
}

function rootClass(path: string, text: string): string | null {
  if (!path.endsWith('.svelte')) return null;
  const outsideSnippets = markup(text).replace(/\{#snippet[\s\S]*?\{\/snippet\}/g, '');
  // The first tag, `svelte:` ones aside; a component tag (`<Page>`) means the root is not an element of this file.
  const first = /<(?!svelte:)([A-Za-z][\w.-]*)\b([^>]*)>/.exec(outsideSnippets);
  if (!first || !/^[a-z]/.test(first[1])) return null;
  const classes = /\bclass="([^"]*)"/.exec(first[2])?.[1];
  return classes?.split(/\s+/).find((name) => /^[\w-]+$/.test(name)) ?? null;
}

function selectsRoot(selector: string, root: string): boolean {
  return selector
    .split(',')
    .map((part) => part.trim())
    .some((part) => new RegExp(`^\\.${root}(?![\\w-])[^\\s>+~]*$`).test(part));
}

function violations(): string[] {
  const scale = tokens();
  const bad: string[] = [];
  for (const path of files) {
    const text = read(path);
    const css = styles(path, text);
    const rules = [...css.matchAll(RULE)].map((m) => ({ selector: m[1].trim().split('\n').pop()?.trim() ?? '', body: m[2] }));
    const root = rootClass(path, text);
    const rootIsolates = root !== null && rules.some((rule) => selectsRoot(rule.selector, root) && ISOLATES.test(rule.body));
    for (const rule of rules) {
      for (const m of rule.body.matchAll(Z)) {
        const value = m[1].trim().replace(/\s*!important$/, '');
        const token = TOKEN.exec(value);
        if (token && scale.has(token[1])) continue;
        if (LOCAL.has(value) && (ISOLATES.test(rule.body.replace(m[0], '')) || rootIsolates)) continue;
        bad.push(`${path} ${rule.selector}: ${value}`);
      }
    }
    if (path.endsWith('.svelte')) {
      const html = markup(text);
      for (const m of html.matchAll(/style:z-index=\{?["']?([^"'}\s>]+)/g)) bad.push(`${path} style:z-index: ${m[1]}`);
      for (const m of html.matchAll(/style="[^"]*z-index\s*:\s*([^;"]+)/g)) bad.push(`${path} style="z-index: ${m[1].trim()}"`);
    }
  }
  return bad;
}

describe('z-index scale', () => {
  it('reads the stylesheets', () => {
    expect(files.length).toBeGreaterThan(200);
    expect(files).toContain('lib/components/Modal.svelte');
    expect(files).toContain('lib/bigpicture/styles.css');
  });

  it('defines every layer once, in shell.css, in the documented order', () => {
    const scale = tokens();
    expect([...scale.keys()].filter((name) => !name.startsWith('bp-'))).toEqual(ORDER);
    expect([...scale.keys()].filter((name) => name.startsWith('bp-'))).toEqual(BIG_PICTURE_ORDER);
    for (const order of [ORDER, BIG_PICTURE_ORDER]) {
      const values = order.map((name) => scale.get(name) as number);
      expect(values, order.join(' < ')).toEqual([...values].sort((a, b) => a - b));
      expect(new Set(values).size, order.join(', ')).toBe(values.length);
    }
    const elsewhere = files.filter((path) => path !== SHELL && /(?<![-\w])--z-[a-z-]+\s*:/.test(styles(path, read(path))));
    expect(elsewhere).toEqual([]);
  });

  it('keeps the title bar above dialog scrims, dialogs (the image viewer among them) above Big Picture and the chat', () => {
    const scale = tokens();
    const z = (name: string) => scale.get(name) as number;
    expect(z('titlebar')).toBeGreaterThan(z('dialog'));
    expect(z('titlebar')).toBeGreaterThan(z('gate'));
    expect(z('dialog')).toBeGreaterThan(z('bigpicture'));
    expect(z('dialog')).toBeGreaterThan(z('chat'));
    expect(z('update')).toBeGreaterThan(z('titlebar'));
    expect(z('toast')).toBeGreaterThan(z('dialog'));
    expect(z('toast')).toBeLessThan(z('titlebar'));
    expect(z('toast')).toBeGreaterThan(z('corner'));
  });

  // Paint order of modal layers follows the stack: every component that registers a layer puts its
  // root on the one dialog token, and portal() appends each at the end of <body> when it opens.
  it('stacks every layer root on --z-dialog, so the layer opened last paints on top', () => {
    const layered = files.filter((path) => path.endsWith('.svelte') && /\{@attach layer\(/.test(read(path)));
    expect(layered.length).toBeGreaterThan(1);
    const tokensUsed = layered.map((path) => [path, [...styles(path, read(path)).matchAll(/z-index:\s*var\(--z-([a-z-]+)\)/g)].map((m) => m[1])]);
    expect(tokensUsed).toEqual(layered.map((path) => [path, ['dialog']]));
  });

  it('takes every z-index from the scale or stacks locally inside an isolated component', () => {
    expect(violations()).toEqual([]);
  });

  // Script can stack an element too. The only z-index written from script is the theme guard host's:
  // a theme's CSS can redefine any custom property, so that host pins a literal inline with !important
  // above every layer; the exception is named here and must stay true of that file.
  it('writes no z-index from script, except the theme guard host', () => {
    const scripts = fs
      .readdirSync(SRC, { recursive: true })
      .map((path) => path.replaceAll('\\', '/'))
      .filter((path) => /\.(ts|js|svelte)$/.test(path) && !/\.test\.ts$/.test(path));
    const writes = scripts.filter((path) => SCRIPT_Z.test(read(path)));
    expect(writes).toEqual([THEME_GUARD]);
    expect(read(THEME_GUARD)).toMatch(/setProperty\('z-index', '2147483647', 'important'\)/);
  });

  // An isolated root confines whatever it renders: a popover inside it (the beta badge's menu in the
  // sidebar) is painted under later page content where it sticks out. Limit: only components imported
  // by path are followed; a popover passed in as a snippet by the parent is not seen.
  it('does not isolate the root of a component that renders a popover, directly or through its children', () => {
    expect(isolatedPopoverHosts()).toEqual([]);
  });
});

const THEME_GUARD = 'lib/components/ThemeGuard.ts';
const SCRIPT_Z = /\.zIndex\b|setProperty\(\s*['"`]z-index|cssText[^\n]*z-index|['"`]z-index\s*:/;

const POPOVERS = ['DropdownMenu', 'Select', 'Tooltip', 'ContextMenu'];

function svelteImports(path: string, text: string): string[] {
  const dir = path.slice(0, path.lastIndexOf('/') + 1);
  return [...text.matchAll(/from\s+['"](\.{1,2}\/[^'"]+\.svelte)['"]/g)].map((m) => new URL(m[1], `file:///${dir}`).pathname.slice(1));
}

function isolatedPopoverHosts(): string[] {
  const svelte = files.filter((path) => path.endsWith('.svelte'));
  const imports = new Map(svelte.map((path) => [path, svelteImports(path, read(path))]));
  const reaches = (start: string): boolean => {
    const seen = new Set<string>();
    const queue = [start];
    while (queue.length) {
      const next = queue.pop() as string;
      for (const child of imports.get(next) ?? []) {
        if (POPOVERS.some((name) => child.endsWith(`/${name}.svelte`))) return true;
        if (!seen.has(child)) {
          seen.add(child);
          queue.push(child);
        }
      }
    }
    return false;
  };
  return svelte.filter((path) => {
    const text = read(path);
    const root = rootClass(path, text);
    if (!root) return false;
    const isolates = [...styles(path, text).matchAll(RULE)].some(
      (m) => selectsRoot(m[1].trim().split('\n').pop()?.trim() ?? '', root) && /(?<![-\w])isolation\s*:\s*isolate/.test(m[2]),
    );
    return isolates && reaches(path);
  });
}
