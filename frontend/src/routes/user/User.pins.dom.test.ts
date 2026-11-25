import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { accountMessage } from '../../lib/services/accountMessages';
import {
  accept,
  block,
  chatPeerOf,
  decline,
  profile as fetchProfile,
  refresh,
  sendRequest,
  unfriend,
  type CommonGame,
  type PublicProfile,
  type SendResult,
} from '../../lib/services/social';
import { inviteText } from '../../lib/social/invite';
import { draftKey, openChat } from '../../lib/stores/messaging';
import { resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import UserProfile from './UserProfile.svelte';
import { draftStorage, player } from './userTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));

let drafts = new Map<string, string>();

const said = () => get(toasts).map((item) => [item.kind, item.message]);
const button = (name: string) => screen.getByRole('button', { name });

function pending<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => (resolve = res));
  return { promise, resolve };
}

// The page asks for the profile, then again after an action: each answer in turn, the last one kept.
function answers(...profiles: PublicProfile[]) {
  const mock = vi.mocked(fetchProfile);
  for (const next of profiles) mock.mockResolvedValueOnce(next);
  mock.mockResolvedValue(profiles[profiles.length - 1]);
}

async function mount(first: PublicProfile, ...after: PublicProfile[]) {
  answers(first, ...after);
  const rendered = renderDom(UserProfile, { username: first.username });
  await screen.findByText(`@${first.username}`, { selector: '.username' });
  return rendered;
}

beforeEach(() => {
  authState.set('authenticated');
  resetBindings();
  toasts.set([]);
  drafts = draftStorage();
  vi.mocked(refresh).mockResolvedValue(undefined);
});

afterEach(() => {
  expect(calls).toEqual([]);
  for (const mock of [fetchProfile, refresh, sendRequest, accept, decline, unfriend, block, openChat]) vi.mocked(mock).mockReset();
  document.body.replaceChildren();
  authState.set('bootstrapping');
  resetHistory();
});

describe("a user's page: what each action does", () => {
  it('Add sends the request by name, says it was sent, and the page shows the request with Cancel', async () => {
    vi.mocked(sendRequest).mockResolvedValue({ accepted: false } as SendResult);
    const { user } = await mount(player(), player({ relation: 'outgoing' }));

    await user.click(button(msg('social.relationNone')));

    await screen.findByRole('button', { name: msg('social.cancelRequestButton') });
    expect(vi.mocked(sendRequest)).toHaveBeenCalledWith('ann');
    expect(screen.getByText(msg('social.relationOutgoing'))).toBeTruthy();
    expect(said()).toEqual([['success', msg('social.relationOutgoing')]]);
  });

  it('Add to a player who had asked first says you are friends', async () => {
    vi.mocked(sendRequest).mockResolvedValue({ accepted: true } as SendResult);
    const { user } = await mount(player(), player({ relation: 'friend' }));

    await user.click(button(msg('social.relationNone')));

    await screen.findByText(msg('social.relationFriend'));
    expect(said()).toEqual([['success', msg('social.friendsNowFriends')]]);
  });

  it('Accept makes you friends: it says so and the page offers Write', async () => {
    vi.mocked(accept).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'incoming' }), player({ relation: 'friend' }));

    await user.click(button(msg('social.relationIncoming')));

    await screen.findByRole('button', { name: msg('social.chatWrite') });
    expect(vi.mocked(accept)).toHaveBeenCalledWith('u1');
    expect(said()).toEqual([['success', msg('social.friendsNowFriends')]]);
  });

  it('Decline says the request was declined and the page offers Add', async () => {
    vi.mocked(decline).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'incoming' }), player());

    await user.click(button(msg('social.declineButton')));

    await screen.findByRole('button', { name: msg('social.relationNone') });
    expect(vi.mocked(decline)).toHaveBeenCalledWith('u1');
    expect(said()).toEqual([['success', msg('social.requestDeclined')]]);
  });

  it('Cancel withdraws the request, says so, and the page offers Add', async () => {
    vi.mocked(decline).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'outgoing' }), player());

    await user.click(button(msg('social.cancelRequestButton')));

    await screen.findByRole('button', { name: msg('social.relationNone') });
    expect(vi.mocked(decline)).toHaveBeenCalledWith('u1');
    expect(said()).toEqual([['success', msg('social.requestCancelled')]]);
  });

  it('Unfriend asks first; confirmed, it unfriends, says so and the page offers Add', async () => {
    vi.mocked(unfriend).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'friend' }), player());

    await user.click(button(msg('social.moreLabel')));
    await user.click(screen.getByRole('menuitem', { name: msg('social.unfriendLabel') }));
    const dialog = screen.getByRole('dialog');
    expect(vi.mocked(unfriend)).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: msg('social.unfriendLabel') }));

    await screen.findByRole('button', { name: msg('social.relationNone') });
    expect(vi.mocked(unfriend)).toHaveBeenCalledWith('u1');
    expect(dialog.isConnected).toBe(false);
    expect(said()).toEqual([['success', msg('social.friendsUnfriended')]]);
  });

  it('Block asks first; confirmed, it blocks, says so and opens the blocked players', async () => {
    vi.mocked(block).mockResolvedValue(undefined);
    const { user } = await mount(player());

    await user.click(button(msg('social.moreLabel')));
    await user.click(screen.getByRole('menuitem', { name: msg('social.blockLabel') }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: msg('social.blockLabel') }));

    await vi.waitFor(() => expect(get(route)).toMatchObject({ name: 'friends', params: { tab: 'blocked' } }));
    expect(vi.mocked(block)).toHaveBeenCalledWith('u1');
    expect(said()).toEqual([['success', msg('social.userBlocked')]]);
  });

  it("a refused Add is said once in a toast, in the code's catalogued words", async () => {
    vi.mocked(sendRequest).mockRejectedValue(new AccountError('request_blocked'));
    const { user } = await mount(player());

    await user.click(button(msg('social.relationNone')));

    await vi.waitFor(() => expect(said()).toEqual([['danger', accountMessage('request_blocked')]]));
    expect(button(msg('social.relationNone'))).toBeTruthy();
  });
});

