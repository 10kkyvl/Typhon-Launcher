import { describe, expect, it } from 'vitest';
import { eligibilityMessage, retryText } from './reviewEligibility';
import type { Eligibility } from '../services/reviews';

const NOW = Date.parse('2026-09-28T12:00:00Z');

function eligibility(overrides: Partial<Eligibility>): Eligibility {
  return {
    canPost: false,
    reason: '',
    retryAt: '',
    playtimeSeconds: 0,
    requiredPlaytimeSeconds: 0,
    ...overrides,
  };
}

describe('retryText', () => {
  it('never promises "soon" when retryAt is missing or unparsable', () => {
    expect(retryText('')).toEqual({ key: 'reviews.retryLater' });
    expect(retryText('not-a-date')).toEqual({ key: 'reviews.retryLater' });
  });

  it('treats anything within a minute as "soon"', () => {
    const retryAt = new Date(NOW + 30_000).toISOString();
    expect(retryText(retryAt, NOW)).toEqual({ key: 'reviews.retrySoon' });
  });

  it('buckets under an hour into minutes', () => {
    const retryAt = new Date(NOW + 5 * 60_000).toISOString();
    expect(retryText(retryAt, NOW)).toEqual({ key: 'reviews.retryMinutes', params: { count: 5 } });
  });

  it('buckets under a day into hours', () => {
    const retryAt = new Date(NOW + 3 * 3600_000).toISOString();
    expect(retryText(retryAt, NOW)).toEqual({ key: 'reviews.retryHours', params: { count: 3 } });
  });

  it('buckets a day or more into days', () => {
    const retryAt = new Date(NOW + 2 * 86400_000).toISOString();
    expect(retryText(retryAt, NOW)).toEqual({ key: 'reviews.retryDays', params: { count: 2 } });
  });
});

describe('eligibilityMessage', () => {
  it('has no message when the reason is empty (canPost)', () => {
    expect(eligibilityMessage(eligibility({ canPost: true, reason: '' }))).toEqual({ kind: 'none' });
  });

  it('reports muted without touching the numeric fields', () => {
    expect(eligibilityMessage(eligibility({ reason: 'account_muted' }))).toEqual({
      kind: 'muted',
      key: 'reviews.eligibilityMuted',
    });
  });

  it('reports playtime progress from the eligibility numbers, not hardcoded', () => {
    const result = eligibilityMessage(
      eligibility({ reason: 'review_not_played', playtimeSeconds: 900, requiredPlaytimeSeconds: 1800 }),
    );
    expect(result).toEqual({
      kind: 'playtime',
      key: 'reviews.eligibilityNotPlayed',
      params: { required: 30, current: 15 },
    });
  });

  it('rounds required minutes even for a non-round backend value', () => {
    const result = eligibilityMessage(
      eligibility({ reason: 'review_not_played', playtimeSeconds: 61, requiredPlaytimeSeconds: 1800 }),
    );
    expect(result).toMatchObject({ kind: 'playtime', params: { required: 30, current: 1 } });
  });

  it.each([
    ['review_account_too_new', 'reviews.eligibilityAccountTooNew'],
    ['review_post_cooldown', 'reviews.eligibilityPostCooldown'],
    ['review_daily_limit', 'reviews.eligibilityDailyLimit'],
    ['review_repost_cooldown', 'reviews.eligibilityRepostCooldown'],
    ['review_edit_cooldown', 'reviews.eligibilityEditCooldown'],
  ])('resolves a retry message with a relative time for %s', (reason, key) => {
    const retryAt = new Date(NOW + 10 * 60_000).toISOString();
    const result = eligibilityMessage(eligibility({ reason, retryAt }), NOW);
    expect(result).toEqual({
      kind: 'retry',
      key,
      retry: { key: 'reviews.retryMinutes', params: { count: 10 } },
    });
  });

  it('explains a refusal with an unknown or empty reason instead of showing nothing', () => {
    const unavailable = { kind: 'unavailable', key: 'reviews.eligibilityUnavailable' };
    expect(eligibilityMessage(eligibility({ reason: 'something_new' }))).toEqual(unavailable);
    expect(eligibilityMessage(eligibility({ reason: '' }))).toEqual(unavailable);
  });

  it('says "later", not "soon", for account_too_new without retryAt', () => {
    expect(eligibilityMessage(eligibility({ reason: 'review_account_too_new' }), NOW)).toEqual({
      kind: 'retry',
      key: 'reviews.eligibilityAccountTooNew',
      retry: { key: 'reviews.retryLater' },
    });
  });
});
