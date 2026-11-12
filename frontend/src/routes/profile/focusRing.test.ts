import { describe, expect, it } from 'vitest';
import { frontendSources } from '../../lib/testing/sources';

/**
 * The profile's screens draw no outline of their own on a control: the outline is the shared focus ring
 * (`:focus-visible` in styles/global.css), and a rule that sets one on a control beats it or moves it,
 * so keyboard focus on that control goes unseen. A chosen tile, a hover or an error is drawn with a
 * border, a shadow or a fill instead. A source tripwire: it reads the rules' selectors, not the DOM.
 */

const SCOPES = ['routes/profile/', 'lib/components/profile/', 'lib/components/reactions/'];

// Rules allowed an outline, with what they draw it on.
const ALLOWED: Record<string, string> = {
  // The drop target of a dragged block: a frame, which takes no focus.
  'lib/components/profile/BlockGrid.svelte .cell.over .frame': 'a block frame',
};

function outlineRules(path: string, source: string): string[] {
  const css = (/<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
  const out: string[] = [];
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    if (!/(^|[;\s])outline(-[\w-]+)?\s*:/.test(m[2])) continue;
    for (const selector of m[1].split(',')) out.push(`${path} ${selector.trim()}`);
  }
  return out;
}

describe('the profile screens', () => {
  it('set no outline on a control, so the shared focus ring always shows', () => {
    const found = Object.entries(frontendSources)
      .filter(([path]) => SCOPES.some((scope) => path.startsWith(scope)) && path.endsWith('.svelte'))
      .flatMap(([path, source]) => outlineRules(path, source));
    expect(found.length).toBeGreaterThan(0);
    expect(found.filter((rule) => !(rule in ALLOWED))).toEqual([]);
  });
});
