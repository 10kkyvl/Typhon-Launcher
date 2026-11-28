import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { inspectRemoval, removeGame, type RemovalInfo } from '../services/install';
import { getGames, type LibraryGame } from '../services/library';
import { libraryGames } from '../stores/library';
import { navigate, resetHistory } from '../stores/router';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import RemoveGameModal from './RemoveGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/install')>()),
  inspectRemoval: vi.fn(),
  removeGame: vi.fn(),
}));
// What the library holds after a refused removal: by default the card is there and installed.
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  getGames: vi.fn(),
}));

function info(patch: Partial<RemovalInfo> = {}): RemovalInfo {
  return {
    gameId: 'g1',
    title: 'Hades',
    installDir: '/games/hades',
    installType: 'portable',
    method: 'files',
    owned: true,
    uninstallUnknown: false,
    sizeBytes: 1024,
    sizeUnknown: false,
    dirMissing: false,
    quietUninstall: false,
    running: false,
    busy: false,
    downloadId: '',
    downloadPresent: false,
    downloadSeeding: false,
    downloadBytes: 0,
    downloadPath: '',
    sharedWith: null,
    ...patch,
  };
}

const coded = (code: string) => new Error(`typhon:${code}`);
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(getGames).mockResolvedValue([{ id: 'g1', title: 'Hades' } as LibraryGame]);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  vi.mocked(inspectRemoval).mockReset();
  vi.mocked(removeGame).mockReset();
  vi.mocked(getGames).mockReset();
  libraryGames.set([]);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

type Props = { gameId?: string; title?: string; mode?: 'disk' | 'library'; external?: string; emulated?: boolean; onremoved?: (mode: 'disk' | 'library') => void };

async function openDialog(patch: Partial<RemovalInfo> = {}, props: Props = {}) {
  vi.mocked(inspectRemoval).mockResolvedValue(info(patch));
  const view = render(RemoveGameModal, { target: app, props: { open: true, gameId: 'g1', title: 'Hades', ...props } });
  const dialog = screen.getByRole('dialog', { name: msg('modals.removeGameTitle') });
  await within(dialog).findByText(msg('modals.removeGameLibrarySub'));
  return { dialog, view };
}

const radio = (dialog: HTMLElement, key: 'modals.removeGameModeDiskLabel' | 'modals.removeGameModeLibraryLabel') =>
  within(dialog).getByRole('radio', { name: new RegExp(msg(key)) });
const confirmButton = (dialog: HTMLElement, key: 'modals.removeGameModeDiskLabel' | 'modals.removeGameModeLibraryLabel' | 'modals.removeGameWorking') =>
  within(dialog).getByRole('button', { name: msg(key) });

