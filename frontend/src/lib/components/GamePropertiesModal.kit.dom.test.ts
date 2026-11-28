import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { executableCandidates } from '../services/install';
import { clearSavesDir, getLaunchArgsLine, setExecutable, setHidePresence, setLaunchArgsLine, setRunAsAdmin, type LibraryGame } from '../services/library';
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
const celeste = { id: 'g2', title: 'Celeste', uninstalled: false, installDir: '/games/celeste', executable: '/games/celeste/Celeste.exe' } as LibraryGame;

function wrapper(patch: Partial<WrapperInfo> = {}): WrapperInfo {
  return { supported: true, arch: 'x86', imports: ['d3d9.dll'], suggested: 'dgvoodoo', active: null, problem: null, ...patch } as WrapperInfo;
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([hades, celeste]);
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

const because = (action: Parameters<typeof msg>[0], code: Parameters<typeof msg>[0]) => msg('common.failedReason', { action: msg(action), reason: msg(code) });

const toastTexts = () => get(toasts).map((item) => [item.kind, item.message]);

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

async function openDialog(id = 'g1', title = 'Hades') {
  openGameProperties(id);
  return screen.findByRole('dialog', { name: msg('modals.propertiesTitle', { title }) });
}

function mount() {
  render(GamePropertiesModal, { target: app });
}

describe('GamePropertiesModal: the executable choice', () => {
  it('is one radio group whose arrows only move, and Enter sets the focused file once', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setExecutable).mockReturnValueOnce(saved.promise);
    mount();
    const dialog = await openDialog();
    const keys = user();
    const group = await within(dialog).findByRole('radiogroup', { name: msg('modals.propertiesExeCandidates') });
    const [current, other] = within(group).getAllByRole('radio');
    expect(current.getAttribute('aria-checked')).toBe('true');
    expect(other.getAttribute('aria-checked')).toBe('false');
    current.focus();

    await keys.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(other);
    expect(vi.mocked(setExecutable)).not.toHaveBeenCalled();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    expect(vi.mocked(setExecutable).mock.calls).toEqual([['g1', '/games/hades/x64/Hades.exe']]);
    expect(group.getAttribute('aria-disabled')).toBe('true');
    expect(document.activeElement).toBe(other);
    saved.resolve(hades);
  });

  it('says a refused file in the dialog, announced, and raises no toast', async () => {
    vi.mocked(setExecutable).mockRejectedValueOnce(new Error('typhon:library.game_not_found: gone'));
    mount();
    const dialog = await openDialog();
    const keys = user();
    const [, other] = within(await within(dialog).findByRole('radiogroup')).getAllByRole('radio');

    await keys.click(other);

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(because('modals.propertiesExeSaveFailed', 'library.game_not_found'));
    expect(toastTexts()).toEqual([]);
  });
});

describe('GamePropertiesModal: the launch arguments', () => {
  it('are a kit field described by their hint, and a refused save is said at the field, not in a toast', async () => {
    vi.mocked(setLaunchArgsLine).mockRejectedValueOnce(new Error('typhon:library.launch_args_too_long: long'));
    mount();
    const dialog = await openDialog();
    const keys = user();
    const field = await within(dialog).findByRole('textbox', { name: msg('modals.propertiesArgsTitle') });
    await vi.waitFor(() => expect(field).toHaveProperty('disabled', false));
    expect(describedBy(field)).toContain(msg('modals.propertiesArgsHint'));

    await keys.type(field, '-windowed{Enter}');

    await vi.waitFor(() => expect(field.getAttribute('aria-invalid')).toBe('true'));
    expect(describedBy(field)).toContain(because('modals.propertiesArgsSaveFailed', 'library.launch_args_too_long'));
    expect(toastTexts()).toEqual([]);
  });
});

describe('GamePropertiesModal: a setting refused while the dialog is open', () => {
  it.each([
    ['admin', () => vi.mocked(setRunAsAdmin), msg('modals.propertiesAdminTitle'), 'modals.propertiesAdminFailed'],
    ['presence', () => vi.mocked(setHidePresence), msg('modals.propertiesPresenceTitle'), 'modals.propertiesPresenceFailed'],
  ] as const)('%s: is said in the dialog, announced, the switch keeps the focus while it is held, and no toast is raised', async (_name, call, name, failed) => {
    const answer = held<LibraryGame>();
    call().mockReturnValueOnce(answer.promise);
    mount();
    const dialog = await openDialog();
    const keys = user();
    const toggle = await within(dialog).findByRole('switch', { name });
    toggle.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(call()).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(toggle);
    expect(toggle.getAttribute('aria-disabled')).toBe('true');
    answer.reject(new Error('disk'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg(failed));
    expect(document.activeElement).toBe(toggle);
    expect(toastTexts()).toEqual([]);
  });
});

describe('GamePropertiesModal while compatibility mode turns on', () => {
  it('cannot be closed, and the pressed switch keeps the focus, held, until Go answers', async () => {
    const enabling = held<void>();
    vi.mocked(enableWrapper).mockReturnValueOnce(enabling.promise);
    mount();
    const dialog = await openDialog();
    const keys = user();
    const toggle = await within(dialog).findByRole('switch', { name: msg('modals.compatToggle') });
    toggle.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Escape}');

    expect(dialog.isConnected).toBe(true);
    expect(within(dialog).getByRole('button', { name: msg('common.close') })).toHaveProperty('disabled', true);
    expect(document.activeElement).toBe(toggle);
    expect(toggle.getAttribute('aria-disabled')).toBe('true');
    // Chromium drops the focus of a control that turns disabled; jsdom does not.
    expect(toggle).toHaveProperty('disabled', false);
    expect(vi.mocked(enableWrapper)).toHaveBeenCalledTimes(1);
    enabling.resolve();
    await vi.waitFor(() => expect(toggle.getAttribute('aria-disabled')).toBeNull());
  });
});

