import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg, type MessageKey } from '../i18n';
import { propertiesErrorText } from '../game/propertiesErrors';
import { executableCandidates } from '../services/install';
import { getLaunchArgsLine, setGameModeOverride, type LibraryGame } from '../services/library';
import { appInfo } from '../services/system';
import { inspectWrapper, type WrapperInfo } from '../services/wrappers';
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

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([hades]);
  // A queued answer one test left unused must not reach the next.
  vi.mocked(setGameModeOverride).mockReset();
  vi.mocked(executableCandidates).mockResolvedValue([{ path: '/games/hades/Hades.exe', score: 1 }]);
  vi.mocked(getLaunchArgsLine).mockResolvedValue('');
  vi.mocked(appInfo).mockResolvedValue({ version: '2.0.0', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' });
  vi.mocked(inspectWrapper).mockResolvedValue({ supported: true, arch: 'x86', imports: ['d3d9.dll'], suggested: 'dgvoodoo', active: null, problem: null } as WrapperInfo);
  render(GamePropertiesModal, { target: app });
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

async function openDialog() {
  openGameProperties('g1');
  const dialog = await screen.findByRole('dialog', { name: msg('modals.propertiesTitle', { title: 'Hades' }) });
  await within(dialog).findByRole('combobox', { name: msg('settings.gameModePowerLabel') });
  return dialog;
}

async function closeDialog(dialog: HTMLElement) {
  closeGameProperties();
  await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
}

async function chooseIn(dialog: HTMLElement, label: MessageKey, option: MessageKey) {
  const keys = user();
  const select = within(dialog).getByRole('combobox', { name: msg(label) });
  await keys.click(select);
  await keys.click(await screen.findByRole('option', { name: msg(option) }));
  return select;
}

/** Three changes, the first held: the other two wait behind it. */
async function queueThree() {
  const dialog = await openDialog();
  await chooseIn(dialog, 'settings.gameModePowerLabel', 'modals.propertiesGameModeOn');
  await chooseIn(dialog, 'settings.gameModeHideLabel', 'modals.propertiesGameModeOff');
  await chooseIn(dialog, 'settings.gameModePriorityLabel', 'modals.propertiesGameModeOff');
  return dialog;
}

// The toast store folds the same words raised twice into one with a count: compare the count too.
const toastList = () => get(toasts).map((t) => ({ kind: t.kind, message: t.message, count: t.count ?? 1 }));

const saved = { kind: 'success', message: msg('modals.propertiesLateSaved', { title: 'Hades' }), count: 1 };

function refused(rows: MessageKey[], reason = msg('modals.propertiesGameModeFailed')) {
  const names = rows.map((row) => msg(row)).join(msg('modals.propertiesGameModeLateSeparator'));
  return { kind: 'danger', message: msg('modals.propertiesGameModeLateRefused', { title: 'Hades', rows: names, reason }), count: 1 };
}

describe('GamePropertiesModal: the game mode changes of a closed opening are summed up once', () => {
  it('all stored after the dialog closed: one saved toast, raised once', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockResolvedValueOnce(hades).mockResolvedValueOnce(hades);
    const dialog = await queueThree();
    await closeDialog(dialog);

    first.resolve(hades);

    await vi.waitFor(() => expect(vi.mocked(setGameModeOverride)).toHaveBeenCalledTimes(3));
    await vi.waitFor(() => expect(toastList()).toEqual([saved]));
  });

  it('the last refused: one toast names that row and its reason, and does not say nothing changed', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockResolvedValueOnce(hades).mockRejectedValueOnce(new Error('disk'));
    const dialog = await queueThree();
    await closeDialog(dialog);

    first.resolve(hades);

    await vi.waitFor(() => expect(toastList()).toEqual([refused(['settings.gameModePriorityLabel'])]));
  });

  it('the middle one refused: one toast once the last is answered, not a refusal and a saved one', async () => {
    const first = held<LibraryGame>();
    const last = held<LibraryGame>();
    const gone = new Error('typhon:library.game_not_found: g1');
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockRejectedValueOnce(gone).mockReturnValueOnce(last.promise);
    const dialog = await queueThree();
    await closeDialog(dialog);

    first.resolve(hades);
    await vi.waitFor(() => expect(vi.mocked(setGameModeOverride)).toHaveBeenCalledTimes(3));
    expect(toastList()).toEqual([]);
    last.resolve(hades);

    const reason = propertiesErrorText(gone, msg('modals.propertiesGameModeFailed'));
    await vi.waitFor(() => expect(toastList()).toEqual([refused(['settings.gameModeHideLabel'], reason)]));
  });

  it('several refused: their rows are listed in one toast', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockRejectedValueOnce(new Error('disk')).mockRejectedValueOnce(new Error('disk'));
    const dialog = await queueThree();
    await closeDialog(dialog);

    first.resolve(hades);

    await vi.waitFor(() => expect(toastList()).toEqual([refused(['settings.gameModeHideLabel', 'settings.gameModePriorityLabel'])]));
  });

  // A reason may hold a semicolon of its own (the library's unreadable state in Russian): each row is
  // its own sentence with its own reason, and what failed is said once, at the start.
  it.each([
    [
      'ru',
      '«Hades»: не сохранено. Схема питания «Высокая производительность»: файлы библиотеки не удалось прочитать, изменение не сохранено; перезапустите лаунчер. Скрывать лаунчер в трей: игра не найдена. Повышать приоритет игры: причина неизвестна.',
    ],
    [
      'en',
      '"Hades": not saved. “High performance” power plan: The library files could not be read, so the change was not saved. Restart the launcher. Hide the launcher in the tray: Game not found. Raise game priority: reason unknown.',
    ],
  ] as const)('%s: refused for different reasons, each row is said with its own reason', async (lang, message) => {
    locale.set(lang);
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride)
      .mockReturnValueOnce(first.promise)
      .mockRejectedValueOnce(new Error('typhon:library.game_not_found: g1'))
      .mockRejectedValueOnce(new Error('disk'));
    const dialog = await queueThree();
    await closeDialog(dialog);

    first.reject(new Error('typhon:library.state_unreadable: library.json'));

    await vi.waitFor(() => expect(toastList()).toEqual([{ kind: 'danger', message, count: 1 }]));
  });

  it('a row changed twice, refused first and stored last: it holds the last, so all is saved', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockRejectedValueOnce(new Error('disk')).mockResolvedValueOnce(hades);
    const dialog = await openDialog();
    await chooseIn(dialog, 'settings.gameModePowerLabel', 'modals.propertiesGameModeOn');
    await chooseIn(dialog, 'settings.gameModeHideLabel', 'modals.propertiesGameModeOff');
    await chooseIn(dialog, 'settings.gameModeHideLabel', 'modals.propertiesGameModeOn');
    await closeDialog(dialog);

    first.resolve(hades);

    await vi.waitFor(() => expect(vi.mocked(setGameModeOverride)).toHaveBeenCalledTimes(3));
    await vi.waitFor(() => expect(toastList()).toEqual([saved]));
  });

  // Counted by game, the closed opening's change would wait for the reopened dialog's and never be said.
  it("a closed opening's change stored while the reopened dialog of the game has one queued is said", async () => {
    const first = held<LibraryGame>();
    const second = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const dialog = await openDialog();
    await chooseIn(dialog, 'settings.gameModePowerLabel', 'modals.propertiesGameModeOn');
    await closeDialog(dialog);
    const again = await openDialog();
    await chooseIn(again, 'settings.gameModeHideLabel', 'modals.propertiesGameModeOff');

    first.resolve(hades);

    await vi.waitFor(() => expect(toastList()).toEqual([saved]));
    await vi.waitFor(() => expect(vi.mocked(setGameModeOverride)).toHaveBeenCalledTimes(2));
    second.resolve(hades);
    await vi.waitFor(() => expect(within(again).getByRole('combobox', { name: msg('settings.gameModeHideLabel') }).textContent).toContain(msg('modals.propertiesGameModeOff')));
    expect(toastList()).toEqual([saved]);
  });

  it('all answered while the dialog is open: no toast', async () => {
    const first = held<LibraryGame>();
    vi.mocked(setGameModeOverride).mockReturnValueOnce(first.promise).mockResolvedValueOnce(hades).mockResolvedValueOnce(hades);
    const dialog = await queueThree();

    first.resolve(hades);

    await vi.waitFor(() => expect(vi.mocked(setGameModeOverride)).toHaveBeenCalledTimes(3));
    // A macrotask runs once every answer's microtasks are done.
    await new Promise((done) => setTimeout(done, 0));
    await closeDialog(dialog);
    expect(toastList()).toEqual([]);
  });
});
