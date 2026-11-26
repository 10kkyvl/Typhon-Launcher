import { describe, expect, it } from 'vitest';
import { sourcesUnder } from '../../lib/testing/sources';

/**
 * The own profile's markup and stylesheets, read from the source: jsdom lays nothing out, so what can be
 * held here is that the parts space themselves with the scale, write no native hint, and that only the
 * page's own rearrangement of the editor holds a rule by window width, hiding nothing by it. The frame
 * itself is for a real browser.
 */
const screens = Object.entries(sourcesUnder('routes/profile/')).filter(([path]) => path.endsWith('.svelte'));
const style = (source: string) => (/<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
const markup = (source: string) => source.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');

// A step of the scale, or one taken back (a row's hover set flush with the card's edge).
const onScale = (part: string) => part === '0' || part === 'auto' || /^var\(--space-\d+\)$/.test(part) || /^calc\(-1 \* var\(--space-\d+\)\)$/.test(part);

function offScale(css: string): string[] {
  return [...css.matchAll(/(?:^|[;{\s])((?:gap|row-gap|column-gap|padding|margin|top)(?:-[a-z]+)?)\s*:\s*([^;}]+)/g)]
    .filter(([, , value]) => (value.trim().match(/calc\([^)]*\)\)?|\S+/g) ?? []).some((part) => !onScale(part)))
    .map(([, prop, value]) => `${prop}: ${value.trim()}`);
}

describe("the own profile's parts", () => {
  it('are all read', () => {
    expect(screens.map(([path]) => path)).toContain('routes/profile/ProfileAppearancePanel.svelte');
    expect(screens.length).toBeGreaterThanOrEqual(9);
  });

  it('space themselves with the spacing scale only', () => {
    expect(offScale('.a { gap: 0.6rem; top: 1.6rem; }')).toEqual(['gap: 0.6rem', 'top: 1.6rem']);
    expect(offScale('.a { margin: 0 calc(-1 * var(--space-2)); padding: var(--space-1) var(--space-3); }')).toEqual([]);
    expect(screens.flatMap(([path, source]) => offScale(style(source)).map((rule) => `${path}: ${rule}`))).toEqual([]);
  });

  it('write no native hint on an element', () => {
    expect(screens.filter(([, source]) => /<[a-z][\w-]*\b[^>]*\stitle=/.test(markup(source))).map(([path]) => path)).toEqual([]);
  });

  it('hold a rule by window width only for the editor beside the page, and hide nothing by it', () => {
    const media = screens.filter(([, source]) => /@media/.test(style(source))).map(([path]) => path);
    expect(media).toEqual(['routes/profile/Profile.svelte']);
    const rule = /@media[^{]*\{([\s\S]*)\}\s*$/.exec(style(screens.find(([path]) => path === 'routes/profile/Profile.svelte')?.[1] ?? ''))?.[1] ?? '';
    expect(rule).not.toMatch(/display\s*:\s*none|visibility\s*:\s*hidden/);
  });
});
