import { balanced } from './scan';

/**
 * Markup read for the call-site guards (`*.callers.test.ts`): where a component is used, the element it
 * sits in and the SettingsRow snippet that holds it, so a label or hint id is checked in the use's own
 * row, not anywhere in its file.
 *
 * Limit: the markup is read as tags, `{...}` expressions and `{#snippet}` blocks; markup built in
 * script is not seen. Comments, `<script>` and `<style>` are not markup, nor is the text an expression
 * or an attribute's value holds.
 */

export interface MarkupElement {
  name: string;
  start: number;
  end: number;
}

/** A `{#snippet}` a SettingsRow is given (`children`, `below`) and the names it takes from the row. */
export interface RowSnippet {
  /** The opening tag of the SettingsRow. */
  row: string;
  /** Where that SettingsRow starts in its file: two rows with the same tag stay apart. */
  at: number;
  name: string;
  /** What the snippet destructures: the row's name for a value (`labelId`) → the local name. */
  bound: Record<string, string>;
}

export interface Use {
  path: string;
  /** The opening tag of the use: its attributes, not what it holds. */
  tag: string;
  /** The markup of the element the use sits in. The whole file when it sits in none. */
  row: string;
  /** The innermost SettingsRow snippet the use sits in, at any depth; null outside every row. */
  snippet: RowSnippet | null;
}

/** A `{#snippet name(params)}…{/snippet}` block, from its opening tag to the end of its closing one. */
export interface SnippetBlock {
  name: string;
  params: string;
  start: number;
  end: number;
}

/** A `{…}` of the markup itself (`{@render x()}`, `{#each …}`, `{value}`), not one in a tag: its text
 *  inside the braces, and where it starts and ends. */
