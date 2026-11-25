import { describe, expect, it } from 'vitest';
import { attributes, hasSpread, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use OptionList, read from their markup. The type checker holds a list to one name
 * (OptionList.types.test.ts); a spread would hide that from it and from this file, so none is taken.
 * A list is held one way: bound, or held by the caller through `onchange`; a list given a plain
 * `value` and no `onchange` writes a choice the caller never sees, and with `onchange` the list never
 * writes a bound value (OptionList.svelte), so both on one list leave the binding unwritten. A list
 * with `activation="manual"` is for a caller that applies the choice in `onchange`.
 *
 * Limit: see lib/testing/markup.ts.
 */

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'));
const uses = callers.flatMap(([path, text]) => usesIn(path, text, 'OptionList'));
const where = ({ path, tag }: Use) => `${path}: ${tag.replace(/\s+/g, ' ')}`;

function namedOnce(tag: string): boolean {
  const own = attributes(tag);
  return own.has('label') !== own.has('labelledby');
}

function heldByCaller(tag: string): boolean {
  const own = attributes(tag);
  return own.has('bind:value') || own.has('onchange');
}

describe('OptionList call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<OptionList\b/g)?.length ?? 0), 0));
  });

  it('name every list once', () => {
    expect(namedOnce('<OptionList label="Folder" {options} bind:value />')).toBe(true);
    expect(namedOnce('<OptionList labelledby={hintId} {options} bind:value />')).toBe(true);
    expect(namedOnce('<OptionList {options} bind:value />')).toBe(false);
    expect(uses.filter((use) => !namedOnce(use.tag)).map(where)).toEqual([]);
  });

  it('spread nothing onto a list', () => {
    expect(hasSpread('<OptionList label="Folder" {...rest} />')).toBe(true);
    expect(uses.filter((use) => hasSpread(use.tag)).map(where)).toEqual([]);
  });

  it('hold the choice: bound, or through onchange', () => {
    expect(heldByCaller('<OptionList label="F" {options} bind:value={chosen} />')).toBe(true);
    expect(heldByCaller('<OptionList label="F" {options} value={chosen} onchange={pick} />')).toBe(true);
    expect(heldByCaller('<OptionList label="F" {options} value={chosen} />')).toBe(false);
    expect(uses.filter((use) => !heldByCaller(use.tag)).map(where)).toEqual([]);
  });

  it('bind the value or hold it with onchange, never both', () => {
    expect(bothWays('<OptionList label="F" {options} bind:value={chosen} onchange={pick} />')).toBe(true);
    expect(bothWays('<OptionList label="F" {options} bind:value {onchange} />')).toBe(true);
    expect(bothWays('<OptionList label="F" {options} value={chosen} {onchange} />')).toBe(false);
    expect(bothWays('<OptionList label="F" {options} bind:value={chosen} />')).toBe(false);
    expect(uses.filter((use) => bothWays(use.tag)).map(where)).toEqual([]);
  });

  it('apply a manual list\'s choice in onchange', () => {
    expect(manualUnapplied('<OptionList label="F" {options} activation="manual" bind:value={chosen} />')).toBe(true);
    expect(manualUnapplied('<OptionList label="F" {options} activation="manual" value={chosen} />')).toBe(true);
    expect(manualUnapplied('<OptionList label="F" {options} activation="manual" value={chosen} onchange={apply} />')).toBe(false);
    expect(manualUnapplied('<OptionList label="F" {options} activation="manual" value={chosen} {onchange} />')).toBe(false);
    expect(manualUnapplied('<OptionList label="F" {options} bind:value={chosen} />')).toBe(false);
    expect(manualUnapplied('<OptionList label="F" {options} activation="auto" bind:value={chosen} />')).toBe(false);
    expect(manualUnapplied('<OptionList label="F" {options} activation={mode} bind:value={chosen} />')).toBe(true);
    const unapplied = uses.filter((use) => manualUnapplied(use.tag)).map(where);
    expect(unapplied, 'activation="manual" chooses only on Space, Enter or a press, for a caller that applies the choice at once: give it value and onchange, or drop activation for a bound list').toEqual([]);
  });
});

function bothWays(tag: string): boolean {
  const own = attributes(tag);
  return own.has('bind:value') && own.has('onchange');
}

function manualUnapplied(tag: string): boolean {
  const own = attributes(tag);
  const activation = own.get('activation');
  return activation !== undefined && activation !== '"auto"' && !own.has('onchange');
}
