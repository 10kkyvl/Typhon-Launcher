import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import { locale, translator, type Locale } from './index';
import { ru } from './catalog/ru';
import { en } from './catalog/en';
import { accountMessage } from '../services/accountMessages';
import { frontendSources, goSources } from '../testing/sources';

const CODE = String.raw`[a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+`;

function goErrorCodes(): Map<string, string> {
  const found = new Map<string, string>();
  const patterns = [
    new RegExp(String.raw`uierr\.(?:New|Wrap)\(\s*"(${CODE})"`, 'g'),
    new RegExp(String.raw`\b(?:ErrCode|code)[A-Za-z0-9]*\s*(?::=|=)\s*"(${CODE})"`, 'g'),
  ];
  for (const [path, text] of Object.entries(goSources)) {
    for (const pattern of patterns) {
      for (const match of text.matchAll(pattern)) found.set(match[1], path);
    }
  }
  return found;
}

function frontendLiterals(): Set<string> {
  const found = new Set<string>();
  const pattern = new RegExp(String.raw`(['"` + '`' + String.raw`])(${CODE})\1`, 'g');
  for (const text of Object.values(frontendSources)) {
    for (const match of text.matchAll(pattern)) found.add(match[2]);
  }
  return found;
}

function accountCodes(): Map<string, string> {
  const found = new Map<string, string>();
  const source = goSources['internal/account/errors.go'];
  expect(source, 'internal/account/errors.go').toBeTypeOf('string');
  for (const match of source.matchAll(/\b(Code\w+)\s*=\s*"([a-z_]+)"/g)) found.set(match[2], match[1]);
  return found;
}

function knownAccountCodes(): Set<string> {
  const source = frontendSources['lib/services/account.ts'];
  expect(source, 'lib/services/account.ts').toBeTypeOf('string');
  const start = source.indexOf('const KNOWN_CODES = new Set([');
  expect(start).toBeGreaterThan(-1);
  const end = source.indexOf(']);', start);
  return new Set([...source.slice(start, end).matchAll(/'([a-z_]+)'/g)].map((match) => match[1]));
}

afterEach(() => locale.set('ru'));

describe('backend error codes', () => {
  const codes = goErrorCodes();

  it('finds the codes the backend attaches to user-facing errors', () => {
    expect(codes.size).toBeGreaterThan(300);
  });

  it('has a place in the frontend for every one of them', () => {
    const literals = frontendLiterals();
    const orphaned = [...codes]
      .filter(([code]) => !literals.has(code) && !Object.prototype.hasOwnProperty.call(ru, code))
      .map(([code, path]) => `${code} (${path})`);
    expect(orphaned).toEqual([]);
  });

  it('defines in both languages every code the frontend reads as a message key', () => {
    const keyed = [...codes.keys()].filter((code) => Object.prototype.hasOwnProperty.call(ru, code));
    expect(keyed.length).toBeGreaterThan(40);
    const missing = keyed.filter((code) => !Object.prototype.hasOwnProperty.call(en, code));
    expect(missing).toEqual([]);
  });
});

describe('account error codes', () => {
  const fromGo = accountCodes();
  const known = knownAccountCodes();

  it('finds the codes the account client raises', () => {
    expect(fromGo.size).toBeGreaterThan(15);
  });

  it('lets every one of them through the error contract instead of turning it into a server error', () => {
    const lost = [...fromGo].filter(([code]) => !known.has(code)).map(([code, name]) => `${name} = ${code}`);
    expect(lost).toEqual([]);
  });

  it.each<Locale>(['ru', 'en'])('says something specific for every one of them in %s', (language) => {
    locale.set(language);
    const generic = translator(language)('state.accountErrorGeneric');
    const vague = [...fromGo.keys()].filter((code) => accountMessage(code) === generic);
    expect(vague).toEqual([]);
  });
});