describe('GamePropertiesModal: resetting the saves folder', () => {
  async function askReset() {
    mount();
    const dialog = await openDialog();
    const keys = user();
    await keys.click(await within(dialog).findByRole('button', { name: msg('modals.propertiesSavesReset') }));
    const question = await screen.findByRole('dialog', { name: msg('modals.propertiesSavesResetTitle') });
    return { dialog, question, keys };
  }

  it('asks first, saying what is forgotten and that no file is deleted, with Cancel focused; Cancel sends nothing', async () => {
    const { question, keys } = await askReset();

    expect(question.textContent).toContain(msg('modals.propertiesSavesResetText', { path: '/saves/hades', title: 'Hades' }));
    expect(question.textContent).toContain(msg('modals.propertiesSavesResetNote'));
    expect(document.activeElement).toBe(within(question).getByRole('button', { name: msg('common.cancel') }));

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(question.isConnected).toBe(false));
    expect(vi.mocked(clearSavesDir)).not.toHaveBeenCalled();
  });

  it('confirmed, resets the folder of the game it was asked for, once per press, and the dialog cannot be closed meanwhile', async () => {
    const clearing = held<LibraryGame>();
    vi.mocked(clearSavesDir).mockReturnValueOnce(clearing.promise);
    const { dialog, question, keys } = await askReset();
    const confirm = within(question).getByRole('button', { name: msg('modals.propertiesSavesResetConfirm') });

    await keys.click(confirm);
    await keys.click(confirm);
    await keys.keyboard('{Escape}');

    expect(vi.mocked(clearSavesDir).mock.calls).toEqual([['g1']]);
    expect(question.isConnected).toBe(true);
    expect(dialog.isConnected).toBe(true);
    clearing.resolve({ ...hades, savesDir: '' });
    await vi.waitFor(() => expect(question.isConnected).toBe(false));
  });

  it('refused, says why in the question by code, and raises no toast', async () => {
    vi.mocked(clearSavesDir).mockRejectedValueOnce(new Error('typhon:library.game_not_found: gone'));
    const { question, keys } = await askReset();

    await keys.click(within(question).getByRole('button', { name: msg('modals.propertiesSavesResetConfirm') }));

    expect((await within(question).findByRole('alert')).textContent?.trim()).toBe(because('modals.propertiesSavesFailed', 'library.game_not_found'));
    expect(toastTexts()).toEqual([]);
  });
});

describe('GamePropertiesModal: an answer that comes after the dialog was closed or opened again', () => {
  it('for a refused file: paints nothing in the new dialog and is said once in a toast naming the game, after the new data', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setExecutable).mockReturnValueOnce(saved.promise);
    mount();
    const first = await openDialog();
    const keys = user();
    const [, other] = within(await within(first).findByRole('radiogroup')).getAllByRole('radio');
    await keys.click(other);
    await keys.keyboard('{Escape}');
    const dialog = await openDialog('g2', 'Celeste');
    await within(dialog).findByRole('radiogroup');

    saved.reject(new Error('typhon:library.game_not_found: gone'));

    await vi.waitFor(() => expect(toastTexts()).toEqual([['danger', msg('modals.propertiesLateFailed', { title: 'Hades', reason: because('modals.propertiesExeSaveFailed', 'library.game_not_found') })]]));
    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(within(dialog).queryByText(because('modals.propertiesExeSaveFailed', 'library.game_not_found'))).toBeNull();
  });

  it('for saved arguments: one success toast naming the game, and the reopened field keeps what it read', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setLaunchArgsLine).mockReturnValueOnce(saved.promise);
    mount();
    const first = await openDialog();
    const keys = user();
    const field = await within(first).findByRole('textbox', { name: msg('modals.propertiesArgsTitle') });
    await vi.waitFor(() => expect(field).toHaveProperty('disabled', false));
    await keys.type(field, '-windowed{Enter}');
    await keys.keyboard('{Escape}');
    vi.mocked(getLaunchArgsLine).mockResolvedValueOnce('-dx11');
    const dialog = await openDialog();
    const again = await within(dialog).findByRole('textbox', { name: msg('modals.propertiesArgsTitle') });
    await vi.waitFor(() => expect((again as HTMLInputElement).value).toBe('-dx11'));

    saved.resolve(hades);

    await vi.waitFor(() => expect(toastTexts()).toEqual([['success', msg('modals.propertiesLateSaved', { title: 'Hades' })]]));
    expect((again as HTMLInputElement).value).toBe('-dx11');
    expect(within(dialog).getByRole('button', { name: msg('modals.propertiesArgsSave') }).getAttribute('aria-busy')).toBeNull();
  });
});
