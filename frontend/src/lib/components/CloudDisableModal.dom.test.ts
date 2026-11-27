import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/settings', async (original) => (await import('../testing/settingsDialogsTesting')).settingsService(await original()));
vi.mock('../services/cloudsaves', async (original) => (await import('../testing/settingsDialogsTesting')).cloudsavesService(await original()));
vi.mock('../../../bindings/typhon/internal/settings', async (original) => (await import('../testing/settingsDialogsTesting')).fakeService(await original(), 'settings'));
vi.mock('../../../bindings/typhon/internal/cloudsaves', async (original) => (await import('../testing/settingsDialogsTesting')).fakeService(await original(), 'cloudsaves'));

import { msg } from '../i18n';
import { cloudForget } from '../services/cloudsaves';
import { saveSettingsPatch, type Settings as SettingsData } from '../services/settings';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { authState } from '../stores/user';
import { user } from '../testing/dom';
import { calls, resetBindings } from '../testing/fakeBinding';
import { deferred } from '../testing/settingsDialogsTesting';
import Settings from '../../routes/settings/Settings.svelte';
import CloudDisableModal from './CloudDisableModal.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  authState.set('bootstrapping');
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  for (const mock of [saveSettingsPatch, cloudForget]) vi.mocked(mock).mockReset();
  expect(calls).toEqual([]);
  resetBindings();
});

type Choice = () => Promise<unknown>;

function show(onkeep: Choice = vi.fn(async () => {}), ondelete: Choice = vi.fn(async () => {})) {
  const rendered = render(CloudDisableModal, { target: app, props: { open: true, onkeep, ondelete } });
  const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.disableTitle') });
  const buttons = {
    cancel: () => within(dialog).getByRole('button', { name: msg('common.cancel') }),
    keep: () => within(dialog).getByRole('button', { name: msg('cloudsaves.disableKeep') }),
    remove: () => within(dialog).getByRole('button', { name: new RegExp(`^(${msg('cloudsaves.disableDelete')}|${msg('cloudsaves.disableDeleting')})$`) }),
  };
  return { ...rendered, dialog, ...buttons };
}

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.replace(/\s+/g, ' ').trim())
    .join(' ');