describe('RemoveGameModal: where the game goes', () => {
  it('is one named radio group: Tab reaches the chosen tile', async () => {
    const { dialog } = await openDialog();
    const group = within(dialog).getByRole('radiogroup', { name: msg('modals.removeGameTitle') });
    const disk = within(group).getByRole('radio', { name: new RegExp(msg('modals.removeGameModeDiskLabel')) });
    const library = within(group).getByRole('radio', { name: new RegExp(msg('modals.removeGameModeLibraryLabel')) });
    expect(disk.getAttribute('aria-checked')).toBe('true');
    expect(library.getAttribute('aria-checked')).toBe('false');
    expect(disk.tabIndex).toBe(0);
    expect(library.tabIndex).toBe(-1);
  });

  it('lets the arrows walk the modes without choosing one; Space chooses', async () => {
    const { dialog } = await openDialog();
    const keys = user();
    const disk = radio(dialog, 'modals.removeGameModeDiskLabel');
    const library = radio(dialog, 'modals.removeGameModeLibraryLabel');
    disk.focus();

    await keys.keyboard('{ArrowRight}');

    expect(document.activeElement).toBe(library);
    expect(disk.getAttribute('aria-checked')).toBe('true');
    expect(library.getAttribute('aria-checked')).toBe('false');
    expect(confirmButton(dialog, 'modals.removeGameModeDiskLabel')).toBeTruthy();

    await keys.keyboard(' ');
    expect(library.getAttribute('aria-checked')).toBe('true');
    expect(confirmButton(dialog, 'modals.removeGameModeLibraryLabel')).toBeTruthy();
  });

  it('holds the disk tile with its reason when the game cannot be uninstalled', async () => {
    const { dialog } = await openDialog({ owned: false, method: 'record' });
    const keys = user();
    const disk = radio(dialog, 'modals.removeGameModeDiskLabel');
    const library = radio(dialog, 'modals.removeGameModeLibraryLabel');
    await vi.waitFor(() => expect(library.getAttribute('aria-checked')).toBe('true'));
    expect(disk.getAttribute('aria-disabled')).toBe('true');
    expect(disk.textContent).toContain(msg('modals.removeGameDiskSubNotOwned'));

    await keys.click(disk);
    expect(library.getAttribute('aria-checked')).toBe('true');
  });

  it('starts with focus on Cancel, never on the removal', async () => {
    const pending = held<RemovalInfo>();
    vi.mocked(inspectRemoval).mockReturnValueOnce(pending.promise);
    render(RemoveGameModal, { target: app, props: { open: true, gameId: 'g1', title: 'Hades' } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.removeGameTitle') });
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    await vi.waitFor(() => expect(document.activeElement).toBe(cancel));
    pending.resolve(info());
    await within(dialog).findByText(msg('modals.removeGameLibrarySub'));
    expect(document.activeElement).toBe(cancel);
  });
});

describe('RemoveGameModal: what each mode sends', () => {
  type Case = [string, Partial<RemovalInfo>, 'disk' | 'library', string[], { deleteFiles: boolean; deleteDownload: boolean; keepInLibrary: boolean }, string];
  it.each<Case>([
    ['disk, owned files', {}, 'disk', [], { deleteFiles: true, deleteDownload: false, keepInLibrary: true }, ''],
    ['disk, an installer Typhon did not make', { method: 'installer', owned: false } as Partial<RemovalInfo>, 'disk', [], { deleteFiles: false, deleteDownload: false, keepInLibrary: true }, ''],
    ['library, owned files kept on', {}, 'library', [], { deleteFiles: true, deleteDownload: false, keepInLibrary: false }, ''],
    ['library, files switched off', {}, 'library', ['files'], { deleteFiles: false, deleteDownload: false, keepInLibrary: false }, ''],
    ['library, a folder that is gone', { dirMissing: true } as Partial<RemovalInfo>, 'library', [], { deleteFiles: false, deleteDownload: false, keepInLibrary: false }, ''],
    ['disk, with its download', { downloadPresent: true, downloadBytes: 2048 } as Partial<RemovalInfo>, 'disk', ['download'], { deleteFiles: true, deleteDownload: true, keepInLibrary: true }, ''],
    ['an external game', {}, 'library', [], { deleteFiles: false, deleteDownload: false, keepInLibrary: false }, 'Steam'],
  ])('%s: one RemoveGame with exactly the chosen options', async (_case, patch, mode, switches, expected, external) => {
    const onremoved = vi.fn();
    vi.mocked(removeGame).mockResolvedValue(undefined);
    const { dialog } = await openDialog(patch, { mode, external, onremoved });
    const keys = user();
    if (switches.includes('files')) await keys.click(within(dialog).getByRole('switch', { name: msg('modals.removeGameDeleteGameFiles') }));
    if (switches.includes('download')) await keys.click(within(dialog).getByRole('switch', { name: msg('modals.removeGameDeleteDownloadLabel') }));
    const label = mode === 'disk' ? 'modals.removeGameModeDiskLabel' : 'modals.removeGameModeLibraryLabel';

    await keys.click(confirmButton(dialog, label));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(removeGame).mock.calls).toEqual([['g1', expected]]);
    expect(onremoved.mock.calls).toEqual([[mode]]);
    const words = mode === 'disk' ? msg('modals.removeGameRemovedFromDisk', { title: 'Hades' }) : msg('modals.removeGameRemovedFromLibrary', { title: 'Hades' });
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[words, 'info']]);
  });

  it('sends one RemoveGame for any number of presses while it runs', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const { dialog } = await openDialog();
    const keys = user();
    const remove = confirmButton(dialog, 'modals.removeGameModeDiskLabel');
    remove.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    await keys.click(remove);

    expect(vi.mocked(removeGame)).toHaveBeenCalledTimes(1);
    pending.resolve();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('sends nothing while the game is running', async () => {
    const { dialog } = await openDialog({ running: true });
    await user().click(confirmButton(dialog, 'modals.removeGameModeDiskLabel'));
    expect(vi.mocked(removeGame)).not.toHaveBeenCalled();
  });
});

