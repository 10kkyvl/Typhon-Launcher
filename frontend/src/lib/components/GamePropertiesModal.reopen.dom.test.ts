import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../i18n';
import { executableCandidates } from '../services/install';
import { clearSavesDir, getLaunchArgsLine, selectGameExecutable, setExecutable, setGameModeOverride, setLaunchArgsLine, setSavesDir, type LibraryGame } from '../services/library';
import { selectFolder } from '../services/settings';
import { appInfo } from '../services/system';
import { enableWrapper, inspectWrapper, type WrapperInfo } from '../services/wrappers';
import { closeGameProperties, openGameProperties } from '../stores/gameProperties';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import GamePropertiesModal from './GamePropertiesModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/install')>()),
  executableCandidates: vi.fn(),
}));
vi.mock('../services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/system')>()),
  appInfo: vi.fn(),
}));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  selectFolder: vi.fn(),
}));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  getLaunchArgsLine: vi.fn(),
  setLaunchArgsLine: vi.fn(),
  setExecutable: vi.fn(),
  selectGameExecutable: vi.fn(),
  clearSavesDir: vi.fn(),
  setSavesDir: vi.fn(),
  setRunAsAdmin: vi.fn(),
  setGameModeOverride: vi.fn(),
  setHidePresence: vi.fn(),
  getProfiles: vi.fn(async () => []),
  profileSteamStatus: vi.fn(async () => ({ available: false })),
}));
vi.mock('../services/wrappers', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/wrappers')>()),
  inspectWrapper: vi.fn(),
  enableWrapper: vi.fn(),
  disableWrapper: vi.fn(),
  disableWrapperKeepChanged: vi.fn(),
}));

const hades = { id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: '/games/hades/Hades.exe', savesDir: '/saves/hades' } as LibraryGame;
const celeste = { id: 'g2', title: 'Celeste', uninstalled: false, installDir: '/games/celeste', executable: '/games/celeste/Celeste.exe', savesDir: '/saves/celeste' } as LibraryGame;

