import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

/**
 * A notice with a retry must stay on screen while the retry runs (Notice.svelte): the page binds
 * `retrying` and shows the notice while it is true. A site that does not bind it is listed here with
 * why its notice cannot go during its own retry. A source tripwire: that the bound value is used in
 * the condition that shows the notice is the page's to get right, and Notice names a drop in the
 * development console.
 */
const NEVER_DROPPED: Record<string, string> = {
  'lib/components/OfflineBanner.svelte':
    'the shell shows it for as long as the session is offline, and the session stays offline until the check it retries has answered',
};

// Every opening <Notice …> tag of `text`, braces in attributes skipped.
function noticeTags(text: string): string[] {
  const tags: string[] = [];
  for (const start of text.matchAll(/<Notice\b/g)) {
    let depth = 0;
    let end = start.index;
    for (; end < text.length; end += 1) {
      const c = text[end];
      if (c === '{') depth += 1;
      else if (c === '}') depth -= 1;
      else if (c === '>' && depth === 0) break;
    }
    tags.push(text.slice(start.index, end + 1));
  }
  return tags;
}

const unbound = (text: string) => noticeTags(text).filter((tag) => /\sretry=/.test(tag) && !/\sbind:retrying\b/.test(tag));

describe('a notice with a retry is kept while the retry runs', () => {
  const sites = Object.entries(frontendSources).filter(
    ([path, text]) => path.endsWith('.svelte') && (path.startsWith('routes/') || path.startsWith('lib/components/')) && /<Notice\b[^]*?\sretry=/.test(text),
  );

  it('every site binds `retrying`, or is listed with why its notice cannot go meanwhile', () => {
    expect(sites.length).toBeGreaterThanOrEqual(4);
    const found = sites.filter(([path, text]) => !(path in NEVER_DROPPED) && unbound(text).length > 0).map(([path]) => path);
    expect(found).toEqual([]);
  });

  it('keeps the list true: each listed site still has a retry it does not bind', () => {
    for (const path of Object.keys(NEVER_DROPPED)) expect(unbound(frontendSources[path] ?? ''), path).toHaveLength(1);
  });

  it('sees a site that stops binding it', () => {
    const card = frontendSources['routes/settings/EmulatorsCard.svelte'];
    expect(unbound(card)).toEqual([]);
    const broken = card.replace(' bind:retrying', '');
    expect(broken).not.toBe(card);
    expect(unbound(broken)).toHaveLength(1);
    expect(unbound('<Notice tone="danger" retry={{ label: a, run: () => go({ b: 1 }) }}>x</Notice><Notice tone="info">y</Notice>')).toHaveLength(1);
  });
});
