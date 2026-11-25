import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { accountMessage } from '../../lib/services/accountMessages';
import {
  accept,
  block,
  decline,
  profile as fetchProfile,
  refresh,
  sendRequest,
  unfriend,
  type PublicProfile,
  type SendResult,
} from '../../lib/services/social';
import { resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import UserProfile from './UserProfile.svelte';
import { player } from './userTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));

const said = () => get(toasts).map((item) => [item.kind, item.message]);
const button = (name: string) => screen.getByRole('button', { name });

// The page asks for the profile, then again after an action: each answer in turn, the last one kept.
async function mount(first: PublicProfile, ...after: PublicProfile[]) {
  const mock = vi.mocked(fetchProfile);
  for (const next of [first, ...after]) mock.mockResolvedValueOnce(next);
  mock.mockResolvedValue(after.at(-1) ?? first);
  const rendered = renderDom(UserProfile, { username: first.username });
  await screen.findByText(`@${first.username}`, { selector: '.username' });
  return rendered;
}

async function confirmFromMenu(user: ReturnType<typeof renderDom>['user'], item: string) {
  await user.click(button(msg('social.moreLabel')));
  await user.click(screen.getByRole('menuitem', { name: item }));
  const dialog = screen.getByRole('dialog');
  await user.click(within(dialog).getByRole('button', { name: item }));
  return dialog;
}

beforeEach(() => {
  authState.set('authenticated');
  resetBindings();
  toasts.set([]);
  vi.mocked(refresh).mockResolvedValue(undefined);
});

afterEach(() => {
  expect(calls).toEqual([]);
  for (const mock of [fetchProfile, refresh, sendRequest, accept, decline, unfriend, block]) vi.mocked(mock).mockReset();
  document.body.replaceChildren();
  authState.set('bootstrapping');
  resetHistory();
});

describe("a user's page: a failed action is said where the player looks", () => {
  it('a failed unfriend keeps its dialog open with the catalogued words and Retry, and no toast', async () => {
    vi.mocked(unfriend).mockRejectedValue(new AccountError('not_friends'));
    const { user } = await mount(player({ relation: 'friend' }));

    const dialog = await confirmFromMenu(user, msg('social.unfriendLabel'));

    await vi.waitFor(() => expect(within(dialog).getByRole('alert').textContent).toBe(accountMessage('not_friends')));
    expect(dialog.isConnected).toBe(true);
    expect(within(dialog).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(said()).toEqual([]);
  });

  it('a failed unfriend without a code of its own says that unfriending failed', async () => {
    vi.mocked(unfriend).mockRejectedValue(new Error('offline'));
    const { user } = await mount(player({ relation: 'friend' }));

    const dialog = await confirmFromMenu(user, msg('social.unfriendLabel'));

    await vi.waitFor(() => expect(within(dialog).getByRole('alert').textContent).toBe(msg('social.friendsUnfriendFailed')));
    expect(said()).toEqual([]);
  });

  it("the dialog's Retry runs it again; done, the dialog closes and says it once", async () => {
    vi.mocked(unfriend).mockRejectedValueOnce(new AccountError('network_error')).mockResolvedValueOnce(undefined);
    const { user } = await mount(player({ relation: 'friend' }), player({ relation: 'friend' }), player());

    const dialog = await confirmFromMenu(user, msg('social.unfriendLabel'));
    await user.click(await within(dialog).findByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(vi.mocked(unfriend)).toHaveBeenCalledTimes(2);
    expect(said()).toEqual([['success', msg('social.friendsUnfriended')]]);
  });

  it('a failed block keeps its dialog open with the catalogued words, and the page stays', async () => {
    vi.mocked(block).mockRejectedValue(new AccountError('block_limit'));
    const { user } = await mount(player());

    const dialog = await confirmFromMenu(user, msg('social.blockLabel'));

    await vi.waitFor(() => expect(within(dialog).getByRole('alert').textContent).toBe(accountMessage('block_limit')));
    expect(dialog.isConnected).toBe(true);
    expect(get(route).name).not.toBe('friends');
    expect(said()).toEqual([]);
  });

  it('a failed Accept is said once in a toast, by what failed when it has no code of its own', async () => {
    vi.mocked(accept).mockRejectedValue(new Error('offline'));
    const { user } = await mount(player({ relation: 'incoming' }));

    await user.click(button(msg('social.relationIncoming')));

    await vi.waitFor(() => expect(said()).toEqual([['danger', msg('social.acceptRequestFailed')]]));
  });
});

describe("a user's page: a success stays a success", () => {
  it('Add done and the friends list not re-read: the success, then the re-read failure, not "the action failed"', async () => {
    vi.mocked(sendRequest).mockResolvedValue({ accepted: false } as SendResult);
    vi.mocked(refresh).mockRejectedValue(new Error('offline'));
    const { user } = await mount(player(), player({ relation: 'outgoing' }));

    await user.click(button(msg('social.relationNone')));

    await vi.waitFor(() => expect(said()).toHaveLength(2));
    expect(said()).toEqual([
      ['success', msg('social.relationOutgoing')],
      ['danger', msg('social.userRefreshFailed')],
    ]);
    await screen.findByRole('button', { name: msg('social.cancelRequestButton') });
  });

  it('Accept done and the profile not re-read: the success, then the re-read failure once', async () => {
    vi.mocked(accept).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'incoming' }));
    vi.mocked(fetchProfile).mockRejectedValue(new Error('offline'));

    await user.click(button(msg('social.relationIncoming')));

    await vi.waitFor(() => expect(said()).toHaveLength(2));
    expect(said()).toEqual([
      ['success', msg('social.friendsNowFriends')],
      ['danger', msg('social.userRefreshFailed')],
    ]);
  });

  it('a confirmed unfriend whose re-read fails closes its dialog: the success, then the re-read failure', async () => {
    vi.mocked(unfriend).mockResolvedValue(undefined);
    vi.mocked(refresh).mockRejectedValue(new Error('offline'));
    const { user } = await mount(player({ relation: 'friend' }), player());

    const dialog = await confirmFromMenu(user, msg('social.unfriendLabel'));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(said()).toEqual([
      ['success', msg('social.friendsUnfriended')],
      ['danger', msg('social.userRefreshFailed')],
    ]);
  });

  it('a block whose list re-read fails still opens the blocked players and says the block', async () => {
    vi.mocked(block).mockResolvedValue(undefined);
    vi.mocked(refresh).mockRejectedValue(new Error('offline'));
    const { user } = await mount(player());

    await confirmFromMenu(user, msg('social.blockLabel'));

    await vi.waitFor(() => expect(get(route)).toMatchObject({ name: 'friends', params: { tab: 'blocked' } }));
    expect(said()).toEqual([
      ['success', msg('social.userBlocked')],
      ['danger', msg('social.friendsRefreshFailed')],
    ]);
  });
});
