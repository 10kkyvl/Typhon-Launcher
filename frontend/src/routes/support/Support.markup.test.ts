import { describe, expect, it } from 'vitest';
import { sourcesUnder } from '../../lib/testing/sources';

/**
 * The support screens' markup and stylesheets, read from the source: jsdom lays nothing out, so what
 * can be held here is that the parts space themselves with the scale, write no native hint and keep
 * nothing from shrinking at the smallest window (1100 px). The frame itself is for a real browser.
 */
const screens = Object.entries(sourcesUnder('routes/support/')).filter(([path]) => path.endsWith('.svelte'));
const style = (source: string) => (/<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
const markup = (source: string) => source.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');

// A step of the scale, or one taken back (a ghost button set flush with the page's edge).
const onScale = (part: string) => part === '0' || part === 'auto' || /^var\(--space-\d+\)$/.test(part) || /^calc\(-1 \* var\(--space-\d+\)\)$/.test(part);

function offScale(css: string): string[] {
  return [...css.matchAll(/(?:^|[;{\s])((?:gap|row-gap|column-gap|padding|margin)(?:-[a-z]+)?)\s*:\s*([^;}]+)/g)]
    .filter(([, , value]) => (value.trim().match(/calc\([^)]*\)\)?|\S+/g) ?? []).some((part) => !onScale(part)))
    .map(([, prop, value]) => `${prop}: ${value.trim()}`);
}

describe('the support screens', () => {
  it('reads every screen', () => {
    expect(screens.map(([path]) => path)).toContain('routes/support/TicketView.svelte');
    expect(screens.length).toBeGreaterThanOrEqual(6);
  });

  it('space their parts with the spacing scale only', () => {
    expect(offScale('.a { gap: 0.6rem; }')).toEqual(['gap: 0.6rem']);
    expect(offScale('.a { margin: 0 0 var(--space-4) calc(-1 * var(--space-2)); padding: var(--space-1) var(--space-3); }')).toEqual([]);
    expect(screens.flatMap(([path, source]) => offScale(style(source)).map((rule) => `${path}: ${rule}`))).toEqual([]);
  });

  it('write no native hint on an element', () => {
    expect(screens.filter(([, source]) => /<[a-z][\w-]*\b[^>]*\stitle=/.test(markup(source))).map(([path]) => path)).toEqual([]);
  });

  // The unread poll (once a minute) and the refresh on window focus are lib/stores/support.ts's, which
  // App.svelte starts and stops; a screen reacts to the count and owns no clock or window listener.
  it('run no clock and listen to no window of their own', () => {
    const own = Object.entries(sourcesUnder('routes/support/'));
    expect(own.filter(([, source]) => /\bset(?:Interval|Timeout)\(|addEventListener\(|<svelte:window\b|\bon\(window\b/.test(source)).map(([path]) => path)).toEqual([]);
  });

  it('hold no rule by window width and hide nothing', () => {
    for (const [path, source] of screens) {
      expect(style(source), path).not.toMatch(/@media/);
      expect(style(source), path).not.toMatch(/display\s*:\s*none/);
    }
  });
});
