import { describe, expect, it } from 'vitest';
import { balanced } from '../testing/scan';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use Toggle, read from their markup. With `onchange` the caller holds the state
 * and Toggle never writes it (Toggle.svelte), so a `bind:checked` next to `onchange` would never be
 * written: such a caller sets its own state in `onchange`.
 *
 * Limit: a tag is read up to its first `>` outside braces; markup built in script is not seen, and a
 * `labelledby` is checked only for an `id=` with the same expression somewhere in that file.
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

const uses = Object.entries(frontendSources)
  .filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'))
  .flatMap(([path, text]) => tags(text, 'Toggle').map((tag) => ({ path, tag })));

function bothWays(tag: string): boolean {
  return /\bbind:checked\b/.test(tag) && /(\bonchange=|\{\s*onchange\s*\})/.test(tag);
}

function spreads(tag: string): boolean {
  return /\{\s*\.\.\./.test(tag.replace(/=\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}/g, ''));
}

// The name comes from the visible label next to the switch (`labelledby`, with the same expression as
// an `id=` in that file); `label` is for a switch whose row shows no label of its own, so a `label`
// whose text the same file also shows as an element's content is a hidden copy and is refused.
function named(path: string, tag: string, text = frontendSources[path] ?? ''): boolean {
  const own = /\blabel=\{(msg\('[\w.]+'\))\}/.exec(tag)?.[1];
  if (own) return !text.includes(`>{${own}}<`);
  const by = /\blabelledby=("[^"]*"|\{[^}]*\})/.exec(tag)?.[1];
  return by !== undefined && text.includes(`id=${by}`);
}

describe('Toggle call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBeGreaterThanOrEqual(50);
  });

  it('name every switch: from the visible label next to it, or with label where none is shown', () => {
    expect(named('x.svelte', "<Toggle checked={on} label={msg('a.b')} />")).toBe(true);
    expect(named('x.svelte', "<Toggle checked={on} label={msg('a.b')} />", "<span>{msg('a.b')}</span>")).toBe(false);
    expect(named('x.svelte', '<Toggle checked={on} labelledby="row" />', '<span id="row">Sound</span>')).toBe(true);
    expect(named('x.svelte', '<Toggle checked={on} />')).toBe(false);
    expect(named('x.svelte', '<Toggle checked={on} labelledby="nowhere" />')).toBe(false);
    expect(uses.filter(({ path, tag }) => !named(path, tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  it('bind the state or hold it with onchange, never both', () => {
    expect(bothWays('<Toggle bind:checked={x} onchange={save} />')).toBe(true);
    expect(bothWays('<Toggle bind:checked {onchange} />')).toBe(true);
    expect(bothWays('<Toggle checked={x} {onchange} />')).toBe(false);
    expect(bothWays('<Toggle bind:checked={x} />')).toBe(false);
    expect(uses.filter(({ tag }) => bothWays(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  // A spread hides which of the two ways a caller took, and whether it names the switch.
  it('spread nothing onto a switch', () => {
    expect(spreads('<Toggle {...props} />')).toBe(true);
    expect(spreads('<Toggle checked={[...a].length > 0} label={msg("x")} />')).toBe(false);
    expect(uses.filter(({ tag }) => spreads(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });
});
