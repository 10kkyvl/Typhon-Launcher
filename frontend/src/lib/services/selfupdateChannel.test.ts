import { describe, expect, it } from 'vitest';
import { betaReportRoute, betaToggleShown, isBeta, stableOfferVersion } from './selfupdateChannel';

describe('betaToggleShown', () => {
  it('shows only for a signed-in tester', () => {
    expect(betaToggleShown('authenticated', { tester: true })).toBe(true);
  });

  it.each([
    ['authenticated', { tester: false }],
    ['authenticated', {}],
    ['authenticated', null],
    ['guest', { tester: true }],
    ['offline', { tester: true }],
    ['unauthenticated', null],
  ] as const)('hides for %s %j', (auth, user) => {
    expect(betaToggleShown(auth, user)).toBe(false);
  });
});

describe('isBeta', () => {
  it('is true only for the beta channel', () => {
    expect(isBeta('beta')).toBe(true);
    expect(isBeta('stable')).toBe(false);
    expect(isBeta(undefined)).toBe(false);
  });
});

describe('stableOfferVersion', () => {
  it('returns the offered version', () => {
    expect(stableOfferVersion({ stableOffer: '1.3.0' })).toBe('1.3.0');
  });

  it('is empty without an offer', () => {
    expect(stableOfferVersion({})).toBe('');
    expect(stableOfferVersion({ stableOffer: '  ' })).toBe('');
  });
});

describe('betaReportRoute', () => {
  it('files a ticket for accounts and exports logs otherwise', () => {
    expect(betaReportRoute('authenticated')).toBe('ticket');
    expect(betaReportRoute('guest')).toBe('logs');
    expect(betaReportRoute('offline')).toBe('logs');
  });
});
