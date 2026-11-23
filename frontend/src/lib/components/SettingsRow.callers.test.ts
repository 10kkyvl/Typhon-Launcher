import { describe, expect, it } from 'vitest';
import { attributes, destructured, hasSpread, markup, tagEnd, usesIn } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites disable a SettingsRow's control, read from their markup. A row with
 * `disabledReason` makes nothing inert (SettingsRow.svelte): its `children` snippet takes `disabled`
 * and hands it to every base control in it, at any depth, as `{disabled}`, or as one side of an or
 * (`disabled={disabled || saving}`), so no live control stays under the reason. `below` is the
 * caller's: it may keep a link that says why.
 *
 * The guard refuses what it cannot follow. In a disabled row's `children` it accepts the base controls
 * of CONTROLS, each given the snippet's flag; TextLink; plain text and layout elements; and a
 * `{@render}` of a snippet defined there. It refuses a raw `<button>`, `<input>`, `<select>` or
 * `<textarea>`, any other component (one it does not know may hold a control: a new control is added
 * to CONTROLS on purpose, once it takes `disabled`), a `{@render}` of a snippet from outside the row,
 * `{@html}`, a block that takes the flag's name again, a control outside a `children` snippet (which
 * takes no flag), `children` passed as a prop, and a row that takes its props through a spread (its
 * reason cannot be read).
 *
 * Limit: see lib/testing/markup.ts.
 */

/** The base controls that take `disabled` and turn their own element off with it. */
const CONTROLS = ['Toggle', 'Select', 'Slider', 'IntegerInput', 'Button', 'IconButton', 'Checkbox', 'SegmentedControl'];
CONTROLS.push('RateLimitInput');
/** Components that may stand under a reason as they are: a link that says why or how to fix it. */
const QUIET = ['TextLink'];
const RAW = ['button', 'input', 'select', 'textarea'];

const named = (name: string) => new RegExp(String.raw`(?<![\w$.])${name.replace(/[$.]/g, '\\$&')}(?![\w$])`);
const lineOf = (text: string, at: number) => text.slice(0, at).split('\n').length;

// The parts of an expression joined by a top-level `||`; null when it holds a top-level ternary, comma,
// assignment or arrow, whose value an or of its parts does not give.
function orTerms(expression: string): string[] | null {
  const terms: string[] = [];
  let depth = 0;
  let from = 0;
  for (let at = 0; at < expression.length; at += 1) {
    const ch = expression[at];
    if (ch === '"' || ch === "'" || ch === '`') {
      for (at += 1; at < expression.length && expression[at] !== ch; at += 1) if (expression[at] === '\\') at += 1;
      continue;
    }
    if ('([{'.includes(ch)) depth += 1;
    else if (')]}'.includes(ch)) depth -= 1;
    if (depth > 0) continue;
    const pair = expression.slice(at, at + 2);
    if (pair === '||') {
      terms.push(expression.slice(from, at));
      from = at + 2;
      at += 1;
    } else if (ch === ',' || pair === '=>') return null;
    else if (ch === '?' && expression[at + 1] !== '.' && expression[at + 1] !== '?' && expression[at - 1] !== '?') return null;
    else if (ch === '=' && !'=!<>'.includes(expression[at - 1]) && expression[at + 1] !== '=') return null;
  }
  return [...terms, expression.slice(from)];
}

const unwrapped = (term: string): string => {
  const trimmed = term.trim();
  return /^\((.*)\)$/s.test(trimmed) ? unwrapped(trimmed.slice(1, -1)) : trimmed;
};

// `{flag}` or `{flag || …}`: the control is off whenever the row is.
function takesFlag(value: string | undefined, flag: string | undefined): boolean {
  if (value === undefined || flag === undefined || !/^\{[\s\S]*\}$/.test(value)) return false;
  return (orTerms(value.slice(1, -1)) ?? []).some((term) => unwrapped(term) === flag);
}

// A block inside the snippet that binds the flag's name (or, for `ids.disabled`, `ids`) again.
function rebinds(tag: string, flag: string): boolean {
  const name = named(flag.split('.')[0]);
  const bound =
    /^#each\b[\s\S]*?\bas\b([\s\S]*)$/.exec(tag)?.[1] ??
    /^@const\s+([\s\S]*?)=/.exec(tag)?.[1] ??
    /^#snippet\s+[\w$]+\s*\(([\s\S]*)\)\s*$/.exec(tag)?.[1] ??
    /^(?::then|:catch)\s+([\s\S]*)$/.exec(tag)?.[1] ??
    /^#await\b[\s\S]*?\b(?:then|catch)\b([\s\S]*)$/.exec(tag)?.[1];
  return bound !== undefined && name.test(bound);
}

