import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { saveSettingsPatch, type Settings } from '../services/settings';
import { emptyFriendsPage, friends } from '../services/social';
import { settings, updateSettings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { authState } from '../stores/user';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import SocialConsentScreen from './SocialConsentScreen.svelte';

// The consent is a legal act: what saves it and what reads the friends after it are mocks, and the
// recording runtime under them fails the test if anything else would have reached Go.
vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));
vi.mock('../services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/social')>()),
  friends: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: false, language: 'ru' } as Settings);
  vi.mocked(friends).mockResolvedValue(emptyFriendsPage());
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.mocked(friends).mockReset();
  authState.set('bootstrapping');
  settings.set(null);
  toasts.set([]);
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const said = () => get(toasts).map((toast) => [toast.message, toast.kind]);

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

function openConsent() {
  const onclose = vi.fn();
  const view = render(SocialConsentScreen, { target: app, props: { open: true, onclose } });
  const dialog = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
  const enable = within(dialog).getByRole('button', { name: msg('modals.socialConsentEnable') });
  const notNow = within(dialog).getByRole('button', { name: msg('modals.socialConsentNotNow') });
  return { ...view, dialog, enable, notNow, onclose, keys: user() };
}

async function pressEnable(answer: Promise<Settings>) {
  vi.mocked(saveSettingsPatch).mockReturnValueOnce(answer);
  const opened = openConsent();
  opened.enable.focus();
  await opened.keys.keyboard('{Enter}');
  return opened;
}

describe('SocialConsentScreen: what it does today', () => {
  it('opens on "Not now", the safe button, and sends nothing on its own', async () => {
    const { notNow } = openConsent();
    await tick();

    expect(document.activeElement).toBe(notNow);
    expect(saveSettingsPatch).not.toHaveBeenCalled();
  });

  it('keeps the words that say what leaves the device', () => {
    const { dialog } = openConsent();
    for (const key of [
      'modals.socialConsentIntro',
      'modals.socialConsentListIntro',
      'modals.socialConsentListGames',
      'modals.socialConsentListPlaytime',
      'modals.socialConsentListFavoritesStatus',
      'modals.socialConsentListDates',
      'modals.socialConsentOutro',
    ] as const) {
      expect(within(dialog).getByText(msg(key))).toBeTruthy();
    }
  });

  it('"Not now" closes it and gives nothing', async () => {
    const { dialog, notNow, keys, onclose } = openConsent();

    await keys.click(notNow);

    expect(dialog.isConnected).toBe(false);
    expect(onclose).toHaveBeenCalledTimes(1);
    expect(saveSettingsPatch).not.toHaveBeenCalled();
    expect(get(settings)?.accountSync).toBe(false);
  });

  it('sends the consent once per press, and only the consent', async () => {
    const save = held<Settings>();
    const { enable, keys } = await pressEnable(save.promise);
    await keys.click(enable);
    await keys.keyboard('{Enter}');

    expect(vi.mocked(saveSettingsPatch).mock.calls).toEqual([[{ accountSync: true }]]);
    save.resolve({ accountSync: true } as Settings);
  });

  it('once saved: reads the friends once and closes', async () => {
    const save = held<Settings>();
    const { dialog, onclose } = await pressEnable(save.promise);

    save.resolve({ accountSync: true } as Settings);

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(friends).toHaveBeenCalledTimes(1);
    expect(onclose).toHaveBeenCalledTimes(1);
    expect(get(settings)?.accountSync).toBe(true);
  });

  it('a save that fails leaves the dialog open, the consent not given and nothing read', async () => {
    const save = held<Settings>();
    const { dialog } = await pressEnable(save.promise);

    save.reject(new Error('save settings: disk full'));

    await vi.waitFor(() => expect(get(settings)?.accountSync).toBe(false));
    await tick();
    expect(dialog.isConnected).toBe(true);
    expect(friends).not.toHaveBeenCalled();
  });
});