describe('RemoveGameModal: the inspect read', () => {
  it('says it is reading the game with a labelled spinner', async () => {
    const pending = held<RemovalInfo>();
    vi.mocked(inspectRemoval).mockReturnValueOnce(pending.promise);
    render(RemoveGameModal, { target: app, props: { open: true, gameId: 'g1', title: 'Hades' } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.removeGameTitle') });
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('modals.removeGameLoading', { title: 'Hades' }));
    expect(confirmButton(dialog, 'modals.removeGameModeDiskLabel').getAttribute('aria-disabled') === 'true' || confirmButton(dialog, 'modals.removeGameModeDiskLabel').hasAttribute('disabled')).toBe(true);
    pending.resolve(info());
    await within(dialog).findByText(msg('modals.removeGameLibrarySub'));
  });

  it('says a failed read by its code, with a Retry that reads again', async () => {
    vi.mocked(inspectRemoval).mockRejectedValueOnce(coded('install.no_library_access')).mockResolvedValueOnce(info());
    render(RemoveGameModal, { target: app, props: { open: true, gameId: 'g1', title: 'Hades' } });
    const dialog = screen.getByRole('dialog', { name: msg('modals.removeGameTitle') });
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent).toContain(msg('errInstall.installNoLibraryAccess'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();

    await user().click(within(dialog).getByRole('button', { name: msg('common.retry') }));

    await within(dialog).findByText(msg('modals.removeGameLibrarySub'));
    expect(vi.mocked(inspectRemoval).mock.calls).toEqual([['g1'], ['g1']]);
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('paints only the answer for the game it is open for, when the earlier game answers last', async () => {
    const first = held<RemovalInfo>();
    const second = held<RemovalInfo>();
    vi.mocked(inspectRemoval).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const view = render(RemoveGameModal, { target: app, props: { open: true, gameId: 'g1', title: 'Hades' } });
    await view.rerender({ gameId: 'g2', title: 'Celeste' });
    second.resolve(info({ gameId: 'g2', title: 'Celeste', installDir: '/games/celeste' }));
    const dialog = screen.getByRole('dialog', { name: msg('modals.removeGameTitle') });
    await within(dialog).findByText('/games/celeste');

    first.resolve(info());
    await settle();

    expect(within(dialog).queryByText('/games/hades')).toBeNull();
    expect(within(dialog).getByText('/games/celeste')).toBeTruthy();
  });
});

describe('RemoveGameModal: messages', () => {
  it('states that the game is running in a warning notice that does not announce', async () => {
    const { dialog } = await openDialog({ running: true });
    const notice = dialog.querySelector('[data-tone="warning"]');
    expect(notice?.textContent?.trim()).toBe(msg('modals.removeGameRunningWarning'));
    expect(notice?.querySelector('[role="status"], [role="alert"]')).toBeNull();
    const remove = confirmButton(dialog, 'modals.removeGameModeDiskLabel');
    expect(remove.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(remove)).toContain(msg('modals.removeGameRunningWarning'));
  });

  it('says a refused removal by its code in a danger notice inside the dialog', async () => {
    vi.mocked(removeGame).mockRejectedValueOnce(coded('install.files_locked'));
    const { dialog } = await openDialog();
    const remove = confirmButton(dialog, 'modals.removeGameModeDiskLabel');
    remove.focus();
    await user().keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errInstall.installFilesLocked'));
    const notice = alert.closest<HTMLElement>('[data-tone="danger"]');
    expect(notice).not.toBeNull();
    await vi.waitFor(() => expect(document.activeElement).toBe(notice));
    expect(get(toasts)).toEqual([]);
    expect(vi.mocked(removeGame)).toHaveBeenCalledTimes(1);
  });

  it('says an uncoded failure in its own words, never the error text', async () => {
    vi.mocked(removeGame).mockRejectedValueOnce(new Error('rename C:\\Games\\Hades: access is denied'));
    const { dialog } = await openDialog();
    await user().click(confirmButton(dialog, 'modals.removeGameModeDiskLabel'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.removeGameDeleteFailed'));
  });
});

describe('RemoveGameModal: while the removal runs', () => {
  it('keeps the focus on the pressed button, busy, and holds the rest with a reason', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const { dialog } = await openDialog({ downloadPresent: true, downloadBytes: 10 }, { mode: 'library' });
    const keys = user();
    const remove = confirmButton(dialog, 'modals.removeGameModeLibraryLabel');
    remove.focus();

    await keys.keyboard('{Enter}');

    const busy = confirmButton(dialog, 'modals.removeGameWorking');
    expect(busy).toBe(remove);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(busy).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(busy);

    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(cancel).toHaveProperty('disabled', false);
    expect(describedBy(cancel)).toContain(msg('modals.actionRunning'));
    expect(within(dialog).getByRole('radiogroup').getAttribute('aria-disabled')).toBe('true');
    for (const toggle of within(dialog).getAllByRole('switch')) expect(toggle).toHaveProperty('disabled', true);

    await keys.click(cancel);
    await keys.keyboard('{Escape}');
    await keys.click(radio(dialog, 'modals.removeGameModeDiskLabel'));
    expect(screen.getByRole('dialog')).toBe(dialog);
    expect(radio(dialog, 'modals.removeGameModeLibraryLabel').getAttribute('aria-checked')).toBe('true');

    pending.resolve();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
});

describe('RemoveGameModal: an answer after the dialog is gone', () => {
  it('a destroyed dialog that hears success after the player left the page says it once, naming the game, and goes nowhere', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const onremoved = vi.fn();
    const { dialog, view } = await openDialog({}, { mode: 'library', onremoved });
    await user().click(confirmButton(dialog, 'modals.removeGameModeLibraryLabel'));
    view.unmount();
    navigate('settings');

    pending.resolve();
    await settle();

    expect(onremoved).not.toHaveBeenCalled();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[msg('modals.removeGameRemovedFromLibrary', { title: 'Hades' }), 'info']]);
  });

  it('a destroyed dialog that hears success while the player is still on its page finishes the page’s flow', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const onremoved = vi.fn();
    const { dialog, view } = await openDialog({}, { mode: 'library', onremoved });
    await user().click(confirmButton(dialog, 'modals.removeGameModeLibraryLabel'));
    view.unmount();

    pending.resolve();
    await settle();

    expect(onremoved.mock.calls).toEqual([['library']]);
    expect(get(toasts).map((t) => t.message)).toEqual([msg('modals.removeGameRemovedFromLibrary', { title: 'Hades' })]);
  });

  it('a destroyed dialog that hears a refusal says it once, naming the game, with the reason by code', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const onremoved = vi.fn();
    const { dialog, view } = await openDialog({}, { mode: 'library', onremoved });
    await user().click(confirmButton(dialog, 'modals.removeGameModeLibraryLabel'));
    view.unmount();

    pending.reject(coded('install.files_locked'));
    await settle();

    expect(onremoved).not.toHaveBeenCalled();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [msg('modals.removeGameFailedForReason', { title: 'Hades', reason: msg('errInstall.installFilesLocked') }), 'danger'],
    ]);
  });

  it('a dead dialog says an uncoded refusal without a reason it does not have', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const { dialog, view } = await openDialog();
    await user().click(confirmButton(dialog, 'modals.removeGameModeDiskLabel'));
    view.unmount();

    pending.reject(new Error('boom'));
    await settle();

    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[msg('modals.removeGameFailedFor', { title: 'Hades' }), 'danger']]);
  });

  it('a dialog turned to another game does not close or report the first one as the second', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    const onremoved = vi.fn();
    const { dialog, view } = await openDialog({}, { mode: 'library', onremoved });
    await user().click(confirmButton(dialog, 'modals.removeGameModeLibraryLabel'));
    vi.mocked(inspectRemoval).mockResolvedValue(info({ gameId: 'g2', title: 'Celeste', installDir: '/games/celeste' }));
    await view.rerender({ gameId: 'g2', title: 'Celeste' });
    await within(dialog).findByText('/games/celeste');

    pending.resolve();
    await settle();

    expect(onremoved).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBe(dialog);
    expect(get(toasts).map((t) => t.message)).toEqual([msg('modals.removeGameRemovedFromLibrary', { title: 'Hades' })]);
    expect(confirmButton(dialog, 'modals.removeGameModeLibraryLabel').getAttribute('aria-busy')).toBeNull();
  });
});

