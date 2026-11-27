import { Events } from '@wailsio/runtime';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import { blocks, friendCode, friends, kick, refresh, type FriendsPage } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { initSocial } from '../../lib/stores/social';
import { toast } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { FRIEND_CODE, NoResize, card, friend } from './friendsTesting';

// The screen's first reads go through the social store as in the app: the store is started once
// (initSocial), and every test begins as a fresh sign-in, so nothing has been read for the account.
// A read that has not answered is loading, a failed one is a danger notice with Retry, and "nobody
// here" is said only after a read found nobody. The same holds for the blocked list and the code.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/stores/toasts', async (original) => ({
  ...(await original<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(),
}));

const PAGE: FriendsPage = { friends: [friend('u-ana', 'Ana', 'online'), friend('u-gleb', 'Gleb', 'offline')], incoming: [], outgoing: [] };
const NOBODY: FriendsPage = { friends: [], incoming: [], outgoing: [] };

const loadingMark = () => screen.queryByText(msg('social.loadingEllipsis'))?.closest('[role="status"]') ?? null;
const nobody = () => screen.queryByRole('heading', { name: msg('social.friendsEmptyNobodyTitle') });
const tabRadio = (label: string) => screen.getAllByRole('radio').find((radio) => radio.textContent?.trim().startsWith(label)) as HTMLElement;
const search = () => screen.getByRole('textbox', { name: msg('social.friendsSearchFriendsPlaceholder') });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

let friendsEvent: ((event: { data: unknown }) => void) | undefined;

/** What Go sends with `social:friends` after a read of its own. */
function emitFriends(page: FriendsPage) {
  if (!friendsEvent) throw new Error('the store has not subscribed to social:friends');
  friendsEvent({ data: page });
}

function mount(props: { tab?: string } = {}) {
  render(Friends, { target: document.getElementById('app') as HTMLElement, props });
  return user();
}