export interface MarkupTag {
  inner: string;
  start: number;
  end: number;
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

function scan(text: string): { elements: MarkupElement[]; snippets: SnippetBlock[]; tags: MarkupTag[] } {
  const found: MarkupElement[] = [];
  const snippets: SnippetBlock[] = [];
  const tags: MarkupTag[] = [];
  const open: { name: string; start: number }[] = [];
  const openSnippets: { name: string; params: string; start: number }[] = [];
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
      if (inner !== null) tags.push({ inner, start: at, end: at + inner.length + 2 });
      const header = inner === null ? null : /^#snippet\s+([\w$]+)\s*\(([\s\S]*)\)\s*$/.exec(inner);
      if (header) openSnippets.push({ name: header[1], params: header[2], start: at });
      else if (inner !== null && /^\/snippet\s*$/.test(inner)) {
        const block = openSnippets.pop();
        if (block) snippets.push({ ...block, end: at + inner.length + 2 });
      }
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
  return { elements: found, snippets, tags };
}

/** Every element of a component's markup, spanning from its opening tag to its closing one. */
export function elements(text: string): MarkupElement[] {
  return scan(text).elements;
}

/** A component's markup read at once: its elements, its `{#snippet}` blocks and its `{…}` tags. */
export function markup(text: string): { elements: MarkupElement[]; snippets: SnippetBlock[]; tags: MarkupTag[] } {
  return scan(text);
}

/** Whether an opening tag takes attributes through a spread (`{...props}`), which `attributes` skips. */
export function hasSpread(tag: string): boolean {
  let at = /^<[\w:.-]+/.exec(tag)?.[0].length ?? 0;
  while (at < tag.length) {
    const ch = tag[at];
    if (ch === '"' || ch === "'") {
      const close = tag.indexOf(ch, at + 1);
      at = close < 0 ? tag.length : close + 1;
      continue;
    }
    if (ch === '{') {
      const inner = balanced(tag, at);
      if (inner === null) return false;
      if (/^\s*\.\.\./.test(inner)) return true;
      at += inner.length + 2;
      continue;
    }
    at += 1;
  }
  return false;
}

/**
 * The attributes of an opening tag, each with its value as written: `"…"`, `'…'`, `{…}` or a bare
 * word; `{name}` for the shorthand `{name}`; `true` for one with no value. A spread is skipped.
 */
export function attributes(tag: string): Map<string, string> {
  const found = new Map<string, string>();
  let at = /^<[\w:.-]+/.exec(tag)?.[0].length ?? 0;
  while (at < tag.length) {
    const ch = tag[at];
    if (/\s/.test(ch) || ch === '/' || ch === '>') {
      at += 1;
      continue;
    }
    if (ch === '{') {
      const inner = balanced(tag, at);
      if (inner === null) break;
      const name = inner.trim();
      if (/^[\w$]+$/.test(name)) found.set(name, `{${name}}`);
      at += inner.length + 2;
      continue;
    }
    const name = /^[^\s=/>{]+/.exec(tag.slice(at))?.[0] ?? '';
    if (!name) break;
    at += name.length;
    if (tag[at] !== '=') {
      found.set(name, 'true');
      continue;
    }
    at += 1;
    const open = tag[at];
    let value: string;
    if (open === '"' || open === "'") {
      const close = tag.indexOf(open, at + 1);
      value = tag.slice(at, close < 0 ? tag.length : close + 1);
    } else if (open === '{') {
      const inner = balanced(tag, at);
      value = inner === null ? tag.slice(at) : `{${inner}}`;
    } else value = /^[^\s>]*/.exec(tag.slice(at))?.[0] ?? '';
    found.set(name, value);
    at += value.length;
  }
  return found;
}

/** Every value the attribute `name` takes on an element of the markup. */
export function attributeValues(text: string, name: string): string[] {
  return elements(text).flatMap((element) => {
    const value = attributes(text.slice(element.start, tagEnd(text, element.start))).get(name);
    return value === undefined ? [] : [value];
  });
}

/** The names a SettingsRow's ids take in a snippet's parameter: `{ labelId, hintId: hint, disabled = false }`
 *  gives labelId → labelId, hintId → hint, disabled → disabled; a parameter not destructured (`ids`, or
 *  `ids: SettingsRowIds`) gives each as `ids.<name>`. */
export function destructured(params: string): Record<string, string> {
  const whole = /^\s*([\w$]+)\s*(?::[\s\S]*)?$/.exec(params)?.[1];
  if (whole) return Object.fromEntries(['labelId', 'hintId', 'disabled'].map((name) => [name, `${whole}.${name}`]));
  const pattern = /^\s*\{([^{}]*)\}/.exec(params)?.[1];
  const bound: Record<string, string> = {};
  for (const entry of pattern?.split(',') ?? []) {
    const match = /^\s*([\w$]+)\s*(?::\s*([\w$]+))?\s*(?:=[\s\S]*)?$/.exec(entry);
    if (match) bound[match[1]] = match[2] ?? match[1];
  }
  return bound;
}

/** Each use of the component `name` in `text`, with the element it sits in and its row's snippet. */
export function usesIn(path: string, text: string, name: string): Use[] {
  const { elements: all, snippets } = scan(text);
  const around = (start: number, end: number) => all.filter((element) => element.start < start && element.end >= end).sort((a, b) => b.start - a.start);
  const rows = snippets.flatMap((block) => {
    const owner = around(block.start, block.end)[0];
    return owner?.name === 'SettingsRow' ? [{ ...block, owner }] : [];
  });
  return all
    .filter((element) => element.name === name)
    .map((use) => {
      const parent = around(use.start, use.end).find((element) => element !== use);
      const block = rows.filter((row) => row.start < use.start && row.end >= use.end).sort((a, b) => b.start - a.start)[0];
      const snippet: RowSnippet | null = block
        ? { row: text.slice(block.owner.start, tagEnd(text, block.owner.start)), at: block.owner.start, name: block.name, bound: destructured(block.params) }
        : null;
      return { path, tag: text.slice(use.start, tagEnd(text, use.start)), row: parent ? text.slice(parent.start, parent.end) : text, snippet };
    });
}

/**
 * A control inside a SettingsRow's snippet, at any depth, takes the ids the row hands it
 * (`{#snippet children({ labelId, hintId })}`): the row draws its label and hint with them. True when
 * the snippet that holds the use takes `key`, and `by` is the name it took it under.
 */
export function fromSettingsRow(snippet: RowSnippet | null, key: 'labelId' | 'hintId', by: string): boolean {
  const local = snippet?.bound[key];
  return local !== undefined && by === `{${local}}`;
}