describe('CloudDisableModal', () => {
  it('opens with the focus on Cancel, never on the destructive Delete', () => {
    const { cancel } = show();
    expect(document.activeElement).toBe(cancel());
  });

  it('closes on Cancel and on Escape alike, choosing nothing', async () => {
    const onkeep = vi.fn(async () => {});
    const ondelete = vi.fn(async () => {});
    const first = show(onkeep, ondelete);
    const keys = user();
    await keys.click(first.cancel());
    expect(screen.queryByRole('dialog')).toBeNull();
    first.unmount();

    show(onkeep, ondelete);
    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(onkeep).not.toHaveBeenCalled();
    expect(ondelete).not.toHaveBeenCalled();
  });

  it.each([
    ['keep', 'onkeep'],
    ['remove', 'ondelete'],
  ] as const)('sends %s once, and only its own choice, however often it is pressed while it runs', async (button, called) => {
    const running = deferred<void>();
    const choices = { onkeep: vi.fn(() => running.promise), ondelete: vi.fn(() => running.promise) };
    const shown = show(choices.onkeep, choices.ondelete);
    const keys = user();
    shown[button]().focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    await keys.click(shown[button]());

    expect(choices[called]).toHaveBeenCalledTimes(1);
    expect(choices[called === 'onkeep' ? 'ondelete' : 'onkeep']).not.toHaveBeenCalled();
  });

  it('keeps the focus on the pressed choice, busy, and holds the other buttons with the reason; it cannot be dismissed', async () => {
    const running = deferred<void>();
    const { dialog, cancel, keep, remove } = show(() => running.promise);
    const keys = user();
    keep().focus();

    await keys.keyboard('{Enter}');

    expect(keep().getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(keep());
    expect(remove().textContent?.trim()).toBe(msg('cloudsaves.disableDelete'));
    for (const other of [cancel(), remove()]) {
      expect(other.getAttribute('aria-disabled')).toBe('true');
      expect(other).toHaveProperty('disabled', false);
      expect(describedBy(other)).toBe(msg('cloudsaves.disableBusy'));
    }
    await keys.keyboard('{Escape}');
    await keys.click(cancel());
    expect(dialog.isConnected).toBe(true);
  });

  it('says it is deleting on Delete while the deletion runs', async () => {
    const running = deferred<void>();
    const { remove } = show(undefined, () => running.promise);
    await user().click(remove());
    expect(remove().textContent?.trim()).toBe(msg('cloudsaves.disableDeleting'));
    expect(remove().getAttribute('aria-busy')).toBe('true');
  });

  it.each([
    ['answers with the dialog still open', (running: ReturnType<typeof deferred<void>>) => running.resolve()],
    ['fails', (running: ReturnType<typeof deferred<void>>) => running.reject(new Error('typhon:settings.write_failed: disk'))],
  ])('says in the dialog that nothing was turned off or deleted when its caller %s', async (_case, settle) => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const running = deferred<void>();
    const { dialog, remove, cancel } = show(undefined, () => running.promise);
    const keys = user();
    remove().focus();
    await keys.keyboard('{Enter}');

    settle(running);

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('cloudsaves.disableFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(describedBy(remove())).toBe(msg('cloudsaves.disableFailed'));
    expect(remove().textContent?.trim()).toBe(msg('cloudsaves.disableDelete'));
    expect(document.activeElement).toBe(remove());
    expect(cancel().getAttribute('aria-disabled')).toBeNull();
  });

  it('paints nothing for an answer that lands after the dialog was closed and opened again', async () => {
    const running = deferred<void>();
    const { rerender, remove } = show(undefined, () => running.promise);
    await user().click(remove());

    await rerender({ open: false });
    await rerender({ open: true });
    running.resolve();
    await running.promise;
    await new Promise((resolve) => setTimeout(resolve, 0));

    const again = screen.getByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    expect(within(again).queryByRole('alert')).toBeNull();
    expect(within(again).getByRole('button', { name: msg('cloudsaves.disableDelete') }).getAttribute('aria-busy')).toBeNull();
  });
});

const SAVED = { cloudSaves: true, downloadSchedule: [] } as unknown as SettingsData;

function openSettings() {
  // jsdom has no ResizeObserver; the settings tabs measure their marker with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  authState.set('authenticated');
  settings.set({ ...SAVED });
  const { unmount } = render(Settings, { target: app, props: { tab: 'downloads' } });
  return { keys: user(), toggle: screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') }), unmount };
}

const saved = () => vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...get(settings), ...patch }) as SettingsData);

