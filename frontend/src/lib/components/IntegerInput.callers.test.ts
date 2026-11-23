import { describe, expect, it } from 'vitest';
import { attributeValues, attributes, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites name IntegerInput, read from their markup. A field has one name: its own `label`
 * (an aria-label), or an `id` that a visible label names it through: a SettingsRow's `labelFor` on the
 * row it sits in, or a `<label for>` in its file. An `id` no label points at leaves the field nameless;
 * a `label` next to such an id is a hidden name that would take over from the visible one (the field
 * drops it, IntegerInput.svelte, but the caller then says two names).
 *
 * Limit: see lib/testing/markup.ts; the row of a use is the SettingsRow whose snippet holds it, at any
 * depth.
 */

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte'));
const uses = callers.flatMap(([path, text]) => usesIn(path, text, 'IntegerInput'));

// The id is named by a visible label: the SettingsRow whose snippet holds the field says `labelFor`
// with the same expression, or a `<label for>` in the file does.
function labelledFor({ tag, snippet }: Use, text: string): boolean {
  const id = attributes(tag).get('id');
  if (id === undefined) return false;
  return (snippet !== null && attributes(snippet.row).get('labelFor') === id) || attributeValues(text, 'for').includes(id);
}

function named(use: Use, text = frontendSources[use.path] ?? ''): boolean {
  const own = attributes(use.tag).has('label');
  return own !== labelledFor(use, text);
}

const at = (row: string, tag: string): Use => ({ path: 'x.svelte', tag, row, snippet: null });
const first = (text: string): Use => usesIn('x.svelte', text, 'IntegerInput')[0];

describe('IntegerInput call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBeGreaterThanOrEqual(3);
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<IntegerInput\b/g)?.length ?? 0), 0));
  });

  it('name every field once: with its own label, or by an id a visible label points at', () => {
    const settings = (labelFor: string, field = '<IntegerInput id="{uid}-threads" />') =>
      `<SettingsRow label="Threads" labelFor=${labelFor}>{#snippet children({ hintId })}${field}{/snippet}</SettingsRow>`;
    const field = '<IntegerInput id="{uid}-threads" />';
    expect(named(at('<div>x</div>', "<IntegerInput label={msg('a.b')} />"), '')).toBe(true);
    expect(named(at('<div>x</div>', '<IntegerInput label="Frame limit" />'), '')).toBe(true);
    expect(named(first(settings('"{uid}-threads"')), '')).toBe(true);
    expect(named(at('<div><label for="{uid}-threads">Threads</label><IntegerInput /></div>', field), '<label for="{uid}-threads">Threads</label>')).toBe(true);
    // An id no label points at: a nameless field.
    expect(named(first(settings('"{uid}-other"')), '')).toBe(false);
    expect(named(at('<div>x</div>', field), '')).toBe(false);
    // Neither: a nameless field.
    expect(named(at('<div>x</div>', '<IntegerInput value={4} />'), '')).toBe(false);
    // Both: a hidden name over the visible one.
    expect(named(first(settings('"{uid}-threads"', '<IntegerInput id="{uid}-threads" label="Threads" />')), '')).toBe(false);
    // A labelFor on another row names nothing here.
    const other = `<div>${settings('"{uid}-threads"')}<IntegerInput id="{uid}-threads" /></div>`;
    expect(usesIn('x.svelte', other, 'IntegerInput').map((use) => named(use, ''))).toEqual([true, false]);

    expect(uses.filter((use) => !named(use)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  // The row's `labelFor` names a field anywhere in the row's snippet; `labelFor=` or `for=` in a comment
  // or inside another attribute's text is not markup.
  it('read the label of a field wrapped in its row, and no label from a comment or a string', () => {
    const named1 = (text: string) => usesIn('x.svelte', text, 'IntegerInput').map((use) => named(use, text));
    expect(named1('<SettingsRow label="Threads" labelFor="{uid}-t">{#snippet children({ hintId })}<div class="pair"><IntegerInput id="{uid}-t" /></div>{/snippet}</SettingsRow>')).toEqual([true]);
    expect(named1('<SettingsRow label="Threads" hint=\'set labelFor="{uid}-t" here\'>{#snippet children()}<IntegerInput id="{uid}-t" />{/snippet}</SettingsRow>')).toEqual([false]);
    expect(named1('<!-- <label for="{uid}-t">Threads</label> --><div><IntegerInput id="{uid}-t" /></div>')).toEqual([false]);
    expect(named1('<label for="{uid}-t">Threads</label><div><IntegerInput id="{uid}-t" /></div>')).toEqual([true]);
  });

  // A spread hides whether the caller names the field.
  it('spread nothing onto a number field', () => {
    expect(uses.filter(({ tag }) => /\s\{\s*\.\.\./.test(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });
});
