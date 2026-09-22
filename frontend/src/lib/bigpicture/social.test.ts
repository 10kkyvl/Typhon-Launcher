import { describe, expect, it } from 'vitest';
import { createRequestGate, ownsFriendsSnapshot, socialAvailability } from './social';

describe('Big Picture social state', () => {
  it('allows friend-network screens only for an authenticated online session', () => {
    expect(socialAvailability('authenticated')).toBe('available');
    expect(socialAvailability('guest')).toBe('guest');
    expect(socialAvailability('offline')).toBe('offline');
    expect(socialAvailability('unauthenticated')).toBe('sign-in');
  });

  it('ignores an older profile response after a newer search starts', () => {
    const requests = createRequestGate();
    const older = requests.begin();
    const newer = requests.begin();

    expect(requests.isCurrent(older)).toBe(false);
    expect(requests.isCurrent(newer)).toBe(true);
    requests.invalidate();
    expect(requests.isCurrent(newer)).toBe(false);
  });

  it('does not apply a shared friend update to a different signed-in account', () => {
    expect(ownsFriendsSnapshot('account-a', 'account-a')).toBe(true);
    expect(ownsFriendsSnapshot('account-b', 'account-a')).toBe(false);
    expect(ownsFriendsSnapshot('', 'account-a')).toBe(false);
  });
});
