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
import { fileWrites } from '../game/fileWrites.svelte';
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
  // The file writes are the session's, shared by every mount: a test leaves none running.
  const left = ['g1', 'g2'].filter((id) => fileWrites.of(id) !== null);
  resetBindings();
  closeGameProperties();
  libraryGames.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
  expect(left).toEqual([]);
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

  it("two games: one game's switch runs, the other's file dialog ends: the first game stays held", async () => {
    const enabling = held<void>();
    const picking = held<string>();
    vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
    vi.mocked(selectGameExecutable).mockReturnValueOnce(picking.promise);
    render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.compat(first));
    const other = await openDialog('g2', 'Celeste');
    await keys.click(q.exeBrowse(other));
    picking.resolve('');
    await vi.waitFor(() => expect(q.exeBrowse(other).getAttribute('aria-busy')).toBeNull());

    const dialog = await openDialog();

    expectHeld(q.exeBrowse(dialog), 'modals.propertiesHeldCompat');
    await keys.click(q.exeOther(dialog));
    expect(vi.mocked(setExecutable)).not.toHaveBeenCalled();
    enabling.resolve();
    await vi.waitFor(() => expect(q.exeBrowse(dialog).getAttribute('aria-disabled')).toBeNull());
  });
});

// A language switch ({#key $locale} in App), entering or leaving Big Picture, or a sign-out and
// sign-in tear the dialog down and mount a new one while a write of the old one still runs.
describe('GamePropertiesModal: a dialog mounted again knows the file writes its predecessor began', () => {
  const saved = { kind: 'success', message: msg('modals.propertiesLateSaved', { title: 'Hades' }), count: 1 };
  // Every answer's microtasks are done once a macrotask runs.
  const settled = () => new Promise((done) => setTimeout(done, 0));

  async function remounted(mounted: { unmount: () => void }) {
    mounted.unmount();
    render(GamePropertiesModal, { target: app });
    return opened();
  }

  it('compatibility mode turning on: the executable and the switch are held with its reason, and the game is read again once it is done', async () => {
    const enabling = held<void>();
    vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
    const mounted = render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.compat(first));
    const dialog = await remounted(mounted);

    expectHeld(q.compat(dialog), 'modals.propertiesHeldCompat');
    expectHeld(q.exeBrowse(dialog), 'modals.propertiesHeldCompat');
    expectHeld(within(dialog).getByRole('radiogroup'), 'modals.propertiesHeldCompat');
    expectHeld(q.savesBrowse(dialog), 'modals.propertiesHeldCompat');
    await keys.click(q.exeOther(dialog));
    await keys.click(q.exeBrowse(dialog));
    await keys.click(q.compat(dialog));
    expect(vi.mocked(setExecutable)).not.toHaveBeenCalled();
    expect(vi.mocked(selectGameExecutable)).not.toHaveBeenCalled();
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);
    const reads = vi.mocked(inspectWrapper).mock.calls.length;
    vi.mocked(inspectWrapper).mockResolvedValue(wrapper({ active: { wrapper: 'dgvoodoo', version: '2.8', preset: 'default', files: ['d3d9.dll'] } } as Partial<WrapperInfo>));

    enabling.resolve();

    await vi.waitFor(() => expect(q.compat(dialog).getAttribute('aria-checked')).toBe('true'));
    expect(q.compat(dialog).getAttribute('aria-disabled')).toBeNull();
    expect(q.exeBrowse(dialog).getAttribute('aria-disabled')).toBeNull();
    await settled();
    // Read by the dialog on screen; the torn-down one reads nothing.
    expect(vi.mocked(inspectWrapper).mock.calls.slice(reads)).toEqual([['g1']]);
    expect(toastList()).toEqual([saved]);
    await keys.click(q.exeOther(dialog));
    expect(vi.mocked(setExecutable).mock.calls).toEqual([['g1', '/games/hades/x64/Hades.exe']]);
  });

  it('a switch it did not begin does not keep it open: Escape closes it, and the outcome is said once', async () => {
    const enabling = held<void>();
    vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
    const mounted = render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.compat(first));
    const dialog = await remounted(mounted);

    await keys.keyboard('{Escape}');

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    enabling.reject(new Error('typhon:wrappers.download_failed: offline'));
    await vi.waitFor(() => expect(toastList()).toHaveLength(1));
    await settled();
    expect(toastList()).toHaveLength(1);
    expect(toastList()[0].kind).toBe('danger');
  });

  it('the saves folder being chosen: compatibility mode is held with its reason, and the folder picked then is not set, which is said once', async () => {
    const picking = held<string>();
    vi.mocked(selectFolder).mockReturnValueOnce(picking.promise);
    vi.mocked(enableWrapper).mockResolvedValueOnce(undefined);
    const mounted = render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.savesBrowse(first));
    const dialog = await remounted(mounted);

    expectHeld(q.compat(dialog), 'modals.propertiesHeldSaves');
    expectHeld(q.savesBrowse(dialog), 'modals.propertiesHeldSaves');
    await keys.click(q.compat(dialog));
    expect(vi.mocked(enableWrapper)).not.toHaveBeenCalled();

    picking.resolve('/saves/elsewhere');

    await vi.waitFor(() => expect(q.compat(dialog).getAttribute('aria-disabled')).toBeNull());
    await settled();
    expect(vi.mocked(setSavesDir)).not.toHaveBeenCalled();
    expect(toastList()).toEqual([{ kind: 'warning', message: msg('modals.propertiesSavesLateDropped', { title: 'Hades' }), count: 1 }]);
    expect(q.savesBrowse(dialog).getAttribute('aria-disabled')).toBeNull();
  });

  it('a found executable being set: compatibility mode is held with its reason until Go answers, then read again for the new file', async () => {
    const setting = held<LibraryGame>();
    vi.mocked(setExecutable).mockReturnValueOnce(setting.promise);
    const mounted = render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.exeOther(first));
    const dialog = await remounted(mounted);

    expectHeld(q.compat(dialog), 'modals.propertiesHeldExe');
    expectHeld(q.exeBrowse(dialog), 'modals.propertiesHeldExe');
    await keys.click(q.compat(dialog));
    expect(vi.mocked(enableWrapper)).not.toHaveBeenCalled();
    const reads = vi.mocked(inspectWrapper).mock.calls.length;

    setting.resolve(hades);

    await vi.waitFor(() => expect(q.compat(dialog).getAttribute('aria-disabled')).toBeNull());
    await settled();
    expect(vi.mocked(inspectWrapper).mock.calls.slice(reads)).toEqual([['g1']]);
    expect(toastList()).toEqual([saved]);
  });

  it('the saves folder being reset: the saves controls are held with its reason, Escape closes it, and the outcome is said once', async () => {
    const resetting = held<LibraryGame>();
    vi.mocked(clearSavesDir).mockReturnValueOnce(resetting.promise);
    const mounted = render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.savesReset(first));
    await keys.click(await screen.findByRole('button', { name: msg('modals.propertiesSavesResetConfirm') }));
    expect(vi.mocked(clearSavesDir)).toHaveBeenCalledTimes(1);
    const dialog = await remounted(mounted);

    expectHeld(q.savesBrowse(dialog), 'modals.propertiesHeldSaves');
    expectHeld(q.savesReset(dialog), 'modals.propertiesHeldSaves');
    expectHeld(q.compat(dialog), 'modals.propertiesHeldSaves');
    await keys.keyboard('{Escape}');
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    const again = await openDialog();
    expectHeld(q.compat(again), 'modals.propertiesHeldSaves');

    resetting.resolve(hades);

    await vi.waitFor(() => expect(q.compat(again).getAttribute('aria-disabled')).toBeNull());
    await settled();
    expect(toastList()).toEqual([saved]);
  });

  // The other side of the rule: its own Reset or switch keeps the dialog that began it open.
  it('its own reset still running when the dialog comes back to the game: Escape does not close it', async () => {
    const resetting = held<LibraryGame>();
    vi.mocked(clearSavesDir).mockReturnValueOnce(resetting.promise);
    render(GamePropertiesModal, { target: app });
    const keys = user();
    const first = await openDialog();
    await keys.click(q.savesReset(first));
    await keys.click(await screen.findByRole('button', { name: msg('modals.propertiesSavesResetConfirm') }));
    await openDialog('g2', 'Celeste');
    const dialog = await openDialog();
    expect(screen.queryByRole('button', { name: msg('modals.propertiesSavesResetConfirm') })).toBeNull();

    await keys.keyboard('{Escape}');
    await settled();

    expect(dialog.isConnected).toBe(true);
    resetting.resolve(hades);
    await vi.waitFor(() => expect(q.savesBrowse(dialog).getAttribute('aria-disabled')).toBeNull());
    await keys.keyboard('{Escape}');
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  // A dialog torn down while its file dialog was open: the file picked then is not set, and that is said.
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
