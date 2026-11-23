import { describe, expect, it } from 'vitest';
import { attributes, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites disable a SettingsRow's control, read from their markup. A row with
 * `disabledReason` makes nothing inert (SettingsRow.svelte): its `children` snippet takes `disabled`
 * and hands it to every base control in it, at any depth, as `{disabled}` or in an expression that
 * reads it (`disabled={disabled || saving}`), so no live control stays under the reason. `below` is
 * the caller's: it may keep a link that says why.
 *
 * Limit: see lib/testing/markup.ts; a control passed in through a component of the caller's own is not
 * seen.
 */

/** The base controls that take `disabled`. */
const CONTROLS = ['Toggle', 'Select', 'Slider', 'IntegerInput', 'Button', 'IconButton', 'Checkbox', 'SegmentedControl'];

const controlsIn = (path: string, text: string): Use[] => CONTROLS.flatMap((name) => usesIn(path, text, name));

// A control in the `children` of a row with a reason: it must take the `disabled` that snippet took.
function heldOff({ tag, snippet }: Use): boolean {
  if (!snippet || snippet.name !== 'children' || !attributes(snippet.row).has('disabledReason')) return true;
  const local = snippet.bound.disabled;
  const value = attributes(tag).get('disabled');
  return local !== undefined && value !== undefined && /^\{[\s\S]*\}$/.test(value) && new RegExp(String.raw`(?<![\w$.])${local}(?![\w$])`).test(value);
}

const sources = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte'));
const uses = sources.flatMap(([path, text]) => controlsIn(path, text));
const check = (text: string) => controlsIn('x.svelte', text).map(heldOff);

describe('SettingsRow call sites', () => {
  it('reads them', () => {
    const rows = sources.flatMap(([path, text]) => usesIn(path, text, 'SettingsRow'));
    expect(rows.length).toBeGreaterThanOrEqual(10);
    expect(uses.filter((use) => use.snippet !== null).length).toBeGreaterThanOrEqual(10);
  });

  it('hand the row\'s disabled to every base control under a disabled reason', () => {
    const row = (snippet: string, body: string, reason = ' disabledReason={why}') => `<SettingsRow label="L"${reason}>{#snippet children(${snippet})}${body}{/snippet}</SettingsRow>`;
    expect(check(row('{ labelId, disabled }', '<Toggle labelledby={labelId} {disabled} />'))).toEqual([true]);
    expect(check(row('{ disabled }', '<Select disabled={disabled || saving} />'))).toEqual([true]);
    expect(check(row('{ disabled: off }', '<div class="pair"><Slider disabled={off} /><IntegerInput id="x" disabled={off} /></div>'))).toEqual([true, true]);
    // Forgotten, or not the row's.
    expect(check(row('{ labelId }', '<Toggle labelledby={labelId} />'))).toEqual([false]);
    expect(check(row('{ disabled }', '<Toggle />'))).toEqual([false]);
    expect(check(row('{ disabled }', '<Button disabled={saving}>Clear</Button>'))).toEqual([false]);
    expect(check(row('{ disabled }', '<Toggle disabled />'))).toEqual([false]);
    expect(check(row('{ disabled: off }', '<Toggle {disabled} />'))).toEqual([false]);
    expect(check('<SettingsRow label="L" disabledReason={why}>{#snippet children()}<Toggle {disabled} />{/snippet}{#snippet below({ disabled })}<a href="#why">Why</a>{/snippet}</SettingsRow>')).toEqual([false]);
    // Not asked: a row without a reason, the part below, a row of its own inside a disabled row's part below.
    expect(check(row('{ labelId }', '<Toggle labelledby={labelId} />', ''))).toEqual([true]);
    expect(check('<SettingsRow label="L" disabledReason={why}>{#snippet below()}<Button onclick={fix}>Fix</Button>{/snippet}</SettingsRow>')).toEqual([true]);
    expect(check(`<SettingsRow label="Outer" disabledReason={why}>{#snippet below()}${row('{ labelId }', '<Toggle labelledby={labelId} />', '')}{/snippet}</SettingsRow>`)).toEqual([true]);

    expect(uses.filter((use) => !heldOff(use)).map(({ path, tag }) => `${path}: ${tag}`)).toEqual([]);
  });
});