describe('RemoveGameModal: a refusal for a removal Go has made', () => {
  const unfinished = (key: 'modals.removeGameRemovedFromDiskUnfinished' | 'modals.removeGameRemovedFromLibraryUnfinished', reason: string) =>
    msg(key, { title: 'Hades', reason });

  it('the card is gone from the library: says it was removed with what did not finish, and closes as after a removal', async () => {
    vi.mocked(removeGame).mockRejectedValueOnce(coded('download.cleanup_in_use'));
    vi.mocked(getGames).mockResolvedValue([]);
    const onremoved = vi.fn();
    const { dialog } = await openDialog({}, { mode: 'library', onremoved });

    await user().click(confirmButton(dialog, 'modals.removeGameModeLibraryLabel'));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(onremoved.mock.calls).toEqual([['library']]);
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [unfinished('modals.removeGameRemovedFromLibraryUnfinished', msg('errInstall.downloadCleanupInUse')), 'warning'],
    ]);
    expect(vi.mocked(removeGame)).toHaveBeenCalledTimes(1);
  });

  it('a card that was uninstalled before the press: a refusal stays a refusal, nothing is said as removed', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: true } as LibraryGame]);
    vi.mocked(removeGame).mockRejectedValueOnce(coded('install.files_locked'));
    vi.mocked(getGames).mockResolvedValue([{ id: 'g1', title: 'Hades', uninstalled: true } as LibraryGame]);
    const onremoved = vi.fn();
    const { dialog } = await openDialog({}, { mode: 'disk', onremoved });

    await user().click(confirmButton(dialog, 'modals.removeGameModeDiskLabel'));

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('errInstall.installFilesLocked'));
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([]);
    expect(onremoved).not.toHaveBeenCalled();
  });

  it('the card stays, uninstalled: says it was removed from this computer, and closes', async () => {
    vi.mocked(removeGame).mockRejectedValueOnce(new Error('save installs: disk full'));
    vi.mocked(getGames).mockResolvedValue([{ id: 'g1', title: 'Hades', uninstalled: true } as LibraryGame]);
    const onremoved = vi.fn();
    const { dialog } = await openDialog({}, { mode: 'disk', onremoved });

    await user().click(confirmButton(dialog, 'modals.removeGameModeDiskLabel'));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(onremoved.mock.calls).toEqual([['disk']]);
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [unfinished('modals.removeGameRemovedFromDiskUnfinished', msg('errInstall.fallback')), 'warning'],
    ]);
  });

  it('a destroyed dialog after the player left: the same words once, and goes nowhere', async () => {
    const pending = held<void>();
    vi.mocked(removeGame).mockReturnValueOnce(pending.promise);
    vi.mocked(getGames).mockResolvedValue([]);
    const onremoved = vi.fn();
    const { dialog, view } = await openDialog({}, { mode: 'library', onremoved });
    await user().click(confirmButton(dialog, 'modals.removeGameModeLibraryLabel'));
    view.unmount();
    navigate('settings');

    pending.reject(coded('download.cleanup_in_use'));
    await settle();

    expect(onremoved).not.toHaveBeenCalled();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [unfinished('modals.removeGameRemovedFromLibraryUnfinished', msg('errInstall.downloadCleanupInUse')), 'warning'],
    ]);
  });

  it('the card is still installed: the refusal stays a refusal, in the dialog', async () => {
    vi.mocked(removeGame).mockRejectedValueOnce(coded('install.files_locked'));
    const { dialog } = await openDialog({}, { mode: 'disk' });

    await user().click(confirmButton(dialog, 'modals.removeGameModeDiskLabel'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errInstall.installFilesLocked'));
    expect(vi.mocked(getGames)).toHaveBeenCalledTimes(1);
    expect(get(toasts)).toEqual([]);
  });

  it('a library it cannot read: the refusal stays a refusal, in the dialog', async () => {
    vi.mocked(removeGame).mockRejectedValueOnce(coded('install.files_locked'));
    vi.mocked(getGames).mockRejectedValue(new Error('unavailable'));
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { dialog } = await openDialog({}, { mode: 'library' });

    await user().click(confirmButton(dialog, 'modals.removeGameModeLibraryLabel'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errInstall.installFilesLocked'));
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });
});
