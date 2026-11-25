import { cleanup, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import type { Settings } from '../../lib/services/settings';
import {
  accept,
  blocks,
  decline,
  friendCode,
  friends,
  profile,
  profileByCode,
  refresh,
  rotateFriendCode,
  sendRequest,
  unblock,
  type FriendsPage,
  type PublicProfile,
  type SendResult,
} from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { friendsPage } from '../../lib/stores/social';
import { toast } from '../../lib/stores/toasts';
import { authState, leaveGuest } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { FRIEND_CODE, NEW_FRIEND_CODE, NoResize, card, friend, request } from './friendsTesting';

// What the screen does today and keeps: the guest and consent screens, each tab's empty words, the
// friend code card, the words of the actions that are said in a toast, and the add dialog's lookup by
// code. Defects of today are not pinned here: their tests come with their fixes.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/stores/toasts', async (original) => ({
  ...(await original<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(),
}));
vi.mock('../../lib/stores/user', async (original) => ({
  ...(await original<typeof import('../../lib/stores/user')>()),
  leaveGuest: vi.fn(async () => {}),
}));

const FRIENDS = [friend('u-ana', 'Ana', 'online'), friend('u-gleb', 'Gleb', 'offline')];
const INCOMING = [request('r-inna', 'Inna')];
const OUTGOING = [request('r-lev', 'Lev')];
const BLOCKED = [card('b-oleg', 'Oleg')];

const tabRadio = (label: string) => screen.getAllByRole('radio').find((radio) => radio.textContent?.trim().startsWith(label)) as HTMLElement;
const cardOf = (name: string) => screen.getByText(name).closest('.request') as HTMLElement;

/** The page the screen is shown with: in the store, and what a read of it answers. */
function seed(page: FriendsPage) {
  friendsPage.set(page);
  vi.mocked(friends).mockResolvedValue(page);
}

function mount(props: { tab?: string } = {}) {
  render(Friends, { target: document.getElementById('app') as HTMLElement, props });
  return user();
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: true } as Settings);
  seed({ friends: FRIENDS, incoming: INCOMING, outgoing: OUTGOING });
  vi.mocked(blocks).mockResolvedValue(BLOCKED);
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  friendsPage.set({ friends: [], incoming: [], outgoing: [] });
  authState.set('bootstrapping');
  settings.set(null);
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends for a guest', () => {
  it('asks to sign in, reads nothing and offers no code or add', async () => {
    authState.set('guest');
    const keys = mount();
    await tick();

    expect(screen.getByRole('heading', { name: msg('social.friendsGuestTitle') })).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('social.friendsMyCodeButton') })).toBeNull();
    expect(screen.queryByRole('button', { name: msg('social.friendsAddFriend') })).toBeNull();
    await keys.click(screen.getByRole('button', { name: msg('social.signInButton') }));
    await keys.click(screen.getByRole('button', { name: msg('social.createAccountButton') }));

    expect(vi.mocked(leaveGuest).mock.calls).toEqual([['login'], ['register']]);
    expect([refresh, friends, blocks, friendCode].map((read) => vi.mocked(read).mock.calls.length)).toEqual([0, 0, 0, 0]);
  });
});

