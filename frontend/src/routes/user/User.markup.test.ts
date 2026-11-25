import { describe, expect, it } from 'vitest';
import { sourcesUnder } from '../../lib/testing/sources';

/**
 * The user page's markup and stylesheets, read from the source: jsdom lays nothing out, so what can be
 * held here is that the parts space themselves with the scale, write no native hint and hide nothing
 * by width (the header's container rule only lets its row wrap). The frame itself is for a real browser.
 */
const screens = Object.entries(sourcesUnder('routes/user/')).filter(([path]) => path.endsWith('.svelte'));
const style = (source: string) => (/<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
const markup = (source: string) => source.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');

// A step of the scale, or one taken back (a row's hover set flush with the card's edge).
const onScale = (part: string) => part === '0' || part === 'auto' || /^var\(--space-\d+\)$/.test(part) || /^calc\(-1 \* var\(--space-\d+\)\)$/.test(part);

function offScale(css: string): string[] {
  return [...css.matchAll(/(?:^|[;{\s])((?:gap|row-gap|column-gap|padding|margin)(?:-[a-z]+)?)\s*:\s*([^;}]+)/g)]
    .filter(([, , value]) => (value.trim().match(/calc\([^)]*\)\)?|\S+/g) ?? []).some((part) => !onScale(part)))
    .map(([, prop, value]) => `${prop}: ${value.trim()}`);
}

describe("another player's page", () => {
  it('reads every part', () => {
    expect(screens.map(([path]) => path)).toContain('routes/user/UserHeader.svelte');
    expect(screens.length).toBeGreaterThanOrEqual(10);
  });

  it('spaces its parts with the spacing scale only', () => {
    expect(offScale('.a { gap: 0.6rem; }')).toEqual(['gap: 0.6rem']);
    expect(offScale('.a { margin: 0 calc(-1 * var(--space-2)); padding: var(--space-1) var(--space-3); }')).toEqual([]);
    expect(screens.flatMap(([path, source]) => offScale(style(source)).map((rule) => `${path}: ${rule}`))).toEqual([]);
  });

  it('writes no native hint on an element', () => {
    expect(screens.filter(([, source]) => /<[a-z][\w-]*\b[^>]*\stitle=/.test(markup(source))).map(([path]) => path)).toEqual([]);
  });

  it('holds no rule by window width and hides nothing', () => {
    for (const [path, source] of screens) {
      expect(style(source), path).not.toMatch(/@media/);
      expect(style(source), path).not.toMatch(/display\s*:\s*none/);
    }
  });

  it("the header's container rule only lets its row wrap and its side follow", () => {
    const header = screens.find(([path]) => path === 'routes/user/UserHeader.svelte')?.[1] ?? '';
    const rule = /@container[^{]*\{([\s\S]*?\})\s*\}/.exec(style(header))?.[1] ?? '';
    const props = [...rule.matchAll(/([a-z-]+)\s*:/g)].map(([, prop]) => prop).sort();
    expect(props).toEqual(['align-items', 'flex-wrap', 'width']);
  });
});
