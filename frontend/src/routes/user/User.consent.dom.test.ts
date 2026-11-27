import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { saveSettingsPatch, type Settings } from '../../lib/services/settings';
import { emptyFriendsPage, friends, profile as fetchProfile } from '../../lib/services/social';
import { resetHistory } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { held, sentToGo } from '../../lib/testing/gameDialogsTesting';
import UserProfile from './UserProfile.svelte';
import { player } from './userTesting';

// The sync consent given on a player's page, in its consent dialog. Go refuses every social call until
// it has the consent, so the page asks for the player only once the dialog's save has landed, and
// once; the dialog reads the friends. The recording runtime under the folder's mocks fails the test if
// anything would have reached Go.
vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/gameDialogsTesting')).goRuntime());
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

beforeEach(() => {
  authState.set('authenticated');
  settings.set({ accountSync: false } as Settings);
  resetBindings();
  vi.mocked(fetchProfile).mockResolvedValue(player());
  vi.mocked(friends).mockResolvedValue(emptyFriendsPage());
});

afterEach(() => {
  const sent = sentToGo();
  expect(calls).toEqual([]);
  resetBindings();
  vi.mocked(fetchProfile).mockReset();
  vi.mocked(friends).mockReset();
  vi.mocked(saveSettingsPatch).mockReset();
  settings.set(null);
  toasts.set([]);
  document.body.replaceChildren();
  authState.set('bootstrapping');
  resetHistory();
  expect(sent).toEqual([]);
});

async function giveConsent(save: Promise<Settings>) {
  vi.mocked(saveSettingsPatch).mockReturnValueOnce(save);
  const { user } = renderDom(UserProfile, { username: 'ann' });
  await user.click(await screen.findByRole('button', { name: msg('social.friendsEnableSync') }));
  const dialog = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
  await user.click(within(dialog).getByRole('button', { name: msg('modals.socialConsentEnable') }));
  await tick();
  return dialog;
}

describe("a user's page: the sync consent given on it", () => {
  it('asks for the player only once the save lands, and once; the dialog reads the friends once and closes', async () => {
    const save = held<Settings>();
    const dialog = await giveConsent(save.promise);
    const beforeTheSave = [fetchProfile, friends].map((call) => vi.mocked(call).mock.calls.length);

    save.resolve({ accountSync: true } as Settings);

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    await tick();
    expect(beforeTheSave).toEqual([0, 0]);
    expect([fetchProfile, friends].map((call) => vi.mocked(call).mock.calls)).toEqual([[['ann']], [[]]]);
  });

  it('a save that fails asks for nothing, and the page still asks for the consent', async () => {
    const save = held<Settings>();
    const dialog = await giveConsent(save.promise);

    save.reject(new Error('save settings: disk full'));

    expect(await within(dialog).findByRole('alert')).toBeTruthy();
    expect(screen.getByRole('heading', { level: 2, name: msg('social.friendsConsentTitle') })).toBeTruthy();
    expect([fetchProfile, friends].map((call) => vi.mocked(call).mock.calls.length)).toEqual([0, 0]);
  });
});