describe('Friends without the sync consent', () => {
  it('asks for it, reads nothing, and opens the consent dialog', async () => {
    settings.set({ accountSync: false } as Settings);
    const keys = mount();
    await tick();

    expect(screen.getByRole('heading', { name: msg('social.friendsConsentTitle') })).toBeTruthy();
    await keys.click(screen.getByRole('button', { name: msg('social.friendsEnableSync') }));

    expect(screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') })).toBeTruthy();
    expect([refresh, friends, blocks, friendCode].map((read) => vi.mocked(read).mock.calls.length)).toEqual([0, 0, 0, 0]);
  });
});

describe('Friends: each tab’s empty words', () => {
  it('says nobody is here yet, with an add, when the read found no friend', async () => {
    seed({ friends: [], incoming: [], outgoing: [] });
    mount();

    expect(await screen.findByRole('heading', { name: msg('social.friendsEmptyNobodyTitle') })).toBeTruthy();
    expect(screen.getAllByRole('button', { name: new RegExp(msg('social.friendsAddFriend')) })).toHaveLength(2);
  });

  it.each([
    ['online', 'social.presenceOnline', [friend('u-gleb', 'Gleb', 'offline')], 'social.friendsEmptyOnlineTitle'],
    ['away', 'social.friendsTabAway', [friend('u-ana', 'Ana', 'online')], 'social.friendsEmptyAwayTitle'],
    ['offline', 'social.presenceOffline', [friend('u-ana', 'Ana', 'online')], 'social.friendsEmptyOfflineTitle'],
  ] as const)('says the %s tab has nobody when no friend is in it', async (_tab, label, list, title) => {
    seed({ friends: [...list], incoming: [], outgoing: [] });
    const keys = mount();
    await screen.findByText(list[0].displayName);

    await keys.click(tabRadio(msg(label)));

    expect(screen.getByRole('heading', { name: msg(title) })).toBeTruthy();
  });

  it('says a search found nobody', async () => {
    const keys = mount();
    await screen.findByText('Ana');

    await keys.type(screen.getByRole('textbox', { name: msg('social.friendsSearchFriendsPlaceholder') }), 'zzz');

    expect(screen.getByRole('heading', { name: msg('social.friendsEmptySearchTitle') })).toBeTruthy();
  });

  it('says there are no requests', async () => {
    seed({ friends: FRIENDS, incoming: [], outgoing: [] });
    mount({ tab: 'requests' });

    expect(await screen.findByRole('heading', { name: msg('social.friendsEmptyRequestsTitle') })).toBeTruthy();
  });

  it('says nobody is blocked once the blocked list is read empty', async () => {
    vi.mocked(blocks).mockResolvedValue([]);
    mount({ tab: 'blocked' });

    expect(await screen.findByRole('heading', { name: msg('social.friendsEmptyBlockedTitle') })).toBeTruthy();
  });
});

describe('Friends: the friend code', () => {
  async function openCode() {
    const keys = mount();
    const toggle = screen.getByRole('button', { name: msg('social.friendsMyCodeButton') });
    await keys.click(toggle);
    await screen.findByText(FRIEND_CODE);
    return { keys, toggle };
  }

  it('is shown under "My code", read from the account', async () => {
    const { toggle } = await openCode();

    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(friendCode).toHaveBeenCalledTimes(1);
  });

  it('copies itself, says so once and keeps the focus on Copy', async () => {
    const { keys } = await openCode();
    const writeText = vi.fn(async () => {});
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    const copy = screen.getByRole('button', { name: msg('social.friendsCopyCode') });

    await keys.click(copy);

    expect(writeText).toHaveBeenCalledWith(FRIEND_CODE);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg('social.copied'), 'info']]);
    expect(copy.textContent?.trim()).toBe(msg('social.copied'));
    expect(document.activeElement).toBe(copy);
  });

  it('says a failed copy once and keeps the focus on Copy', async () => {
    const { keys } = await openCode();
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn(async () => Promise.reject(new Error('denied'))) }, configurable: true });
    const copy = screen.getByRole('button', { name: msg('social.friendsCopyCode') });

    await keys.click(copy);

    await vi.waitFor(() => expect(vi.mocked(toast).mock.calls).toEqual([[msg('social.copyFailed'), 'danger']]));
    expect(document.activeElement).toBe(copy);
  });

  it('is changed for a new one after its warning', async () => {
    const { keys } = await openCode();

    await keys.click(screen.getByRole('button', { name: msg('social.friendsGenerateNew') }));
    const dialog = screen.getByRole('dialog', { name: msg('social.friendsChangeCodeTitle') });
    expect(within(dialog).getByText(msg('social.friendsChangeCodeWarning'))).toBeTruthy();
    await keys.click(within(dialog).getByRole('button', { name: msg('social.friendsGenerateNew') }));

    expect(await screen.findByText(NEW_FRIEND_CODE)).toBeTruthy();
    expect(rotateFriendCode).toHaveBeenCalledTimes(1);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg('social.friendsCodeRotated'), 'success']]);
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});