function wrapper(patch: Partial<WrapperInfo> = {}): WrapperInfo {
  return { supported: true, arch: 'x86', imports: ['d3d9.dll'], suggested: 'dgvoodoo', active: null, problem: null, ...patch } as WrapperInfo;
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([hades, celeste]);
  // A queued answer one test left unused must not reach the next.
  for (const call of [setGameModeOverride, setExecutable, setLaunchArgsLine, selectGameExecutable, selectFolder, clearSavesDir, enableWrapper]) {
    vi.mocked(call).mockReset();
  }
  vi.mocked(executableCandidates).mockResolvedValue([
    { path: '/games/hades/Hades.exe', score: 1 },
    { path: '/games/hades/x64/Hades.exe', score: 0.5 },
  ]);
  vi.mocked(getLaunchArgsLine).mockResolvedValue('');
  vi.mocked(appInfo).mockResolvedValue({ version: '2.0.0', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' });
  vi.mocked(inspectWrapper).mockResolvedValue(wrapper());
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  closeGameProperties();
  libraryGames.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

async function opened(title = 'Hades') {
  const dialog = await screen.findByRole('dialog', { name: msg('modals.propertiesTitle', { title }) });
  await within(dialog).findByRole('radiogroup');
  await within(dialog).findByRole('switch', { name: msg('modals.compatToggle') });
  return dialog;
}

async function openDialog(id = 'g1', title = 'Hades') {
  openGameProperties(id);
  return opened(title);
}

async function closeDialog(dialog: HTMLElement) {
  closeGameProperties();
  await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
}

const q = {
  exeOther: (d: HTMLElement) => within(within(d).getByRole('radiogroup')).getAllByRole('radio')[1],
  exeBrowse: (d: HTMLElement) => within(d).getByRole('button', { name: msg('modals.propertiesExeBrowse') }),
  compat: (d: HTMLElement) => within(d).getByRole('switch', { name: msg('modals.compatToggle') }),
  savesBrowse: (d: HTMLElement) => within(d).getByRole('button', { name: msg('modals.propertiesSavesBrowse') }),
  savesReset: (d: HTMLElement) => within(d).getByRole('button', { name: msg('modals.propertiesSavesReset') }),
};

function expectHeld(el: HTMLElement, reason: MessageKey) {
  expect(el.getAttribute('aria-disabled')).toBe('true');
  expect(describedBy(el)).toContain(msg(reason));
}

// The toast store folds the same words raised twice into one with a count: compare the count too.
const toastList = () => get(toasts).map((t) => ({ kind: t.kind, message: t.message, count: t.count ?? 1 }));

describe('GamePropertiesModal: a file write goes on across openings of its game', () => {
  // The native file dialog has no owner window: the launcher stays usable while it is open. Go's
  // Enable takes the executable's folder at its start: a file set while it runs starts without the wrapper.
  it('Browse for the executable, closed and reopened: compatibility mode is held until the file is set, then read again', async () => {
    const picking = held<string>();
    vi.mocked(selectGameExecutable).mockReturnValueOnce(picking.promise);
    vi.mocked(setExecutable).mockResolvedValueOnce(hades);
    vi.mocked(enableWrapper).mockResolvedValueOnce(undefined);
    render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.exeBrowse(first));
    await closeDialog(first);

    const dialog = await openDialog();

    expectHeld(q.compat(dialog), 'modals.propertiesHeldExe');
    expectHeld(q.savesBrowse(dialog), 'modals.propertiesHeldExe');
    expectHeld(q.savesReset(dialog), 'modals.propertiesHeldExe');
    await keys.click(q.compat(dialog));
    expect(vi.mocked(enableWrapper)).not.toHaveBeenCalled();
    const reads = vi.mocked(inspectWrapper).mock.calls.length;

    picking.resolve('/games/hades/x64/Hades.exe');

    await vi.waitFor(() => expect(q.compat(dialog).getAttribute('aria-disabled')).toBeNull());
    expect(vi.mocked(setExecutable).mock.calls).toEqual([['g1', '/games/hades/x64/Hades.exe']]);
    await vi.waitFor(() => expect(vi.mocked(inspectWrapper).mock.calls.length).toBe(reads + 1));
    expect(toastList()).toEqual([{ kind: 'success', message: msg('modals.propertiesLateSaved', { title: 'Hades' }), count: 1 }]);
    await keys.click(q.compat(dialog));
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(setExecutable).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(enableWrapper).mock.invocationCallOrder[0]);
  });

  it('a found executable being set, closed and reopened: the others are held with its reason until Go answers', async () => {
    const setting = held<LibraryGame>();
    vi.mocked(setExecutable).mockReturnValueOnce(setting.promise);
    render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.exeOther(first));
    await closeDialog(first);

    const dialog = await openDialog();

    expectHeld(q.compat(dialog), 'modals.propertiesHeldExe');
    expectHeld(q.savesBrowse(dialog), 'modals.propertiesHeldExe');
    expect(q.exeBrowse(dialog).getAttribute('aria-disabled')).toBe('true');
    await keys.click(q.exeBrowse(dialog));
    expect(vi.mocked(selectGameExecutable)).not.toHaveBeenCalled();
    const reads = vi.mocked(inspectWrapper).mock.calls.length;

    setting.resolve(hades);

    await vi.waitFor(() => expect(q.compat(dialog).getAttribute('aria-disabled')).toBeNull());
    expect(q.exeBrowse(dialog).getAttribute('aria-disabled')).toBeNull();
    await vi.waitFor(() => expect(vi.mocked(inspectWrapper).mock.calls.length).toBe(reads + 1));
    expect(toastList()).toEqual([{ kind: 'success', message: msg('modals.propertiesLateSaved', { title: 'Hades' }), count: 1 }]);
  });

  it('an executable set in the same opening has compatibility mode read again for the new file', async () => {
    vi.mocked(setExecutable).mockResolvedValueOnce(hades);
    render(GamePropertiesModal, { target: app });
    const dialog = await openDialog();
    const reads = vi.mocked(inspectWrapper).mock.calls.length;
    vi.mocked(inspectWrapper).mockResolvedValue(wrapper({ supported: false, imports: [], suggested: '' }));

    await user().click(q.exeOther(dialog));

    await vi.waitFor(() => expect(within(dialog).queryByRole('switch', { name: msg('modals.compatToggle') })).toBeNull());
    expect(vi.mocked(inspectWrapper).mock.calls.slice(reads)).toEqual([['g1']]);
    expect(toastList()).toEqual([]);
  });

  it('Browse for the saves folder, closed and reopened: the executable and compatibility mode are held with its reason', async () => {
    const picking = held<string>();
    vi.mocked(selectFolder).mockReturnValueOnce(picking.promise);
    render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.savesBrowse(first));
    await closeDialog(first);

    const dialog = await openDialog();

    expectHeld(q.compat(dialog), 'modals.propertiesHeldSaves');
    expectHeld(q.exeBrowse(dialog), 'modals.propertiesHeldSaves');
    expectHeld(within(dialog).getByRole('radiogroup'), 'modals.propertiesHeldSaves');
    await keys.click(q.compat(dialog));
    expect(vi.mocked(enableWrapper)).not.toHaveBeenCalled();

    picking.resolve('');

    await vi.waitFor(() => expect(q.compat(dialog).getAttribute('aria-disabled')).toBeNull());
    expect(vi.mocked(setSavesDir)).not.toHaveBeenCalled();
    expect(toastList()).toEqual([]);
  });

  it("a dialog of another game is free while one game's file dialog is open", async () => {
    const picking = held<string>();
    vi.mocked(selectGameExecutable).mockReturnValueOnce(picking.promise);
    vi.mocked(enableWrapper).mockResolvedValueOnce(undefined);
    render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.exeBrowse(first));

    const other = await openDialog('g2', 'Celeste');

    for (const el of [q.compat(other), q.exeBrowse(other), q.savesBrowse(other), q.savesReset(other)]) expect(el.getAttribute('aria-disabled')).toBeNull();
    await keys.click(q.compat(other));
    expect(vi.mocked(enableWrapper).mock.calls).toEqual([['g2', 'dgvoodoo', 'default']]);
    picking.resolve('');
    await vi.waitFor(() => expect(vi.mocked(selectGameExecutable)).toHaveBeenCalledTimes(1));
    expect(vi.mocked(setExecutable)).not.toHaveBeenCalled();
  });

  // A dialog torn down (the language changed, Big Picture turned on) and mounted again knows nothing of
  // the write its predecessor began: the file picked then may land while the new one turns
  // compatibility mode on, so it is not set, and that is said.
  it('a file picked after its dialog was torn down is not set, and that is said', async () => {
    const picking = held<string>();
    vi.mocked(selectGameExecutable).mockReturnValueOnce(picking.promise);
    const mounted = render(GamePropertiesModal, { target: app });
    const first = await openDialog();
    await user().click(q.exeBrowse(first));
    mounted.unmount();
    render(GamePropertiesModal, { target: app });
    await opened();

    picking.resolve('/games/hades/x64/Hades.exe');

    await vi.waitFor(() => expect(toastList()).toEqual([{ kind: 'warning', message: msg('modals.propertiesExeLateDropped', { title: 'Hades' }), count: 1 }]));
    expect(vi.mocked(setExecutable)).not.toHaveBeenCalled();
  });
});