describe('SocialConsentScreen on the kit', () => {
  it('shows the consent as given only once Go has saved it', async () => {
    const save = held<Settings>();
    await pressEnable(save.promise);

    expect(get(settings)?.accountSync).toBe(false);
    save.resolve({ accountSync: true } as Settings);
    await vi.waitFor(() => expect(get(settings)?.accountSync).toBe(true));
  });

  it('while it saves: Enable stays focused and busy, "Not now" is held with a reason, Escape does nothing', async () => {
    const save = held<Settings>();
    const { dialog, enable, notNow, keys, onclose } = await pressEnable(save.promise);

    expect(enable.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(enable);
    expect(notNow).toHaveProperty('disabled', false);
    expect(notNow.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(notNow)).toContain(msg('modals.actionRunning'));
    await keys.click(notNow);
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);
    expect(onclose).not.toHaveBeenCalled();
    save.resolve({ accountSync: true } as Settings);
  });

  it('a failed save is said in the dialog, by its code, not in a toast; focus goes to the words', async () => {
    const save = held<Settings>();
    const { dialog } = await pressEnable(save.promise);

    save.reject(new Error('typhon:settings.network_mode_invalid: bad mode'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.socialConsentEnableFailedReason', { reason: msg('settings.network_mode_invalid') }));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    await tick();
    expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true);
    expect(said()).toEqual([]);
  });

  it('a failure without a code says that sync was not turned on, never Go’s text', async () => {
    const save = held<Settings>();
    const { dialog } = await pressEnable(save.promise);

    save.reject(new Error('save settings: disk full'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.socialConsentEnableFailed'));
    expect(said()).toEqual([]);
  });

  it('opened again after a failure, starts clean', async () => {
    const save = held<Settings>();
    const { dialog, rerender } = await pressEnable(save.promise);
    save.reject(new Error('save settings: disk full'));
    await within(dialog).findByRole('alert');

    await rerender({ open: false });
    await rerender({ open: true });

    const again = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
    expect(within(again).queryByRole('alert')).toBeNull();
  });
});

describe('SocialConsentScreen: an answer after the screen is gone', () => {
  it('a saved consent says once that sync is on, and reads and calls back nothing', async () => {
    const save = held<Settings>();
    const { unmount, onclose } = await pressEnable(save.promise);
    unmount();

    save.resolve({ accountSync: true } as Settings);
    await vi.waitFor(() => expect(said()).toHaveLength(1));

    expect(said()).toEqual([[msg('modals.socialConsentEnabled'), 'success']]);
    expect(get(settings)?.accountSync).toBe(true);
    expect(friends).not.toHaveBeenCalled();
    expect(onclose).not.toHaveBeenCalled();
  });

  it('a failed save says once, in a danger toast, by its code', async () => {
    const save = held<Settings>();
    const { unmount } = await pressEnable(save.promise);
    unmount();

    save.reject(new Error('save settings: disk full'));
    await vi.waitFor(() => expect(said()).toHaveLength(1));

    expect(said()).toEqual([[msg('modals.socialConsentEnableFailed'), 'danger']]);
  });
});

describe('SocialConsentScreen beside another save of the settings', () => {
  it('writes only its own key: a save still queued keeps the value the player chose', async () => {
    settings.set({ accountSync: false, language: 'ru', theme: 'dark' } as Settings);
    const theme = held<Settings>();
    const consent = held<Settings>();
    vi.mocked(saveSettingsPatch).mockReturnValueOnce(theme.promise).mockReturnValueOnce(consent.promise);
    const queued = updateSettings({ theme: 'light' });
    render(SocialConsentScreen, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.socialConsentTitle') });
    await user().click(within(dialog).getByRole('button', { name: msg('modals.socialConsentEnable') }));

    // Go answers the consent with the settings it had when that save ran: the theme before the queued one.
    consent.resolve({ accountSync: true, language: 'ru', theme: 'dark' } as Settings);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    theme.resolve({ accountSync: true, language: 'ru', theme: 'light' } as Settings);
    await queued;

    expect([get(settings)?.theme, get(settings)?.accountSync]).toEqual(['light', true]);
  });
});
