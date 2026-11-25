import { describe, expect, it } from 'vitest';
import { attributes, hasSpread, usesIn, type Use } from '../testing/markup';
import { frontendSources } from '../testing/sources';

/**
 * How the call sites use TextField, read from their markup. The type checker holds a field to one name
 * (TextField.types.test.ts); a spread would hide that from it and from this file, so none is taken. A
 * password field says what a password manager should fill (`current-password` or `new-password`):
 * left out, the manager guesses, and a sign-up form gets a saved password offered instead of a new one.
 *
 * Limit: see lib/testing/markup.ts.
 */

const callers = Object.entries(frontendSources).filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'));
const uses = callers.flatMap(([path, text]) => usesIn(path, text, 'TextField'));
const where = ({ path, tag }: Use) => `${path}: ${tag.replace(/\s+/g, ' ')}`;

function namedOnce(tag: string): boolean {
  const own = attributes(tag);
  return own.has('label') !== own.has('labelledby');
}

// A password field, by a literal type or one that can turn to it (a reveal toggle).
const password = (tag: string) => /password/.test(attributes(tag).get('type') ?? '');
const saysWhatToFill = (tag: string) => /^["{].*(current-password|new-password)/.test(attributes(tag).get('autocomplete') ?? '');

describe('TextField call sites', () => {
  it('reads them', () => {
    expect(uses.length).toBe(callers.reduce((count, [, text]) => count + (text.match(/<TextField\b/g)?.length ?? 0), 0));
  });

  it('name every field once', () => {
    expect(namedOnce('<TextField label="E-mail" />')).toBe(true);
    expect(namedOnce('<TextField labelledby={labelId} />')).toBe(true);
    expect(namedOnce('<TextField value="x" />')).toBe(false);
    expect(namedOnce('<TextField label="E-mail" labelledby={labelId} />')).toBe(false);
    expect(uses.filter((use) => !namedOnce(use.tag)).map(where)).toEqual([]);
  });

  it('spread nothing onto a field', () => {
    expect(hasSpread('<TextField label="E-mail" {...rest} />')).toBe(true);
    expect(uses.filter((use) => hasSpread(use.tag)).map(where)).toEqual([]);
  });

  it('tell a password manager which password a password field takes', () => {
    expect(saysWhatToFill('<TextField label="P" type="password" autocomplete="new-password" />')).toBe(true);
    expect(saysWhatToFill("<TextField label=\"P\" type={shown ? 'text' : 'password'} autocomplete={isRegister ? 'new-password' : 'current-password'} />")).toBe(true);
    expect(saysWhatToFill('<TextField label="P" type="password" />')).toBe(false);
    expect(saysWhatToFill('<TextField label="P" type="password" autocomplete="off" />')).toBe(false);
    expect(uses.filter((use) => password(use.tag) && !saysWhatToFill(use.tag)).map(where)).toEqual([]);
  });
});