beforeAll(async () => {
  settings.set({ accountSync: true } as Settings);
  await initSocial();
  // Started signed out: each test then signs in, a change of account that clears what the start read.
  authState.set('unauthenticated');
  friendsEvent = vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1] as typeof friendsEvent;
});

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ accountSync: true } as Settings);
  vi.mocked(friends).mockResolvedValue(PAGE);
  authState.set('authenticated');
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  authState.set('unauthenticated');
  // Back to each mock's own answer: a queued answer a test left unused must not reach the next one.
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: the first read of the friends', () => {
  it('is loading while it runs, not "nobody here", and counts nobody yet', async () => {
    const read = deferred<void>();
    vi.mocked(refresh).mockReturnValueOnce(read.promise);
    vi.mocked(friends).mockResolvedValue(NOBODY);
    mount();
    await tick();

    expect(loadingMark()).not.toBeNull();
    expect(nobody()).toBeNull();
    expect(tabRadio(msg('social.friendsTabAll')).textContent).not.toMatch(/\d/);

    read.resolve();

    expect(await screen.findByRole('heading', { name: msg('social.friendsEmptyNobodyTitle') })).toBeTruthy();
    expect(loadingMark()).toBeNull();
  });

  it('that failed is a danger notice with Retry in place of the lists, said once', async () => {
    vi.mocked(refresh).mockRejectedValueOnce(new Error('timeout'));
    mount();

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('state.socialLoadFriendsFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(screen.getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(nobody()).toBeNull();
    expect(loadingMark()).toBeNull();
    expect(toast).not.toHaveBeenCalled();
  });

  it('is read again by Retry, which shows the lists and hands focus to them', async () => {
    vi.mocked(refresh).mockRejectedValueOnce(new Error('timeout'));
    const keys = mount();
    const retry = await screen.findByRole('button', { name: msg('common.retry') });

    await keys.click(retry);

    expect(await screen.findByText('Ana')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(document.activeElement).toBe(search());
  });

  it('that fails again keeps the notice, says it again and keeps the focus on Retry', async () => {
    vi.mocked(refresh).mockRejectedValueOnce(new Error('timeout')).mockRejectedValueOnce(new Error('timeout'));
    const keys = mount();
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    const region = notice.querySelector('.body') as HTMLElement;
    const before = region.firstElementChild;

    await keys.click(retry);

    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(refresh).toHaveBeenCalledTimes(2);
    expect(notice.isConnected).toBe(true);
    expect(region.textContent?.trim()).toBe(msg('state.socialLoadFriendsFailed'));
    expect(document.activeElement).toBe(retry);
    expect(screen.queryByText('Ana')).toBeNull();
  });

  it('is taken when it comes from Go on its own', async () => {
    vi.mocked(refresh).mockReturnValueOnce(new Promise(() => {}));
    mount();
    await tick();

    emitFriends(PAGE);

    expect(await screen.findByText('Ana')).toBeTruthy();
    expect(loadingMark()).toBeNull();
  });

  it('runs when a guest signs in on the screen', async () => {
    authState.set('guest');
    mount();
    await tick();
    expect(refresh).not.toHaveBeenCalled();

    authState.set('authenticated');

    expect(await screen.findByText('Ana')).toBeTruthy();
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('keeps the lists when a later read fails, and says that once in a toast', async () => {
    emitFriends(PAGE);
    vi.mocked(refresh).mockRejectedValueOnce(new Error('timeout'));
    mount();

    await vi.waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(vi.mocked(toast).mock.calls[0]).toEqual([msg('social.friendsRefreshFailed'), 'danger']);
    expect(screen.getByText('Ana')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

describe('Friends: the reads of a sign-in', () => {
  // A start is pinned in Friends.start.dom.test.ts (the screen as the first route) and in
  // stores/social.test.ts (no screen); Go's poll after the kick sends the page with social:friends.
  it('the screen there at the sign-in reads once: one kick, one refresh, one Friends(), one blocked list', async () => {
    authState.set('unauthenticated');
    vi.clearAllMocks();
    authState.set('authenticated');
    mount();
    expect(await screen.findByText('Ana')).toBeTruthy();
    await vi.waitFor(() => expect(blocks).toHaveBeenCalled());
    emitFriends(PAGE);
    await tick();

    const counts = [kick, refresh, friends, blocks].map((call) => vi.mocked(call).mock.calls.length);
    expect(counts).toEqual([1, 1, 1, 1]);
  });
});

describe('Friends: the first read of the blocked list', () => {
  it('is loading while it runs, not "nobody blocked"', async () => {
    vi.mocked(blocks).mockReturnValueOnce(new Promise(() => {}));
    mount({ tab: 'blocked' });
    await tick();

    expect(loadingMark()).not.toBeNull();
    expect(screen.queryByRole('heading', { name: msg('social.friendsEmptyBlockedTitle') })).toBeNull();
  });

  it('that failed is a danger notice with Retry, and Retry shows the list', async () => {
    vi.mocked(blocks).mockRejectedValueOnce(new Error('timeout')).mockResolvedValueOnce([card('b-oleg', 'Oleg')]);
    const keys = mount({ tab: 'blocked' });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('social.friendsBlockedLoadFailed'));
    expect(screen.queryByRole('heading', { name: msg('social.friendsEmptyBlockedTitle') })).toBeNull();
    expect(toast).not.toHaveBeenCalled();

    await keys.click(screen.getByRole('button', { name: msg('common.retry') }));

    expect(await screen.findByText('Oleg')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

describe('Friends: the first read of the friend code', () => {
  it('that failed says so in the card with Retry, not a dash, and Retry shows the code', async () => {
    vi.mocked(friendCode).mockRejectedValueOnce(new Error('timeout'));
    const keys = mount();

    await keys.click(screen.getByRole('button', { name: msg('social.friendsMyCodeButton') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('social.friendsCodeFetchFailed'));
    expect(screen.queryByText('—')).toBeNull();
    expect(toast).not.toHaveBeenCalled();

    await keys.click(within(alert.closest('.notice') as HTMLElement).getByRole('button', { name: msg('common.retry') }));

    expect(await screen.findByText(FRIEND_CODE)).toBeTruthy();
  });
});