describe('CloudDisableModal on the settings page', () => {
  it('turns cloud saves off on Keep with one write, and deletes nothing in the cloud', async () => {
    saved();
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableKeep') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ cloudSaves: false });
    expect(cloudForget).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it('turns cloud saves off on Delete with one write, then deletes the cloud copies once', async () => {
    saved();
    vi.mocked(cloudForget).mockResolvedValueOnce(undefined);
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));

    await vi.waitFor(() => expect(cloudForget).toHaveBeenCalledTimes(1));
    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ cloudSaves: false });
    expect(vi.mocked(saveSettingsPatch).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(cloudForget).mock.invocationCallOrder[0]);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('says a setting that was not saved in the dialog, which stays, and deletes nothing', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('typhon:settings.write_failed: disk'));
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('cloudsaves.disableFailed'));
    expect(dialog.isConnected).toBe(true);
    expect(saveSettingsPatch).toHaveBeenCalledTimes(1);
    expect(cloudForget).not.toHaveBeenCalled();
    expect(get(settings)?.cloudSaves).toBe(true);
    expect(screen.getAllByRole('alert')).toHaveLength(1);
    expect(get(toasts)).toEqual([]);
  });

  it('holds the switch, saying the copies are being deleted, while they are: it keeps the focus and opens no dialog', async () => {
    saved();
    const forget = deferred<void>();
    vi.mocked(cloudForget).mockImplementation(() => forget.promise);
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));
    await vi.waitFor(() => expect(cloudForget).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(document.activeElement).toBe(toggle));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(toggle).toHaveProperty('disabled', false);
    expect(toggle.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(toggle)).toContain(msg('cloudsaves.heldDeleting'));
    await keys.click(toggle);
    await keys.keyboard('{Enter}');
    await keys.keyboard(' ');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(toggle);

    forget.resolve();
    await vi.waitFor(() => expect(toggle.getAttribute('aria-disabled')).toBeNull());
    expect(describedBy(toggle)).not.toContain(msg('cloudsaves.heldDeleting'));
    expect(saveSettingsPatch).toHaveBeenCalledTimes(1);
    expect(cloudForget).toHaveBeenCalledTimes(1);
  });

  it('holds the switch with its reason on a page opened again while the deletion still runs, and says its end once', async () => {
    saved();
    const forget = deferred<void>();
    vi.mocked(cloudForget).mockImplementation(() => forget.promise);
    const { keys, toggle, unmount } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));
    await vi.waitFor(() => expect(cloudForget).toHaveBeenCalledTimes(1));
    unmount();

    try {
      render(Settings, { target: app, props: { tab: 'downloads' } });
      const again = screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') });
      expect(again.getAttribute('aria-disabled')).toBe('true');
      expect(describedBy(again)).toContain(msg('cloudsaves.heldDeleting'));
      await keys.click(again);
      expect(screen.queryByRole('dialog')).toBeNull();

      forget.resolve();
      await vi.waitFor(() => expect(again.getAttribute('aria-disabled')).toBeNull());
      expect(describedBy(again)).not.toContain(msg('cloudsaves.heldDeleting'));
      expect(get(toasts).map(({ message, kind }) => [message, kind])).toEqual([[msg('cloudsaves.forgetDone'), 'success']]);
      expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ cloudSaves: false });
      expect(cloudForget).toHaveBeenCalledTimes(1);
    } finally {
      forget.resolve();
    }
  });

  it('says once that the cloud copies were deleted, in a polite toast, when the deletion ends', async () => {
    saved();
    const forget = deferred<void>();
    vi.mocked(cloudForget).mockImplementation(() => forget.promise);
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));
    await vi.waitFor(() => expect(cloudForget).toHaveBeenCalledTimes(1));
    expect(get(toasts)).toEqual([]);

    forget.resolve();

    await vi.waitFor(() => expect(get(toasts).map(({ message, kind }) => [message, kind])).toEqual([[msg('cloudsaves.forgetDone'), 'success']]));
  });

  it.each([
    ['fails', 'typhon:cloud.network: down'],
    ['fails part-way', 'typhon:cloud.network: g1: down\ng2: down'],
  ])('says a deletion that %s, offers Retry, and never says the copies were deleted', async (_case, failure) => {
    saved();
    vi.mocked(cloudForget).mockRejectedValueOnce(new Error(failure));
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableDelete') }));

    await screen.findByRole('button', { name: msg('cloudsaves.retryForget') });
    expect(get(toasts).map(({ message, kind }) => [message, kind])).toEqual([[msg('cloud.network'), 'danger']]);
    expect(cloudForget).toHaveBeenCalledTimes(1);
  });

  it('says nothing deleted after Keep', async () => {
    saved();
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableKeep') }));
    await vi.waitFor(() => expect(toggle.getAttribute('aria-disabled')).toBeNull());

    expect(get(toasts)).toEqual([]);
  });

  it('holds the switch, saying the setting is being saved, while the write runs', async () => {
    const write = deferred<SettingsData>();
    vi.mocked(saveSettingsPatch).mockImplementation(() => write.promise);
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.disableTitle') });
    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.disableKeep') }));

    expect(toggle.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(toggle)).toContain(msg('cloudsaves.heldSaving'));

    write.resolve({ ...get(settings), cloudSaves: false } as SettingsData);
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(toggle.getAttribute('aria-disabled')).toBeNull());
    expect(describedBy(toggle)).not.toContain(msg('cloudsaves.heldSaving'));
    expect(cloudForget).not.toHaveBeenCalled();
  });
});
