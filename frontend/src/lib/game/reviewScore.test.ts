import { describe, expect, it } from 'vitest';
import { reviewScore, reviewScoreBadgeKind, reviewScoreTier } from './reviewScore';

describe('reviewScoreTier boundaries', () => {
  it('total = 0 has no score', () => {
    expect(reviewScoreTier(0, 0)).toBe('none');
  });

  it('a single review: 100% vs 0%', () => {
    expect(reviewScoreTier(1, 1)).toBe('positive');
    expect(reviewScoreTier(1, 0)).toBe('negative');
  });

  it('49 vs 50 total at 80%+ ratio: positive tier needs total >= 50', () => {
    expect(reviewScoreTier(49, 40)).toBe('positive');
    expect(reviewScoreTier(50, 40)).toBe('veryPositive');
  });

  it('499 vs 500 total at <20% ratio: overwhelming tier needs total >= 500', () => {
    expect(reviewScoreTier(499, 50)).toBe('veryNegative');
    expect(reviewScoreTier(500, 50)).toBe('overwhelminglyNegative');
  });

  it('94.9% vs 95.0% with total >= 500', () => {
    expect(reviewScoreTier(1000, 949)).toBe('veryPositive');
    expect(reviewScoreTier(1000, 950)).toBe('overwhelminglyPositive');
  });

  it('79% vs 80%', () => {
    expect(reviewScoreTier(100, 79)).toBe('mostlyPositive');
    expect(reviewScoreTier(100, 80)).toBe('veryPositive');
  });

  it('69% vs 70%', () => {
    expect(reviewScoreTier(100, 69)).toBe('mixed');
    expect(reviewScoreTier(100, 70)).toBe('mostlyPositive');
  });

  it('39% vs 40%', () => {
    expect(reviewScoreTier(100, 39)).toBe('mostlyNegative');
    expect(reviewScoreTier(100, 40)).toBe('mixed');
  });

  it('19% vs 20%', () => {
    expect(reviewScoreTier(100, 19)).toBe('veryNegative');
    expect(reviewScoreTier(100, 20)).toBe('mostlyNegative');
  });
});

describe('reviewScore', () => {
  it('rounds the display percent independently of the classification ratio', () => {
    const justUnder = reviewScore(1000, 949);
    expect(justUnder.tier).toBe('veryPositive');
    expect(justUnder.percent).toBe(95);

    const atThreshold = reviewScore(1000, 950);
    expect(atThreshold.tier).toBe('overwhelminglyPositive');
    expect(atThreshold.percent).toBe(95);
  });

  it('returns a non-empty label and the matching tone for every tier', () => {
    const cases: [number, number, string][] = [
      [0, 0, 'none'],
      [1000, 950, 'positive'],
      [100, 80, 'positive'],
      [100, 79, 'positive'],
      [100, 50, 'mixed'],
      [100, 30, 'negative'],
      [100, 10, 'negative'],
    ];
    for (const [total, positive, tone] of cases) {
      const score = reviewScore(total, positive);
      expect(score.label.length).toBeGreaterThan(0);
      expect(score.tone).toBe(tone);
    }
  });

  it('none has 0% and an empty tone', () => {
    expect(reviewScore(0, 0)).toEqual({ tier: 'none', label: 'Нет отзывов', percent: 0, tone: 'none' });
  });
});

describe('reviewScoreBadgeKind', () => {
  it('maps tones to StatusBadge kinds', () => {
    expect(reviewScoreBadgeKind('positive')).toBe('accent');
    expect(reviewScoreBadgeKind('mixed')).toBe('warning');
    expect(reviewScoreBadgeKind('negative')).toBe('danger');
    expect(reviewScoreBadgeKind('none')).toBe('neutral');
  });
});
