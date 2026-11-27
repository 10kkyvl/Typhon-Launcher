import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CrashRecord } from '../redist/crashView';
import type { LibraryGame } from '../services/library';
import { openFolder } from '../services/settings';
import { exportLogs, type LogBundle } from '../services/system';
import { crashTarget } from '../stores/crash';
import { libraryGames } from '../stores/library';
import { route } from '../stores/router';
import { toasts } from '../stores/toasts';
import { verify } from '../stores/updates';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import GameCrashModal from './GameCrashModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/system')>()),
  exportLogs: vi.fn(),
}));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  openFolder: vi.fn(async () => {}),
}));
vi.mock('../stores/updates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../stores/updates')>()),
  verify: vi.fn(),
}));

// A crash Go could not explain: the dialog offers to collect a report and to check the files.
const unknown = (gameId: string): CrashRecord => ({
  gameId,
  exitCode: 3221225477,
  exitKnown: true,
  reasonCode: '',
  sessionSeconds: 4,
  at: 0,
  count: 1,
  late: true,
  detail: { faultModule: '', exceptionCode: '', hint: 'unknown', missingFile: '', dumpFound: false, journalUnknown: false },
});

const bundle: LogBundle = { dir: 'C:/Users/me/AppData/Typhon/logs', path: 'C:/Users/me/AppData/Typhon/logs/report-1.zip', name: 'report-1.zip', sizeBytes: 2048 };

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  crashTarget.set(null);
  libraryGames.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

async function openDialog(record: CrashRecord = unknown('g1')) {
  render(GameCrashModal, { target: app });
  crashTarget.set(record);
  return screen.findByRole('dialog', { name: msg('redist.crashTitleLate') });
}

const footerClose = (dialog: HTMLElement) => within(dialog).getAllByRole('button', { name: msg('common.close') }).find((button) => !button.closest('.head')) as HTMLElement;

describe('GameCrashModal: what it says and offers', () => {
  it('names the game and how long it ran, and opens with the focus on Close', async () => {
    const dialog = await openDialog();

    expect(dialog.textContent).toContain(msg('redist.crashGame', { title: 'Hades', seconds: 4 }));
    expect(document.activeElement).toBe(footerClose(dialog));
  });

  it('closes on Close and forgets the crash', async () => {
    const dialog = await openDialog();

    await user().click(footerClose(dialog));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(get(crashTarget)).toBeNull();
  });

  it('checks the files on the game page', async () => {
    const dialog = await openDialog();

    await user().click(within(dialog).getByRole('button', { name: msg('redist.crashVerify') }));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(verify).toHaveBeenCalledExactlyOnceWith('g1');
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
  });

  it('collects a report once, says it is saved and opens its folder', async () => {
    vi.mocked(exportLogs).mockResolvedValueOnce(bundle);
    const dialog = await openDialog();

    await user().click(within(dialog).getByRole('button', { name: msg('redist.crashReport') }));

    await vi.waitFor(() => expect(openFolder).toHaveBeenCalledExactlyOnceWith(bundle.dir));
    expect(exportLogs).toHaveBeenCalledTimes(1);
    expect(get(toasts).map((toast) => toast.message)).toEqual([msg('redist.crashReportSaved')]);
  });
});

describe('GameCrashModal: while a report is collected', () => {
  it('keeps the focus on the report button, busy, holds the rest with the reason, and Escape does not close', async () => {
    const collected = held<LogBundle>();
    vi.mocked(exportLogs).mockReturnValueOnce(collected.promise);
    const dialog = await openDialog();
    const keys = user();
    const reportButton = within(dialog).getByRole('button', { name: msg('redist.crashReport') });
    reportButton.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(reportButton.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(reportButton);
    expect(exportLogs).toHaveBeenCalledTimes(1);
    for (const button of [footerClose(dialog), within(dialog).getByRole('button', { name: msg('redist.crashVerify') })]) {
      expect(button).toHaveProperty('disabled', false);
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(msg('redist.crashReportRunning'));
    }
    await keys.click(within(dialog).getByRole('button', { name: msg('redist.crashVerify') }));
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);
    expect(verify).not.toHaveBeenCalled();

    collected.resolve(bundle);
    await vi.waitFor(() => expect(reportButton.getAttribute('aria-busy')).toBeNull());
  });

  it('says a failed report in the dialog and moves the focus there', async () => {
    vi.mocked(exportLogs).mockRejectedValueOnce(new Error('typhon:logs.something_new: zip failed'));
    const dialog = await openDialog();
    const keys = user();
    within(dialog).getByRole('button', { name: msg('redist.crashReport') }).focus();

    await keys.keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('redist.crashReportFailed'));
    expect(dialog.textContent).not.toContain('zip failed');
    expect(get(toasts)).toEqual([]);
    await vi.waitFor(() => expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true));
  });

  it('says nothing of a report that fails after the dialog turned to another game\'s crash', async () => {
    const collected = held<LogBundle>();
    vi.mocked(exportLogs).mockReturnValueOnce(collected.promise);
    const dialog = await openDialog();
    await user().click(within(dialog).getByRole('button', { name: msg('redist.crashReport') }));

    crashTarget.set(unknown('g2'));
    await vi.waitFor(() => expect(dialog.textContent).toContain(msg('redist.crashGame', { title: 'Celeste', seconds: 4 })));
    collected.reject(new Error('zip failed'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(screen.getByRole('dialog')).queryByRole('alert')).toBeNull();
    expect(get(toasts)).toEqual([]);
  });
});

describe('GameCrashModal: the same game crashes again while the dialog is open', () => {
  const again = (): CrashRecord => ({ ...unknown('g1'), at: 100, sessionSeconds: 9 });

  it('drops the failed report of the crash before', async () => {
    vi.mocked(exportLogs).mockRejectedValueOnce(new Error('zip failed'));
    const dialog = await openDialog();
    await user().click(within(dialog).getByRole('button', { name: msg('redist.crashReport') }));
    await within(dialog).findByRole('alert');

    crashTarget.set(again());
    await vi.waitFor(() => expect(dialog.textContent).toContain(msg('redist.crashGame', { title: 'Hades', seconds: 9 })));

    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('says nothing of a report for the crash before that fails after the new one is shown', async () => {
    const collected = held<LogBundle>();
    vi.mocked(exportLogs).mockReturnValueOnce(collected.promise);
    const dialog = await openDialog();
    await user().click(within(dialog).getByRole('button', { name: msg('redist.crashReport') }));

    crashTarget.set(again());
    await vi.waitFor(() => expect(dialog.textContent).toContain(msg('redist.crashGame', { title: 'Hades', seconds: 9 })));
    collected.reject(new Error('zip failed'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(screen.getByRole('dialog')).queryByRole('alert')).toBeNull();
    expect(get(toasts)).toEqual([]);
  });
});
