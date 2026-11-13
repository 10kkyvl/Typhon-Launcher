import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

/**
 * A source tripwire: the sign-in and support forms leave their fields' edge, fill, ring and locked look
 * to the shared `.input` rule (global.css), which draws an edge that can be seen on any surface. A
 * selector can name a field in more ways than a guard can list (`#reply`, `[class~="area"]`,
 * `:where(textarea)`, `.form > *`), so the check is turned around: in these files no rule at all sets
 * a border, a background, an outline, a shadow or `all`, and no element carries them inline, except
 * the rules listed here, each on a class no field carries. Nor does a rule set a custom property the
 * shared field look reads, and a field's class list holds no expression.
 *
 * Limits: the source is read as written. A selector or a property spelled with CSS escapes, and a
 * field drawn through `<svelte:element>` or by a child component, are not seen.
 */
const LOOK = /^(border|background|outline)(-[\w-]+)?$|^box-shadow$|^all$/;

// The red edge of a field holding a bad value: the one look a form gives a field itself.
const INVALID_EDGE = ['.input.invalid', 'border-color: var(--danger);'] as const;

type Allowed = Record<string, string>;

const FORMS: Record<string, Allowed> = {
  'routes/auth/AuthScreen.svelte': {
    '.auth': 'the page the screen paints itself',
    '.panel': 'the card the form sits in',
    '.guest-divider::before, .guest-divider::after': 'the rule either side of "or"',
  },
  'routes/support/NewTicket.svelte': {
    '.counter': "the character count's plate over the backdrop",
    '.counter.over': 'the count past its limit',
    '.chosen': 'the chosen game, a row with its clear button',
    '.error': "a field message's plate over the backdrop",
  },
  'routes/support/TicketView.svelte': {
    '.bubble': 'a message of the thread',
    '.bubble.mine': "the user's own message",
    '.counter': "the character count's plate over the backdrop",
    '.counter.over': 'the count past its limit',
    '.error': "a field message's plate over the backdrop",
  },
};

const squeeze = (text: string) => text.replace(/\s+/g, ' ').trim();

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

