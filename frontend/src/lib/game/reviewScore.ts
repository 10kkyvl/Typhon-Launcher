import { msg } from '../i18n';

export type ReviewScoreTier =
  | 'overwhelminglyPositive'
  | 'veryPositive'
  | 'positive'
  | 'mostlyPositive'
  | 'mixed'
  | 'mostlyNegative'
  | 'negative'
  | 'veryNegative'
  | 'overwhelminglyNegative'
  | 'none';

export type ReviewScoreTone = 'positive' | 'mixed' | 'negative' | 'none';

export interface ReviewScore {
  tier: ReviewScoreTier;
  label: string;
  percent: number;
  tone: ReviewScoreTone;
}

const TONE_BY_TIER: Record<ReviewScoreTier, ReviewScoreTone> = {
  overwhelminglyPositive: 'positive',
  veryPositive: 'positive',
  positive: 'positive',
  mostlyPositive: 'positive',
  mixed: 'mixed',
  mostlyNegative: 'negative',
  negative: 'negative',
  veryNegative: 'negative',
  overwhelminglyNegative: 'negative',
  none: 'none',
};

const LABEL_BY_TIER: Record<ReviewScoreTier, () => string> = {
  overwhelminglyPositive: () => msg('reviews.scoreOverwhelminglyPositive'),
  veryPositive: () => msg('reviews.scoreVeryPositive'),
  positive: () => msg('reviews.scorePositive'),
  mostlyPositive: () => msg('reviews.scoreMostlyPositive'),
  mixed: () => msg('reviews.scoreMixed'),
  mostlyNegative: () => msg('reviews.scoreMostlyNegative'),
  negative: () => msg('reviews.scoreNegative'),
  veryNegative: () => msg('reviews.scoreVeryNegative'),
  overwhelminglyNegative: () => msg('reviews.scoreOverwhelminglyNegative'),
  none: () => msg('reviews.scoreNone'),
};

// Пороги — по точной доле, не по округлённой для показа: 94.9% не должно стать «Крайне положительными».
export function reviewScoreTier(total: number, positive: number): ReviewScoreTier {
  if (!Number.isFinite(total) || total <= 0) return 'none';
  const ratio = (positive / total) * 100;
  if (ratio >= 95 && total >= 500) return 'overwhelminglyPositive';
  if (ratio >= 80 && total >= 50) return 'veryPositive';
  if (ratio >= 80) return 'positive';
  if (ratio >= 70) return 'mostlyPositive';
  if (ratio >= 40) return 'mixed';
  if (ratio >= 20) return 'mostlyNegative';
  if (total >= 500) return 'overwhelminglyNegative';
  if (total >= 50) return 'veryNegative';
  return 'negative';
}

export function reviewScore(total: number, positive: number): ReviewScore {
  const tier = reviewScoreTier(total, positive);
  const percent = total > 0 ? Math.round((positive / total) * 100) : 0;
  return { tier, label: LABEL_BY_TIER[tier](), percent, tone: TONE_BY_TIER[tier] };
}

export function reviewScoreBadgeKind(tone: ReviewScoreTone): 'accent' | 'warning' | 'danger' | 'neutral' {
  switch (tone) {
    case 'positive':
      return 'accent';
    case 'mixed':
      return 'warning';
    case 'negative':
      return 'danger';
    default:
      return 'neutral';
  }
}
