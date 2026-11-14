import { describe, expect, it } from 'vitest';
import { GAME_PAGE_PARTS } from '../testing/gamePageParts';
import { frontendSources } from '../testing/sources';

/**
 * A notice with a retry must stay on screen while the retry runs (Notice.svelte): the page binds
 * `retrying` and shows the notice while it is true. A site that does not bind it is listed here with
 * why its notice cannot go during its own retry. A spread onto a notice may carry a retry, so it
 * counts as one unless listed. Every component of the frontend is read, Big Picture and the overlay
 * included. A source tripwire: that the bound value is used in the condition that shows the notice
 * is the page's to get right, and Notice names a drop in the development console.
 */
const NEVER_DROPPED: Record<string, string> = {
  'lib/components/OfflineBanner.svelte':
    'the shell shows it for as long as the session is offline, and the session stays offline until the check it retries has answered; the only other way it goes is the user closing it, which the notice knows',
};

/** The frontend's own components: everything but the test harnesses. */
const scanned = (path: string) => path.endsWith('.svelte') && !path.startsWith('lib/testing/');

interface Tag {
  /** The attributes outside braces and quotes, each expression reduced to a marker. */
  attrs: string;
  text: string;
}

// Every opening <Notice …> tag of `text`. A `>` inside braces or a quoted value does not end it; each
// expression becomes `{}`, a shorthand keeps its name (`{retry}`) and a spread is `{...}`.
function noticeTags(text: string): Tag[] {
  const tags: Tag[] = [];
  for (const start of text.matchAll(/<Notice\b/g)) {
    let depth = 0;
    let quote = '';
    let attrs = '';
    let expression = '';
    let end = start.index + start[0].length;
    for (; end < text.length; end += 1) {
      const c = text[end];
      if (depth === 0 && quote) {
        if (c === quote) quote = '';
        attrs += c === '>' ? ' ' : c;
      } else if (depth === 0 && (c === '"' || c === "'")) {
        quote = c;
        attrs += c;
      } else if (c === '{') {
        depth += 1;
        if (depth > 1) expression += c;
      } else if (c === '}') {
        depth -= 1;
        if (depth > 0) expression += c;
        else {
          const inner = expression.trim();
          attrs += inner.startsWith('...') ? '{...}' : /^\w+$/.test(inner) ? `{${inner}}` : '{}';
          expression = '';
        }
      } else if (depth > 0) expression += c;
      else if (c === '>') break;
      else attrs += c;
    }
    tags.push({ attrs, text: text.slice(start.index, end + 1) });
  }
  return tags;
}

const hasRetry = ({ attrs }: Tag) => /(?:^|\s)retry=|\{retry\}|\{\.\.\.\}/.test(attrs);
const binds = ({ attrs }: Tag) => /(?:^|\s)bind:retrying\b/.test(attrs);
const unbound = (text: string) => noticeTags(text).filter((tag) => hasRetry(tag) && !binds(tag));

describe('a notice with a retry is kept while the retry runs', () => {
  const sites = Object.entries(frontendSources).filter(([path, text]) => scanned(path) && noticeTags(text).some(hasRetry));

  it('every site binds `retrying`, or is listed with why its notice cannot go meanwhile', () => {
    expect(sites.length).toBeGreaterThanOrEqual(4);
    const found = sites.filter(([path, text]) => !(path in NEVER_DROPPED) && unbound(text).length > 0).map(([path]) => path);
    expect(found).toEqual([]);
  });

  it('keeps the list true: each listed site still has a retry it does not bind', () => {
    for (const path of Object.keys(NEVER_DROPPED)) expect(unbound(frontendSources[path] ?? ''), path).toHaveLength(1);
  });

  it('reads every component, Big Picture and the overlay included, and no test harness', () => {
    const gamePage = GAME_PAGE_PARTS.filter((path) => path.endsWith('.svelte'));
    expect(['lib/bigpicture/components/GamePage.svelte', 'overlay/Overlay.svelte', 'App.svelte', ...gamePage].every(scanned)).toBe(true);
    expect(scanned('lib/testing/RetryHarness.svelte')).toBe(false);
    expect(Object.keys(frontendSources).some((path) => path.startsWith('lib/bigpicture/') && scanned(path))).toBe(true);
  });

  it('sees a site that stops binding it', () => {
    const card = frontendSources['routes/settings/EmulatorsCard.svelte'];
    expect(unbound(card)).toEqual([]);
    const broken = card.replace(' bind:retrying', '');
    expect(broken).not.toBe(card);
    expect(unbound(broken)).toHaveLength(1);
  });

  it.each([
    ['a retry written out', '<Notice tone="danger" retry={{ label: a, run: () => go({ b: 1 }) }}>x</Notice>', 1],
    ['the shorthand', '<Notice tone="danger" {retry}>x</Notice>', 1],
    ['a spread that may carry one', '<Notice {...props}>x</Notice>', 1],
    ['a > in a quoted value before it', '<Notice title="1 > 0" retry={again}>x</Notice>', 1],
    ['a bound one, in any of those forms', '<Notice {retry} bind:retrying>x</Notice><Notice {...props} bind:retrying={busy}>y</Notice>', 0],
    ['none: a notice with no retry', '<Notice tone="info" title="a > b">y</Notice>', 0],
  ])('sees %s', (_, text, count) => {
    expect(unbound(text)).toHaveLength(count);
  });
});
