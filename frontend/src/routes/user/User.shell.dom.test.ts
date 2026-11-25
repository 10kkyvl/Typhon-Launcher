import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { accountMessage } from '../../lib/services/accountMessages';
import {
  block,
  profile as fetchProfile,
  refresh,
  sendRequest,
  unfriend,
  type PublicProfile,
  type SendResult,
} from '../../lib/services/social';
import { navigate, resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import UserRouteHarness from '../../lib/testing/UserRouteHarness.svelte';
import { player } from './userTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));

const said = () => get(toasts).map((item) => [item.kind, item.message]);
const bob = () => player({ id: 'u2', username: 'bob', displayName: 'Bob' });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => ((resolve = res), (reject = rej)));
  return { promise, resolve, reject };
}

// Lets a late answer run all it would run: its toast, a re-read, a navigation, a focus hand-off.
async function settle() {
  await tick();
  await new Promise((resolve) => setTimeout(resolve, 0));
  await tick();
}

async function openAnn(first: PublicProfile) {
  vi.mocked(fetchProfile).mockResolvedValue(first);
  const rendered = renderDom(UserRouteHarness, {});
  await rendered.user.click(screen.getByRole('button', { name: 'Open Ann' }));
  await screen.findByText('@ann', { selector: '.username' });
  return rendered;
}

async function pressAdd(user: ReturnType<typeof renderDom>['user']) {
  screen.getByRole('button', { name: msg('social.relationNone') }).focus();
  await user.keyboard('{Enter}');
}

async function confirmFromMenu(user: ReturnType<typeof renderDom>['user'], item: string) {
  await user.click(screen.getByRole('button', { name: msg('social.moreLabel') }));
  await user.click(screen.getByRole('menuitem', { name: item }));
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: item }));
}

async function leaveForLibrary() {
  navigate('library');
  await screen.findByRole('heading', { level: 1, name: 'Library' });
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  authState.set('authenticated');
  resetBindings();
  toasts.set([]);
  vi.mocked(refresh).mockResolvedValue(undefined);
});

afterEach(() => {
  expect(calls).toEqual([]);
  for (const mock of [fetchProfile, refresh, sendRequest, unfriend, block]) vi.mocked(mock).mockReset();
  document.body.replaceChildren();
  authState.set('bootstrapping');
  resetHistory();
  vi.unstubAllGlobals();
});

describe("a user's page in the shell: a block answered on the page", () => {
  it('opens the blocked players with the focus on their heading', async () => {
    vi.mocked(block).mockResolvedValue(undefined);
    const { user } = await openAnn(player());

    await confirmFromMenu(user, msg('social.blockLabel'));

    await vi.waitFor(() => expect(get(route)).toMatchObject({ name: 'friends', params: { tab: 'blocked' } }));
    await settle();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Friends' }));
  });
});

