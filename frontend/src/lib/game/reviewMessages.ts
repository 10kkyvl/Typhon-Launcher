import { msg, type MessageKey } from '../i18n';
import type { Limits } from '../services/reviews';

export function reviewFieldMessage(code: string, limits: Limits): string {
  switch (code) {
    case 'review_too_short':
      return msg('reviews.errorTooShort', { min: limits.minBodyRunes });
    case 'review_too_long':
      return msg('reviews.errorTooLong', { max: limits.maxBodyRunes });
    case 'review_low_effort':
      return msg('reviews.errorLowEffort');
    case 'review_links':
      return msg('reviews.errorLinks');
    case 'review_duplicate':
      return msg('reviews.errorDuplicate');
    case 'review_bad_reason':
      return msg('reviews.errorBadReason');
    default:
      return msg('reviews.errorGeneric');
  }
}

const TOAST_KEYS: Partial<Record<string, MessageKey>> = {
  review_not_found: 'reviews.errorNotFound',
  review_own: 'reviews.errorOwn',
  review_account_too_new: 'reviews.errorAccountTooNew',
  review_report_limit: 'reviews.errorReportLimit',
  account_muted: 'reviews.errorMuted',
  unknown_game: 'reviews.errorUnknownGame',
  rate_limited: 'reviews.errorRateLimited',
};

export function reviewToastMessage(code: string, fallback = msg('reviews.errorGeneric')): string {
  const key = TOAST_KEYS[code];
  return key ? msg(key) : fallback;
}
