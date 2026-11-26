import { cleanup, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { EMPTY_SNAPSHOT, getProfileSnapshot } from '../../lib/services/profile';
import { emptyFriendsPage, type FriendView } from '../../lib/services/social';
import { profileSnapshot } from '../../lib/stores/profile';
import { navigate, resetHistory } from '../../lib/stores/router';
import { friendsLoaded, friendsPage } from '../../lib/stores/social';
import { authState, currentUser } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Profile from './Profile.svelte';
import { player, SNAPSHOT } from './profileFixtures';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));

// What the page adds from data it already has: the friends count beside the figures, read from the
// friends store the shell keeps.

const friend = (id: string): FriendView => ({ id, username: id, displayName: id, avatarUrl: '', since: '2026-01-01T00:00:00Z', presence: { status: 'offline' } }) as FriendView;
const friendsTile = () => screen.queryByText(msg('profile.statFriends'))?.closest('.tile') ?? null;

beforeEach(() => {
  locale.set('en');
  resetBindings();
  navigate('profile');
  authState.set('authenticated');
  currentUser.set(player());
  vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(getProfileSnapshot).mockReset();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  friendsPage.set(emptyFriendsPage());
  friendsLoaded.set(false);
  document.body.replaceChildren();
});

describe('the friends count', () => {
  it('stands beside the figures once the friends list has been read', async () => {
    friendsPage.set({ ...emptyFriendsPage(), friends: [friend('a'), friend('b'), friend('c')] });
    friendsLoaded.set(true);
    renderDom(Profile);
    await screen.findAllByText('12');

    expect(friendsTile()?.querySelector('.value')?.textContent).toBe('3');
  });

  it('is left out while the friends list has not been read: no zero as a fact', async () => {
    renderDom(Profile);
    await screen.findAllByText('12');

    expect(friendsTile()).toBeNull();
  });
});
