import { describe, expect, it } from 'vitest';
import { fromSettingsRow, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use Select, read from their markup. With `onchange` the caller holds the value and
 * Select never writes it (Select.svelte), so a `bind:value` next to `onchange` would never be written:
 * such a caller sets its own state in `onchange`.
 *
 * Limit: see lib/testing/markup.ts; the row of a use is the element it sits in.
 */

// A screen reader announces an unnamed field as a bare combobox; see the test below for the ways a
// field is named. A `<label for>` and an element with the `labelledby` id are looked for in the whole
// file (the settings cards put the label in a column beside the field's own box); the id a SettingsRow
// hands its snippet only in that row, as Toggle's guard reads it.
function named({ path, tag, row }: Use, text: string): boolean {
  if (/\blabel=\{msg\(/.test(tag)) return true;
  const id = /\bid=("[^"]*"|\{[^}]*\})/.exec(tag)?.[1];
  if (id && text.includes(`for=${id}`)) return true;
  const by = /\blabelledby=("[^"]*"|\{[^}]*\})/.exec(tag)?.[1];
  if (!by) return path === 'lib/components/RateLimitInput.svelte' && /\{labelledby\}/.test(tag);
  return (by !== '{labelId}' && text.includes(`id=${by}`)) || fromSettingsRow(row, 'labelId', by);
}

function uses(name: string): Use[] {
  return Object.entries(frontendSources)
    .filter(([path]) => path.endsWith('.svelte'))
    .flatMap(([path, text]) => usesIn(path, text, name));
}

describe('Select call sites', () => {
  it('reads them', () => {
    expect(uses('Select').length).toBeGreaterThanOrEqual(25);
  });

  // A screen reader announces an unnamed field as a bare combobox. A visible label next to the field
  // names it through `labelledby` (the same id expression is on an element of that file) or through a
  // `<label for>` matching the field's `id` (the same expression in `for=` in that file); `label` is for
  // a field with no visible label; RateLimitInput passes its caller's `labelledby` on. An `id` alone,
  // without such a `<label for>`, names nothing.
  it('name every field: from the visible label next to it, or with label where none is shown', () => {
    const unnamed = [...uses('Select'), ...uses('RateLimitInput')].filter((use) => !named(use, frontendSources[use.path] ?? ''));
    expect(unnamed.map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  it('take the label id only inside the SettingsRow that hands it', () => {
    const outside = '<script lang="ts"></script>\n<SettingsRow label="Sound">{#snippet children({ labelId, hintId })}<Toggle labelledby={labelId} />{/snippet}</SettingsRow>\n<Select labelledby={labelId} />';
    const inside = '<SettingsRow label="Language">{#snippet children({ labelId })}<Select labelledby={labelId} />{/snippet}</SettingsRow>';
    expect(usesIn('x.svelte', outside, 'Select').map((use) => named(use, outside))).toEqual([false]);
    expect(usesIn('x.svelte', inside, 'Select').map((use) => named(use, inside))).toEqual([true]);
    const label = '<div class="row"><span id="lang">Language</span><Select labelledby="lang" /></div>';
    expect(usesIn('x.svelte', label, 'Select').map((use) => named(use, label))).toEqual([true]);
  });

  it('take the label id a SettingsRow hands its snippet', () => {
    const row = '<SettingsRow label="Language">{#snippet children({ labelId, hintId })}<Select labelledby={labelId} />{/snippet}</SettingsRow>';
    expect(fromSettingsRow(row, 'labelId', '{labelId}')).toBe(true);
    expect(fromSettingsRow(row.replace('labelId, ', ''), 'labelId', '{labelId}')).toBe(false);
    expect(fromSettingsRow('<div>{#snippet children({ labelId })}<Select labelledby={labelId} />{/snippet}</div>', 'labelId', '{labelId}')).toBe(false);
  });

  it('bind the value or hold it with onchange, never both', () => {
    expect(bothWays('<Select bind:value={x} onchange={save} />')).toBe(true);
    expect(bothWays('<Select bind:value={x} {onchange} />')).toBe(true);
    expect(bothWays('<Select value={x} {onchange} />')).toBe(false);
    expect(bothWays('<Select bind:value={x} {options} />')).toBe(false);
    const both = uses('Select').filter(({ tag }) => bothWays(tag));
    expect(both.map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });

  // A spread hides which of the two ways a caller took, and whether it names the field.
  it('spread nothing onto a select', () => {
    expect(spreads('<Select {...props} />')).toBe(true);
    expect(spreads('<Select options={[...a, ...b]} value={x} />')).toBe(false);
    const spread = uses('Select').filter(({ path, tag }) => spreads(tag) && !SPREAD_ALLOWED.includes(path));
    expect(spread.map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });
});

const SPREAD_ALLOWED: string[] = [];

function bothWays(tag: string): boolean {
  return /\bbind:value\b/.test(tag) && /(\bonchange=|\{\s*onchange\s*\})/.test(tag);
}

function spreads(tag: string): boolean {
  return /\s\{\s*\.\.\./.test(tag);
}
