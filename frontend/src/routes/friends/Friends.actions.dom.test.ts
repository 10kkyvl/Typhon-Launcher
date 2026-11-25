import { cleanup, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import {
  accept,
  block,
  blocks,
  friends,
  profile,
  refresh,
  rotateFriendCode,
  sendRequest,
  unfriend,
  type FriendsPage,
  type PublicProfile,
  type SendResult,
} from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { friendsPage } from '../../lib/stores/social';
import { toast } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { FRIEND_CODE, NoResize, card, friend, request } from './friendsTesting';

// An action says its failure where the player is looking: in the confirmation that asked (the dialog
// stays, with the words and Retry), in the add dialog, else once in a toast. An action that went
// through stays a success when the read that follows it fails: that is said as a read failure.

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

const PAGE: FriendsPage = { friends: [friend('u-ana', 'Ana', 'online'), friend('u-gleb', 'Gleb', 'offline')], incoming: [request('r-inna', 'Inna')], outgoing: [] };
const lena = { ...card('u-lena', 'Lena'), relation: 'none', mutualCount: 0 } as unknown as PublicProfile;

const cardOf = (name: string) => screen.getByText(name).closest('.request') as HTMLElement;
const toasts = () => vi.mocked(toast).mock.calls;

async function mount(props: { tab?: string } = {}) {
  render(Friends, { target: document.getElementById('app') as HTMLElement, props });
  const keys = user();
  await screen.findByText('Ana');
  await vi.waitFor(() => expect(blocks).toHaveBeenCalled());
  await tick();
  return keys;
}

async function openMenuItem(keys: ReturnType<typeof user>, name: string, item: string) {
  await keys.click(screen.getByRole('button', { name: msg('social.friendMenuFor', { name }) }));
  await keys.click(screen.getByRole('menuitem', { name: msg(item as 'social.unfriendLabel') }));
  return screen.getByRole('dialog');
}

const confirmButton = (dialog: HTMLElement) => {
  const buttons = within(dialog).getAllByRole('button');
  return buttons[buttons.length - 1];
};

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: true } as Settings);
  friendsPage.set(PAGE);
  vi.mocked(friends).mockResolvedValue(PAGE);
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  friendsPage.set({ friends: [], incoming: [], outgoing: [] });
  authState.set('bootstrapping');
  settings.set(null);
  // Back to each mock's own answer: a queued answer a test left unused must not reach the next one.
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: a confirmed action that fails', () => {
  it.each([
    ['unfriend', 'social.unfriendLabel', unfriend, 'social.friendsUnfriendFailed', 'social.friendsUnfriended'],
    ['block', 'social.blockLabel', block, 'social.blockFailed', 'social.userBlocked'],
  ] as const)('%s: the dialog stays with the words and Retry, no toast, and Retry runs it again', async (_action, item, call, words, done) => {
    vi.mocked(call).mockRejectedValueOnce(new Error('server said no'));
    const keys = await mount();
    const dialog = await openMenuItem(keys, 'Gleb', item);

    await keys.click(confirmButton(dialog));

    const alert = await within(dialog).findByText(msg(words));
    expect(alert.closest('[role="alert"]')).not.toBeNull();
    expect(dialog.isConnected).toBe(true);
    expect(confirmButton(dialog).textContent?.trim()).toBe(msg('common.retry'));
    expect(toast).not.toHaveBeenCalled();

    await keys.click(confirmButton(dialog));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(call).mock.calls).toEqual([['u-gleb'], ['u-gleb']]);
    expect(toasts()).toEqual([[msg(done), 'success']]);
  });

  it('change of the code: the dialog stays with the words, no toast', async () => {
    vi.mocked(rotateFriendCode).mockRejectedValueOnce(new Error('server said no'));
    const keys = await mount();
    await keys.click(screen.getByRole('button', { name: msg('social.friendsMyCodeButton') }));
    await screen.findByText(FRIEND_CODE);
    await keys.click(screen.getByRole('button', { name: msg('social.friendsGenerateNew') }));
    const dialog = screen.getByRole('dialog', { name: msg('social.friendsChangeCodeTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('social.friendsGenerateNew') }));

    const words = await within(dialog).findByText(msg('social.friendsCodeRotateFailed'));
    expect(words.closest('[role="alert"]')).not.toBeNull();
    expect(dialog.isConnected).toBe(true);
    expect(toast).not.toHaveBeenCalled();
    expect(screen.getByText(FRIEND_CODE)).toBeTruthy();
  });
});

describe('Friends: an action that went through, and the read after it failed', () => {
  it('a card’s action says its success, then the failed read as a read failure', async () => {
    const keys = await mount();
    vi.mocked(refresh).mockRejectedValueOnce(new Error('timeout'));

    await keys.click(within(cardOf('Inna')).getByRole('button', { name: msg('social.relationIncoming') }));

    await vi.waitFor(() => expect(toasts()).toHaveLength(2));
    expect(accept).toHaveBeenCalledWith('r-inna');
    expect(toasts()).toEqual([
      [msg('social.friendsNowFriends'), 'success'],
      [msg('social.friendsRefreshFailed'), 'danger'],
    ]);
  });

  it('a confirmed one closes its dialog as done, then says the failed read', async () => {
    const keys = await mount();
    const dialog = await openMenuItem(keys, 'Gleb', 'social.unfriendLabel');
    vi.mocked(refresh).mockRejectedValueOnce(new Error('timeout'));

    await keys.click(confirmButton(dialog));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(toasts()).toEqual([
      [msg('social.friendsUnfriended'), 'success'],
      [msg('social.friendsRefreshFailed'), 'danger'],
    ]);
  });
});

describe('Friends: the add dialog says both of its failures in itself', () => {
  async function found(keys: ReturnType<typeof user>) {
    await keys.click(screen.getByRole('button', { name: msg('social.friendsAddFriend') }));
    const dialog = screen.getByRole('dialog', { name: msg('social.friendsAddFriend') });
    const field = within(dialog).getByRole('textbox', { name: msg('social.friendsSearchLabel') });
    return { dialog, field };
  }

  it('a failed send: a danger notice in the dialog, no toast, the dialog stays', async () => {
    vi.mocked(profile).mockResolvedValue(lena);
    vi.mocked(sendRequest).mockRejectedValueOnce(new Error('server said no'));
    const keys = await mount();
    const { dialog, field } = await found(keys);
    await keys.type(field, 'lena{Enter}');

    await keys.click(await within(dialog).findByRole('button', { name: msg('social.friendsSendRequest') }));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('social.friendsSendRequestFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(dialog.isConnected).toBe(true);
    expect(toast).not.toHaveBeenCalled();
  });

  it('Enter while a lookup runs does not look up again', async () => {
    let answer!: (value: PublicProfile) => void;
    vi.mocked(profile).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const keys = await mount();
    const { dialog, field } = await found(keys);
    await keys.type(field, 'lena{Enter}');

    await keys.keyboard('{Enter}');
    answer(lena);

    expect(await within(dialog).findByText('@lena')).toBeTruthy();
    expect(profile).toHaveBeenCalledTimes(1);
  });

  it('Enter in the field while a send runs neither looks up again nor sends twice', async () => {
    let sent!: (value: SendResult) => void;
    vi.mocked(profile).mockResolvedValue(lena);
    vi.mocked(sendRequest).mockReturnValueOnce(new Promise((resolve) => (sent = resolve)));
    const keys = await mount();
    const { dialog, field } = await found(keys);
    await keys.type(field, 'lena{Enter}');
    await keys.click(await within(dialog).findByRole('button', { name: msg('social.friendsSendRequest') }));

    field.focus();
    await keys.keyboard('{Enter}');
    sent({ accepted: false } as SendResult);

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect([vi.mocked(profile).mock.calls.length, vi.mocked(sendRequest).mock.calls.length]).toEqual([1, 1]);
  });
});
