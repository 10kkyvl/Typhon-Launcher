import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../i18n';
import { executableCandidates } from '../services/install';
import { clearSavesDir, getLaunchArgsLine, selectGameExecutable, setExecutable, setGameModeOverride, setLaunchArgsLine, setSavesDir, type LibraryGame } from '../services/library';
import { selectFolder } from '../services/settings';
import { appInfo } from '../services/system';
import { disableWrapper, enableWrapper, inspectWrapper, type WrapperInfo } from '../services/wrappers';
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

function wrapper(patch: Partial<WrapperInfo> = {}): WrapperInfo {
  return { supported: true, arch: 'x86', imports: ['d3d9.dll'], suggested: 'dgvoodoo', active: null, problem: null, ...patch } as WrapperInfo;
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([hades]);
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

async function openDialog() {
  render(GamePropertiesModal, { target: app });
  openGameProperties('g1');
  const dialog = await screen.findByRole('dialog', { name: msg('modals.propertiesTitle', { title: 'Hades' }) });
  await within(dialog).findByRole('radiogroup');
  await within(dialog).findByRole('switch', { name: msg('modals.compatToggle') });
  return dialog;
}

const finders = {
  exeList: (dialog: HTMLElement) => within(dialog).getByRole('radiogroup'),
  exeOther: (dialog: HTMLElement) => within(within(dialog).getByRole('radiogroup')).getAllByRole('radio')[1],
  exeBrowse: (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.propertiesExeBrowse') }),
  compat: (dialog: HTMLElement) => within(dialog).getByRole('switch', { name: msg('modals.compatToggle') }),
  wrapperSelect: (dialog: HTMLElement) => within(dialog).getByRole('combobox', { name: msg('modals.compatWrapper') }),
  presetSelect: (dialog: HTMLElement) => within(dialog).getByRole('combobox', { name: msg('modals.compatPresetLabel') }),
  savesBrowse: (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.propertiesSavesBrowse') }),
  savesReset: (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.propertiesSavesReset') }),
};

type Control = keyof typeof finders;

const controls = (dialog: HTMLElement) =>
  new Proxy({} as Record<Control, HTMLElement>, { get: (_target, name) => finders[name as Control](dialog) });

/** A held control: says so and why, keeps its place in the Tab order, and is not natively disabled. */
function expectHeld(el: HTMLElement, reason: string) {
  expect(el.getAttribute('aria-disabled')).toBe('true');
  expect(describedBy(el)).toContain(reason);
}

const writes: {
  name: string;
  start: (dialog: HTMLElement) => Promise<() => void>;
  reason: MessageKey;
  held: Control[];
}[] = [
  {
    name: 'the wrapper switch',
    start: async (dialog) => {
      const enabling = held<void>();
      vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
      await user().click(controls(dialog).compat);
      expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);
      return () => enabling.resolve();
    },
    reason: 'modals.propertiesHeldCompat',
    held: ['exeList', 'exeBrowse', 'savesBrowse', 'savesReset'],
  },
  {
    name: 'setting a found executable',
    start: async (dialog) => {
      const setting = held<LibraryGame>();
      vi.mocked(setExecutable).mockReturnValueOnce(setting.promise);
      await user().click(controls(dialog).exeOther);
      expect(vi.mocked(setExecutable)).toHaveBeenCalledTimes(1);
      return () => setting.resolve(hades);
    },
    reason: 'modals.propertiesHeldExe',
    held: ['compat', 'savesBrowse', 'savesReset'],
  },
  {
    name: 'browsing for the executable',
    start: async (dialog) => {
      const picking = held<string>();
      vi.mocked(selectGameExecutable).mockReturnValueOnce(picking.promise);
      await user().click(controls(dialog).exeBrowse);
      expect(vi.mocked(selectGameExecutable)).toHaveBeenCalledTimes(1);
      return () => picking.resolve('');
    },
    reason: 'modals.propertiesHeldExe',
    held: ['compat', 'savesBrowse', 'savesReset'],
  },
  {
    name: 'browsing for the saves folder',
    start: async (dialog) => {
      const picking = held<string>();
      vi.mocked(selectFolder).mockReturnValueOnce(picking.promise);
      await user().click(controls(dialog).savesBrowse);
      expect(vi.mocked(selectFolder)).toHaveBeenCalledTimes(1);
      return () => picking.resolve('');
    },
    reason: 'modals.propertiesHeldSaves',
    held: ['exeList', 'exeBrowse', 'compat'],
  },
];

describe('GamePropertiesModal: one write that changes what the game starts or its files at a time', () => {
  // Go's Enable has taken the folder of the old executable: a file set now would start without the wrapper.
  it('while compatibility mode turns on, a press on another found executable sets nothing', async () => {
    const enabling = held<void>();
    vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
    const dialog = await openDialog();
    const keys = user();
    await keys.click(controls(dialog).compat);

    await keys.click(controls(dialog).exeOther);

    expect(vi.mocked(setExecutable)).not.toHaveBeenCalled();
    expect(controls(dialog).exeOther.getAttribute('aria-checked')).toBe('false');
    enabling.resolve();
  });

  it.each(writes)('while $name runs, the other such controls are held with its reason', async ({ start, reason, held: names }) => {
    const dialog = await openDialog();
    const finish = await start(dialog);

    const now = controls(dialog);
    for (const name of names) expectHeld(now[name], msg(reason));
    if (names.includes('compat')) {
      expect(now.wrapperSelect).toHaveProperty('disabled', true);
      expect(describedBy(now.wrapperSelect)).toContain(msg(reason));
      expect(describedBy(now.presetSelect)).toContain(msg(reason));
    }
    finish();
    await vi.waitFor(() => {
      for (const name of names) expect(controls(dialog)[name].getAttribute('aria-disabled')).toBeNull();
    });
  });

  it.each(writes)('while $name runs, a press on a held control sends nothing and changes nothing on screen', async ({ start, held: names }) => {
    const dialog = await openDialog();
    const finish = await start(dialog);
    const keys = user();
    const before = {
      setExecutable: vi.mocked(setExecutable).mock.calls.length,
      selectGameExecutable: vi.mocked(selectGameExecutable).mock.calls.length,
      enableWrapper: vi.mocked(enableWrapper).mock.calls.length,
      selectFolder: vi.mocked(selectFolder).mock.calls.length,
    };
    const checked = controls(dialog).compat.getAttribute('aria-checked');
    const chosen = controls(dialog).exeOther.getAttribute('aria-checked');

    // The list is pressed on the file it does not start.
    for (const name of names) await keys.click(controls(dialog)[name === 'exeList' ? 'exeOther' : name]);

    expect(vi.mocked(setExecutable).mock.calls.length).toBe(before.setExecutable);
    expect(vi.mocked(selectGameExecutable).mock.calls.length).toBe(before.selectGameExecutable);
    expect(vi.mocked(enableWrapper).mock.calls.length).toBe(before.enableWrapper);
    expect(vi.mocked(selectFolder).mock.calls.length).toBe(before.selectFolder);
    expect(vi.mocked(setSavesDir)).not.toHaveBeenCalled();
    expect(vi.mocked(clearSavesDir)).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog', { name: msg('modals.propertiesSavesResetTitle') })).toBeNull();
    expect(controls(dialog).compat.getAttribute('aria-checked')).toBe(checked);
    expect(controls(dialog).exeOther.getAttribute('aria-checked')).toBe(chosen);
    finish();
    await vi.waitFor(() => expect(controls(dialog).compat.getAttribute('aria-disabled')).toBeNull());
  });

  it('while the saves folder is reset, the executable and compatibility controls are held with its reason', async () => {
    const clearing = held<LibraryGame>();
    vi.mocked(clearSavesDir).mockReturnValueOnce(clearing.promise);
    const dialog = await openDialog();
    const keys = user();
    await keys.click(controls(dialog).savesReset);
    const question = await screen.findByRole('dialog', { name: msg('modals.propertiesSavesResetTitle') });
    await keys.click(within(question).getByRole('button', { name: msg('modals.propertiesSavesResetConfirm') }));
    expect(vi.mocked(clearSavesDir).mock.calls).toEqual([['g1']]);

    const now = controls(dialog);
    for (const name of ['exeList', 'exeBrowse', 'compat'] as const) expectHeld(now[name], msg('modals.propertiesHeldSaves'));
    clearing.resolve({ ...hades, savesDir: '' });
    await vi.waitFor(() => expect(question.isConnected).toBe(false));
  });

  it("while an executable is set, a problem's Revert is held with its reason and sends nothing", async () => {
    vi.mocked(inspectWrapper).mockResolvedValue(
      wrapper({ active: { wrapper: 'dgvoodoo', version: '2.8', preset: 'default', files: ['d3d9.dll'], appliedAt: '' }, problem: { code: 'wrappers.recovery_failed' } } as Partial<WrapperInfo>),
    );
    const setting = held<LibraryGame>();
    vi.mocked(setExecutable).mockReturnValueOnce(setting.promise);
    const dialog = await openDialog();
    const keys = user();
    const revert = await within(dialog).findByRole('button', { name: msg('modals.compatRevert') });

    await keys.click(controls(dialog).exeOther);
    expectHeld(revert, msg('modals.propertiesHeldExe'));
    await keys.click(revert);

    expect(vi.mocked(disableWrapper)).not.toHaveBeenCalled();
    expect(get(toasts)).toEqual([]);
    setting.resolve(hades);
    await vi.waitFor(() => expect(revert.getAttribute('aria-disabled')).toBeNull());
  });
});

describe('GamePropertiesModal: game mode changes made while one is written', () => {
  async function chooseIn(dialog: HTMLElement, label: MessageKey, option: MessageKey) {
    const keys = user();
    const select = within(dialog).getByRole('combobox', { name: msg(label) });
    await keys.click(select);
    await keys.click(await screen.findByRole('option', { name: msg(option) }));
    return select;
  }

  it('wait their turn, show the chosen value meanwhile, and are sent in order, each on top of the one before', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockResolvedValueOnce(hades);
    const dialog = await openDialog();
    await within(dialog).findByRole('combobox', { name: msg('settings.gameModePowerLabel') });

    await chooseIn(dialog, 'settings.gameModePowerLabel', 'modals.propertiesGameModeOn');
    const hide = await chooseIn(dialog, 'settings.gameModeHideLabel', 'modals.propertiesGameModeOff');

    expect(hide.textContent).toContain(msg('modals.propertiesGameModeOff'));
    expect(vi.mocked(setGameModeOverride).mock.calls).toEqual([['g1', { highPerformance: true }]]);
    first.resolve(hades);
    await vi.waitFor(() =>
      expect(vi.mocked(setGameModeOverride).mock.calls).toEqual([
        ['g1', { highPerformance: true }],
        ['g1', { highPerformance: true, hideLauncher: false }],
      ]),
    );
    expect(hide.textContent).toContain(msg('modals.propertiesGameModeOff'));
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('refused after waiting, shows the stored value again and says why in the dialog', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockRejectedValueOnce(new Error('disk'));
    const dialog = await openDialog();
    await within(dialog).findByRole('combobox', { name: msg('settings.gameModePowerLabel') });

    const power = await chooseIn(dialog, 'settings.gameModePowerLabel', 'modals.propertiesGameModeOn');
    const hide = await chooseIn(dialog, 'settings.gameModeHideLabel', 'modals.propertiesGameModeOff');
    first.resolve(hades);

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('modals.propertiesGameModeFailed'));
    expect(hide.textContent).not.toContain(msg('modals.propertiesGameModeOff'));
    expect(power.textContent).toContain(msg('modals.propertiesGameModeOn'));
    expect(vi.mocked(setGameModeOverride)).toHaveBeenCalledTimes(2);
    expect(get(toasts)).toEqual([]);
  });

  it('still waiting when the dialog closes, are sent in order and said once in a toast naming the game', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockResolvedValueOnce(hades);
    const dialog = await openDialog();
    await within(dialog).findByRole('combobox', { name: msg('settings.gameModePowerLabel') });
    await chooseIn(dialog, 'settings.gameModePowerLabel', 'modals.propertiesGameModeOn');
    await chooseIn(dialog, 'settings.gameModeHideLabel', 'modals.propertiesGameModeOff');
    closeGameProperties();
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));

    first.resolve(hades);

    await vi.waitFor(() => expect(vi.mocked(setGameModeOverride)).toHaveBeenCalledTimes(2));
    // The toast store folds the same words raised twice into one with a count: the count is compared too.
    await vi.waitFor(() => expect(get(toasts).map((t) => [t.kind, t.message, t.count ?? 1])).toEqual([['success', msg('modals.propertiesLateSaved', { title: 'Hades' }), 1]]));
  });
});