describe("a user's page: what it shows", () => {
  it("a friends-only profile seen by a stranger says so, as a standing fact, and shows none of it", async () => {
    await mount(player({ visibility: 'friends', recentlyPlayed: [{ igdbId: 1, title: 'Hades', coverUrl: '', status: '', favorite: false, lastPlayedAt: null }] }));

    const notice = screen.getByText(msg('social.userRestToFriends')).closest('.notice');
    expect(notice?.getAttribute('data-tone')).toBe('info');
    expect(notice?.querySelector('[role="status"], [role="alert"]')).toBeNull();
    expect(screen.queryByText('Hades')).toBeNull();
  });

  it('a guest is offered to sign in and nothing is asked of the server', async () => {
    authState.set('guest');
    renderDom(UserProfile, { username: 'ann' });

    expect(screen.getByRole('heading', { level: 2, name: msg('social.userGuestTitle') })).toBeTruthy();
    expect(button(msg('social.signInButton'))).toBeTruthy();
    expect(button(msg('social.createAccountButton'))).toBeTruthy();
    expect(vi.mocked(fetchProfile)).not.toHaveBeenCalled();
  });

  it("one's own name opens one's own profile", async () => {
    vi.mocked(fetchProfile).mockResolvedValue(player({ relation: 'self' }));
    renderDom(UserProfile, { username: 'ann' });

    await vi.waitFor(() => expect(get(route).name).toBe('profile'));
  });

  it('an answer for the name the page has left is not shown while the new name loads', async () => {
    const ann = pending<PublicProfile>();
    const bob = pending<PublicProfile>();
    vi.mocked(fetchProfile).mockImplementation((name) => (name === 'ann' ? ann.promise : bob.promise));
    const { rerender } = renderDom(UserProfile, { username: 'ann' });

    await rerender({ username: 'bob' });
    ann.resolve(player({ displayName: 'Ann' }));
    await Promise.resolve();
    expect(screen.queryByText('Ann')).toBeNull();
    expect(screen.getByText(msg('social.loadingEllipsis'))).toBeTruthy();

    bob.resolve(player({ id: 'u2', username: 'bob', displayName: 'Bob' }));
    await screen.findByText('Bob');
    expect(screen.queryByText('Ann')).toBeNull();
  });

  it('an answer for the name the page has left does not replace the new name once it has painted', async () => {
    const ann = pending<PublicProfile>();
    const bob = pending<PublicProfile>();
    vi.mocked(fetchProfile).mockImplementation((name) => (name === 'ann' ? ann.promise : bob.promise));
    const { rerender } = renderDom(UserProfile, { username: 'ann' });

    await rerender({ username: 'bob' });
    bob.resolve(player({ id: 'u2', username: 'bob', displayName: 'Bob' }));
    await screen.findByText('Bob');
    ann.resolve(player({ displayName: 'Ann' }));
    await ann.promise;
    await tick();

    expect(screen.queryByText('Ann')).toBeNull();
    expect(screen.getByText('Bob')).toBeTruthy();
    expect(screen.queryByText(msg('social.loadingEllipsis'))).toBeNull();
  });

  it("Write opens the chat with the friend", async () => {
    const friend = player({ relation: 'friend' });
    const { user } = await mount(friend);

    await user.click(button(msg('social.chatWrite')));

    expect(vi.mocked(openChat)).toHaveBeenCalledWith(chatPeerOf(friend));
  });

  it("inviting a friend to a common game writes the invitation into the chat's draft and opens the chat with the friend as Write does", async () => {
    const game = { igdbId: 7, title: 'Hades', coverUrl: '', viewerOwned: true, targetOwned: true } as CommonGame;
    const friend = player({
      relation: 'friend',
      common: { count: 1, games: [game] },
      appearance: { avatarFrame: 'gold', frameColor: '#ffaa00' } as unknown as PublicProfile['appearance'],
    });
    const { user } = await mount(friend);
    drafts.set(draftKey('u1'), 'hi');

    await user.click(button(msg('social.userCommonInvite')));

    expect(drafts.get(draftKey('u1'))).toBe(`hi\n${inviteText('Hades')}`);
    expect(vi.mocked(openChat).mock.calls).toEqual([[chatPeerOf(friend)]]);
    expect(chatPeerOf(friend)).toMatchObject({ avatarFrame: 'gold', frameColor: '#ffaa00' });
  });

  it('a stranger is not offered an invitation', async () => {
    const game = { igdbId: 7, title: 'Hades', coverUrl: '', viewerOwned: true, targetOwned: true } as CommonGame;
    await mount(player({ common: { count: 1, games: [game] } }));

    expect(screen.queryByRole('button', { name: msg('social.userCommonInvite') })).toBeNull();
  });
});
