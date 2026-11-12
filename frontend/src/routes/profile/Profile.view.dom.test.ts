import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/profile', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/profile')>()),
  getProfileSnapshot: vi.fn(),
}));
vi.mock('../../lib/stores/user', async (actual) => ({
  ...(await actual<typeof import('../../lib/stores/user')>()),
  saveProfile: vi.fn(),
}));

import { locale } from '../../lib/i18n';
import { DEFAULT_PROFILE, type CurrentUser, type ProfileSettings } from '../../lib/services/account';
import { EMPTY_SNAPSHOT, getProfileSnapshot } from '../../lib/services/profile';
import { profileSnapshot } from '../../lib/stores/profile';
import { navigate, route } from '../../lib/stores/router';
import { authState, currentUser, saveProfile } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { description, tabbable } from '../../lib/testing/hints';
import Profile from './Profile.svelte';

// The profile page of a signed-in user, as it is shown (not edited).
function signIn(profile: Partial<ProfileSettings> = {}) {
  const me: CurrentUser = {
    id: 'u1',
    username: 'ann',
    displayName: 'Ann',
    email: 'ann@example.com',
    avatarUrl: '',
    bio: '',
    profile: { ...DEFAULT_PROFILE, ...profile },
    createdAt: '2025-01-01T00:00:00Z',
  };
  currentUser.set(me);
}

beforeEach(() => {
  locale.set('en');
  authState.set('authenticated');
  vi.mocked(getProfileSnapshot).mockResolvedValue(EMPTY_SNAPSHOT);
  vi.mocked(saveProfile).mockReset();
  navigate('profile');
});

afterEach(() => {
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  document.body.replaceChildren();
});

describe('the profile page', () => {
  it('stats hidden from others carry the status badge "Hidden", a Tab stop described by who it is hidden from', async () => {
    signIn({ showStats: false });
    renderDom(Profile);

    const badges = screen.getAllByText('Hidden').filter((element) => description(element) === 'Stats hidden from others. You can see them.');
    expect(badges).toHaveLength(1);
    expect(tabbable(badges[0])).toBe(true);
    expect(badges[0].classList.contains('badge')).toBe(true);
  });
});
