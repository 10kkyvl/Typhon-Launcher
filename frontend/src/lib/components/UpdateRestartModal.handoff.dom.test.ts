import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { applyUpdate, beginHandoff, SelfUpdateError } from '../services/selfupdate';
import { selfUpdateStatus } from '../stores/selfupdate';
import { toasts } from '../stores/toasts';
import { endUpdateShow, updateShow } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import SidebarStatus from './SidebarStatus.svelte';
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

describe('a seamless restart whose undo failed', () => {
  // Go refuses every update until the launcher is closed and started again: the screen says so and
  // offers no restart of either kind.
  const UNDO_FAILED = { ...READY, error: 'selfupdate: the new launcher exited before it showed its window\nselfupdate: put back', errorCode: 'handoff_undo_failed' } as const;
  const footer = (dialog: HTMLElement) => Array.from(dialog.querySelectorAll('.foot button')).map((button) => button.textContent?.trim());

  async function undoFailed() {
    vi.mocked(beginHandoff).mockResolvedValue(undefined);
    const result = await confirmRestart();
    renderDom(SidebarStatus);
    await vi.waitFor(() => expect(beginHandoff).toHaveBeenCalledOnce());
    selfUpdateStatus.set(APPLYING);
    selfUpdateStatus.set(UNDO_FAILED);
    return result;
  }

  it('opens the dialog once the show has gone home, saying so with one button that closes it', async () => {
    const { user } = await undoFailed();
    const dialog = await failureDialog();
    expect(get(updateShow)).toBeNull();
    expect(screen.getAllByRole('dialog')).toHaveLength(1);
    expect(dialog.textContent).toContain(msg('state.selfupdateHandoffUndoFailed'));
    expect(dialog.textContent).not.toContain(msg('state.selfupdateHandoffFailed'));
    expect(dialog.textContent).not.toContain('put back');
    expect(footer(dialog)).toEqual([msg('common.gotIt')]);
    const only = within(dialog).getByRole('button', { name: msg('common.gotIt') });
    expect(document.activeElement).toBe(only);

    await user.click(only);
    expect(screen.queryByRole('dialog')).toBeNull();
    selfUpdateStatus.set({ ...UNDO_FAILED });
    await tick();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(beginHandoff).toHaveBeenCalledOnce();
    expect(applyUpdate).not.toHaveBeenCalled();
  });

  it('closes on Escape like the rolled-back dialog, asking Go for nothing', async () => {
    const { user } = await undoFailed();
    await failureDialog();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(beginHandoff).toHaveBeenCalledOnce();
    expect(applyUpdate).not.toHaveBeenCalled();
  });

  it('is said on the About banner as a failure, with no restart and no "Not now"', async () => {
    selfUpdateStatus.set(UNDO_FAILED);
    renderDom(UpdateBanner);
    const notice = screen.getByText(msg('state.selfupdateHandoffUndoFailed')).closest('.notice') as HTMLElement;
    expect(notice).toBeTruthy();
    expect(notice.dataset.tone).toBe('danger');
    expect(notice.textContent).not.toContain(msg('ui.updateReadyMessage', { version: '1.5.0' }));
    expect(screen.queryByRole('button', { name: msg('ui.restartAndUpdate') })).toBeNull();
    expect(screen.queryByRole('button', { name: msg('ui.notNow') })).toBeNull();
    expect(screen.queryByRole('button', { name: msg('common.retry') })).toBeNull();
  });

  it('leaves no control on the banner or the sidebar that reaches Go', async () => {
    selfUpdateStatus.set(UNDO_FAILED);
    const { user } = renderDom(UpdateBanner);
    renderDom(SidebarStatus);
    for (const button of screen.queryAllByRole('button')) await user.click(button);
    for (const button of screen.queryAllByRole('button')) await user.click(button);
    expect(beginHandoff).not.toHaveBeenCalled();
    expect(applyUpdate).not.toHaveBeenCalled();
    expect(document.querySelector('[data-update="ready"]')).toBeNull();
  });
});
