import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

// The top bar's search results are a layer: Escape, presses outside, paint order and motion are the
// stack's and the shared stylesheet's. The one window listener the search keeps is its shortcut.
const SEARCH = 'lib/components/TopbarSearch.svelte';
const FIELD = 'lib/components/SearchInput.svelte';
const TOPBAR = 'lib/components/Topbar.svelte';
const source = (path: string) => frontendSources[path] ?? '';
const style = (text: string) => [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
const zIndexes = (text: string) => [...style(text).matchAll(/(?<![-\w])z-index\s*:\s*([^;}]+)/g)].map((m) => m[1].trim());

describe('the top bar search and its results', () => {
  it('reads them', () => {
    for (const path of [SEARCH, FIELD, TOPBAR]) expect(source(path).length, path).toBeGreaterThan(0);
  });

  it('listens on the window for its shortcut only: no clickOutside, no Escape or press handling of its own', () => {
    for (const path of [SEARCH, FIELD]) {
      const text = source(path);
      expect(text, path).not.toMatch(/clickOutside/);
      expect(text, path).not.toMatch(/<svelte:(document|body)\b/);
      expect(text, path).not.toMatch(/(window|document)\.addEventListener\(/);
    }
    expect(source(FIELD)).not.toMatch(/<svelte:window\b/);
    expect([...source(SEARCH).matchAll(/<svelte:window\b[^>]*>/g)].map((m) => m[0])).toEqual(['<svelte:window onkeydown={onShortcut} />']);
  });

  it('stacks the results on the layer token, and the top bar on nothing', () => {
    expect(zIndexes(source(SEARCH))).toEqual(['var(--z-layer)']);
    expect(zIndexes(source(FIELD))).toEqual([]);
    expect(zIndexes(source(TOPBAR))).toEqual([]);
  });

  it('declares no keyframes of its own: the results enter with the shared popup-in', () => {
    expect(style(source(SEARCH))).not.toMatch(/@keyframes/);
    expect(style(source(SEARCH))).toMatch(/animation:\s*popup-in\b/);
  });

  it('puts the results on the stack as a non-modal layer owned by the island that takes no focus, placed by anchorTo', () => {
    const text = source(SEARCH);
    const attach = /\{@attach layer\(\{([^}]*)\}\)\}/.exec(text)?.[1] ?? '';
    expect(attach).toMatch(/modal: false/);
    expect(attach).toMatch(/owner:/);
    expect(attach).not.toMatch(/initialFocus/);
    expect(text).toMatch(/anchorTo\([^;]*matchWidth: true[^;]*onhide:/);
  });

  it('gives the field its look as a variant, not by reaching into it', () => {
    expect(source(SEARCH)).not.toMatch(/:global\(/);
  });
});
