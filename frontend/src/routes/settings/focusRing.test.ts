import { describe, expect, it } from 'vitest';
import { frontendSources } from '../../lib/testing/sources';

/**
 * The settings page draws no outline of its own on a control: the outline is the shared focus ring
 * (`:focus-visible` in styles/global.css), and a rule of the page that sets one on a control beats it,
 * so keyboard focus on that control goes unseen. A chosen mark, a hover or an error is drawn with a
 * border, a shadow or a fill instead. A source tripwire: it reads the rules' selectors, not the DOM.
 */

// Rules allowed an outline, with what they draw it on.
const ALLOWED: Record<string, string> = {
  // The drop target of a dragged row: a list, which takes no focus.
  'routes/settings/LayoutEditor.svelte .list.drop-end': 'a list',
};

// ThemePreview is a picture of the theme, not controls.
const MOCK = 'routes/settings/ThemePreview.svelte';

function outlineRules(path: string, source: string): string[] {
  const css = path.endsWith('.css') ? source : (/<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '');
  const plain = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const out: string[] = [];
  for (const m of plain.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    if (!/(^|[;\s])outline(-[\w-]+)?\s*:/.test(m[2])) continue;
    for (const selector of m[1].split(',')) out.push(`${path} ${selector.trim()}`);
  }
  return out;
}

describe('the settings page', () => {
  it('sets no outline on a control, so the shared focus ring always shows', () => {
    const found = Object.entries(frontendSources)
      .filter(([path]) => path.startsWith('routes/settings/') && /\.(svelte|css)$/.test(path) && path !== MOCK)
      .flatMap(([path, source]) => outlineRules(path, source));
    expect(found.length).toBeGreaterThan(0);
    expect(found.filter((rule) => !(rule in ALLOWED))).toEqual([]);
  });
});
