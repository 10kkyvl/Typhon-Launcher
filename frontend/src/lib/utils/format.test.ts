import { describe, expect, it } from 'vitest';
import { makeFormat, truncateMiddle } from './format';
import { translator } from '../i18n';
import type { Locale } from '../i18n';

const MB = 1024 * 1024;
const GB = 1024 ** 3;

const f = (locale: Locale) => makeFormat(locale, translator(locale));
const ru = f('ru');
const en = f('en');

describe('rateLimitLabel', () => {
  it.each([
    [0, 'Без ограничений'],
    [-1, 'Без ограничений'],
    [10 * MB, '10 МБ/с'],
    [Math.round(37.5 * MB), '37,5 МБ/с'],
    [Math.round(0.1 * MB), '0,1 МБ/с'],
    [Math.round(1000 * MB), '1000 МБ/с'],
  ])('formats %i bytes/s as %s in russian', (bytes, expected) => {
    expect(ru.rateLimitLabel(bytes)).toBe(expected);
  });

  it.each([
    [0, 'Unlimited'],
    [10 * MB, '10 MB/s'],
    [Math.round(37.5 * MB), '37.5 MB/s'],
  ])('formats %i bytes/s as %s in english', (bytes, expected) => {
    expect(en.rateLimitLabel(bytes)).toBe(expected);
  });
});

describe('rateMbText', () => {
  it.each([
    [Math.round(2.25 * MB), '2,25'],
    [Math.round(2.256 * MB), '2,26'],
    [3 * MB, '3'],
  ])('renders %i bytes/s as %s in russian', (bytes, expected) => {
    expect(ru.rateMbText(bytes)).toBe(expected);
  });

  it('uses a dot in english', () => {
    expect(en.rateMbText(Math.round(2.25 * MB))).toBe('2.25');
  });
});

describe('bytesSize', () => {
  it.each([
    [512, '512 Б'],
    [2 * 1024, '2 КБ'],
    [5 * MB, '5 МБ'],
    [Math.round(1.5 * GB), '1,5 ГБ'],
  ])('formats %i bytes as %s in russian', (bytes, expected) => {
    expect(ru.bytesSize(bytes)).toBe(expected);
  });

  it.each([
    [512, '512 B'],
    [5 * MB, '5 MB'],
    [Math.round(1.5 * GB), '1.5 GB'],
  ])('formats %i bytes as %s in english', (bytes, expected) => {
    expect(en.bytesSize(bytes)).toBe(expected);
  });
});

describe('playtime', () => {
  it('describes short sessions', () => {
    expect(ru.playtime(0)).toBe('—');
    expect(ru.playtime(30)).toBe('меньше минуты');
    expect(en.playtime(30)).toBe('less than a minute');
  });

  it('describes hours and minutes', () => {
    expect(ru.playtime(3 * 3600 + 25 * 60)).toBe('3 ч 25 мин');
    expect(en.playtime(3 * 3600 + 25 * 60)).toBe('3 h 25 min');
    expect(ru.playtime(40 * 60)).toBe('40 мин');
  });
});

describe('relativeDate', () => {
  const daysAgo = (days: number) => {
    const d = new Date();
    d.setDate(d.getDate() - days);
    return d.toISOString();
  };

  it('names today and yesterday', () => {
    expect(ru.relativeDate(daysAgo(0))).toBe('Сегодня');
    expect(ru.relativeDate(daysAgo(1))).toBe('Вчера');
    expect(en.relativeDate(daysAgo(1))).toBe('Yesterday');
  });

  it('declines days and weeks in russian', () => {
    expect(ru.relativeDate(daysAgo(2))).toBe('2 дня назад');
    expect(ru.relativeDate(daysAgo(5))).toBe('5 дней назад');
    expect(ru.relativeDate(daysAgo(14))).toBe('2 недели назад');
  });

  it('pluralises days and weeks in english', () => {
    expect(en.relativeDate(daysAgo(2))).toBe('2 days ago');
    expect(en.relativeDate(daysAgo(14))).toBe('2 weeks ago');
  });

  it('returns a dash for a missing date', () => {
    expect(ru.relativeDate(null)).toBe('—');
    expect(ru.relativeDate('not a date')).toBe('—');
  });
});

describe('etaLabel', () => {
  it('formats hours and minutes', () => {
    expect(ru.etaLabel(3 * 3600 + 12 * 60)).toBe('3 ч 12 мин');
    expect(en.etaLabel(3 * 3600 + 12 * 60)).toBe('3 h 12 min');
  });

  it('formats minutes and seconds', () => {
    expect(ru.etaLabel(90)).toBe('1 мин 30 сек');
    expect(en.etaLabel(90)).toBe('1 min 30 s');
  });

  it('returns a dash for a negative estimate', () => {
    expect(ru.etaLabel(-1)).toBe('—');
  });
});

describe('formatCount', () => {
  it('groups thousands per locale', () => {
    expect(ru.formatCount(1234)).toBe('1\u00A0234');
    expect(en.formatCount(1234)).toBe('1,234');
  });
});

describe('truncateMiddle', () => {
  it('keeps short text intact', () => {
    expect(truncateMiddle('short', 10)).toBe('short');
  });

  it('elides the middle of long text', () => {
    expect(truncateMiddle('abcdefghij', 5)).toBe('ab…ij');
  });
});
