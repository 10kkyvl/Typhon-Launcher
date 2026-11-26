import { Events } from '@wailsio/runtime';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import { accept, blocks, decline, friends, refresh, unblock, unfriend, type FriendsPage, type UserCard } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { initSocial } from '../../lib/stores/social';
import { toast } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { NoResize, card, friend, request } from './friendsTesting';

// An action waits on Go by its own card or row: that card's pressed button is busy and a second press
// on the card sends nothing, while a press on another card, or a confirmed Unfriend or Block, is sent
// at once with its own busy mark and its own answer. The lists are read again after each answer; reads
// asked while one runs make one more read after it, and no read holds another card.

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

const ANA = friend('u-ana', 'Ana', 'online');
const GLEB = friend('u-gleb', 'Gleb', 'online');
const INNA = request('r-inna', 'Inna');
const KIRILL = request('r-kirill', 'Kirill');
const OLEG = card('b-oleg', 'Oleg');
const PETR = card('b-petr', 'Petr');

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
const emitFriends = (page: FriendsPage) => friendsEvent?.({ data: page });
const cardOf = (name: string) => screen.getByText(name).closest('.request') as HTMLElement;
const acceptOf = (name: string) => within(cardOf(name)).getByRole('button', { name: msg('social.relationIncoming') });
const declineOf = (name: string) => within(cardOf(name)).getByRole('button', { name: msg('social.declineButton') });
const unblockOf = (name: string) => within(cardOf(name)).getByRole('button', { name: msg('social.unblockButton') });
const isBusy = (button: HTMLElement) => button.getAttribute('aria-busy') === 'true';

async function mount(page: FriendsPage, props: { tab?: string } = {}, blocked: UserCard[] = []) {
  vi.mocked(friends).mockResolvedValue(page);
  vi.mocked(blocks).mockResolvedValue(blocked);
  render(Friends, { target: document.getElementById('app') as HTMLElement, props });
  const keys = user();
  await screen.findByText(props.tab === 'blocked' ? blocked[0].displayName : page.friends[0].displayName);
  await vi.waitFor(() => expect(blocks).toHaveBeenCalled());
  await tick();
  return keys;
}

async function confirmFor(keys: ReturnType<typeof user>, name: string, item: 'social.unfriendLabel' | 'social.blockLabel') {
  await keys.click(screen.getByRole('button', { name: msg('social.friendMenuFor', { name }) }));
  await keys.click(screen.getByRole('menuitem', { name: msg(item) }));
  const dialog = screen.getByRole('dialog');
  const buttons = within(dialog).getAllByRole('button');
  return { dialog, confirm: buttons[buttons.length - 1] };
}

