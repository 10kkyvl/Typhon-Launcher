import { balanced } from './scan';

/**
 * Markup read for the call-site guards (`*.callers.test.ts`): where a component is used and the element
 * it sits in, so a label or hint id is checked in the use's own row, not anywhere in its file.
 *
 * Limit: the markup is read as tags and `{...}` expressions; markup built in script is not seen, and a
 * snippet is not an element, so the row of a control put straight into `{#snippet}` is the component
 * the snippet belongs to.
 */

export interface MarkupElement {
  name: string;
  start: number;
  end: number;
}

export interface Use {
  path: string;
  /** The opening tag of the use: its attributes, not what it holds. */
  tag: string;
  /** The markup of the element the use sits in: its row. The whole file when it sits in none. */
  row: string;
}

const VOID = new Set(['area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'source', 'track', 'wbr']);

/** The end of the tag that opens at `start` (the index after its `>`), past braces and quoted values. */
export function tagEnd(text: string, start: number): number {
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

/** Every element of a component's markup, spanning from its opening tag to its closing one. */
export function elements(text: string): MarkupElement[] {
  const found: MarkupElement[] = [];
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

/** Each use of the component `name` in `text`, with the element it sits in. */
export function usesIn(path: string, text: string, name: string): Use[] {
  const all = elements(text);
  return all
    .filter((element) => element.name === name)
    .map((use) => {
      const parent = all
        .filter((element) => element !== use && element.start < use.start && element.end >= use.end)
        .sort((a, b) => b.start - a.start)[0];
      return { path, tag: text.slice(use.start, tagEnd(text, use.start)), row: parent ? text.slice(parent.start, parent.end) : text };
    });
}

/**
 * A control put straight into a SettingsRow's snippet takes the ids the row hands it
 * (`{#snippet children({ labelId, hintId })}`): the row draws its label and hint with them. True when
 * `row` is that SettingsRow, its snippet takes `key`, and `by` is that binding. Read by the names the
 * row gives; a renamed binding is not seen.
 */
export function fromSettingsRow(row: string, key: 'labelId' | 'hintId', by: string): boolean {
  const snippet = new RegExp(String.raw`\{#snippet \w+\(\{[^}]*\b${key}\b[^}]*\}\)\}`);
  return /^<SettingsRow\b/.test(row) && by === `{${key}}` && snippet.test(row);
}
