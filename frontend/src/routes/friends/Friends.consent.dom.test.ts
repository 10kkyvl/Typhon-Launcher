import { cleanup, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { saveSettingsPatch, type Settings } from '../../lib/services/settings';
import { blocks, friends, refresh, type FriendsPage } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { friendsLoaded, friendsPage, loadFriends } from '../../lib/stores/social';
import { toast } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { NoResize, friend } from './friendsTesting';

// The sync consent given on the Friends screen: the consent dialog of the screen saves it and reads
// the friends (loadFriends). A failed read is said where the screen shows it, a notice with Retry, and
// not a second time in a toast; a read on another screen, which shows no such state, is said in a toast
// (stores/social.test.ts). Go learns the consent when the dialog's save lands, so the screen reads
// nothing of its own before it: the dialog's read is the one read of the friends, and the blocked list
// is read once the dialog closes.

const PAGE: FriendsPage = { friends: [friend('u-ana', 'Ana', 'online')], incoming: [], outgoing: [] };

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/settings', async (original) => ({
  ...(await original<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));
vi.mock('../../lib/stores/toasts', async (original) => ({
  ...(await original<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(),
}));

/** Opens the screen without the consent and gives it in the screen's dialog; `saved` answers Go's save. */
async function giveConsent(saved: Promise<Settings> = Promise.resolve({ accountSync: true } as Settings)) {
  vi.mocked(saveSettingsPatch).mockReturnValueOnce(saved);
  render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
  const keys = user();
  await tick();
  await keys.click(screen.getByRole('button', { name: msg('social.friendsEnableSync') }));
  const dialog = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
  await keys.click(within(dialog).getByRole('button', { name: msg('modals.socialConsentEnable') }));
  return { dialog, keys };
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: false } as Settings);
  friendsLoaded.set(false);
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  friendsPage.set({ friends: [], incoming: [], outgoing: [] });
  friendsLoaded.set(false);
  authState.set('bootstrapping');
  settings.set(null);
  // Back to each mock's own answer: a queued answer a test left unused must not reach the next one.
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: the sync consent given on the screen', () => {
  it('a read that fails is the screen’s notice with Retry, not a toast too', async () => {
    vi.mocked(refresh).mockRejectedValue(new Error('timeout'));
    vi.mocked(friends).mockRejectedValue(new Error('timeout'));

    await giveConsent();

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('state.socialLoadFriendsFailed'));
    expect(screen.getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(toast).not.toHaveBeenCalled();
  });

  it('reads the friends once, after the save landed, and the blocked list once the dialog closed', async () => {
    vi.mocked(friends).mockResolvedValue(PAGE);
    let saved!: (value: Settings) => void;

    await giveConsent(new Promise<Settings>((resolve) => (saved = resolve)));
    await tick();
    const beforeTheSave = [refresh, friends, blocks].map((call) => vi.mocked(call).mock.calls.length);

    saved({ accountSync: true } as Settings);

    expect(beforeTheSave).toEqual([0, 0, 0]);

    expect(await screen.findByText('Ana')).toBeTruthy();
    await vi.waitFor(() => expect(blocks).toHaveBeenCalledTimes(1));
    await tick();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect([refresh, friends, blocks].map((call) => vi.mocked(call).mock.calls.length)).toEqual([0, 1, 1]);
  });

  it('that is not saved reads nothing and leaves the dialog open', async () => {
    let saved!: (value: Settings) => void;
    vi.mocked(saveSettingsPatch).mockReturnValueOnce(new Promise<Settings>((_resolve, reject) => (saved = () => reject(new Error('disk full')))));
    render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
    const keys = user();
    await tick();
    await keys.click(screen.getByRole('button', { name: msg('social.friendsEnableSync') }));
    const dialog = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('modals.socialConsentEnable') }));

    saved({} as Settings);

    await vi.waitFor(() => expect(screen.getByRole('heading', { name: msg('social.friendsConsentTitle') })).toBeTruthy());
    expect(dialog.isConnected).toBe(true);
    expect([refresh, friends, blocks].map((call) => vi.mocked(call).mock.calls.length)).toEqual([0, 0, 0]);
  });
});

describe('Friends: the hold on the store’s toast', () => {
  it('R5 is released when the screen goes: a consent read that fails elsewhere then is said once in a toast', async () => {
    const shown = render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
    await tick();
    shown.unmount();
    vi.mocked(friends).mockRejectedValueOnce(new Error('timeout'));

    await loadFriends();

    expect(vi.mocked(toast).mock.calls).toEqual([[msg('state.socialLoadFriendsFailed'), 'danger']]);
  });
});
