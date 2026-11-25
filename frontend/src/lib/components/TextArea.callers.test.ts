import { describe, expect, it } from 'vitest';
import { attributes, hasSpread, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use TextArea, read from their markup. The type checker holds a field to one name
 * (TextArea.types.test.ts); a spread would hide that from it and from this file, so none is taken.
 *
 * Limit: see lib/testing/markup.ts.
 */

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'));
const uses = callers.flatMap(([path, text]) => usesIn(path, text, 'TextArea'));
const where = ({ path, tag }: Use) => `${path}: ${tag.replace(/\s+/g, ' ')}`;

function namedOnce(tag: string): boolean {
  const own = attributes(tag);
  return own.has('label') !== own.has('labelledby');
}

describe('TextArea call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<TextArea\b/g)?.length ?? 0), 0));
  });

  it('name every field once', () => {
    expect(namedOnce('<TextArea label="Details" />')).toBe(true);
    expect(namedOnce('<TextArea labelledby={labelId} />')).toBe(true);
    expect(namedOnce('<TextArea value="x" />')).toBe(false);
    expect(uses.filter((use) => !namedOnce(use.tag)).map(where)).toEqual([]);
  });

  it('spread nothing onto a field', () => {
    expect(hasSpread('<TextArea label="Details" {...rest} />')).toBe(true);
    expect(uses.filter((use) => hasSpread(use.tag)).map(where)).toEqual([]);
  });
});
