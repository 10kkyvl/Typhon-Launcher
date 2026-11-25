import { cleanup, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { accept, blocks, decline, friends, refresh, unblock, unfriend, type FriendView, type RequestView, type UserCard } from '../../lib/services/social';
import type { Settings } from '../../lib/services/settings';
import { commonLine } from '../../lib/social/view';
import { navigate } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { friendsPage } from '../../lib/stores/social';
import { friendsView } from '../../lib/stores/ui';
import { authState } from '../../lib/stores/user';
import { user as userEvents } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Friends from './Friends.svelte';

// The page keeps what its parts share and hands it down: the search and the sorting (bound), the
// presence group, the list's frame reserve, the busy action, the add, menu and profile callbacks, and
// the buttons of each card list as a snippet. A prop passed without `bind:`, a callback not wired or a
// reserve taken from the wrong list breaks nothing a type check sees: these tests go through each such
// seam the way a player does, and through the page's reads and what makes them run again.
// Not held here: that a card slides in only when one comes and not when a tab mounts the list (the transitions are local; no test counts them), the blocked list read again once the
// sync consent is given (only the sign-in is driven), the grid view's rows (the same props as the
// list's), the friend code card, the consent dialog and the add dialog's own lookup, which are the
// page's and its children's, not seams of this split.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/stores/router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/router')>()),
  navigate: vi.fn(),
}));
vi.mock('../../lib/stores/toasts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(),
}));

const framed = {
  accent: '#3b82f6',
  avatarFrame: 'custom:0b5a6c1e-1d2f-4a3b-9c8d-7e6f5a4b3c2d',
  frameImage: { url: 'https://example.invalid/frame.png', hole: 0.4 },
  nameStyle: 'plain',
} as unknown as UserCard['card'];

const card = (id: string, name: string, extra: Partial<UserCard> = {}): UserCard => ({ id, username: name.toLowerCase(), displayName: name, avatarUrl: '', ...extra });
const friend = (id: string, name: string, status: string, extra: Partial<UserCard> = {}): FriendView => ({ ...card(id, name, extra), since: '2026-01-01T00:00:00Z', presence: { status } });
const request = (id: string, name: string, commonCount: number): RequestView => ({ ...card(id, name), createdAt: '2026-11-20T11:00:00Z', mutualCount: 0, commonCount });

const FRIENDS = [friend('u-ana', 'Ana', 'online', { card: framed }), friend('u-boris', 'Boris', 'busy'), friend('u-vera', 'Vera', 'away'), friend('u-gleb', 'Gleb', 'offline')];
const INCOMING = [request('r-inna', 'Inna', 3), request('r-kirill', 'Kirill', 0)];
const OUTGOING = [request('r-lev', 'Lev', 1)];
const BLOCKED = [card('b-oleg', 'Oleg')];

const rowNames = () => [...document.querySelectorAll('.row .name')].map((name) => name.textContent);
const tabRadio = (label: string) => screen.getAllByRole('radio').find((radio) => radio.textContent?.trim().startsWith(label)) as HTMLElement;
const cardOf = (name: string) => screen.getByText(name).closest('.request') as HTMLElement;
const search = () => screen.getByRole('textbox', { name: msg('social.friendsSearchFriendsPlaceholder') }) as HTMLInputElement;

async function mount(props: { tab?: string } = {}) {
  const app = document.getElementById('app') as HTMLElement;
  const view = render(Friends, { target: app, props });
  await tick();
  await vi.waitFor(() => expect(blocks).toHaveBeenCalled());
  return view;
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: true } as Settings);
  friendsPage.set({ friends: FRIENDS, incoming: INCOMING, outgoing: OUTGOING });
  vi.mocked(friends).mockResolvedValue({ friends: FRIENDS, incoming: INCOMING, outgoing: OUTGOING });
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
  friendsView.set('list');
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: the list part', () => {
  it('keeps the search and the sorting picked in the list through a trip to the requests tab', async () => {
    await mount();
    const keys = userEvents();

    await keys.type(search(), 'a');
    await keys.click(screen.getByRole('button', { name: new RegExp(msg('social.friendsSortStatus')) }));
    await keys.click(screen.getByRole('menuitemradio', { name: msg('social.friendsSortName') }));
    expect(rowNames()).toEqual(['Ana', 'Vera']);

    await keys.click(tabRadio(msg('social.friendsTabRequests')));
    await keys.click(tabRadio(msg('social.friendsTabAll')));

    expect(search().value).toBe('a');
    expect(rowNames()).toEqual(['Ana', 'Vera']);
    expect(screen.getByRole('button', { name: new RegExp(msg('social.friendsSortName').replace(/[()]/g, '\\$&')) })).toBeTruthy();
  });

  it('shows the group of the presence tab and that tab’s own empty words', async () => {
    await mount();
    const keys = userEvents();

    await keys.click(tabRadio(msg('social.friendsTabAway')));
    expect(rowNames()).toEqual(['Vera']);

    friendsPage.set({ friends: FRIENDS.filter((f) => f.presence?.status !== 'away'), incoming: [], outgoing: [] });
    await tick();
    expect(rowNames()).toEqual([]);
    expect(screen.getByText(msg('social.friendsEmptyAwayTitle'))).toBeTruthy();
  });

  it('frames the rows by the whole group, not by what the search leaves', async () => {
    await mount();

    await userEvents().type(search(), 'bor');

    expect(rowNames()).toEqual(['Boris']);
    expect((document.querySelector('.row') as HTMLElement).style.getPropertyValue('--reserve-l')).toBe('0.5');
  });

  it('opens the page’s add dialog from the empty list', async () => {
    friendsPage.set({ friends: [], incoming: [], outgoing: [] });
    await mount();

    const adds = screen.getAllByRole('button', { name: new RegExp(msg('social.friendsAddFriend')) });
    await userEvents().click(adds[adds.length - 1]);

    expect(screen.getByRole('dialog', { name: msg('social.friendsAddFriend') })).toBeTruthy();
  });

  it('takes a row’s menu to the page’s confirmation and runs its action', async () => {
    await mount();
    const keys = userEvents();

    await keys.click(screen.getByRole('button', { name: msg('social.friendMenuFor', { name: 'Gleb' }) }));
    await keys.click(screen.getByRole('menuitem', { name: msg('social.unfriendLabel') }));
    const dialog = screen.getByRole('dialog');
    const buttons = within(dialog).getAllByRole('button');
    await keys.click(buttons[buttons.length - 1]);

    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(2));
    expect(unfriend).toHaveBeenCalledWith('u-gleb');
  });

  it('opens a friend’s profile from the row', async () => {
    await mount();

    await userEvents().click(screen.getByText('Boris').closest('button.who') as HTMLElement);

    expect(navigate).toHaveBeenCalledWith('user', { username: 'boris' });
  });
});

