import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

// The menu is a layer: Escape, presses outside, paint order and motion are the stack's and the shared
// stylesheet's, so the menu components keep no copy of their own.
const MENU = ['lib/components/DropdownMenu.svelte', 'lib/components/MenuList.svelte'];
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

  it('take their z-index from the layer token only, the trigger side none', () => {
    expect(style(source('lib/components/DropdownMenu.svelte'))).not.toMatch(/z-index/);
    const values = [...style(source('lib/components/MenuList.svelte')).matchAll(/z-index\s*:\s*([^;}]+)/g)].map((m) => m[1].trim());
    expect(values).toEqual(['var(--z-layer)']);
  });

  it('declare no keyframes: the menu enters with the shared popup-in from global.css', () => {
    for (const path of MENU) expect(style(source(path)), path).not.toMatch(/@keyframes/);
    expect(style(source('lib/components/MenuList.svelte'))).toMatch(/animation:\s*popup-in\b/);
    const global = fs.readFileSync(new URL('../../styles/global.css', import.meta.url), 'utf8');
    expect(global).toMatch(/@keyframes popup-in\s*\{/);
  });
});