// The custom properties the shared field look reads: every `var(--…)` of a rule on `.input` in
// global.css. A form that set one of them would repaint its fields without naming them.
const FIELD_TOKENS = new Set(
  [...fs.readFileSync(new URL('../styles/global.css', import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)]
    .filter((rule) => /\.input\b/.test(rule[1]))
    .flatMap((rule) => [...rule[2].matchAll(/var\((--[\w-]+)/g)].map((m) => m[1])),
);

// A component's own style rules, nested ones too: each with the declarations that are its own. A rule
// nested in another is named by both (`.form .area`, `.input &:hover`); an at-rule (@media) adds
// nothing to the name, and declarations put straight into it belong to the rule around it. Read by
// brace depth: a regex over innermost blocks would miss the declarations beside a nested rule.
function rules(source: string): [selector: string, body: string][] {
  const css = (/<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
  const out: [string, string][] = [];
  // Reads one block from `from` (just past its `{`) and answers where it ends.
  const block = (from: number, name: string): number => {
    let own = '';
    let chunk = '';
    let quote = '';
    let i = from;
    for (; i < css.length; i++) {
      const c = css[i];
      if (quote) {
        chunk += c;
        if (c === quote) quote = '';
      } else if (c === '"' || c === "'") {
        quote = c;
        chunk += c;
      } else if (c === ';') {
        own += `${chunk};`;
        chunk = '';
      } else if (c === '{') {
        const prelude = squeeze(chunk);
        chunk = '';
        i = block(i + 1, prelude.startsWith('@') ? name : squeeze(`${name} ${prelude}`));
      } else if (c === '}') break;
      else chunk += c;
    }
    own += chunk;
    if (name && squeeze(own)) out.push([name, squeeze(own)]);
    return i;
  };
  block(0, '');
  return out;
}

const isLook = (prop: string) => LOOK.test(prop.toLowerCase()) || FIELD_TOKENS.has(prop);

const looks = (body: string) =>
  body
    .split(';')
    .map((part) => part.split(':')[0].trim())
    .filter(isLook);

const markupOf = (source: string) => source.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');

// The opening tag that starts at `from`, up to its closing `>`: quotes and braces in attributes are
// stepped over, so an arrow function or a template string in an expression does not end the tag.
function tagAt(markup: string, from: number): string {
  let depth = 0;
  let quote = '';
  for (let i = from; i < markup.length; i++) {
    const c = markup[i];
    if (quote) {
      if (c === '\\') i++;
      else if (c === quote) quote = '';
    } else if (c === '"' || c === "'" || c === '`') quote = c;
    else if (c === '{') depth++;
    else if (c === '}') depth--;
    else if (c === '>' && depth === 0) return markup.slice(from, i + 1);
  }
  return markup.slice(from);
}

// The opening tags of a component's fields.
function fieldTags(source: string): string[] {
  const markup = markupOf(source);
  return [...markup.matchAll(/<(?:input|textarea|select)\b/g)].map((m) => tagAt(markup, m.index));
}

// Everything in a form that would give its fields a look of their own, in words.
function fieldLookOffences(source: string, allowed: Allowed): string[] {
  const out: string[] = [];
  const tags = fieldTags(source);
  const fieldClasses = new Set<string>();
  for (const tag of tags) {
    if (/\bclass=(\{|"[^"]*\{)/.test(tag)) out.push(`a field's classes are computed, so no rule can be ruled out for it: ${squeeze(tag)}`);
    for (const name of /\bclass="([^"]*)"/.exec(tag)?.[1].split(/\s+/) ?? []) if (name) fieldClasses.add(name);
    for (const state of tag.matchAll(/\bclass:([\w-]+)/g)) fieldClasses.add(state[1]);
    if (/\sstyle(:[\w-]+)?=/.test(tag)) out.push(`a field carries an inline style: ${squeeze(tag)}`);
  }
  for (const inline of markupOf(source).matchAll(/\sstyle(?::([\w-]+)=(?:"[^"]*"|\{[^}]*\})|="([^"]*)")/g)) {
    const props = inline[1] ? [inline[1]] : (inline[2] ?? '').split(';').map((part) => part.split(':')[0].trim());
    if (props.some(isLook)) out.push(`an inline style sets a look: ${squeeze(inline[0])}`);
  }
  for (const [selector, body] of rules(source)) {
    if (looks(body).length === 0) continue;
    if (selector === INVALID_EDGE[0] && body === INVALID_EDGE[1]) continue;
    if (!Object.hasOwn(allowed, selector)) {
      out.push(`${selector} sets ${looks(body).join(', ')} and is not listed`);
      continue;
    }
    // A listed rule names things by class alone, and no field carries one of those classes.
    if (!selector.split(',').every((part) => /^(\.[\w-]+)+(::[\w-]+)?$/.test(part.trim()))) out.push(`${selector} is listed but names more than classes`);
    const carried = [...selector.matchAll(/\.([\w-]+)/g)].map((m) => m[1]).filter((name) => fieldClasses.has(name));
    if (carried.length > 0) out.push(`${selector} is listed, but a field carries .${carried.join(', .')}`);
  }
  return out;
}

describe("the sign-in and support forms' fields", () => {
  it.each(Object.keys(FORMS))('%s gives its fields no look of its own', (path) => {
    const source = frontendSources[path] ?? '';
    expect(fieldTags(source).length, path).toBeGreaterThan(0);
    expect(fieldLookOffences(source, FORMS[path]), path).toEqual([]);
  });

  it.each(Object.entries(FORMS))('%s lists no rule that is gone', (path, allowed) => {
    const present = new Set(rules(frontendSources[path] ?? '').map(([selector]) => selector));
    expect(Object.keys(allowed).filter((selector) => !present.has(selector)), path).toEqual([]);
  });
});

describe('the field look guard', () => {
  const form = (style: string, field = '<textarea id="reply" class="input area" class:invalid={bad}></textarea>') =>
    `<div class="form"><span class="counter">3</span>${field}</div><style>${style}</style>`;
  const allowed: Allowed = { '.counter': 'a plate' };

  it('passes a form whose only looks are listed, on classes no field carries, and the red edge of a bad value', () => {
    expect(fieldLookOffences(form('.counter { background: var(--surface-2); } .input.invalid { border-color: var(--danger); } .area { height: auto; resize: vertical; }'), allowed)).toEqual([]);
  });

  it.each([
    ['.area { border: 1px solid red; }'],
    ['textarea:focus { outline: none; }'],
    ['.field input { background: #222; }'],
    ['#reply { border: 1px solid red; }'],
    ['[class~="area"] { background: #222; }'],
    [':where(textarea) { outline: none; }'],
    ['.form > * { border: 0; }'],
    ['.area { all: unset; }'],
    ['.area { box-shadow: none; }'],
    ['.area:focus-visible { outline-offset: 4px; }'],
    ['@media (min-width: 10px) { .area { border-color: red; } }'],
    ['.input.invalid { border-color: var(--danger); background: red; }'],
  ])('refuses %s', (style) => {
    expect(fieldLookOffences(form(style), allowed)).not.toEqual([]);
  });

  it.each([
    ['.input { outline: none; border-color: transparent; &:hover { color: inherit; } }'],
    ['.form { color: inherit; .area { border: 0; } }'],
    ['.form { & > * { background: red; } }'],
    ['@media (min-width: 10px) { .form { textarea { outline: none; } } }'],
    ['.form { & > .label { color: red; } outline: none; }'],
  ])('refuses a look beside or inside a nested rule: %s', (style) => {
    expect(fieldLookOffences(form(style), allowed)).not.toEqual([]);
  });

  it('reads a nested rule that sets no look as no offence', () => {
    expect(fieldLookOffences(form('.form { display: flex; .area { height: auto; } @media (min-width: 10px) { gap: 1rem; } }'), allowed)).toEqual([]);
  });

  it.each([['--field-edge'], ['--accent-ring'], ['--surface-2'], ['--danger']])('refuses %s, which the shared field look reads, set by a rule or inline', (token) => {
    expect(fieldLookOffences(form(`.form { ${token}: transparent; }`), allowed)).not.toEqual([]);
    expect(fieldLookOffences(form('', `<div style="${token}: transparent"><input class="input" /></div>`), allowed)).not.toEqual([]);
    expect(fieldLookOffences(form('', `<div style:${token}="transparent"><input class="input" /></div>`), allowed)).not.toEqual([]);
  });

  it('lets a form set a custom property the field look does not read', () => {
    expect(fieldLookOffences(form('.form { --wails-draggable: drag; }'), allowed)).toEqual([]);
  });

  it('refuses a field whose class list holds an expression', () => {
    expect(fieldLookOffences(form('', '<textarea class="input {kind}"></textarea>'), allowed)).not.toEqual([]);
  });

  it('refuses a look set inline, on a field or around it', () => {
    expect(fieldLookOffences(form('', '<textarea class="input" style="border: 0"></textarea>'), allowed)).not.toEqual([]);
    expect(fieldLookOffences(form('', '<textarea class="input" style:outline="none"></textarea>'), allowed)).not.toEqual([]);
    expect(fieldLookOffences(form('', '<div style="background: red"><input class="input" /></div>'), allowed)).not.toEqual([]);
  });

  it('refuses a field whose classes are computed', () => {
    expect(fieldLookOffences(form('', '<textarea class={kind}></textarea>'), allowed)).not.toEqual([]);
  });

  it('refuses a listed rule once a field carries its class, or once it names more than classes', () => {
    expect(fieldLookOffences(form('.counter { background: red; }', '<input class="input counter" />'), allowed)).not.toEqual([]);
    expect(fieldLookOffences(form('.counter, textarea { background: red; }'), { '.counter, textarea': 'a plate' })).not.toEqual([]);
  });
});
