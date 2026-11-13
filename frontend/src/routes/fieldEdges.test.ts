import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

/**
 * A source tripwire: the sign-in and support forms leave their fields' edge, fill, ring and locked look
 * to the shared `.input` rule (global.css), which draws an edge that can be seen on any surface. A
 * selector can name a field in more ways than a guard can list (`#reply`, `[class~="area"]`,
 * `:where(textarea)`, `.form > *`), so the check is turned around: in these files no rule at all sets
 * a border, a background, an outline, a shadow or `all`, and no element carries them inline, except
 * the rules listed here, each on a class no field carries.
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

// A component's own style rules: each selector list with its declarations, at-rule wrappers left out.
function rules(source: string): [selector: string, body: string][] {
  const style = /<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';
  return [...style.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => [squeeze(m[1]), squeeze(m[2])]);
}

const looks = (body: string) =>
  body
    .split(';')
    .map((part) => part.split(':')[0].trim().toLowerCase())
    .filter((prop) => LOOK.test(prop));

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
    if (/\bclass=\{/.test(tag)) out.push(`a field's classes are computed, so no rule can be ruled out for it: ${squeeze(tag)}`);
    for (const name of /\bclass="([^"]*)"/.exec(tag)?.[1].split(/\s+/) ?? []) if (name) fieldClasses.add(name);
    for (const state of tag.matchAll(/\bclass:([\w-]+)/g)) fieldClasses.add(state[1]);
    if (/\sstyle(:[\w-]+)?=/.test(tag)) out.push(`a field carries an inline style: ${squeeze(tag)}`);
  }
  for (const inline of markupOf(source).matchAll(/\sstyle(?::([\w-]+)=(?:"[^"]*"|\{[^}]*\})|="([^"]*)")/g)) {
    const props = inline[1] ? [inline[1]] : (inline[2] ?? '').split(';').map((part) => part.split(':')[0].trim());
    if (props.some((prop) => LOOK.test(prop.toLowerCase()))) out.push(`an inline style sets a look: ${squeeze(inline[0])}`);
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
