import { describe, expect, it } from 'vitest';
import { balanced } from '../testing/scan';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use Toggle, read from their markup. With `onchange` the caller holds the state
 * and Toggle never writes it (Toggle.svelte), so a `bind:checked` next to `onchange` would never be
 * written: such a caller sets its own state in `onchange`.
 *
 * Limit: the markup is read as tags and `{...}` expressions; markup built in script is not seen, and a
 * `labelledby` is checked only for an `id=` with the same expression inside the element the switch
 * sits in.
 */

interface Element {
  name: string;
  start: number;
  end: number;
}

const VOID = new Set(['area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'source', 'track', 'wbr']);

// The end of the tag that opens at `start` (the index after its `>`), past braces and quoted values.
function tagEnd(text: string, start: number): number {
  let at = start + 1;
  let quote = '';
  while (at < text.length) {
    const ch = text[at];
    if (ch === '{') {
      const inner = balanced(text, at);
      if (inner === null) return text.length;
      at += inner.length + 2;
      continue;
    }
    if (quote) {
      if (ch === quote) quote = '';
    } else if (ch === '"' || ch === "'") quote = ch;
    else if (ch === '>') return at + 1;
    at += 1;
  }
  return text.length;
}

// Every element of a component's markup, spanning from its opening tag to its closing one.
function elements(text: string): Element[] {
  const found: Element[] = [];
  const open: { name: string; start: number }[] = [];
  let at = 0;
  while (at < text.length) {
    const rest = text.slice(at, at + 80);
    const raw = /^<(script|style)\b/.exec(rest)?.[1];
    if (raw) {
      const close = text.indexOf(`</${raw}>`, at);
      at = close < 0 ? text.length : close + raw.length + 3;
    } else if (rest.startsWith('<!--')) {
      const close = text.indexOf('-->', at);
      at = close < 0 ? text.length : close + 3;
    } else if (text[at] === '{') {
      const inner = balanced(text, at);
      at += inner === null ? 1 : inner.length + 2;
    } else if (rest.startsWith('</')) {
      const close = /^<\/([\w:.-]+)\s*>/.exec(rest);
      const index = close ? open.map((element) => element.name).lastIndexOf(close[1]) : -1;
      if (close && index >= 0) {
        const [element] = open.splice(index);
        found.push({ name: element.name, start: element.start, end: at + close[0].length });
      }
      at += close ? close[0].length : 2;
    } else if (/^<[A-Za-z]/.test(rest)) {
      const name = /^<([\w:.-]+)/.exec(rest)?.[1] ?? '';
      const end = tagEnd(text, at);
      if (text[end - 2] === '/' || VOID.has(name.toLowerCase())) found.push({ name, start: at, end });
      else open.push({ name, start: at });
      at = end;
    } else at += 1;
  }
  return found;
}

interface Use {
  path: string;
  tag: string;
  /** The markup of the element the switch sits in: its row. */
  row: string;
}

function toggles(path: string, text: string): Use[] {
  const all = elements(text);
  return all
    .filter((element) => element.name === 'Toggle')
    .map((toggle) => {
      const parent = all
        .filter((element) => element !== toggle && element.start < toggle.start && element.end >= toggle.end)
        .sort((a, b) => b.start - a.start)[0];
      return { path, tag: text.slice(toggle.start, toggle.end), row: parent ? text.slice(parent.start, parent.end) : text };
    });
}

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'));
const uses = callers.flatMap(([path, text]) => toggles(path, text));

function bothWays(tag: string): boolean {
  return /\bbind:checked\b/.test(tag) && /(\bonchange=|\{\s*onchange\s*\})/.test(tag);
}

// A switch with neither does nothing when pressed: nothing reads what it would write.
function wired(tag: string): boolean {
  return /\bbind:checked\b/.test(tag) || /(\bonchange=|\{\s*onchange\s*\})/.test(tag);
}

function spreads(tag: string): boolean {
  return /\{\s*\.\.\./.test(tag.replace(/=\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}/g, ''));
}

// The name comes from the visible label next to the switch (`labelledby`, with the same expression as
// an `id=` inside the element the switch sits in: its row); `label` is for a switch whose row shows no
// label of its own, so a `label` whose text the same file also shows as an element's content is a
// hidden copy and is refused.
function named({ path, tag, row }: Use, text = frontendSources[path] ?? ''): boolean {
  const own = /\blabel=\{(msg\('[\w.]+'\))\}/.exec(tag)?.[1];
  if (own) return !text.includes(`>{${own}}<`);
  const by = /\blabelledby=("[^"]*"|\{[^}]*\})/.exec(tag)?.[1];
  return by !== undefined && row.includes(`id=${by}`);
}

const at = (row: string, tag: string): Use => ({ path: 'x.svelte', tag, row });

describe('Toggle call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBeGreaterThanOrEqual(50);
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<Toggle\b/g)?.length ?? 0), 0));
  });

  it('finds the row a switch sits in', () => {
    const text = '<div class="row"><span id="a">A</span>{#if x > 1}<Toggle labelledby="a" />{/if}</div><span id="b">B</span>';
    expect(toggles('x.svelte', text).map((use) => use.row)).toEqual(['<div class="row"><span id="a">A</span>{#if x > 1}<Toggle labelledby="a" />{/if}</div>']);
  });

  it('name every switch: from the visible label in its row, or with label where none is shown', () => {
    const row = '<div class="row"><span id="row">Sound</span><Toggle /></div>';
    expect(named(at(row, "<Toggle checked={on} label={msg('a.b')} />"))).toBe(true);
    expect(named(at(row, "<Toggle checked={on} label={msg('a.b')} />"), "<span>{msg('a.b')}</span>")).toBe(false);
    expect(named(at(row, '<Toggle checked={on} labelledby="row" />'))).toBe(true);
    expect(named(at(row, '<Toggle checked={on} />'))).toBe(false);
    expect(named(at(row, '<Toggle checked={on} labelledby="nowhere" />'))).toBe(false);
    // The label is in the file, but in another row.
    expect(named(at('<div><Toggle /></div>', '<Toggle checked={on} labelledby="row" />'), row)).toBe(false);
    expect(uses.filter((use) => !named(use)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  it('bind the state or report it with onchange: a switch with neither is dead', () => {
    expect(wired('<Toggle bind:checked={x} />')).toBe(true);
    expect(wired('<Toggle checked={x} onchange={save} />')).toBe(true);
    expect(wired('<Toggle checked={x} {onchange} />')).toBe(true);
    expect(wired('<Toggle checked={x} labelledby="a" />')).toBe(false);
    expect(uses.filter(({ tag }) => !wired(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
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
