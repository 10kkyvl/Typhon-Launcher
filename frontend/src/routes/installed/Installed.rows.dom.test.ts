import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { platformBadge } from '../../lib/library/emulation';
import { compatStatuses, type CompatStatus } from '../../lib/services/compat';
import { selectImageFile } from '../../lib/services/emulators';
import { playGame, selectGameExecutable, setExecutable, stopGame, type LibraryGame } from '../../lib/services/library';
import { openGameFolder } from '../../lib/services/settings';
import { closeGameMenu, gameMenu } from '../../lib/stores/gameMenu';
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { missingImages, refreshMissingImages } from '../../lib/stores/missingImages';
import { route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

// What a row of each kind shows and what its controls call, in the list and in the grid: the
// running game, the one waiting for its image or its executable, an emulated game, an installed
// disc image, a game of another launcher, and each kind's menu.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  playGame: vi.fn(async () => {}),
  stopGame: vi.fn(async () => {}),
  selectGameExecutable: vi.fn(async () => ''),
  setExecutable: vi.fn(async () => ({})),
}));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  openGameFolder: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/emulators')>()),
  selectImageFile: vi.fn(async () => ''),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

function game(id: string, title: string, extra: Partial<LibraryGame> = {}): LibraryGame {
  return {
    id,
    title,
    installDir: `C:\\Games\\${title}`,
    executable: `C:\\Games\\${title}\\game.exe`,
    sizeBytes: 0,
    uninstalled: false,
    tags: [],
    lastPlayed: null,
    ...extra,
  } as unknown as LibraryGame;
}

const normal = game('g-hades', 'Hades');
const external = game('g-portal', 'Portal', { executable: '', external: { provider: 'steam', appId: '400', launchUri: 'steam://run/400' } });
const emulated = game('g-mario', 'Mario', { executable: 'D:\\Roms\\mario.iso', emulation: { platform: 'ps2' } });
const image = game('g-gt', 'Gran Turismo', { executable: 'D:\\Lib\\gt.iso', emulation: { platform: 'ps2' }, owned: true, installType: 'console_image' });
const noExe = game('g-celeste', 'Celeste', { executable: '' });

const toastTexts = () => get(toasts).map((item) => `${item.kind}:${item.message}`);
const menuButton = (title: string) => screen.getByRole('button', { name: msg('games.installedMenuFor', { title }) });
const rowOf = (title: string) => menuButton(title).closest('.row') as HTMLElement;

beforeEach(() => {
  libraryLoaded.set(true);
  installedView.set('list');
});

afterEach(() => {
  cleanup();
  libraryGames.set([]);
  libraryLoaded.set(false);
  runningGames.set(new Set());
  missingImages.set(new Set());
  installedView.set('list');
  toasts.set([]);
  closeGameMenu();
  vi.clearAllMocks();
  document.body.replaceChildren();
});

describe('Installed list: a row in each state', () => {
  it('a game that can start offers Play, which starts it', async () => {
    libraryGames.set([normal]);
    const { user } = renderDom(Installed);

    await user.click(within(rowOf('Hades')).getByRole('button', { name: msg('games.play') }));

    expect(playGame).toHaveBeenCalledWith('g-hades');
  });

  it('a running game says so and offers Stop, which stops it', async () => {
    libraryGames.set([normal]);
    runningGames.set(new Set(['g-hades']));
    const { user } = renderDom(Installed);
    const row = rowOf('Hades');

    expect(row.textContent).toContain(msg('games.runningLabel'));
    expect(within(row).queryByRole('button', { name: msg('games.play') })).toBeNull();
    await user.click(within(row).getByRole('button', { name: msg('games.installedStopButton') }));

    expect(stopGame).toHaveBeenCalledWith('g-hades');
  });

  it('a game without an executable says so and offers to set one, which saves the file picked', async () => {
    libraryGames.set([noExe]);
    vi.mocked(selectGameExecutable).mockResolvedValue('C:\\Games\\Celeste\\Celeste.exe');
    const { user } = renderDom(Installed);
    const row = rowOf('Celeste');

    expect(row.textContent).toContain(msg('games.installedNeedsExeLabel'));
    await user.click(within(row).getByRole('button', { name: msg('games.installedSetExeButton') }));

    expect(selectGameExecutable).toHaveBeenCalledWith(msg('games.installedChooseExeDialog', { title: 'Celeste' }), 'C:\\Games\\Celeste', '');
    await vi.waitFor(() => expect(setExecutable).toHaveBeenCalledWith('g-celeste', 'C:\\Games\\Celeste\\Celeste.exe'));
    await vi.waitFor(() => expect(toastTexts()).toContain(`success:${msg('games.installedExeSavedToast', { title: 'Celeste' })}`));
  });

  it('an emulated game whose image is missing says so and offers to choose it, which saves it', async () => {
    libraryGames.set([emulated]);
    missingImages.set(new Set(['g-mario']));
    vi.mocked(selectImageFile).mockResolvedValue('D:\\Roms\\mario2.iso');
    const { user } = renderDom(Installed);
    const row = rowOf('Mario');

    expect(row.textContent).toContain(msg('emul.imageMissing'));
    expect(row.textContent).toContain(platformBadge('ps2'));
    await user.click(within(row).getByRole('button', { name: msg('emul.pickImage') }));

    expect(selectImageFile).toHaveBeenCalledWith(msg('emul.addImageDialog'), ['ps2']);
    await vi.waitFor(() => expect(setExecutable).toHaveBeenCalledWith('g-mario', 'D:\\Roms\\mario2.iso'));
    await vi.waitFor(() => expect(toastTexts()).toContain(`success:${msg('emul.imageSaved')}`));
    expect(refreshMissingImages).toHaveBeenCalled();
  });

  it('a game the compatibility log calls broken carries the broken badge', async () => {
    libraryGames.set([normal]);
    vi.mocked(compatStatuses).mockResolvedValue(
      new Map([['g-hades', { gameId: 'g-hades', state: 'broken', attempts: 3, failures: 3, bestSeconds: 0 } as CompatStatus]]),
    );
    renderDom(Installed);

    await vi.waitFor(() => expect(rowOf('Hades').textContent).toContain(msg('games.compatBrokenBadge')));
  });

  it('the size is said when it is known', () => {
    libraryGames.set([game('g-big', 'Big', { sizeBytes: 5 * 1024 ** 3 })]);
    renderDom(Installed);

    expect(rowOf('Big').querySelector('.size')?.textContent?.trim()).not.toBe('');
  });

  it('the row opens the game’s page', async () => {
    libraryGames.set([normal]);
    const { user } = renderDom(Installed);

    await user.click(within(rowOf('Hades')).getByText('Hades'));

    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-hades' } });
  });

  it('Open folder opens the game’s folder, and says when it cannot', async () => {
    libraryGames.set([normal]);
    const { user } = renderDom(Installed);

    await user.click(within(rowOf('Hades')).getByRole('button', { name: msg('games.openFolder') }));
    expect(openGameFolder).toHaveBeenCalledWith('C:\\Games\\Hades', 'C:\\Games\\Hades\\game.exe');

    vi.mocked(openGameFolder).mockRejectedValueOnce(new Error('gone'));
    await user.click(within(rowOf('Hades')).getByRole('button', { name: msg('games.openFolder') }));
    await vi.waitFor(() => expect(toastTexts()).toContain(`danger:${msg('games.errorFolderUnavailable')}`));
  });
});

