import { describe, expect, it } from 'vitest';
import { ru } from './catalog/ru';
import { en } from './catalog/en';
import type { Message } from './types';

const keys = Object.keys(ru) as (keyof typeof ru)[];

function placeholders(message: Message): string[] {
  const texts = typeof message === 'string' ? [message] : Object.values(message);
  const found = new Set<string>();
  for (const text of texts) {
    for (const match of (text ?? '').matchAll(/\{(\w+)\}/g)) found.add(match[1]);
  }
  return [...found].sort();
}

const cyrillic = /[а-яА-ЯёЁ]/;

function texts(message: Message): string[] {
  return typeof message === 'string' ? [message] : (Object.values(message) as string[]);
}

describe('catalog', () => {
  it('covers every russian key in english', () => {
    const missing = keys.filter((key) => en[key] === undefined);
    expect(missing).toEqual([]);
  });

  it('carries no cyrillic in the english catalog', () => {
    const leaked = keys.filter((key) => texts(en[key]).some((text) => cyrillic.test(text)));
    expect(leaked).toEqual([]);
  });

  it('uses the same interpolation parameters in both languages', () => {
    const mismatched = keys
      .map((key) => ({ key, ru: placeholders(ru[key]), en: placeholders(en[key]) }))
      .filter((entry) => entry.ru.join() !== entry.en.join());
    expect(mismatched).toEqual([]);
  });

  it('gives russian plurals the one, few and many forms', () => {
    const incomplete = keys.filter((key) => {
      const message = ru[key];
      if (typeof message === 'string') return false;
      return !('one' in message && 'few' in message && 'many' in message);
    });
    expect(incomplete).toEqual([]);
  });

  it('gives english plurals the one and other forms', () => {
    const incomplete = keys.filter((key) => {
      const message = en[key];
      if (typeof message === 'string') return false;
      return !('one' in message && 'other' in message);
    });
    expect(incomplete).toEqual([]);
  });

  it('keeps a plural key plural in both languages', () => {
    const shapes = keys.filter((key) => typeof ru[key] !== typeof en[key]);
    expect(shapes).toEqual([]);
  });
});
