// First: services/library reaches this store back through stores/cloudsaves, and a store loaded
// inside that cycle would keep the real service instead of the mock below.
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { inspectRemoval, removeGame } from '../../lib/services/install';
import { addGame, selectExecutable, type LibraryGame } from '../../lib/services/library';
import { proposeLibraryPath, selectFolder, setupLibrary } from '../../lib/services/settings';
import { getStorageInfo, getStorageInfoFor, type StorageInfo } from '../../lib/services/system';
import { settings } from '../../lib/stores/settings';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

// Where focus goes when a row leaves (a removal here or from the game menu, a game gone by Go's
// doing): the row that took its place, else the row above, else the header's first control, else the
// page heading; and the storage read again after a removal. The add dialog hands focus to the game
// it added when its opener has gone.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
const backend = vi.hoisted(() => ({ inWails: false }));
vi.mock('../../lib/services/backend', () => backend);
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/install')>()),
  inspectRemoval: vi.fn(),
  removeGame: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  addGame: vi.fn(),
  selectExecutable: vi.fn(async () => ''),
}));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getStorageInfo: vi.fn(() => new Promise(() => {})),
  getStorageInfoFor: vi.fn(),
}));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  selectFolder: vi.fn(),
  proposeLibraryPath: vi.fn(),
  setupLibrary: vi.fn(),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

function game(id: string, title: string, lastPlayed: string | null): LibraryGame {
  return { id, title, installDir: `C:\\Games\\${title}`, executable: `C:\\Games\\${title}\\game.exe`, sizeBytes: 0, uninstalled: false, tags: [], lastPlayed } as unknown as LibraryGame;
}

// Sorted by last played: Hades, Celeste, Portal.
const hades = game('g-hades', 'Hades', '2026-11-03T10:00:00Z');
const celeste = game('g-celeste', 'Celeste', '2026-11-02T10:00:00Z');
const portal = game('g-portal', 'Portal', '2026-11-01T10:00:00Z');

const focusedRow = () => document.activeElement?.closest('[data-game-row]')?.getAttribute('data-game-row');
const menu = (title: string) => screen.getByRole('button', { name: msg('games.installedMenuFor', { title }) });

beforeEach(() => {
  libraryLoaded.set(true);
  installedView.set('list');
  settings.set({ libraryPath: 'C:\\Games' } as NonNullable<Parameters<typeof settings.set>[0]>);
  vi.mocked(inspectRemoval).mockResolvedValue({ owned: true, dirMissing: false, method: 'files', running: false, busy: false } as Awaited<ReturnType<typeof inspectRemoval>>);
});

afterEach(() => {
  cleanup();
  libraryGames.set([]);
  libraryLoaded.set(false);
  settings.set(null);
  installedView.set('list');
  backend.inWails = false;
  vi.clearAllMocks();
  document.body.replaceChildren();
});

async function removeFromDisk(title: string, user: ReturnType<typeof renderDom>['user']) {
  menu(title).focus();
  await user.keyboard('{Enter}');
  await user.click(screen.getByRole('menuitem', { name: msg('games.actionUninstall') }));
  const dialog = await screen.findByRole('dialog');
  const confirm = await within(dialog).findByRole('button', { name: msg('modals.removeGameModeDiskLabel') });
  await vi.waitFor(() => expect(confirm.hasAttribute('disabled')).toBe(false));
  await user.click(confirm);
  await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
}