describe('Installed list: the row menu of each kind', () => {
  const items = () => screen.getAllByRole('menuitem').map((item) => item.textContent?.trim());

  it.each([
    ['a game', normal, ['games.openFolder', 'games.installedChooseExeLabel', 'games.actionMove', 'games.actionUninstall', 'games.actionRemoveLibrary']],
    ['an emulated game', emulated, ['games.openFolder', 'games.actionRemoveLibrary']],
    ['an installed disc image', image, ['games.openFolder', 'games.actionUninstall', 'games.actionRemoveLibrary']],
    ['a game of another launcher', external, ['games.openFolder', 'games.actionRemoveLibrary']],
  ] as const)('%s', async (_name, one, keys) => {
    libraryGames.set([one]);
    const { user } = renderDom(Installed);

    await user.click(menuButton(one.title));

    expect(items()).toEqual(keys.map((key) => msg(key)));
  });

  it('its folder item opens the folder, its executable item asks for the file', async () => {
    libraryGames.set([normal]);
    const { user } = renderDom(Installed);

    await user.click(menuButton('Hades'));
    await user.click(screen.getByRole('menuitem', { name: msg('games.openFolder') }));
    await vi.waitFor(() => expect(openGameFolder).toHaveBeenCalledWith('C:\\Games\\Hades', 'C:\\Games\\Hades\\game.exe'));

    await user.click(menuButton('Hades'));
    await user.click(screen.getByRole('menuitem', { name: msg('games.installedChooseExeLabel') }));
    await vi.waitFor(() =>
      expect(selectGameExecutable).toHaveBeenCalledWith(msg('games.installedChooseExeDialog', { title: 'Hades' }), 'C:\\Games\\Hades', 'C:\\Games\\Hades\\game.exe'),
    );
  });
});

describe('Installed grid: a card in each state', () => {
  beforeEach(() => installedView.set('grid'));

  const card = (title: string) => screen.getByRole('button', { name: title }).closest('.card') as HTMLElement;

  it('a running game offers Stop on its cover, which stops it', async () => {
    libraryGames.set([normal]);
    runningGames.set(new Set(['g-hades']));
    const { user } = renderDom(Installed);

    expect(within(card('Hades')).queryByRole('button', { name: msg('games.play') })).toBeNull();
    await user.click(within(card('Hades')).getByRole('button', { name: msg('games.installedStopButton') }));

    expect(stopGame).toHaveBeenCalledWith('g-hades');
  });

  it('a game waiting for its image or its executable has no Play, the image one says why', () => {
    libraryGames.set([emulated, noExe]);
    missingImages.set(new Set(['g-mario']));
    renderDom(Installed);

    expect(screen.queryAllByRole('button', { name: msg('games.play') })).toEqual([]);
    expect(card('Mario').textContent).toContain(msg('emul.imageMissing'));
    expect(card('Mario').textContent).toContain(platformBadge('ps2'));
  });

  it('the cover opens the game’s page', async () => {
    libraryGames.set([normal]);
    const { user } = renderDom(Installed);

    await user.click(screen.getByRole('button', { name: 'Hades' }));

    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-hades' } });
  });

  it('Shift+F10 on a card’s cover opens the game menu for that game: the grid’s actions by keyboard', async () => {
    libraryGames.set([normal, noExe]);
    const { user } = renderDom(Installed);
    const cover = screen.getByRole('button', { name: 'Celeste' });
    cover.focus();

    await user.keyboard('{Shift>}{F10}{/Shift}');

    expect(get(gameMenu)).toMatchObject({ gameId: 'g-celeste', focus: 'first' });
  });
});
