import { describe, expect, it } from 'vitest';
import { attributes, hasSpread, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use OptionList, read from their markup. The type checker holds a list to one name
 * (OptionList.types.test.ts); a spread would hide that from it and from this file, so none is taken.
 * A list is held one way: bound, or held by the caller through `onchange`; a list given a plain
 * `value` and no `onchange` writes a choice the caller never sees.
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
});
