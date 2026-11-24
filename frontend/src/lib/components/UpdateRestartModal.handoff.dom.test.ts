import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { applyUpdate, beginHandoff, SelfUpdateError } from '../services/selfupdate';
import { selfUpdateStatus } from '../stores/selfupdate';
import { toasts } from '../stores/toasts';
import { endUpdateShow, updateShow } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import UpdateBanner from './UpdateBanner.svelte';
import UpdateShow from './UpdateShow.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/selfupdate')>()),
  applyUpdate: vi.fn(),
  beginHandoff: vi.fn(),
}));

// The restart dialog of the old launcher in a seamless restart: the confirmation tries it first,
// a launcher where it is not available restarts as before without a word, and one Go rolled back is
// said in the same dialog with its two ways out once the show has gone home.

const READY = { state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;
const APPLYING = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;
const ROLLED_BACK = { ...READY, error: 'selfupdate: the new launcher exited before it showed its window', errorCode: 'handoff_failed' } as const;

beforeEach(() => {
  vi.mocked(applyUpdate).mockReset();
  vi.mocked(beginHandoff).mockReset();
});

afterEach(() => {
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  endUpdateShow();
  toasts.set([]);
  document.body.replaceChildren();
});

async function confirmRestart() {
  selfUpdateStatus.set(READY);
  renderDom(UpdateShow);
  const { user } = renderDom(UpdateBanner);
  await user.click(screen.getByRole('button', { name: msg('ui.restartAndUpdate') }));
  const dialog = screen.getByRole('dialog', { name: msg('ui.restartLauncherTitle') });
  await user.click(within(dialog).getByRole('button', { name: msg('ui.restartAndUpdate') }));
  return { user };
}

async function rolledBack() {
  vi.mocked(beginHandoff).mockResolvedValue(undefined);
  const result = await confirmRestart();
  await vi.waitFor(() => expect(beginHandoff).toHaveBeenCalledOnce());
  selfUpdateStatus.set(APPLYING);
  selfUpdateStatus.set(ROLLED_BACK);
  return result;
}

const failureDialog = () => screen.findByRole('dialog', { name: msg('ui.handoffFailedTitle') });

describe('confirming the restart', () => {
  it('tries the seamless update first, with no worker restart behind it', async () => {
    vi.mocked(beginHandoff).mockResolvedValue(undefined);
    await confirmRestart();
    await vi.waitFor(() => expect(beginHandoff).toHaveBeenCalledOnce());
    expect(applyUpdate).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog', { name: msg('ui.restartLauncherTitle') })).toBeNull();
    expect(get(updateShow)).toMatchObject({ phase: 'run', handoff: true });
  });

  it('where the seamless update is not available, restarts through the worker and says no error', async () => {
    vi.mocked(beginHandoff).mockRejectedValue(new SelfUpdateError('selfupdate: updating without a restart is not available here'));
    vi.mocked(applyUpdate).mockResolvedValue(undefined);
    await confirmRestart();
    await vi.waitFor(() => expect(applyUpdate).toHaveBeenCalledOnce());
    expect(get(toasts)).toEqual([]);
    expect(screen.queryByRole('dialog', { name: msg('ui.restartLauncherTitle') })).toBeNull();
    expect(screen.queryByRole('dialog', { name: msg('ui.handoffFailedTitle') })).toBeNull();
    expect(get(updateShow)?.phase).toBe('run');
  });
});

describe('a seamless restart Go rolled back', () => {
  it('reopens the dialog once the show has gone home, saying so with its two ways out', async () => {
    await rolledBack();
    const dialog = await failureDialog();
    expect(get(updateShow)).toBeNull();
    expect(dialog.textContent).toContain(msg('state.selfupdateHandoffFailed'));
    expect(dialog.textContent).not.toContain('exited before');
    const buttons = within(dialog).getAllByRole('button').map((button) => button.textContent?.trim());
    expect(buttons).toEqual(expect.arrayContaining([msg('common.retry'), msg('ui.updateWithRestart'), msg('common.cancel')]));
  });

  it('Retry asks for the seamless update again', async () => {
    const { user } = await rolledBack();
    const dialog = await failureDialog();
    await user.click(within(dialog).getByRole('button', { name: msg('common.retry') }));
    await vi.waitFor(() => expect(beginHandoff).toHaveBeenCalledTimes(2));
    expect(applyUpdate).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog', { name: msg('ui.handoffFailedTitle') })).toBeNull();
  });

  it('"update with a restart" goes to the worker, not to the seamless update', async () => {
    const { user } = await rolledBack();
    vi.mocked(applyUpdate).mockResolvedValue(undefined);
    const dialog = await failureDialog();
    await user.click(within(dialog).getByRole('button', { name: msg('ui.updateWithRestart') }));
    await vi.waitFor(() => expect(applyUpdate).toHaveBeenCalledOnce());
    expect(beginHandoff).toHaveBeenCalledOnce();
    expect(get(updateShow)?.phase).toBe('run');
  });

  it('is said on the About banner while the update waits, and its button leads to the same choice', async () => {
    selfUpdateStatus.set(ROLLED_BACK);
    const { user } = renderDom(UpdateBanner);
    const notice = screen.getByText(msg('state.selfupdateHandoffFailed')).closest('.notice') as HTMLElement;
    expect(notice).toBeTruthy();
    await user.click(within(notice).getByRole('button', { name: msg('ui.restartAndUpdate') }));
    expect(await failureDialog()).toBeTruthy();
  });
});
