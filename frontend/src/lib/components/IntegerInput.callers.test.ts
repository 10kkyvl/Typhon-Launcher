import { describe, expect, it } from 'vitest';
import { tagEnd, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites name IntegerInput, read from their markup. A field has one name: its own `label`
 * (an aria-label), or an `id` that a visible label names it through: a SettingsRow's `labelFor` on the
 * row it sits in, or a `<label for>` in its file. An `id` no label points at leaves the field nameless;
 * a `label` next to such an id is a hidden name that would take over from the visible one (the field
 * drops it, IntegerInput.svelte, but the caller then says two names).
 *
 * Limit: see lib/testing/markup.ts.
 */

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte'));
const uses = callers.flatMap(([path, text]) => usesIn(path, text, 'IntegerInput'));

const attribute = (tag: string, name: string) => new RegExp(String.raw`(?<![\w:-])${name}=("[^"]*"|\{(?:[^{}]|\{[^{}]*\})*\})`).exec(tag)?.[1];

// The id is named by a visible label: the SettingsRow it sits in says `labelFor` with the same
// expression, or a `<label for>` in the file does.
function labelledFor({ tag, row }: Use, text: string): boolean {
  const id = attribute(tag, 'id');
  if (id === undefined) return false;
  const rowTag = /^<SettingsRow\b/.test(row) ? row.slice(0, tagEnd(row, 0)) : '';
  return attribute(rowTag, 'labelFor') === id || text.includes(`for=${id}`);
}

function named(use: Use, text = frontendSources[use.path] ?? ''): boolean {
  const own = attribute(use.tag, 'label') !== undefined;
  return own !== labelledFor(use, text);
}

const at = (row: string, tag: string): Use => ({ path: 'x.svelte', tag, row });

describe('IntegerInput call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBeGreaterThanOrEqual(3);
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<IntegerInput\b/g)?.length ?? 0), 0));
  });

  it('name every field once: with its own label, or by an id a visible label points at', () => {
    const settings = (labelFor: string) =>
      `<SettingsRow label="Threads" labelFor=${labelFor}>{#snippet children({ hintId })}<IntegerInput id="{uid}-threads" />{/snippet}</SettingsRow>`;
    const field = '<IntegerInput id="{uid}-threads" />';
    expect(named(at('<div>x</div>', "<IntegerInput label={msg('a.b')} />"), '')).toBe(true);
    expect(named(at('<div>x</div>', '<IntegerInput label="Frame limit" />'), '')).toBe(true);
    expect(named(at(settings('"{uid}-threads"'), field), '')).toBe(true);
    expect(named(at('<div><label for="{uid}-threads">Threads</label><IntegerInput /></div>', field), '<label for="{uid}-threads">')).toBe(true);
    // An id no label points at: a nameless field.
    expect(named(at(settings('"{uid}-other"'), field), '')).toBe(false);
    expect(named(at('<div>x</div>', field), '')).toBe(false);
    // Neither: a nameless field.
    expect(named(at('<div>x</div>', '<IntegerInput value={4} />'), '')).toBe(false);
    // Both: a hidden name over the visible one.
    expect(named(at(settings('"{uid}-threads"'), '<IntegerInput id="{uid}-threads" label="Threads" />'), '')).toBe(false);
    // A labelFor on another row names nothing here.
    const other = `<div>${settings('"{uid}-threads"')}<IntegerInput id="{uid}-threads" /></div>`;
    expect(usesIn('x.svelte', other, 'IntegerInput').map((use) => named(use, ''))).toEqual([true, false]);

    expect(uses.filter((use) => !named(use)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  // A spread hides whether the caller names the field.
  it('spread nothing onto a number field', () => {
    expect(uses.filter(({ tag }) => /\s\{\s*\.\.\./.test(tag)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });
});
