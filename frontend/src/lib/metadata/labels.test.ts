import { afterEach, describe, expect, it } from 'vitest';
import { applyLanguage } from '../i18n';
import { genreLabel, themeLabel } from './labels';
import { metaLine, tagList } from '../game/view';
import { recommendationReason } from '../recommendations/display';

afterEach(() => applyLanguage('ru'));

describe('provider labels in game details and recommendations', () => {
  it('translates metadata tags and deduplicates aliases after translation', () => {
    applyLanguage('ru');
    expect(metaLine({ year: 2026, genres: ['Racing'] })).toEqual(['2026', 'Гонки']);
    expect(tagList([['Simulation', 'Simulator'].map(genreLabel), ['Open world', 'Science fiction'].map(themeLabel)]))
      .toEqual(['Симуляторы', 'Открытый мир', 'Научная фантастика']);
    expect(recommendationReason({ game: { id: 'g', title: 'Game', sortTitle: '', createdAt: '' }, reason: 'genre', reasonGenre: 'Puzzle' }))
      .toBe('Вам нравится жанр «Головоломки»');
  });
  it('handles saved Russian labels and case differences when switching to English', () => {
    applyLanguage('en');
    expect(genreLabel('Симуляторы')).toBe('Simulator');
    expect(genreLabel('  SPORTS  ')).toBe('Sport');
    expect(themeLabel('Открытый мир')).toBe('Open world');
  });
  it('preserves unknown provider labels', () => {
    expect(genreLabel('constructor')).toBe('constructor');
    expect(themeLabel('__proto__')).toBe('__proto__');
    expect(genreLabel('User genre')).toBe('User genre');
    expect(themeLabel('User theme')).toBe('User theme');
  });
});
