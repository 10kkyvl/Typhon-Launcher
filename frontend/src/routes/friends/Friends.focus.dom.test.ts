import { cleanup, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import { accept, blocks, decline, friends, type FriendsPage, type UserCard } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { friendsPage } from '../../lib/stores/social';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { NoResize, card, friend, request } from './friendsTesting';

// A pressed control keeps focus, busy, until the answer; when its card or row leaves, focus goes on
// by the screen's row rule: the same control of the card now in its place, else of the one above,
// else the list's own first control, else the chosen tab. A dialog whose row left returns focus the
// same way. jsdom does not drop focus from a disabled button as Chromium does: the tests assert that
// the pressed one is not disabled.

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

const ANA = friend('u-ana', 'Ana', 'online');
const GLEB = friend('u-gleb', 'Gleb', 'offline');
const INNA = request('r-inna', 'Inna');
const KIRILL = request('r-kirill', 'Kirill');
const LEV = request('r-lev', 'Lev');
const OLEG = card('b-oleg', 'Oleg');

const cardOf = (name: string) => screen.getByText(name).closest('.request') as HTMLElement;
const tabRadio = (label: string) => screen.getAllByRole('radio').find((radio) => radio.textContent?.trim().startsWith(label)) as HTMLElement;
const menuOf = (name: string) => screen.getByRole('button', { name: msg('social.friendMenuFor', { name }) });

/** The page the screen first reads, then the one each read after it answers. */
function pages(first: FriendsPage, next: FriendsPage) {
  friendsPage.set(first);
  vi.mocked(friends).mockResolvedValueOnce(first).mockResolvedValue(next);
}

async function mount(props: { tab?: string } = {}, blocked: UserCard[] = []) {
  vi.mocked(blocks).mockResolvedValueOnce(blocked).mockResolvedValue([]);
  render(Friends, { target: document.getElementById('app') as HTMLElement, props });
  const keys = user();
  await vi.waitFor(() => expect(friends).toHaveBeenCalled());
  await vi.waitFor(() => expect(blocks).toHaveBeenCalled());
  await tick();
  return keys;
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: true } as Settings);
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

describe('Friends: a pressed card button', () => {
  it('keeps the focus, busy and not disabled, and a press on another card sends its own action at once', async () => {
    pages({ friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] }, { friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] });
    let answer!: () => void;
    vi.mocked(accept).mockReturnValueOnce(new Promise<void>((resolve) => (answer = resolve)));
    const keys = await mount();
    const pressed = within(cardOf('Inna')).getByRole('button', { name: msg('social.relationIncoming') });

    await keys.click(pressed);

    expect(document.activeElement).toBe(pressed);
    expect(pressed.getAttribute('aria-busy')).toBe('true');
    expect((pressed as HTMLButtonElement).disabled).toBe(false);
    const other = within(cardOf('Kirill')).getByRole('button', { name: msg('social.relationIncoming') }) as HTMLButtonElement;
    expect(other.disabled).toBe(false);
    await keys.click(other);
    expect(vi.mocked(accept).mock.calls).toEqual([['r-inna'], ['r-kirill']]);
    expect(document.activeElement).toBe(other);
    expect(pressed.getAttribute('aria-busy')).toBe('true');
    answer();
    await vi.waitFor(() => expect(pressed.getAttribute('aria-busy')).not.toBe('true'));
  });
});

describe('Friends: focus when a card leaves', () => {
  it('an accepted card hands it to the same button of the card now in its place', async () => {
    pages({ friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] }, { friends: [ANA, friend('r-inna', 'Inna', 'online')], incoming: [KIRILL], outgoing: [] });
    const keys = await mount();

    await keys.click(within(cardOf('Inna')).getByRole('button', { name: msg('social.relationIncoming') }));

    await vi.waitFor(() => expect(document.activeElement).toBe(within(cardOf('Kirill')).getByRole('button', { name: msg('social.relationIncoming') })));
  });

  it('the last card of a list hands it to the same button of the card above', async () => {
    pages({ friends: [ANA], incoming: [INNA, KIRILL], outgoing: [] }, { friends: [ANA], incoming: [INNA], outgoing: [] });
    const keys = await mount({ tab: 'requests' });

    await keys.click(within(cardOf('Kirill')).getByRole('button', { name: msg('social.declineButton') }));

    await vi.waitFor(() => expect(document.activeElement).toBe(within(cardOf('Inna')).getByRole('button', { name: msg('social.declineButton') })));
    expect(decline).toHaveBeenCalledWith('r-kirill');
  });

  it('the only card of its list hands it to the chosen tab when the list goes', async () => {
    pages({ friends: [ANA], incoming: [INNA], outgoing: [LEV] }, { friends: [ANA], incoming: [INNA], outgoing: [] });
    const keys = await mount({ tab: 'requests' });

    await keys.click(within(cardOf('Lev')).getByRole('button', { name: msg('social.cancelRequestButton') }));

    await vi.waitFor(() => expect(document.activeElement).toBe(tabRadio(msg('social.friendsTabRequests'))));
  });

  it('the only blocked player, unblocked, hands it to the chosen tab', async () => {
    pages({ friends: [ANA], incoming: [], outgoing: [] }, { friends: [ANA], incoming: [], outgoing: [] });
    const keys = await mount({ tab: 'blocked' }, [OLEG]);

    await keys.click(within(cardOf('Oleg')).getByRole('button', { name: msg('social.unblockButton') }));

    await vi.waitFor(() => expect(document.activeElement).toBe(tabRadio(msg('social.friendsTabBlocked'))));
  });
});

describe('Friends: focus after a row’s confirmation', () => {
  async function confirmFor(keys: ReturnType<typeof user>, name: string, item: 'social.unfriendLabel' | 'social.blockLabel') {
    await keys.click(menuOf(name));
    await keys.click(screen.getByRole('menuitem', { name: msg(item) }));
    const dialog = screen.getByRole('dialog');
    const buttons = within(dialog).getAllByRole('button');
    return { dialog, cancel: buttons[0], confirm: buttons[buttons.length - 1] };
  }

  it.each([
    ['unfriend', 'social.unfriendLabel'],
    ['block', 'social.blockLabel'],
  ] as const)('%s done: the row above takes it on its menu button when the last row goes', async (_action, item) => {
    pages({ friends: [ANA, GLEB], incoming: [], outgoing: [] }, { friends: [ANA], incoming: [], outgoing: [] });
    const keys = await mount();
    const { confirm } = await confirmFor(keys, 'Gleb', item);

    await keys.click(confirm);

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(menuOf('Ana')));
  });

  it('cancelled: the row’s own menu button takes it back', async () => {
    pages({ friends: [ANA, GLEB], incoming: [], outgoing: [] }, { friends: [ANA, GLEB], incoming: [], outgoing: [] });
    const keys = await mount();
    const { cancel } = await confirmFor(keys, 'Gleb', 'social.unfriendLabel');

    await keys.click(cancel);

    await vi.waitFor(() => expect(document.activeElement).toBe(menuOf('Gleb')));
  });
});
