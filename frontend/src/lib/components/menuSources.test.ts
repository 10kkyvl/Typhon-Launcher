import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

// The menu is a layer: Escape, presses outside, paint order and motion are the stack's and the shared
// stylesheet's, so the menu components keep no copy of their own.
const MENU = ['lib/components/DropdownMenu.svelte', 'lib/components/ContextMenu.svelte', 'lib/components/MenuList.svelte'];
const CONTEXT_MENU = 'lib/components/ContextMenu.svelte';
const source = (path: string) => frontendSources[path] ?? '';
const style = (text: string) => [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');

describe('menu components', () => {
  it('reads them', () => {
    for (const path of MENU) expect(source(path).length, path).toBeGreaterThan(0);
  });

  it('listen for no Escape or press of their own: no clickOutside, no window or document listener', () => {
    for (const path of MENU) {
      const text = source(path);
      expect(text, path).not.toMatch(/clickOutside/);
      expect(text, path).not.toMatch(/<svelte:(window|document|body)\b/);
      expect(text, path).not.toMatch(/(window|document)\.addEventListener\(\s*['"](keydown|keyup|pointerdown|mousedown|click)['"]/);
    }
  });

  // A menu at a point closes on scroll, resize and window blur through placeAtPoint, the one place
  // every popup at a point gets that from; the context menu adds no listener of its own.
  it('the context menu keeps no item type, listener or stylesheet of its own: MenuList is its body', () => {
    const text = source(CONTEXT_MENU);
    expect(text).not.toMatch(/addEventListener|<svelte:window|<svelte:document|\bon(resize|blur|scroll)=/);
    expect(text).not.toMatch(/interface Item\b|type Item\b/);
    expect(text).toMatch(/<MenuList\b/);
    expect(text).toMatch(/placeAtPoint\([^)]*onleave/);
    expect(style(text)).toBe('');
  });

  it('take their z-index from the layer token only, the trigger side none', () => {
    expect(style(source('lib/components/DropdownMenu.svelte'))).not.toMatch(/z-index/);
    expect(style(source(CONTEXT_MENU))).not.toMatch(/z-index/);
    const values = [...style(source('lib/components/MenuList.svelte')).matchAll(/z-index\s*:\s*([^;}]+)/g)].map((m) => m[1].trim());
    expect(values).toEqual(['var(--z-layer)']);
  });

  it('declare no keyframes: the menu enters with the shared popup-in from global.css', () => {
    for (const path of MENU) expect(style(source(path)), path).not.toMatch(/@keyframes/);
    expect(style(source('lib/components/MenuList.svelte'))).toMatch(/animation:\s*popup-in\b/);
    const global = fs.readFileSync(new URL('../../styles/global.css', import.meta.url), 'utf8');
    expect(global).toMatch(/@keyframes popup-in\s*\{/);
  });

  // A popup settles from the side of its anchor: from above when below it, from below when above it,
  // from the left when on its right, from the right when on its left.
  it('enter from the anchor whichever side of it they open on', () => {
    const global = fs.readFileSync(new URL('../../styles/global.css', import.meta.url), 'utf8');
    const from = (side: string) => {
      const rule = new RegExp(`\\[data-placement\\^='${side}'\\]\\s*\\{([^}]*)\\}`).exec(global)?.[1] ?? '';
      const value = (name: string) => /-?[\d.]+rem|\b0\b/.exec(new RegExp(`--popup-from-${name}:\\s*([^;]+);`).exec(rule)?.[1] ?? '')?.[0];
      return { x: value('x'), y: value('y') };
    };
    expect(global).toMatch(/@keyframes popup-in\s*\{\s*from\s*\{[^}]*translate\(var\(--popup-from-x, 0\), var\(--popup-from-y, -0\.3rem\)\)/);
    expect(from('top')).toEqual({ x: undefined, y: '0.3rem' });
    expect(from('right')).toEqual({ x: '-0.3rem', y: '0' });
    expect(from('left')).toEqual({ x: '0.3rem', y: '0' });
  });
});

// The select's list is a layer like the menus: the stack answers Escape and presses outside, anchorTo
// places it, the layer token stacks it and the shared popup-in moves it in.
const SELECT = 'lib/components/Select.svelte';

describe('select', () => {
  it('listens for no Escape or press of its own: no clickOutside, no window or document listener', () => {
    const text = source(SELECT);
    expect(text.length).toBeGreaterThan(0);
    expect(text).not.toMatch(/clickOutside/);
    expect(text).not.toMatch(/<svelte:(window|document|body)\b/);
    expect(text).not.toMatch(/addEventListener/);
  });

  it('puts its list on the layer stack, owned by the field and taking no focus, placed by anchorTo as wide as the field', () => {
    const text = source(SELECT);
    const attach = /\{@attach layer\(\{([^}]*)\}\)\}/.exec(text)?.[1] ?? '';
    expect(attach).toMatch(/modal: false/);
    expect(attach).toMatch(/owner:/);
    expect(attach).not.toMatch(/initialFocus/);
    expect(text).toMatch(/anchorTo\([^;]*matchWidth: true/);
  });

  it('takes its z-index from the layer token only and declares no keyframes', () => {
    const css = style(source(SELECT));
    const values = [...css.matchAll(/z-index\s*:\s*([^;}]+)/g)].map((m) => m[1].trim());
    expect(values).toEqual(['var(--z-layer)']);
    expect(css).not.toMatch(/@keyframes/);
    expect(css).toMatch(/animation:\s*popup-in\b/);
  });
});
