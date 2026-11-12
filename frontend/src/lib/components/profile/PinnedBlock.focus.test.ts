import { describe, expect, it } from 'vitest';
import source from './PinnedBlock.svelte?raw';

/**
 * Where the pinned block's focus ring is painted. The whole block is one button (`.hero`) filled by
 * positioned layers (the artwork, the shade, the text), and in Chromium a ring drawn inside the button
 * is painted under them: it does not show. So the ring is the shared one (styles/global.css), drawn
 * outside the button, and the layers are clipped to the button so none of them reaches it; the block
 * around the button clips nothing, so the ring is not cut off either. A source tripwire: what Chromium
 * paints is checked in the live app.
 */
const css = (/<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
const rules = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => ({ selectors: m[1].split(',').map((s) => s.trim()), body: m[2] }));
const body = (selector: string) => rules.filter((rule) => rule.selectors.includes(selector)).map((rule) => rule.body).join(';');

describe('the pinned block focus ring', () => {
  it('is the shared ring: no rule of the block sets an outline', () => {
    expect(rules.filter((rule) => /(^|[;\s])outline(-[\w-]+)?\s*:/.test(rule.body)).map((rule) => rule.selectors.join(', '))).toEqual([]);
  });

  it('is painted outside the button, where the artwork clipped to the button cannot cover it', () => {
    expect(body('.hero')).toMatch(/overflow:\s*hidden/);
  });

  it('is not cut off by the block around the button', () => {
    expect(body('.pinned')).not.toMatch(/overflow(-[xy])?:\s*(hidden|clip)/);
  });
});
