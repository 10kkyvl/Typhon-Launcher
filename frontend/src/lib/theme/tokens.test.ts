import { describe, expect, it } from 'vitest';
import { ru } from '../i18n/catalog/ru';
import { en } from '../i18n/catalog/en';
import { goSources } from '../testing/sources';
import {
  groupTitleKey,
  groupedTokenNames,
  isHexColor,
  tokenGroups,
  tokenHintKey,
  tokenLabelKey,
} from './tokens';

function goTokens(): string[] {
  const source = goSources['internal/theme/tokens.go'];
  expect(source, 'internal/theme/tokens.go').toBeTypeOf('string');
  const block = /var allowedTokens = \[\]Token\{([\s\S]*?)\n\}/.exec(source)?.[1];
  expect(block, 'allowedTokens block').toBeTypeOf('string');
  return [...(block as string).matchAll(/\{"(--[a-z0-9-]+)",\s*Kind\w+\}/g)].map((match) => match[1]);
}

function has(catalog: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(catalog, key);
}

describe('theme token labels', () => {
  const tokens = goTokens();

  it('reads the token list from Go', () => {
    expect(tokens.length).toBeGreaterThan(40);
    expect(tokens).toContain('--surface');
  });

  it('puts every Go token into exactly one editor group', () => {
    const listed = tokenGroups.flatMap((group) => [...group.tokens]);
    expect(listed.filter((name, index) => listed.indexOf(name) !== index)).toEqual([]);
    expect(tokens.filter((name) => !groupedTokenNames.has(name))).toEqual([]);
    expect(listed.filter((name) => !tokens.includes(name))).toEqual([]);
  });

  it.each([
    ['ru', ru],
    ['en', en],
  ] as const)('has a %s label and hint for every Go token', (_name, catalog) => {
    const missing = tokens.flatMap((name) =>
      [tokenLabelKey(name), tokenHintKey(name)].filter((key) => !has(catalog, key)),
    );
    expect(missing).toEqual([]);
  });

  it.each([
    ['ru', ru],
    ['en', en],
  ] as const)('keeps no %s label for a token Go does not have', (_name, catalog) => {
    const known = new Set(tokens.flatMap((name) => [tokenLabelKey(name), tokenHintKey(name)] as string[]));
    const stale = Object.keys(catalog).filter(
      (key) => /^settings\.themeToken(Hint)?\./.test(key) && !known.has(key),
    );
    expect(stale).toEqual([]);
  });

  it.each([
    ['ru', ru],
    ['en', en],
  ] as const)('has a %s title for every group', (_name, catalog) => {
    const ids = [...tokenGroups.map((group) => group.id), 'other' as const];
    expect(ids.filter((id) => !has(catalog, groupTitleKey(id)))).toEqual([]);
  });

  it('never shows the css variable name as the label', () => {
    for (const name of tokens) {
      expect(ru[tokenLabelKey(name)]).not.toContain('--');
      expect(en[tokenLabelKey(name)]).not.toContain('--');
    }
  });
});

describe('isHexColor', () => {
  it('accepts only the six digit form the color input understands', () => {
    expect(isHexColor('#a1b2c3')).toBe(true);
    expect(isHexColor('#abc')).toBe(false);
    expect(isHexColor('rgba(255, 255, 255, 0.07)')).toBe(false);
  });
});