describe('Friends: what a card’s action says', () => {
  it.each([
    ['accept', 'requests', 'Inna', 'social.relationIncoming', accept, 'social.acceptRequestFailed'],
    ['decline', 'requests', 'Inna', 'social.declineButton', decline, 'social.declineRequestFailed'],
    ['cancel', 'requests', 'Lev', 'social.cancelRequestButton', decline, 'social.cancelRequestFailed'],
    ['unblock', 'blocked', 'Oleg', 'social.unblockButton', unblock, 'social.unblockFailed'],
  ] as const)('says a failed %s once, in its own words', async (_action, tab, name, button, call, words) => {
    vi.mocked(call).mockRejectedValueOnce(new Error('server said no'));
    const keys = mount({ tab });
    await screen.findByText(name);

    await keys.click(within(cardOf(name)).getByRole('button', { name: msg(button) }));

    await vi.waitFor(() => expect(vi.mocked(toast).mock.calls).toEqual([[msg(words), 'danger']]));
  });

  it.each([
    ['accept', 'requests', 'Inna', 'social.relationIncoming', 'social.friendsNowFriends'],
    ['unblock', 'blocked', 'Oleg', 'social.unblockButton', 'social.userUnblocked'],
  ] as const)('says a done %s', async (_action, tab, name, button, words) => {
    const keys = mount({ tab });
    await screen.findByText(name);

    await keys.click(within(cardOf(name)).getByRole('button', { name: msg(button) }));

    await vi.waitFor(() => expect(vi.mocked(toast).mock.calls).toEqual([[msg(words), 'success']]));
  });

  it('says nothing of its own when the session has ended: the sign-in screen does', async () => {
    vi.mocked(decline).mockRejectedValueOnce(new AccountError('unauthenticated'));
    const keys = mount({ tab: 'requests' });
    await screen.findByText('Inna');

    await keys.click(within(cardOf('Inna')).getByRole('button', { name: msg('social.declineButton') }));

    await vi.waitFor(() => expect(decline).toHaveBeenCalledTimes(1));
    await tick();
    expect(toast).not.toHaveBeenCalled();
  });
});

describe('Friends: the add dialog', () => {
  const lena = { ...card('u-lena', 'Lena'), relation: 'none', mutualCount: 0 } as unknown as PublicProfile;

  async function lookUp(query: string) {
    const keys = mount();
    await keys.click(screen.getByRole('button', { name: msg('social.friendsAddFriend') }));
    const dialog = screen.getByRole('dialog', { name: msg('social.friendsAddFriend') });
    await keys.type(within(dialog).getByRole('textbox', { name: msg('social.friendsSearchLabel') }), `${query}{Enter}`);
    return { keys, dialog };
  }

  it('looks a friend code up by the code, not by a name', async () => {
    vi.mocked(profileByCode).mockResolvedValue(lena);
    const { dialog } = await lookUp(FRIEND_CODE);

    expect(await within(dialog).findByText('@lena')).toBeTruthy();
    expect(profileByCode).toHaveBeenCalledWith(FRIEND_CODE);
    expect(profile).not.toHaveBeenCalled();
  });

  it('sends the request, says it is sent, closes and reads the friends again', async () => {
    vi.mocked(profile).mockResolvedValue(lena);
    vi.mocked(sendRequest).mockResolvedValue({ accepted: false } as SendResult);
    const { keys, dialog } = await lookUp('lena');
    const reads = vi.mocked(refresh).mock.calls.length;

    await keys.click(await within(dialog).findByRole('button', { name: msg('social.friendsSendRequest') }));

    expect(sendRequest).toHaveBeenCalledWith('lena');
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(toast).mock.calls).toEqual([[msg('social.relationOutgoing'), 'success']]);
    expect(vi.mocked(refresh).mock.calls.length).toBe(reads + 1);
  });
});
