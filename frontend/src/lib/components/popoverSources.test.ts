import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

// A popover is a layer: Escape, presses outside, paint order and motion are the stack's and the shared
// stylesheet's, so neither the base component nor what is built on it keeps a copy of its own.
const POPOVER = 'lib/components/Popover.svelte';
const BUILT_ON_IT = [
  POPOVER,
  'lib/components/NotificationsPanel.svelte',
  'lib/components/ActivityWidget.svelte',
  'lib/components/reactions/ReactionPicker.svelte',
  'lib/components/reactions/ReactionBar.svelte',
];
const source = (path: string) => frontendSources[path] ?? '';
const style = (text: string) => [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');

describe('popover and what is built on it', () => {
  it('reads them', () => {
    for (const path of BUILT_ON_IT) expect(source(path).length, path).toBeGreaterThan(0);
  });

  it('listen for no Escape or press of their own: no clickOutside, no window or document listener', () => {
    for (const path of BUILT_ON_IT) {
      const text = source(path);
      expect(text, path).not.toMatch(/clickOutside/);
      expect(text, path).not.toMatch(/<svelte:(window|document|body)\b/);
      expect(text, path).not.toMatch(/(window|document)\.addEventListener\(/);
    }
  });

  it('take their z-index from the layer token only, on the popover itself', () => {
    const values = [...style(source(POPOVER)).matchAll(/z-index\s*:\s*([^;}]+)/g)].map((m) => m[1].trim());
    expect(values).toEqual(['var(--z-layer)']);
    for (const path of BUILT_ON_IT.filter((path) => path !== POPOVER)) expect(style(source(path)), path).not.toMatch(/z-index/);
  });

  it('declare no keyframes: the popover enters with the shared popup-in', () => {
    for (const path of BUILT_ON_IT) expect(style(source(path)), path).not.toMatch(/@keyframes/);
    expect(style(source(POPOVER))).toMatch(/animation:\s*popup-in\b/);
  });

  it('puts the popover on the layer stack as a non-modal layer owned by its trigger, placed by anchorTo', () => {
    const text = source(POPOVER);
    const attach = /\{@attach layer\(\{([^}]*)\}\)\}/.exec(text)?.[1] ?? '';
    expect(attach).toMatch(/modal: false/);
    expect(attach).toMatch(/owner:/);
    expect(attach).toMatch(/initialFocus:/);
    expect(text).toMatch(/anchorTo\([^;]*onhide:/);
  });
});