describe('GamePropertiesModal: a wrapper switch that fails after its dialog closed', () => {
  async function enableThenClose() {
    const enabling = held<void>();
    vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
    const dialog = await openDialog();
    await user().click(controls(dialog).compat);
    closeGameProperties();
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    return enabling;
  }

  const toastTexts = () => get(toasts).map((t) => [t.kind, t.message]);

  it('says the switch was not made, with the reason by code, not that the properties were not changed', async () => {
    const enabling = await enableThenClose();

    enabling.reject(new Error('typhon:wrappers.no_target: nothing to replace'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', msg('modals.compatLateFailed', { title: 'Hades', reason: msg('wrappers.no_target') })]]));
  });

  // Enable failed while placing files and its rollback failed too: Go keeps the entry with the
  // problem (files partly replaced), and its error may carry no code at all.
  it.each([
    [{ code: 'wrappers.recovery_failed' }, () => msg('wrappers.recovery_failed')],
    [{ code: 'wrappers.tampered', file: 'd3d9.dll' }, () => msg('modals.compatProblemFile', { file: 'd3d9.dll' })],
  ])('with a problem Go recorded (%o), says what the problem is', async (problem, reason) => {
    const enabling = await enableThenClose();
    vi.mocked(inspectWrapper).mockResolvedValue(wrapper({ problem } as Partial<WrapperInfo>));

    enabling.reject(new Error('write d3d9.dll: no space left on device\nrestore d3d9.dll: access denied'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', msg('modals.compatLateFailed', { title: 'Hades', reason: reason() })]]));
  });

  it('when the problem cannot be read, says the reason the failure carries', async () => {
    const enabling = await enableThenClose();
    vi.mocked(inspectWrapper).mockRejectedValue(new Error('typhon:wrappers.bad_state: unreadable'));

    enabling.reject(new Error('typhon:wrappers.no_target: nothing to replace'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', msg('modals.compatLateFailed', { title: 'Hades', reason: msg('wrappers.no_target') })]]));
  });
});

describe('GamePropertiesModal: presses that must change nothing', () => {
  it('a press on the held compatibility switch leaves it showing the change that runs', async () => {
    const enabling = held<void>();
    vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
    const dialog = await openDialog();
    const keys = user();
    const toggle = controls(dialog).compat;

    await keys.click(toggle);
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    await keys.click(toggle);

    expect(toggle.getAttribute('aria-checked')).toBe('true');
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);
    enabling.resolve();
    await vi.waitFor(() => expect(toggle.getAttribute('aria-disabled')).toBeNull());
  });

  it('Enter while an input method composes does not save the arguments; Enter after it does, once', async () => {
    vi.mocked(setLaunchArgsLine).mockResolvedValue(hades);
    const dialog = await openDialog();
    const keys = user();
    const field = within(dialog).getByRole('textbox', { name: msg('modals.propertiesArgsTitle') });
    await vi.waitFor(() => expect(field).toHaveProperty('disabled', false));
    await keys.type(field, 'x');

    await fireEvent.keyDown(field, { key: 'Enter', isComposing: true });
    expect(vi.mocked(setLaunchArgsLine)).not.toHaveBeenCalled();
    await fireEvent.keyDown(field, { key: 'Enter' });

    expect(vi.mocked(setLaunchArgsLine).mock.calls).toEqual([['g1', 'x']]);
    await vi.waitFor(() => expect(get(toasts).map((t) => t.message)).toEqual([msg('modals.propertiesArgsSaved')]));
  });

  it('the saves reset question asked for one game goes when the dialog is turned to another, and resets nothing', async () => {
    libraryGames.set([hades, { id: 'g2', title: 'Celeste', uninstalled: false, installDir: '/games/celeste', executable: '/games/celeste/Celeste.exe', savesDir: '/saves/celeste' } as LibraryGame]);
    const dialog = await openDialog();
    await user().click(controls(dialog).savesReset);
    const question = await screen.findByRole('dialog', { name: msg('modals.propertiesSavesResetTitle') });

    openGameProperties('g2');

    await screen.findByRole('dialog', { name: msg('modals.propertiesTitle', { title: 'Celeste' }) });
    await vi.waitFor(() => expect(question.isConnected).toBe(false));
    expect(screen.queryByRole('dialog', { name: msg('modals.propertiesSavesResetTitle') })).toBeNull();
    expect(vi.mocked(clearSavesDir)).not.toHaveBeenCalled();
  });
});