beforeAll(async () => {
  settings.set({ accountSync: true } as Settings);
  await initSocial();
  friendsEvent = vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1] as typeof friendsEvent;
});

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ accountSync: true } as Settings);
  authState.set('authenticated');
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  authState.set('bootstrapping');
  // Back to each mock's own answer: a queued answer a test left unused must not reach the next one.
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: a press while another card’s action runs', () => {
  it('P2 the next card’s Accept, pressed while the read after the first accept runs, is sent or said', async () => {
    const keys = await mount({ friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] });
    const reread = deferred<void>();
    vi.mocked(refresh).mockReturnValueOnce(reread.promise);

    await keys.click(acceptOf('Inna'));
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(2));
    // Go's own poll after the accept: the card leaves before the screen's read answers.
    emitFriends({ friends: [ANA, friend('r-inna', 'Inna', 'online')], incoming: [KIRILL], outgoing: [] });
    await vi.waitFor(() => expect(document.activeElement).toBe(acceptOf('Kirill')));

    await keys.keyboard('{Enter}');
    reread.resolve();
    await tick();
    await tick();

    expect(vi.mocked(accept).mock.calls).toEqual([['r-inna'], ['r-kirill']]);
  });

  it('P3 a confirmed Unfriend while a card’s accept runs is sent, or the dialog says why not', async () => {
    const keys = await mount({ friends: [ANA, GLEB], incoming: [INNA], outgoing: [] });
    const answer = deferred<void>();
    vi.mocked(accept).mockReturnValueOnce(answer.promise);
    await keys.click(acceptOf('Inna'));

    await keys.click(screen.getByRole('button', { name: msg('social.friendMenuFor', { name: 'Gleb' }) }));
    await keys.click(screen.getByRole('menuitem', { name: msg('social.unfriendLabel') }));
    const dialog = screen.getByRole('dialog');
    const buttons = within(dialog).getAllByRole('button');
    await keys.click(buttons[buttons.length - 1]);
    await tick();

    const open = dialog.isConnected;
    answer.resolve();
    expect(vi.mocked(unfriend).mock.calls.length > 0 || open).toBe(true);
  });

  it('two cards pressed in a row send two requests, each button busy until its own answer', async () => {
    const keys = await mount({ friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] });
    const inna = deferred<void>();
    const kirill = deferred<void>();
    vi.mocked(accept).mockReturnValueOnce(inna.promise).mockReturnValueOnce(kirill.promise);

    await keys.click(acceptOf('Inna'));
    await keys.click(acceptOf('Kirill'));

    expect(vi.mocked(accept).mock.calls).toEqual([['r-inna'], ['r-kirill']]);
    expect([isBusy(acceptOf('Inna')), isBusy(acceptOf('Kirill'))]).toEqual([true, true]);
    expect([cardOf('Inna').classList.contains('settling'), cardOf('Kirill').classList.contains('settling')]).toEqual([true, true]);

    kirill.resolve();
    await vi.waitFor(() => expect(isBusy(acceptOf('Kirill'))).toBe(false));
    expect(isBusy(acceptOf('Inna'))).toBe(true);
    expect(cardOf('Kirill').classList.contains('settling')).toBe(false);

    inna.resolve();
    await vi.waitFor(() => expect(isBusy(acceptOf('Inna'))).toBe(false));
  });

  it('the same card pressed twice, or its other button, sends one request', async () => {
    const keys = await mount({ friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] });
    const answer = deferred<void>();
    vi.mocked(accept).mockReturnValueOnce(answer.promise);

    await keys.click(acceptOf('Inna'));
    await keys.click(acceptOf('Inna'));
    await keys.click(declineOf('Inna'));

    expect(vi.mocked(accept).mock.calls).toEqual([['r-inna']]);
    expect(decline).not.toHaveBeenCalled();
    expect(isBusy(declineOf('Inna'))).toBe(false);
    answer.resolve();
    await vi.waitFor(() => expect(isBusy(acceptOf('Inna'))).toBe(false));
  });

  it('a first action that fails does not stop the second', async () => {
    const keys = await mount({ friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] });
    const inna = deferred<void>();
    vi.mocked(accept).mockReturnValueOnce(inna.promise);

    await keys.click(acceptOf('Inna'));
    inna.reject(new Error('server said no'));
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith(msg('social.acceptRequestFailed'), 'danger'));
    await keys.click(acceptOf('Kirill'));

    await vi.waitFor(() => expect(vi.mocked(accept).mock.calls).toEqual([['r-inna'], ['r-kirill']]));
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith(msg('social.friendsNowFriends'), 'success'));
  });

  it('a dialog confirmed while a card’s action runs sends its request and closes when it is done', async () => {
    const keys = await mount({ friends: [ANA, GLEB], incoming: [INNA], outgoing: [] });
    const answer = deferred<void>();
    vi.mocked(accept).mockReturnValueOnce(answer.promise);
    await keys.click(acceptOf('Inna'));

    const { confirm } = await confirmFor(keys, 'Gleb', 'social.unfriendLabel');
    await keys.click(confirm);

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(unfriend).mock.calls).toEqual([['u-gleb']]);
    expect(isBusy(acceptOf('Inna'))).toBe(true);
    answer.resolve();
    await vi.waitFor(() => expect(isBusy(acceptOf('Inna'))).toBe(false));
  });
});

describe('Friends: the reads after actions', () => {
  it('reads asked while one runs make one more read after it, and every card waits for its own', async () => {
    const keys = await mount({ friends: [ANA], incoming: [INNA, KIRILL, request('r-lev', 'Lev')], outgoing: [] });
    const first = deferred<void>();
    vi.mocked(refresh).mockReturnValueOnce(first.promise);

    await keys.click(acceptOf('Inna'));
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(2));
    await keys.click(acceptOf('Kirill'));
    await keys.click(acceptOf('Lev'));
    await vi.waitFor(() => expect(vi.mocked(accept).mock.calls).toHaveLength(3));
    await tick();
    expect(refresh).toHaveBeenCalledTimes(2);
    expect([isBusy(acceptOf('Kirill')), isBusy(acceptOf('Lev'))]).toEqual([true, true]);

    first.resolve();

    await vi.waitFor(() => expect(isBusy(acceptOf('Lev'))).toBe(false));
    expect(isBusy(acceptOf('Kirill'))).toBe(false);
    expect(refresh).toHaveBeenCalledTimes(3);
  });

  it('an older blocked list that answers last does not replace the newer one', async () => {
    const keys = await mount({ friends: [ANA], incoming: [], outgoing: [] }, { tab: 'blocked' }, [OLEG, PETR]);
    const older = deferred<UserCard[]>();
    vi.mocked(blocks).mockReturnValueOnce(older.promise).mockResolvedValueOnce([]);

    await keys.click(unblockOf('Oleg'));
    await vi.waitFor(() => expect(blocks).toHaveBeenCalledTimes(2));
    await keys.click(unblockOf('Petr'));
    await vi.waitFor(() => expect(blocks).toHaveBeenCalledTimes(3));
    await vi.waitFor(() => expect(screen.getByRole('heading', { name: msg('social.friendsEmptyBlockedTitle') })).toBeTruthy());

    older.resolve([PETR]);
    await tick();
    await tick();

    expect(screen.queryByText('Petr')).toBeNull();
    expect(vi.mocked(unblock).mock.calls).toEqual([['b-oleg'], ['b-petr']]);
  });
});
