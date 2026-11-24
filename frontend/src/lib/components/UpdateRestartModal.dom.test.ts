import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { applyUpdate } from '../services/selfupdate';
import { selfUpdateStatus } from '../stores/selfupdate';
import { toasts } from '../stores/toasts';
import { endUpdateShow, updateShow } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import UpdateBanner from './UpdateBanner.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/selfupdate')>()),
  applyUpdate: vi.fn(),
}));

// The restart of a downloaded update as the player meets it on the About page: the banner asks
// first, the dialog applies on confirmation, and the ordinary path (the worker that restarts the
// launcher) is what runs where nothing better is available.

const READY = { state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;
const APPLYING = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;

beforeEach(() => {
  vi.mocked(applyUpdate).mockReset();
});

afterEach(() => {
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  endUpdateShow();
  toasts.set([]);
  document.body.replaceChildren();
});

async function confirmRestart() {
  selfUpdateStatus.set(READY);
  const { user } = renderDom(UpdateBanner);
  await user.click(screen.getByRole('button', { name: msg('ui.restartAndUpdate') }));
  const dialog = screen.getByRole('dialog', { name: msg('ui.restartLauncherTitle') });
  return { user, dialog };
}

describe('the restart of a downloaded update', () => {
  it('asks first, saying which version is installed and that the launcher restarts', async () => {
    const { dialog } = await confirmRestart();
    expect(dialog.textContent).toContain(msg('ui.restartLauncherWarning', { version: '1.5.0' }));
    expect(applyUpdate).not.toHaveBeenCalled();
  });

  it('cancelled, applies nothing and plays nothing', async () => {
    const { user, dialog } = await confirmRestart();
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));
    expect(screen.queryByRole('dialog', { name: msg('ui.restartLauncherTitle') })).toBeNull();
    expect(applyUpdate).not.toHaveBeenCalled();
    expect(get(updateShow)).toBeNull();
  });

  it('confirmed, closes the dialog and applies the update; the show runs while the backend applies it', async () => {
    vi.mocked(applyUpdate).mockImplementation(async () => {
      selfUpdateStatus.set(APPLYING);
    });
    const { user, dialog } = await confirmRestart();
    await user.click(within(dialog).getByRole('button', { name: msg('ui.restartAndUpdate') }));
    await vi.waitFor(() => expect(applyUpdate).toHaveBeenCalledOnce());
    expect(screen.queryByRole('dialog', { name: msg('ui.restartLauncherTitle') })).toBeNull();
    await tick();
    expect(get(updateShow)).toMatchObject({ phase: 'run', version: '1.5.0', from: '1.4.0' });
  });

  it('refused by the backend, says why in a toast and leaves no show behind', async () => {
    vi.mocked(applyUpdate).mockRejectedValue(new Error('typhon:selfupdate.not_ready'));
    const { user, dialog } = await confirmRestart();
    await user.click(within(dialog).getByRole('button', { name: msg('ui.restartAndUpdate') }));
    await vi.waitFor(() => expect(get(toasts).some((item) => item.kind === 'danger')).toBe(true));
    expect(get(toasts).map((item) => item.message)).toContain(msg('state.selfupdateReasonInstallerNotFound'));
    expect(get(updateShow)).toBeNull();
  });
});