describe('Friends: the card lists', () => {
  it('dims the card whose action runs and holds a press on any card until it answers', async () => {
    let answer!: () => void;
    vi.mocked(accept).mockReturnValueOnce(new Promise<void>((resolve) => (answer = resolve)));
    await mount();
    const keys = userEvents();

    await keys.click(within(cardOf('Inna')).getByRole('button', { name: msg('social.relationIncoming') }));

    expect(accept).toHaveBeenCalledWith('r-inna');
    expect(cardOf('Inna').classList.contains('settling')).toBe(true);
    expect(cardOf('Kirill').classList.contains('settling')).toBe(false);
    await keys.click(within(cardOf('Kirill')).getByRole('button', { name: msg('social.declineButton') }));
    expect(decline).not.toHaveBeenCalled();

    answer();
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(cardOf('Inna').classList.contains('settling')).toBe(false));
  });

  it('sends a card’s action for that card, not for the first of its list', async () => {
    await mount();

    await userEvents().click(within(cardOf('Kirill')).getByRole('button', { name: msg('social.declineButton') }));

    await vi.waitFor(() => expect(decline).toHaveBeenCalledWith('r-kirill'));
    expect(decline).toHaveBeenCalledTimes(1);
  });

  it('gives each list its own stats and profile link, and a blocked card none', async () => {
    await mount({ tab: 'requests' });
    const keys = userEvents();

    expect([...cardOf('Inna').querySelectorAll('.stat')].map((s) => s.textContent)).toContain(commonLine(0, 3));
    expect(cardOf('Lev').querySelector('.stat:last-child')?.textContent).toContain(msg('friends.pendingReply'));
    await keys.click(within(cardOf('Lev')).getByRole('button', { name: /Lev/ }));
    expect(navigate).toHaveBeenCalledWith('user', { username: 'lev' });

    await keys.click(tabRadio(msg('social.friendsTabBlocked')));
    expect(cardOf('Oleg').querySelector('button.who')).toBeNull();
    expect(cardOf('Oleg').querySelector('div.who')).not.toBeNull();
  });

  it('frames each list by its own users', async () => {
    friendsPage.set({ friends: FRIENDS, incoming: [{ ...INCOMING[0], card: framed }], outgoing: OUTGOING });
    vi.mocked(friends).mockResolvedValue({ friends: FRIENDS, incoming: [{ ...INCOMING[0], card: framed }], outgoing: OUTGOING });
    await mount({ tab: 'requests' });

    const reserve = (name: string) => (cardOf(name).querySelector('.avatar') as HTMLElement).style.getPropertyValue('--reserve-l');
    expect([reserve('Inna'), reserve('Lev')]).toEqual(['0.5', '0']);
  });

  it('reads the blocked list again after an unblock', async () => {
    await mount({ tab: 'blocked' });

    await userEvents().click(within(cardOf('Oleg')).getByRole('button', { name: msg('social.unblockButton') }));

    await vi.waitFor(() => expect(blocks).toHaveBeenCalledTimes(2));
    expect(unblock).toHaveBeenCalledWith('b-oleg');
  });
});

describe('Friends: the page’s reads', () => {
  it('reads nothing for a guest, and the friends and the blocked list once a guest signs in', async () => {
    authState.set('guest');
    render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
    await tick();
    expect([vi.mocked(refresh).mock.calls.length, vi.mocked(blocks).mock.calls.length]).toEqual([0, 0]);

    authState.set('authenticated');
    await vi.waitFor(() => expect(blocks).toHaveBeenCalledTimes(1));
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('opens on the tab it is given, and on a new one when it changes', async () => {
    const view = await mount({ tab: 'blocked' });
    expect(cardOf('Oleg')).toBeTruthy();

    await view.rerender({ tab: 'requests' });

    expect(cardOf('Lev')).toBeTruthy();
  });
});
