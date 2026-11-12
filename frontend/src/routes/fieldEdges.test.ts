import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

// The selectors of a component's own style rules, each with its declarations.
function rules(source: string): [selector: string, body: string][] {
  const style = /<style>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';
  return [...style.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => [m[1].trim(), m[2]]);
}

// What names a field in a component's markup: its tag, and every class its tag carries, written in
// `class="…"` or set by a `class:name` directive.
function fieldNames(source: string): { tags: Set<string>; classes: Set<string> } {
  const markup = source.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');
  const tags = new Set(['input', 'textarea', 'select']);
  const classes = new Set<string>();
  for (const tag of markup.matchAll(/<(input|textarea|select)\b[^>]*>/g)) {
    for (const name of /\bclass="([^"]*)"/.exec(tag[0])?.[1].split(/\s+/) ?? []) if (name && !name.includes('{')) classes.add(name);
    for (const directive of tag[0].matchAll(/\bclass:([\w-]+)/g)) classes.add(directive[1]);
  }
  return { tags, classes };
}

// Whether a selector's subject (its last compound) is a field: by tag or by one of a field's classes.
function namesField(selector: string, names: { tags: Set<string>; classes: Set<string> }): boolean {
  return selector.split(',').some((part) => {
    const subject = part.trim().split(/[\s>+~]+/).pop() ?? '';
    const tag = /^[a-z]+/.exec(subject)?.[0];
    if (tag && names.tags.has(tag)) return true;
    return [...subject.matchAll(/\.([\w-]+)/g)].some((m) => names.classes.has(m[1]));
  });
}

// The look a form may give its fields itself: the red edge of a field holding a bad value.
const INVALID_EDGE = /^\s*border-color:\s*var\(--danger\);?\s*$/;

// The rules of a component that draw an edge, a fill or an outline of their own on its fields,
// whatever the selector names the field by.
function ownFieldLooks(source: string): string[] {
  const names = fieldNames(source);
  return rules(source)
    .filter(([selector, body]) => namesField(selector, names) && /(^|;)\s*(border|background|outline)[\w-]*\s*:/.test(body))
    .filter(([selector, body]) => !(/(\.invalid|\[aria-invalid)/.test(selector) && INVALID_EDGE.test(body)))
    .map(([selector]) => selector);
}

const FORMS = ['routes/auth/AuthScreen.svelte', 'routes/support/NewTicket.svelte', 'routes/support/TicketView.svelte'];

// A source tripwire: the sign-in and support forms leave their fields' edge, background and locked
// look to the shared `.input` rule (global.css), which draws an edge that can be seen on any surface.
// A scoped rule of their own would win over it and bring back the faint one.
describe('the sign-in and support forms\' fields', () => {
  it.each(FORMS)('%s draws no field edge of its own', (path) => {
    const source = frontendSources[path] ?? '';
    expect(rules(source).length, path).toBeGreaterThan(0);
    expect(ownFieldLooks(source), path).toEqual([]);
  });

  it('finds a field look whatever the selector names the field by', () => {
    const form = (style: string) => `<textarea class="input area" class:invalid={bad}></textarea><input class="input" /><style>${style}</style>`;
    expect(ownFieldLooks(form('.area { border: 1px solid red; }'))).toEqual(['.area']);
    expect(ownFieldLooks(form('textarea:focus { outline: none; }'))).toEqual(['textarea:focus']);
    expect(ownFieldLooks(form('.field input { background: #222; }'))).toEqual(['.field input']);
    expect(ownFieldLooks(form('.area:focus-visible { outline: 0; }'))).toEqual(['.area:focus-visible']);
    expect(ownFieldLooks(form('.area { height: auto; resize: vertical; }'))).toEqual([]);
    expect(ownFieldLooks(form('.label { border-bottom: 1px solid; }'))).toEqual([]);
    // The one look a form adds: the red edge of a field holding a bad value.
    expect(ownFieldLooks(form('.input.invalid { border-color: var(--danger); }'))).toEqual([]);
  });
});
