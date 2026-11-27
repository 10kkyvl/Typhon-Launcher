import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { saveSettingsPatch, type Settings } from '../../lib/services/settings';
import { emptyFriendsPage, feed, friends, type FeedPage } from '../../lib/services/social';
import { resetFeed } from '../../lib/stores/feed';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { resetBindings } from '../../lib/testing/fakeBinding';
import { held, sentToGo } from '../../lib/testing/gameDialogsTesting';
import Activity from './Activity.svelte';

// The sync consent given on the Activity screen, in its consent dialog. Go refuses every social call
// until it has the consent, so the screen reads its feed only once the dialog's save has landed, and
// once; the dialog reads the friends. The services are mocks and outside Wails, and the recording
// runtime under them fails the test if anything would have reached Go.
vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/gameDialogsTesting')).goRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  feed: vi.fn(),
  friends: vi.fn(),
}));

beforeEach(() => {
  authState.set('authenticated');
  settings.set({ accountSync: false } as Settings);
  vi.mocked(feed).mockResolvedValue({ events: [], next: 0 } as unknown as FeedPage);
  vi.mocked(friends).mockResolvedValue(emptyFriendsPage());
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  cleanup();
  resetFeed();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.mocked(feed).mockReset();
  vi.mocked(friends).mockReset();
  authState.set('bootstrapping');
  settings.set(null);
  toasts.set([]);
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

describe('Activity: the sync consent given on the screen', () => {
  it('reads nothing before the save lands, then the feed once; the dialog reads the friends once and closes', async () => {
    const save = held<Settings>();
    vi.mocked(saveSettingsPatch).mockReturnValueOnce(save.promise);
    const { user } = renderDom(Activity);
    await tick();
    await user.click(screen.getByRole('button', { name: msg('transfers.activityEnableSync') }));
    const dialog = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
    await user.click(within(dialog).getByRole('button', { name: msg('modals.socialConsentEnable') }));
    await tick();

    const beforeTheSave = [feed, friends].map((call) => vi.mocked(call).mock.calls.length);
    save.resolve({ accountSync: true } as Settings);

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    await tick();
    expect(beforeTheSave).toEqual([0, 0]);
    expect([feed, friends].map((call) => vi.mocked(call).mock.calls.length)).toEqual([1, 1]);
  });

  it('a save that fails reads nothing, and the screen still asks for the consent', async () => {
    const save = held<Settings>();
    vi.mocked(saveSettingsPatch).mockReturnValueOnce(save.promise);
    const { user } = renderDom(Activity);
    await tick();
    await user.click(screen.getByRole('button', { name: msg('transfers.activityEnableSync') }));
    const dialog = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
    await user.click(within(dialog).getByRole('button', { name: msg('modals.socialConsentEnable') }));

    save.reject(new Error('save settings: disk full'));

    expect(await within(dialog).findByRole('alert')).toBeTruthy();
    expect(screen.getByRole('heading', { name: msg('transfers.activityConsentTitle') })).toBeTruthy();
    expect([feed, friends].map((call) => vi.mocked(call).mock.calls.length)).toEqual([0, 0]);
  });
});