/** What a file's disabled rows hold that the guard cannot follow: one line each, empty when none. */
function refusals(path: string, text: string): string[] {
  const { elements: all, snippets, tags } = markup(text);
  const inside = (start: number, end: number, range: { start: number; end: number }) => start >= range.start && end <= range.end;
  const owner = (start: number, end: number) => all.filter((element) => element.start < start && element.end >= end).sort((a, b) => b.start - a.start)[0];
  const out: string[] = [];
  for (const row of all.filter((element) => element.name === 'SettingsRow')) {
    const open = text.slice(row.start, tagEnd(text, row.start));
    const where = `${path}:${lineOf(text, row.start)}`;
    if (hasSpread(open)) {
      out.push(`${where}: the row takes its props through a spread`);
      continue;
    }
    const props = attributes(open);
    if (!props.has('disabledReason')) continue;
    if (props.has('children')) out.push(`${where}: the row takes children as a prop`);
    const body = { start: tagEnd(text, row.start), end: Math.max(tagEnd(text, row.start), text.lastIndexOf('</SettingsRow', row.end)) };
    // The row's own snippets: in its body, in no element and no other snippet of it.
    const own = snippets.filter(
      (block) => inside(block.start, block.end, body) && owner(block.start, block.end) === row && !snippets.some((outer) => outer !== block && inside(outer.start, outer.end, body) && outer.start < block.start && outer.end >= block.end),
    );
    const below = own.filter((block) => block.name !== 'children');
    const children = own.find((block) => block.name === 'children');
    const flag = children ? destructured(children.params).disabled : undefined;
    const held = (start: number, end: number) => inside(start, end, body) && !below.some((block) => inside(start, end, block));

    for (const element of all) {
      if (element === row || !held(element.start, element.end)) continue;
      const tag = text.slice(element.start, tagEnd(text, element.start));
      if (CONTROLS.includes(element.name)) {
        if (!takesFlag(attributes(tag).get('disabled'), flag)) out.push(`${where}: ${tag} does not take the row's disabled`);
      } else if (RAW.includes(element.name)) out.push(`${where}: a raw <${element.name}> the row cannot turn off`);
      else if (!QUIET.includes(element.name) && /^[A-Z]|[:.]/.test(element.name)) out.push(`${where}: <${element.name}>, a component the guard does not know`);
    }

    const defined = snippets.filter((block) => block !== children && held(block.start, block.end)).map((block) => block.name);
    for (const tag of tags) {
      if (!held(tag.start, tag.end) || tag.start === children?.start) continue;
      const inner = tag.inner.trim();
      const render = /^@render\s+([\w$]+)/.exec(inner)?.[1];
      if (render !== undefined && !defined.includes(render)) out.push(`${where}: renders ${render}, a snippet from outside the row`);
      if (/^@html\b/.test(inner)) out.push(`${where}: markup written as html`);
      if (flag !== undefined && rebinds(inner, flag)) out.push(`${where}: {${inner}} takes ${flag.split('.')[0]} again`);
    }
  }
  return out;
}

const sources = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte'));
const refused = (text: string) => refusals('x.svelte', text).length > 0;
const row = (snippet: string, body: string, reason = ' disabledReason={why}') => `<SettingsRow label="L"${reason}>{#snippet children(${snippet})}${body}{/snippet}</SettingsRow>`;

