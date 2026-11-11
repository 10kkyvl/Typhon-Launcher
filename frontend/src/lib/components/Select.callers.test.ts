import { describe, expect, it } from 'vitest';
import { balanced } from '../testing/scan';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use Select, read from their markup. With `onchange` the caller holds the value and
 * Select never writes it (Select.svelte), so a `bind:value` next to `onchange` would never be written:
 * such a caller sets its own state in `onchange`.
 *
 * Limit: a tag is read up to its first `>` outside braces; markup built in script is not seen.
 */

function tags(text: string, name: string): string[] {
  const found: string[] = [];
  for (const m of text.matchAll(new RegExp(`<${name}\\b`, 'g'))) {
    let at = (m.index ?? 0) + m[0].length;
    while (at < text.length && text[at] !== '>') {
      if (text[at] === '{') {
        const inner = balanced(text, at);
        if (inner === null) break;
        at += inner.length + 2;
      } else at += 1;
    }
    found.push(text.slice(m.index, at + 1));
  }
  return found;
}

function uses(name: string): { path: string; tag: string }[] {
  return Object.entries(frontendSources)
    .filter(([path]) => path.endsWith('.svelte'))
    .flatMap(([path, text]) => tags(text, name).map((tag) => ({ path, tag })));
}

describe('Select call sites', () => {
  it('reads them', () => {
    expect(uses('Select').length).toBeGreaterThanOrEqual(25);
  });

  // A screen reader announces an unnamed field as a bare combobox. A visible label next to the field
  // names it through `labelledby` (the same id expression is on an element of that file); `label` is for
  // a field with no visible label; RateLimitInput passes its caller's `labelledby` on.
  it('name every field: from the visible label next to it, or with label where none is shown', () => {
    const unnamed = [...uses('Select'), ...uses('RateLimitInput')].filter(({ path, tag }) => {
      if (/\blabel=\{msg\(/.test(tag)) return false;
      const by = /\blabelledby=("[^"]*"|\{[^}]*\})/.exec(tag)?.[1];
      if (!by) return !(path === 'lib/components/RateLimitInput.svelte' && /\{labelledby\}/.test(tag));
      return !(frontendSources[path] ?? '').includes(`id=${by}`);
    });
    expect(unnamed.map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  it('bind the value or hold it with onchange, never both', () => {
    const both = uses('Select').filter(({ tag }) => /\bbind:value\b/.test(tag) && /\bonchange=/.test(tag));
    expect(both.map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });
});