describe('Installed: removing a game from its row', () => {
  it('reads the storage again once the game is removed', async () => {
    libraryGames.set([hades, celeste, portal]);
    const { user } = renderDom(Installed);
    const before = vi.mocked(getStorageInfo).mock.calls.length;

    await removeFromDisk('Celeste', user);

    expect(removeGame).toHaveBeenCalledWith('g-celeste', expect.objectContaining({ keepInLibrary: true }));
    expect(vi.mocked(getStorageInfo).mock.calls.length).toBeGreaterThan(before);
  });

  it('when the row leaves, focus goes to the row that took its place', async () => {
    libraryGames.set([hades, celeste, portal]);
    const { user } = renderDom(Installed);

    await removeFromDisk('Celeste', user);
    libraryGames.set([hades, portal]);
    await tick();

    expect(focusedRow()).toBe('g-portal');
  });

  it('the last row gone, focus goes to the row above', async () => {
    libraryGames.set([hades, celeste, portal]);
    const { user } = renderDom(Installed);

    await removeFromDisk('Portal', user);
    libraryGames.set([hades, celeste]);
    await tick();

    expect(focusedRow()).toBe('g-celeste');
  });

  it('the only row gone, focus goes to the header’s first control', async () => {
    libraryGames.set([celeste]);
    const { user } = renderDom(Installed);

    await removeFromDisk('Celeste', user);
    libraryGames.set([]);
    await tick();

    expect(document.activeElement).toBe(screen.getByRole('textbox', { name: msg('games.installedSearchPlaceholder') }));
  });

  it('a row already gone when the dialog closes hands focus by the same rule', async () => {
    libraryGames.set([hades, celeste, portal]);
    vi.mocked(removeGame).mockImplementationOnce(async () => {
      libraryGames.set([hades, portal]);
    });
    const { user } = renderDom(Installed);

    await removeFromDisk('Celeste', user);
    await tick();

    expect(focusedRow()).toBe('g-portal');
  });
});

describe('Installed: a card that leaves the grid', () => {
  it('hands focus to the card that took its place', async () => {
    installedView.set('grid');
    libraryGames.set([hades, celeste, portal]);
    renderDom(Installed);
    screen.getByRole('button', { name: 'Celeste' }).focus();

    libraryGames.set([hades, portal]);
    await tick();

    expect(focusedRow()).toBe('g-portal');
  });
});

describe('Installed: adding a game from the empty screen', () => {
  it('hands focus to the game added once the list shows it, its opener being gone', async () => {
    libraryGames.set([]);
    const added = game('g-new', 'New', null);
    vi.mocked(addGame).mockImplementation(async () => {
      libraryGames.set([added]);
      return added;
    });
    const { user } = renderDom(Installed);
    backend.inWails = true;
    const empty = screen.getByRole('heading', { name: msg('games.installedNoGamesTitle') }).closest('.empty') as HTMLElement;
    await user.click(within(empty).getByRole('button', { name: msg('games.installedAddGameButton') }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByRole('textbox', { name: msg('games.executableLabel') }), 'C:\\Games\\New\\new.exe');

    await user.click(within(dialog).getByRole('button', { name: msg('common.add') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(focusedRow()).toBe('g-new'));
    expect(selectExecutable).not.toHaveBeenCalled();
  });
});

describe('Installed: choosing the library folder', () => {
  it('hands focus to the storage card once the new library’s disk is read, its opener being gone', async () => {
    type Next = NonNullable<Parameters<typeof settings.set>[0]>;
    const info: StorageInfo = { path: 'D:\\Typhon', volume: 'D:', filesystem: 'NTFS', totalBytes: 100, freeBytes: 40, usedBytes: 60 };
    settings.set({ libraryPath: '' } as Next);
    vi.mocked(selectFolder).mockResolvedValue('D:\\');
    vi.mocked(proposeLibraryPath).mockResolvedValue('D:\\Typhon');
    vi.mocked(getStorageInfoFor).mockResolvedValue(info);
    vi.mocked(setupLibrary).mockResolvedValue({ libraryPath: 'D:\\Typhon' } as Next);
    vi.mocked(getStorageInfo).mockResolvedValue(info);
    const { user } = renderDom(Installed);

    await user.click(screen.getByRole('button', { name: msg('games.installedChooseFolderButton') }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('modals.librarySetupBrowse') }));
    const create = within(dialog).getByRole('button', { name: msg('modals.librarySetupCreate') });
    await vi.waitFor(() => expect(create.hasAttribute('disabled')).toBe(false));
    await user.click(create);

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('games.installedManageStorage') })));
  });
});
