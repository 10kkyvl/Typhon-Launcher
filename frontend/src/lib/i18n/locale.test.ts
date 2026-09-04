import { describe, expect, it } from 'vitest';
import { resolveLocale } from './locale';

describe('resolveLocale', () => {
  it('honours an explicit choice regardless of the system language', () => {
    expect(resolveLocale('ru', 'en-US')).toBe('ru');
    expect(resolveLocale('en', 'ru-RU')).toBe('en');
  });

  it('follows the system language when the setting is system', () => {
    expect(resolveLocale('system', 'ru-RU')).toBe('ru');
    expect(resolveLocale('system', 'ru')).toBe('ru');
    expect(resolveLocale('system', 'en-GB')).toBe('en');
    expect(resolveLocale('system', 'de-DE')).toBe('en');
  });

  it('falls back to english when the system language is unknown', () => {
    expect(resolveLocale('system', '')).toBe('en');
    expect(resolveLocale('system', undefined)).toBe('en');
  });

  it('treats an unrecognised setting as system', () => {
    expect(resolveLocale('klingon', 'ru-RU')).toBe('ru');
    expect(resolveLocale('', 'en-US')).toBe('en');
  });
});
