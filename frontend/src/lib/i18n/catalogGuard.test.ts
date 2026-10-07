import { describe, expect, it } from 'vitest';
import { ru } from './catalog/ru';
import { en } from './catalog/en';
import { GAME_STATUSES } from '../game/status';
import { keyCalls, secondArgument } from '../testing/scan';
import { frontendSources } from '../testing/sources';
import type { Message } from './types';

const catalogs = { ru, en } as const;
const ruKeys = Object.keys(ru);
const enKeys = Object.keys(en);

function forms(message: Message): string[] {
  return typeof message === 'string' ? [message] : (Object.values(message) as string[]);
}

function placeholders(text: string): string[] {
  return [...text.matchAll(/\{(\w+)\}/g)].map((match) => match[1]);
}

function stripPlaceholders(text: string): string {
  return text.replace(/\{\w+\}/g, '');
}

function has(catalog: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(catalog, key);
}

const used = keyCalls(frontendSources, ['$t', 't', 'msg', 'bp']);

describe('catalog parity', () => {
  it('has no english key that russian lacks', () => {
    const ruSet = new Set(ruKeys);
    expect(enKeys.filter((key) => !ruSet.has(key))).toEqual([]);
  });

  it('has no russian key that english lacks', () => {
    const enSet = new Set(enKeys);
    expect(ruKeys.filter((key) => !enSet.has(key))).toEqual([]);
  });

  it('has the same number of keys in both languages', () => {
    expect(enKeys.length).toBe(ruKeys.length);
  });
});

describe.each(Object.entries(catalogs))('%s catalog text', (_name, catalog) => {
  const entries = Object.entries(catalog as Record<string, Message>);

  it('has no empty message or empty plural form', () => {
    const empty = entries.filter(([, message]) => {
      const texts = forms(message);
      return texts.length === 0 || texts.some((text) => typeof text !== 'string' || text.trim() === '');
    });
    expect(empty.map(([key]) => key)).toEqual([]);
  });

  it('leaves no brace that the interpolation would skip', () => {
    const broken = entries.filter(([, message]) =>
      forms(message).some((text) => /[{}]/.test(stripPlaceholders(text))),
    );
    expect(broken.map(([key]) => key)).toEqual([]);
  });

  it('keeps every plural form on the same named parameters', () => {
    const uneven = entries.filter(([, message]) => {
      if (typeof message === 'string') return false;
      const named = forms(message).map((text) =>
        [...new Set(placeholders(text).filter((name) => name !== 'count'))].sort().join(),
      );
      return new Set(named).size > 1;
    });
    expect(uneven.map(([key]) => key)).toEqual([]);
  });
});

describe('interpolation parameters per key', () => {
  it('uses the same named parameters in russian and english, form by form', () => {
    const mismatched: string[] = [];
    for (const key of ruKeys) {
      const left = ru[key as keyof typeof ru] as Message;
      const right = en[key as keyof typeof en] as Message | undefined;
      if (right === undefined) continue;
      const collect = (message: Message) =>
        [...new Set(forms(message).flatMap(placeholders))].sort().join();
      if (collect(left) !== collect(right)) mismatched.push(key);
    }
    expect(mismatched).toEqual([]);
  });
});

describe('keys used by the interface', () => {
  it('finds the calls it is meant to guard', () => {
    expect(used.length).toBeGreaterThan(300);
  });

  it('resolves every literal key in the russian catalog', () => {
    const missing = used.filter(({ key }) => !has(ru, key));
    expect(missing.map(({ file, key }) => `${file}: ${key}`)).toEqual([]);
  });

  it('resolves every literal key in the english catalog', () => {
    const missing = used.filter(({ key }) => !has(en, key));
    expect(missing.map(({ file, key }) => `${file}: ${key}`)).toEqual([]);
  });

  it('resolves every other literal that names a catalog namespace', () => {
    const namespaces = new Set(ruKeys.map((key) => key.split('.')[0]));
    const fileLike = /\.(json|ts|js|svelte|css|png|jpe?g|svg|webp|gif|ico|exe|txt|md|html)$/;
    const strays: string[] = [];
    for (const [file, text] of Object.entries(frontendSources)) {
      for (const match of text.matchAll(/(['"`])([a-z][A-Za-z0-9]*\.[A-Za-z0-9_.]+)\1(\s*:)?/g)) {
        const literal = match[2];
        if (match[3] || fileLike.test(literal)) continue;
        if (!namespaces.has(literal.split('.')[0])) continue;
        if (!has(ru, literal) || !has(en, literal)) strays.push(`${file}: ${literal}`);
      }
    }
    expect(strays).toEqual([]);
  });

  it('passes a count to every plural message it renders', () => {
    const plural = new Set(
      Object.entries(ru)
        .filter(([, message]) => typeof message !== 'string')
        .map(([key]) => key),
    );
    const bare = used.filter(({ key, args }) => {
      if (!plural.has(key)) return false;
      const params = secondArgument(args, key);
      if (params === null) return true;
      return params.startsWith('{') && !/(^|\W)count(\W|$)/.test(params);
    });
    expect(bare.map(({ file, key }) => `${file}: ${key}`)).toEqual([]);
  });

  it('defines a big picture label for every game status the picker offers', () => {
    const keys = GAME_STATUSES.map((value) => `bp.game.status${value[0].toUpperCase()}${value.slice(1)}`);
    for (const key of keys) {
      expect(has(ru, key), `ru ${key}`).toBe(true);
      expect(has(en, key), `en ${key}`).toBe(true);
    }
  });
});