describe('SettingsRow call sites', () => {
  it('reads them', () => {
    const rows = sources.flatMap(([path, text]) => usesIn(path, text, 'SettingsRow'));
    expect(rows.length).toBeGreaterThanOrEqual(10);
    expect(rows.filter((use) => attributes(use.tag).has('disabledReason')).length).toBeGreaterThanOrEqual(2);
  });

  it("hand the row's disabled to every base control under a disabled reason", () => {
    expect(refused(row('{ labelId, disabled }', '<Toggle labelledby={labelId} {disabled} />'))).toBe(false);
    expect(refused(row('{ disabled }', '<Select disabled={disabled || saving} />'))).toBe(false);
    expect(refused(row('{ disabled }', "<Select disabled={(disabled) || mode === 'off' || count > 3} />"))).toBe(false);
    expect(refused(row('{ disabled: off }', '<div class="pair"><Slider disabled={off} /><IntegerInput id="x" disabled={off} /></div>'))).toBe(false);
    expect(refused(row('{ disabled }', '{#each rows as item (item.id)}<Checkbox {disabled} />{/each}'))).toBe(false);
    expect(refused(`<Card>{#snippet body()}<section>${row('{ disabled }', '<Toggle {disabled} />')}</section>{/snippet}</Card>`)).toBe(false);
    // Forgotten, or not the row's.
    expect(refusals('x.svelte', row('{ labelId }', '<Toggle labelledby={labelId} />'))).toEqual(["x.svelte:1: <Toggle labelledby={labelId} /> does not take the row's disabled"]);
    expect(refused(row('{ disabled }', '<Toggle />'))).toBe(true);
    expect(refused(row('{ disabled }', '<Button disabled={saving}>Clear</Button>'))).toBe(true);
    expect(refused(row('{ disabled }', '<Toggle disabled />'))).toBe(true);
    expect(refused(row('{ disabled }', '<Toggle disabled="{disabled}" />'))).toBe(true);
    expect(refused(row('{ disabled }', '<Toggle bind:disabled />'))).toBe(true);
    expect(refused(row('{ disabled: off }', '<Toggle {disabled} />'))).toBe(true);
    expect(refused('<SettingsRow label="L" disabledReason={why}>{#snippet children()}<Toggle {disabled} />{/snippet}{#snippet below({ disabled })}<a href="#why">Why</a>{/snippet}</SettingsRow>')).toBe(true);
    // Not asked: a row without a reason, the part below, a row of its own inside a disabled row's part below.
    expect(refused(row('{ labelId }', '<Toggle labelledby={labelId} /><button type="button">Go</button>', ''))).toBe(false);
    expect(refused('<SettingsRow label="L" disabledReason={why}>{#snippet below()}<Button onclick={fix}>Fix</Button>{/snippet}</SettingsRow>')).toBe(false);
    expect(refused(`<SettingsRow label="Outer" disabledReason={why}>{#snippet below()}${row('{ labelId }', '<Toggle labelledby={labelId} />', '')}{/snippet}</SettingsRow>`)).toBe(false);
  });

  it('accepts a link that says why, plain text and layout, and a snippet defined in the row', () => {
    expect(refused(row('{ disabled }', '<p class="why">Not <strong>now</strong>: <TextLink tone="muted" onclick={go}>why</TextLink></p>'))).toBe(false);
    expect(refused(row('{ disabled }', '{#snippet pair()}<Toggle {disabled} /><Slider {disabled} />{/snippet}{#if wide}{@render pair()}{/if}'))).toBe(false);
  });

  it("accepts the row's flag taken through a snippet parameter that is not destructured", () => {
    expect(refused(row('ids', '<Toggle disabled={ids.disabled} />'))).toBe(false);
    expect(refused(row('ids: SettingsRowIds', '<Toggle labelledby={ids.labelId} disabled={ids.disabled || saving} />'))).toBe(false);
  });

  for (const [name, text] of [
    ['the flag negated', row('{ disabled }', '<Toggle disabled={!disabled} />')],
    ['the flag cancelled', row('{ disabled }', '<Toggle disabled={disabled && false} />')],
    ['the flag inside a ternary', row('{ disabled }', '<Toggle disabled={disabled || saving ? false : true} />')],
    ['the flag in a string', row('{ disabled }', "<Toggle disabled={'disabled'} />")],
    ['false', row('{ disabled }', '<Toggle disabled={false} />')],
    ['a snippet rendered from outside the row', `{#snippet control()}<Toggle />{/snippet}${row('{ disabled }', '{@render control()}')}`],
    ['a snippet from outside the row handed the flag', `{#snippet control(off)}<Toggle disabled={off} />{/snippet}${row('{ disabled }', '{@render control(disabled)}')}`],
    ['a raw button', row('{ disabled }', '<button type="button" onclick={go}>Go</button>')],
    ['a raw input', row('{ disabled }', '<input type="checkbox" />')],
    ['a raw select', row('{ disabled }', '<select><option>a</option></select>')],
    ['a raw textarea', row('{ disabled }', '<textarea></textarea>')],
    ['a component the guard does not know', row('{ disabled }', '<DiskMeter label="Drive" />')],
    ['a control inside a component the guard does not know', row('{ disabled }', '<Card>{#snippet body()}<Toggle {disabled} />{/snippet}</Card>')],
    ['markup written as html', row('{ disabled }', '{@html controls}')],
    ['the flag taken again by an each block', row('{ disabled }', '{#each rows as disabled}<Toggle {disabled} />{/each}')],
    ['the content outside a children snippet', '<SettingsRow label="L" disabledReason={why}><Toggle {disabled} /></SettingsRow>'],
    ['the control passed as a prop', '<SettingsRow label="L" disabledReason={why} children={control} />'],
    ['the props through a spread','<SettingsRow label="L" {...props}>{#snippet children({ disabled })}<Toggle />{/snippet}</SettingsRow>'],
  ] as const) {
    it(`refuses a disabled row's control with ${name}`, () => {
      expect(refused(text)).toBe(true);
    });
  }

  it('holds the speed limit field to the flag like any base control', () => {
    expect(refused(row('{ labelId, disabled }', '<RateLimitInput value={1} presets={[1]} labelledby={labelId} {disabled} onchange={set} />'))).toBe(false);
    expect(refused(row('{ labelId }', '<RateLimitInput value={1} presets={[1]} labelledby={labelId} onchange={set} />'))).toBe(true);
  });

  it('finds nothing to refuse in the call sites', () => {
    expect(sources.flatMap(([path, text]) => refusals(path, text))).toEqual([]);
  });
});
