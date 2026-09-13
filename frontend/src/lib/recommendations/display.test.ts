import { describe, expect, it } from 'vitest';
import { applyLanguage } from '../i18n';
import { genreLabel, recommendationReason, type Recommendation } from './display';

const game = { id: 'game', title: 'Game', sortTitle: 'game', createdAt: '' };
const item = (reason: string, values: Partial<Recommendation> = {}): Recommendation => ({ game, reason, ...values });

describe('recommendation explanations', () => {
  it('does not invent a similar game, genre, or unknown explanation', () => {
    expect(recommendationReason(item('similar'))).toBe('');
    expect(recommendationReason(item('genre'))).toBe('');
    expect(recommendationReason(item('unknown'))).toBe('');
  });
  it('uses the supplied signal and translates genre groups in English', () => {
    applyLanguage('en');
    expect(recommendationReason(item('similar', { reasonTitle: 'Hades' }))).toBe('Similar to Hades');
    expect(genreLabel('Экшен')).toBe('Action');
    expect(recommendationReason(item('unplayed'))).toBe('You haven’t played this yet');
  });
  it('supports Russian without invented match percentages', () => {
    applyLanguage('ru');
    expect(recommendationReason(item('similar', { reasonTitle: 'Hades' }))).toBe('Похожа на Hades');
    expect(recommendationReason(item('favorite'))).toBe('В избранном');
  });
});
