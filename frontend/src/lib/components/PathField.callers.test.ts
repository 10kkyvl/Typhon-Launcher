import { describe, expect, it } from 'vitest';
import { attributes, hasSpread, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use PathField, read from their markup. The type checker holds a path to one name
 * (PathField.types.test.ts); a spread would hide that from it and from this file, so none is taken. A
 * path the player can type (`editable`) is bound: typed into an unbound field, the path would stay in
 * the field and never reach the caller that installs or moves to it.
 *
 * Limit: see lib/testing/markup.ts.
 */

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'));
const uses = callers.flatMap(([path, text]) => usesIn(path, text, 'PathField'));
const where = ({ path, tag }: Use) => `${path}: ${tag.replace(/\s+/g, ' ')}`;

function namedOnce(tag: string): boolean {
  const own = attributes(tag);
  return own.has('label') !== own.has('labelledby');
}

function typedIsBound(tag: string): boolean {
  const own = attributes(tag);
  return !own.has('editable') || own.get('editable') === '{false}' || own.has('bind:value');
}

describe('PathField call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<PathField\b/g)?.length ?? 0), 0));
  });

  it('name every path once', () => {
    expect(namedOnce('<PathField label="Folder" browseLabel="Browse" onbrowse={pick} />')).toBe(true);
    expect(namedOnce('<PathField labelledby={labelId} browseLabel="Browse" onbrowse={pick} />')).toBe(true);
    expect(namedOnce('<PathField value={path} browseLabel="Browse" onbrowse={pick} />')).toBe(false);
    expect(uses.filter((use) => !namedOnce(use.tag)).map(where)).toEqual([]);
  });

  it('spread nothing onto a path', () => {
    expect(hasSpread('<PathField label="Folder" {...rest} />')).toBe(true);
    expect(uses.filter((use) => hasSpread(use.tag)).map(where)).toEqual([]);
  });

  it('bind the value of a path the player can type', () => {
    expect(typedIsBound('<PathField label="To" editable bind:value={dest} />')).toBe(true);
    expect(typedIsBound('<PathField label="To" {editable} bind:value={dest} />')).toBe(true);
    expect(typedIsBound('<PathField label="To" value={dest} />')).toBe(true);
    expect(typedIsBound('<PathField label="To" editable value={dest} />')).toBe(false);
    expect(typedIsBound('<PathField label="To" editable={typed} value={dest} />')).toBe(false);
    expect(uses.filter((use) => !typedIsBound(use.tag)).map(where)).toEqual([]);
  });
});
