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
import CloudConsentModal from './CloudConsentModal.svelte';

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

function show(onenable: () => Promise<unknown> = vi.fn(async () => {}), onpolicy = vi.fn()) {
  const rendered = render(CloudConsentModal, { target: app, props: { open: true, onenable, onpolicy } });
  const dialog = screen.getByRole('dialog', { name: msg('cloudsaves.consentTitle') });
  const buttons = {
    cancel: () => within(dialog).getByRole('button', { name: msg('common.cancel') }),
    enable: () => within(dialog).getByRole('button', { name: msg('cloudsaves.consentEnable') }),
  };
  return { ...rendered, dialog, ...buttons };
}

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.replace(/\s+/g, ' ').trim())
    .join(' ');

describe('CloudConsentModal', () => {
  it('opens the privacy policy from a text link in its sentence', async () => {
    const onpolicy = vi.fn();
    const { dialog } = show(undefined, onpolicy);
    const keys = user();
    const link = within(dialog).getByRole('button', { name: msg('cloudsaves.consentPolicyLink') });
    expect(link.classList.contains('text-link')).toBe(true);
    link.focus();

    await keys.keyboard('{Enter}');

    expect(onpolicy).toHaveBeenCalledTimes(1);
  });

  it('holds "enable", not busy, with its reason while its caller runs other work; Cancel and Escape still close', async () => {
    const onenable = vi.fn(async () => {});
    const { rerender, enable, cancel } = show(onenable);
    const keys = user();
    const button = enable();
    button.focus();

    await rerender({ busy: true });

    expect(button.getAttribute('aria-busy')).toBeNull();
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button).toHaveProperty('disabled', false);
    expect(describedBy(button)).toBe(msg('cloudsaves.consentHeld'));
    expect(document.activeElement).toBe(button);
    await keys.keyboard('{Enter}');
    await keys.click(button);
    expect(onenable).not.toHaveBeenCalled();
    expect(cancel().getAttribute('aria-disabled')).toBeNull();
    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('stays busy on "enable", the focus on it, while its own press runs and its caller says it is busy', async () => {
    const saving = deferred<void>();
    const { rerender, enable, cancel } = show(() => saving.promise);
    const keys = user();
    enable().focus();
    await keys.keyboard('{Enter}');

    await rerender({ busy: true });

    expect(enable().getAttribute('aria-busy')).toBe('true');
    expect(describedBy(enable())).toBe('');
    expect(document.activeElement).toBe(enable());
    expect(describedBy(cancel())).toBe(msg('cloudsaves.consentBusy'));
  });

  it('closes on Cancel and on Escape alike, turning nothing on', async () => {
    const onenable = vi.fn(async () => {});
    const first = show(onenable);
    const keys = user();
    await keys.click(first.cancel());
    expect(screen.queryByRole('dialog')).toBeNull();
    first.unmount();

    show(onenable);
    await keys.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(onenable).not.toHaveBeenCalled();
  });

  it('asks its question with the focus on the safe button', () => {
    const { cancel } = show();
    expect(document.activeElement).toBe(cancel());
  });

  it('sends the choice once, however often it is pressed while it runs', async () => {
    const saving = deferred<void>();
    const onenable = vi.fn(() => saving.promise);
    const { enable } = show(onenable);
    const keys = user();
    enable().focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    await keys.click(enable());

    expect(onenable).toHaveBeenCalledTimes(1);
    expect(enable().getAttribute('aria-busy')).toBe('true');
  });

  it('holds Cancel with its reason while the choice runs, and Escape with it', async () => {
    const saving = deferred<void>();
    const { dialog, cancel, enable } = show(() => saving.promise);
    const keys = user();
    enable().focus();
    await keys.keyboard('{Enter}');

    expect(cancel().getAttribute('aria-disabled')).toBe('true');
    expect(cancel()).toHaveProperty('disabled', false);
    expect(describedBy(cancel())).toBe(msg('cloudsaves.consentBusy'));
    await keys.click(cancel());
    expect(document.activeElement).toBe(cancel());
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);
  });

  it.each([
    ['answers with the dialog still open', (saving: ReturnType<typeof deferred<void>>) => saving.resolve()],
    ['fails', (saving: ReturnType<typeof deferred<void>>) => saving.reject(new Error('typhon:settings.write_failed: disk'))],
  ])('says in the dialog that cloud saves are not on when its caller %s, the focus kept on "enable"', async (_case, settle) => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const saving = deferred<void>();
    const { dialog, enable, cancel } = show(() => saving.promise);
    const keys = user();
    enable().focus();
    await keys.keyboard('{Enter}');

    settle(saving);

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('cloudsaves.consentFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(describedBy(enable())).toBe(msg('cloudsaves.consentFailed'));
    expect(enable().getAttribute('aria-busy')).toBeNull();
    expect(document.activeElement).toBe(enable());
    expect(cancel().getAttribute('aria-disabled')).toBeNull();
  });

  it('paints nothing for an answer that lands after the dialog was closed and opened again', async () => {
    const saving = deferred<void>();
    const { dialog, rerender, enable } = show(() => saving.promise);
    await user().click(enable());

    await rerender({ open: false });
    await rerender({ open: true });
    saving.resolve();
    await saving.promise;
    await new Promise((resolve) => setTimeout(resolve, 0));

    const again = screen.getByRole('dialog', { name: msg('cloudsaves.consentTitle') });
    expect(dialog.isConnected).toBe(false);
    expect(within(again).queryByRole('alert')).toBeNull();
    expect(within(again).getByRole('button', { name: msg('cloudsaves.consentEnable') }).getAttribute('aria-busy')).toBeNull();
  });
});

const SAVED = { cloudSaves: false, downloadSchedule: [] } as unknown as SettingsData;

function openSettings() {
  // jsdom has no ResizeObserver; the settings tabs measure their marker with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  authState.set('authenticated');
  settings.set({ ...SAVED });
  render(Settings, { target: app, props: { tab: 'downloads' } });
  return { keys: user(), toggle: screen.getByRole('switch', { name: msg('cloudsaves.settingsLabel') }) };
}

describe('CloudConsentModal on the settings page', () => {
  it('turns cloud saves on with one write, closes and gives the focus back to the switch', async () => {
    vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...get(settings), ...patch }) as SettingsData);
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.consentTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.consentEnable') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ cloudSaves: true });
    expect(get(settings)?.cloudSaves).toBe(true);
    await vi.waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it('says a setting that was not saved in the dialog, which stays, the switch still off', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(saveSettingsPatch).mockRejectedValueOnce(new Error('typhon:settings.write_failed: disk'));
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.consentTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.consentEnable') }));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('cloudsaves.consentFailed'));
    expect(dialog.isConnected).toBe(true);
    expect(saveSettingsPatch).toHaveBeenCalledTimes(1);
    expect(get(settings)?.cloudSaves).toBe(false);
  });

  it('closes on Cancel with nothing written, the focus back on the switch', async () => {
    const { keys, toggle } = openSettings();
    await keys.click(toggle);
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.consentTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(saveSettingsPatch).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(document.activeElement).toBe(toggle));
  });
});