describe("a user's page in the shell: an answer that arrives after the player left", () => {
  it('Add answered on another screen: the success names the player once; nothing is re-read', async () => {
    const answer = deferred<SendResult>();
    vi.mocked(sendRequest).mockReturnValue(answer.promise);
    const { user } = await openAnn(player());
    await pressAdd(user);
    navigate('friends');
    await screen.findByRole('heading', { level: 1, name: 'Friends' });
    const reads = vi.mocked(fetchProfile).mock.calls.length;

    answer.resolve({ accepted: false } as SendResult);
    await vi.waitFor(() => expect(said()).toHaveLength(1));
    await settle();

    expect(said()).toEqual([['success', msg('social.userRequestSentTo', { name: 'Ann' })]]);
    expect(vi.mocked(fetchProfile).mock.calls.length).toBe(reads);
    expect(vi.mocked(refresh)).not.toHaveBeenCalled();
    expect(get(route).name).toBe('friends');
  });

  it("Add answered on another player's page: named, nothing re-read, and that page is not touched", async () => {
    const answer = deferred<SendResult>();
    vi.mocked(sendRequest).mockReturnValue(answer.promise);
    const { user } = await openAnn(player());
    await pressAdd(user);
    vi.mocked(fetchProfile).mockImplementation(async (name) => (name === 'bob' ? bob() : player({ relation: 'outgoing' })));
    navigate('user', { username: 'bob' });
    await screen.findByText('@bob', { selector: '.username' });
    const bobAdd = screen.getByRole('button', { name: msg('social.relationNone') });
    expect(bobAdd.hasAttribute('aria-busy')).toBe(false);

    answer.resolve({ accepted: false } as SendResult);
    await vi.waitFor(() => expect(said()).toHaveLength(1));
    await settle();

    expect(said()).toEqual([['success', msg('social.userRequestSentTo', { name: 'Ann' })]]);
    expect(vi.mocked(fetchProfile).mock.calls).toEqual([['ann'], ['bob']]);
    expect(vi.mocked(refresh)).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(bobAdd);
    expect(document.querySelector('.profile')?.classList.contains('refreshing')).toBe(false);
    expect(screen.getByText('@bob', { selector: '.username' })).toBeTruthy();
  });

  it("another player's page is not held by the action still running for the one left", async () => {
    vi.mocked(sendRequest).mockReturnValueOnce(new Promise(() => {})).mockResolvedValueOnce({ accepted: false } as SendResult);
    const { user } = await openAnn(player());
    await pressAdd(user);
    vi.mocked(fetchProfile).mockImplementation(async (name) => (name === 'bob' ? bob() : player()));
    navigate('user', { username: 'bob' });
    await screen.findByText('@bob', { selector: '.username' });

    const bobAdd = screen.getByRole('button', { name: msg('social.relationNone') });
    expect(bobAdd.hasAttribute('aria-busy')).toBe(false);
    expect(screen.getByRole('button', { name: msg('social.moreLabel') }).hasAttribute('aria-disabled')).toBe(false);
    await user.click(bobAdd);

    expect(vi.mocked(sendRequest).mock.calls).toEqual([['ann'], ['bob']]);
  });

  it('a block answered on another screen stays there: named, nothing re-read, no blocked list', async () => {
    const answer = deferred<void>();
    vi.mocked(block).mockReturnValue(answer.promise);
    const { user } = await openAnn(player());
    await confirmFromMenu(user, msg('social.blockLabel'));
    await leaveForLibrary();

    answer.resolve(undefined);
    await vi.waitFor(() => expect(said()).toHaveLength(1));
    await settle();

    expect(get(route).name).toBe('library');
    expect(said()).toEqual([['success', msg('social.userBlockedNamed', { name: 'Ann' })]]);
    expect(vi.mocked(refresh)).not.toHaveBeenCalled();
  });

  it('a failed Add answered on another screen names the player once', async () => {
    const answer = deferred<SendResult>();
    vi.mocked(sendRequest).mockReturnValue(answer.promise);
    const { user } = await openAnn(player());
    await pressAdd(user);
    await leaveForLibrary();

    answer.reject(new Error('offline'));
    await vi.waitFor(() => expect(said()).toHaveLength(1));
    await settle();

    expect(said()).toEqual([['danger', msg('social.userSendRequestFailedTo', { name: 'Ann' })]]);
  });

  it("a failed unfriend whose dialog is gone names the player once, with its code's words", async () => {
    const answer = deferred<void>();
    vi.mocked(unfriend).mockReturnValue(answer.promise);
    const { user } = await openAnn(player({ relation: 'friend' }));
    await confirmFromMenu(user, msg('social.unfriendLabel'));
    await leaveForLibrary();

    answer.reject(new AccountError('not_friends'));
    await vi.waitFor(() => expect(said()).toHaveLength(1));
    await settle();

    expect(said()).toEqual([
      [
        'danger',
        msg('social.userActionFailedWhy', {
          what: msg('social.userUnfriendFailedNamed', { name: 'Ann' }),
          why: accountMessage('not_friends'),
        }),
      ],
    ]);
    expect(vi.mocked(refresh)).not.toHaveBeenCalled();
  });

  it('the page left while the friends list is re-read after a done Add: the profile is not re-read', async () => {
    const list = deferred<void>();
    vi.mocked(sendRequest).mockResolvedValue({ accepted: false } as SendResult);
    vi.mocked(refresh).mockReturnValue(list.promise);
    const { user } = await openAnn(player());
    await pressAdd(user);
    await vi.waitFor(() => expect(vi.mocked(refresh)).toHaveBeenCalled());
    await leaveForLibrary();
    const reads = vi.mocked(fetchProfile).mock.calls.length;

    list.resolve(undefined);
    await settle();

    expect(vi.mocked(fetchProfile).mock.calls.length).toBe(reads);
    expect(said()).toEqual([['success', msg('social.relationOutgoing')]]);
  });

  it('the page left while the friends list is re-read after a done block: no blocked list', async () => {
    const list = deferred<void>();
    vi.mocked(block).mockResolvedValue(undefined);
    vi.mocked(refresh).mockReturnValue(list.promise);
    const { user } = await openAnn(player());
    await confirmFromMenu(user, msg('social.blockLabel'));
    await vi.waitFor(() => expect(vi.mocked(refresh)).toHaveBeenCalled());
    await leaveForLibrary();

    list.resolve(undefined);
    await settle();

    expect(get(route).name).toBe('library');
  });
});

