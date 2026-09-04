import { describe, expect, it } from 'vitest';
import { translate } from './translate';
import type { Message } from './types';

const ru = {
  'common.ok': 'Готово',
  'library.title': 'Библиотека',
  'friends.greeting': 'Привет, {name}',
  'library.count': { one: '{count} игра', few: '{count} игры', many: '{count} игр' },
  'downloads.total': '{count} файлов',
} satisfies Record<string, Message>;

const en = {
  'common.ok': 'Done',
  'library.title': 'Library',
  'friends.greeting': 'Hi, {name}',
  'library.count': { one: '{count} game', other: '{count} games' },
  'downloads.total': '{count} files',
} satisfies Record<string, Message>;

const catalogs = { ru, en };

const t = (locale: 'ru' | 'en', key: keyof typeof ru, params?: Record<string, string | number>) =>
  translate(catalogs, locale, key, params);

describe('translate', () => {
  it('returns the message for the active locale', () => {
    expect(t('ru', 'library.title')).toBe('Библиотека');
    expect(t('en', 'library.title')).toBe('Library');
  });

  it('interpolates named parameters', () => {
    expect(t('ru', 'friends.greeting', { name: 'Егор' })).toBe('Привет, Егор');
    expect(t('en', 'friends.greeting', { name: 'Egor' })).toBe('Hi, Egor');
  });

  it('formats numeric parameters for the locale', () => {
    expect(t('ru', 'downloads.total', { count: 1234 })).toBe('1\u00A0234 файлов');
    expect(t('en', 'downloads.total', { count: 1234 })).toBe('1,234 files');
  });

  it('selects russian plural forms', () => {
    expect(t('ru', 'library.count', { count: 1 })).toBe('1 игра');
    expect(t('ru', 'library.count', { count: 2 })).toBe('2 игры');
    expect(t('ru', 'library.count', { count: 5 })).toBe('5 игр');
    expect(t('ru', 'library.count', { count: 21 })).toBe('21 игра');
    expect(t('ru', 'library.count', { count: 112 })).toBe('112 игр');
  });

  it('selects english plural forms', () => {
    expect(t('en', 'library.count', { count: 1 })).toBe('1 game');
    expect(t('en', 'library.count', { count: 2 })).toBe('2 games');
    expect(t('en', 'library.count', { count: 0 })).toBe('0 games');
  });

  it('falls back to another plural form when the exact one is absent', () => {
    const sparse = { ru: { 'x.y': { other: '{count} шт' } }, en: { 'x.y': { other: '{count} pcs' } } };
    expect(translate(sparse, 'ru', 'x.y', { count: 3 })).toBe('3 шт');
  });

  it('falls back to russian when the english message is missing', () => {
    const partial = { ru, en: { ...en, 'library.title': undefined } as unknown as typeof en };
    expect(translate(partial, 'en', 'library.title')).toBe('Библиотека');
  });

  it('returns the key when no catalog has the message', () => {
    expect(translate(catalogs, 'ru', 'nope.missing' as keyof typeof ru)).toBe('nope.missing');
  });

  it('leaves unknown placeholders untouched', () => {
    expect(t('ru', 'friends.greeting')).toBe('Привет, {name}');
  });
});
