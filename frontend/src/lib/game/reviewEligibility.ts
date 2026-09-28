import type { MessageKey, Params } from '../i18n';
import type { Eligibility } from '../services/reviews';

export interface RetryText {
  key: MessageKey;
  params?: Params;
}

const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export function retryText(retryAt: string, now = Date.now()): RetryText {
  const target = Date.parse(retryAt);
  if (!retryAt || Number.isNaN(target)) return { key: 'reviews.retryLater' };
  const seconds = Math.round((target - now) / 1000);
  if (seconds <= 60) return { key: 'reviews.retrySoon' };
  const minutes = Math.round(seconds / MINUTE);
  if (seconds < HOUR) return { key: 'reviews.retryMinutes', params: { count: minutes } };
  const hours = Math.round(seconds / HOUR);
  if (seconds < DAY) return { key: 'reviews.retryHours', params: { count: hours } };
  const days = Math.round(seconds / DAY);
  return { key: 'reviews.retryDays', params: { count: days } };
}

const RETRY_REASON_KEYS: Partial<Record<string, MessageKey>> = {
  review_account_too_new: 'reviews.eligibilityAccountTooNew',
  review_post_cooldown: 'reviews.eligibilityPostCooldown',
  review_daily_limit: 'reviews.eligibilityDailyLimit',
  review_repost_cooldown: 'reviews.eligibilityRepostCooldown',
  review_edit_cooldown: 'reviews.eligibilityEditCooldown',
};

export type EligibilityMessage =
  | { kind: 'none' }
  | { kind: 'unavailable'; key: MessageKey }
  | { kind: 'muted'; key: MessageKey }
  | { kind: 'playtime'; key: MessageKey; params: Params }
  | { kind: 'retry'; key: MessageKey; retry: RetryText };

export function eligibilityMessage(eligibility: Eligibility, now = Date.now()): EligibilityMessage {
  const { reason } = eligibility;
  if (eligibility.canPost) return { kind: 'none' };
  if (!reason) return { kind: 'unavailable', key: 'reviews.eligibilityUnavailable' };
  if (reason === 'account_muted') return { kind: 'muted', key: 'reviews.eligibilityMuted' };
  if (reason === 'review_not_played') {
    const required = Math.round(eligibility.requiredPlaytimeSeconds / MINUTE);
    const current = Math.floor(eligibility.playtimeSeconds / MINUTE);
    return { kind: 'playtime', key: 'reviews.eligibilityNotPlayed', params: { required, current } };
  }
  const key = RETRY_REASON_KEYS[reason];
  if (key) return { kind: 'retry', key, retry: retryText(eligibility.retryAt, now) };
  return { kind: 'unavailable', key: 'reviews.eligibilityUnavailable' };
}
