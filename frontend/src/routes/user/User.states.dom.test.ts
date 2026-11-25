import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import type { Settings } from '../../lib/services/settings';
import { profile as fetchProfile, sendRequest, type SendResult } from '../../lib/services/social';
import { resetHistory } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { description } from '../../lib/testing/hints';
import UserProfile from './UserProfile.svelte';
import { player } from './userTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));

beforeEach(() => {
  authState.set('authenticated');
  resetBindings();
  toasts.set([]);
});

afterEach(() => {
  expect(calls).toEqual([]);
  vi.mocked(fetchProfile).mockReset();
  vi.mocked(sendRequest).mockReset();
  settings.set(null);
  document.body.replaceChildren();
  authState.set('bootstrapping');
  resetHistory();
});

// The server answers a profile of a player either of the two has blocked as "user not found"
// (typhon-backend social.Service.profile → ErrNotFound → 404 user_not_found): the page cannot tell
// it from a name nobody has, so it says both.
describe("a user's page: a player it cannot show", () => {
  it('"not found" from the server says the name may be wrong or one of the two blocked the other', async () => {
    vi.mocked(fetchProfile).mockRejectedValue(new AccountError('user_not_found'));
    renderDom(UserProfile, { username: 'ann' });

    expect(await screen.findByRole('heading', { level: 2, name: msg('social.userNotFoundTitle') })).toBeTruthy();
    expect(screen.getByText(msg('social.userNotFoundHiddenDesc'))).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('a page opened without a name says only that there is no such player', async () => {
    renderDom(UserProfile, { username: '' });

    expect(await screen.findByRole('heading', { level: 2, name: msg('social.userNotFoundTitle') })).toBeTruthy();
    expect(screen.getByText(msg('social.userNotFoundDesc'))).toBeTruthy();
    expect(vi.mocked(fetchProfile)).not.toHaveBeenCalled();
  });
});

describe("a user's page: account sync not consented", () => {
  it('shows the consent picture of the friends screen and asks the server nothing', async () => {
    settings.set({ accountSync: false } as Settings);
    renderDom(UserProfile, { username: 'ann' });

    expect(await screen.findByRole('heading', { level: 2, name: msg('social.friendsConsentTitle') })).toBeTruthy();
    expect(screen.getByText(msg('social.friendsConsentDesc'))).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(vi.mocked(fetchProfile)).not.toHaveBeenCalled();
  });

  it('its button opens the consent dialog', async () => {
    settings.set({ accountSync: false } as Settings);
    const { user } = renderDom(UserProfile, { username: 'ann' });

    await user.click(await screen.findByRole('button', { name: msg('social.friendsEnableSync') }));

    expect(within(screen.getByRole('dialog')).getByRole('heading', { name: msg('modals.socialConsentTitle') })).toBeTruthy();
  });

  it('once consented, the page reads the profile', async () => {
    settings.set({ accountSync: false } as Settings);
    vi.mocked(fetchProfile).mockResolvedValue(player());
    renderDom(UserProfile, { username: 'ann' });
    await screen.findByRole('heading', { level: 2, name: msg('social.friendsConsentTitle') });

    settings.set({ accountSync: true } as Settings);

    expect(await screen.findByText('@ann', { selector: '.username' })).toBeTruthy();
    expect(vi.mocked(fetchProfile)).toHaveBeenCalledWith('ann');
  });
});

describe("a user's page: offline", () => {
  const add = () => screen.getByRole('button', { name: msg('social.relationNone') });

  it('the relation actions say before the press that they need the server, and a press asks nothing', async () => {
    authState.set('offline');
    vi.mocked(fetchProfile).mockResolvedValue(player());
    const { user } = renderDom(UserProfile, { username: 'ann' });
    await screen.findByText('@ann', { selector: '.username' });

    expect(add().getAttribute('aria-disabled')).toBe('true');
    expect(description(add())).toBe(msg('social.userActionsNeedConnection'));
    expect(description(screen.getByRole('button', { name: msg('social.moreLabel') }))).toBe(msg('social.userActionsNeedConnection'));
    await user.click(add());
    expect(vi.mocked(sendRequest)).not.toHaveBeenCalled();
  });

  it("on a friend's page the friend menu says why it waits", async () => {
    authState.set('offline');
    vi.mocked(fetchProfile).mockResolvedValue(player({ relation: 'friend' }));
    renderDom(UserProfile, { username: 'ann' });
    await screen.findByText('@ann', { selector: '.username' });

    const more = screen.getByRole('button', { name: msg('social.moreLabel') });
    expect(more.getAttribute('aria-disabled')).toBe('true');
    expect(description(more)).toBe(msg('social.userActionsNeedConnection'));
  });

  it('a request from the player: Accept and Decline each say why they wait', async () => {
    authState.set('offline');
    vi.mocked(fetchProfile).mockResolvedValue(player({ relation: 'incoming' }));
    renderDom(UserProfile, { username: 'ann' });
    await screen.findByText('@ann', { selector: '.username' });

    for (const name of [msg('social.relationIncoming'), msg('social.declineButton')]) {
      const control = screen.getByRole('button', { name });
      expect(control.getAttribute('aria-disabled'), name).toBe('true');
      expect(description(control), name).toBe(msg('social.userActionsNeedConnection'));
    }
  });

  it('a request sent to the player: Cancel request says why it waits', async () => {
    authState.set('offline');
    vi.mocked(fetchProfile).mockResolvedValue(player({ relation: 'outgoing' }));
    renderDom(UserProfile, { username: 'ann' });
    await screen.findByText('@ann', { selector: '.username' });

    const cancel = screen.getByRole('button', { name: msg('social.cancelRequestButton') });
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(description(cancel)).toBe(msg('social.userActionsNeedConnection'));
  });

  it('going offline keeps the profile read last and holds its actions; back online they work again', async () => {
    vi.mocked(fetchProfile).mockResolvedValue(player());
    vi.mocked(sendRequest).mockResolvedValue({ accepted: false } as SendResult);
    const { user } = renderDom(UserProfile, { username: 'ann' });
    await screen.findByText('@ann', { selector: '.username' });

    authState.set('offline');

    await vi.waitFor(() => expect(add().getAttribute('aria-disabled')).toBe('true'));
    expect(screen.getByText('@ann', { selector: '.username' })).toBeTruthy();
    expect(vi.mocked(fetchProfile)).toHaveBeenCalledTimes(1);

    authState.set('authenticated');
    await vi.waitFor(() => expect(add().hasAttribute('aria-disabled')).toBe(false));
    await user.click(add());
    expect(vi.mocked(sendRequest)).toHaveBeenCalledWith('ann');
  });
});

describe("a user's page: headings", () => {
  // One h1 names the player from the first frame (the shell moves focus to it on arrival, before the
  // profile has come); the display name heads the profile; its cards are h3, as the kit's Card draws them.
  it('one h1, the @name; then the display name; no level skipped', async () => {
    vi.mocked(fetchProfile).mockResolvedValue(
      player({
        relation: 'friend',
        recentlyPlayed: [{ igdbId: 1, title: 'Hades', coverUrl: '', status: '', favorite: false, lastPlayedAt: null }],
        mutualFriends: [{ id: 'u3', username: 'cid', displayName: 'Cid', avatarUrl: '' }],
        mutualCount: 1,
      }),
    );
    renderDom(UserProfile, { username: 'ann' });
    await screen.findByText('@ann', { selector: '.username' });

    const headings = screen.getAllByRole('heading').map((h) => [Number(h.tagName.slice(1)), h.textContent?.trim()] as const);
    expect(headings.filter(([level]) => level === 1)).toEqual([[1, '@ann']]);
    expect(headings[1]).toEqual([2, 'Ann']);
    expect(headings.length).toBeGreaterThan(2);
    headings.forEach(([level], i) => expect(level - (headings[i - 1]?.[0] ?? 0), `${i}`).toBeLessThanOrEqual(1));
  });
});
