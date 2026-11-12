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

  it('"View all activity" is a text link under the activity; Enter on it opens the history', async () => {
    signIn({ layout: { version: 1, blocks: [{ id: 'a', type: 'activity', width: 'half', config: {} }] } });
    const played = { id: 'g1', title: 'Hades', cover: '', playtimeSeconds: 3600, status: '' };
    vi.mocked(getProfileSnapshot).mockResolvedValue({ ...EMPTY_SNAPSHOT, activity: [{ date: '2026-11-10', entries: [{ game: played, seconds: 3600 }] }] });
    const { user } = renderDom(Profile);
    const all = await screen.findByRole('button', { name: 'View all activity' });
    expect(all.classList.contains('text-link')).toBe(true);
    all.focus();

    await user.keyboard('{Enter}');

    expect(get(route).name).toBe('history');
  });

  it('the privacy dialog says it needs the connection in a warning notice, and a refused save in a danger notice', async () => {
    signIn();
    const { user } = renderDom(Profile);
    await user.click(screen.getByRole('button', { name: 'More' }));
    await user.click(screen.getByRole('menuitem', { name: 'Profile privacy' }));
    const dialog = screen.getByRole('dialog', { name: 'Profile settings' });
    expect(within(dialog).queryByText('Profile settings can only change while connected to the server.')).toBeNull();
    vi.mocked(saveProfile).mockResolvedValue(false);

    await user.click(within(dialog).getByRole('button', { name: 'Save' }));

    const alert = within(dialog).getByRole('alert');
    expect(alert.textContent?.trim()).toBe('The previous save is still running. Try again in a moment.');
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();

    authState.set('offline');
    const offline = await within(dialog).findByText('Profile settings can only change while connected to the server.');
    expect(offline.closest('[data-tone="warning"]')).not.toBeNull();
  });
});
