import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { inspectRemoval, removeGame, type RemovalInfo } from '../services/install';
import { addToSteam, locateSaves, type LibraryGame } from '../services/library';
import { closeGameMenu } from '../stores/gameMenu';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import GameMenuHarness from '../testing/GameMenuHarness.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  steamStatus: vi.fn(async () => ({ supported: true, installed: true, added: false })),
  addToSteam: vi.fn(async () => {}),
  locateSaves: vi.fn(),
}));
vi.mock('../services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/install')>()),
  inspectRemoval: vi.fn(),
  removeGame: vi.fn(),
}));

const hades = { id: 'g1', title: 'Hades', uninstalled: false, executable: 'Hades.exe' } as LibraryGame;
const celeste = { id: 'g2', title: 'Celeste', uninstalled: false, executable: 'Celeste.exe' } as LibraryGame;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([hades]);
});

afterEach(() => {
  closeGameMenu();
  libraryGames.set([]);
  toasts.set([]);
  app.remove();
});

describe('a confirmation opened from the game context menu', () => {
  it('returns focus to the game the menu was opened for, not to the page body, once it closes', async () => {
    render(GameMenuHarness, { target: app });
    const keys = user();
    const game = screen.getByRole('link', { name: 'Hades' });
    await fireEvent.contextMenu(game);

    await keys.click(await screen.findByRole('menuitem', { name: msg('games.actionSteamAdd') }));
    const dialog = screen.getByRole('dialog', { name: msg('games.steamAddTitle') });
    expect(dialog.contains(document.activeElement)).toBe(true);
    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(addToSteam)).toHaveBeenCalledWith('g1');
    expect(document.activeElement).toBe(game);
  });
});

async function pick(game: string, item: string | RegExp) {
  const keys = user();
  const link = screen.getByRole('link', { name: game });
  await fireEvent.contextMenu(link);
  await keys.click(await screen.findByRole('menuitem', { name: item }));
  return { keys, link };
}

describe('dialogs opened from the game context menu return focus to the game', () => {
  it('the status dialog: to the game, not to <body>', async () => {
    render(GameMenuHarness, { target: app });
    const { keys, link } = await pick('Hades', new RegExp(msg('games.actionStatus', { status: '' }).trim().split(':')[0]));
    expect(await screen.findByRole('dialog', { name: msg('modals.gameStatusTitle') })).toBeTruthy();

    await keys.keyboard('{Escape}');

    expect(document.activeElement).toBe(link);
  });

  it('the tags dialog: to the game', async () => {
    render(GameMenuHarness, { target: app });
    const { keys, link } = await pick('Hades', msg('games.actionTags'));
    expect(await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') })).toBeTruthy();

    await keys.keyboard('{Escape}');

    expect(document.activeElement).toBe(link);
  });

  it('the save-folder choice: to the game', async () => {
    vi.mocked(locateSaves).mockResolvedValueOnce({ path: '', candidates: ['C:/Saves/A', 'C:/Saves/B'], unreadable: 0 });
    render(GameMenuHarness, { target: app });
    const { keys, link } = await pick('Hades', msg('games.actionOpenSaves'));
    expect(await screen.findByRole('dialog', { name: msg('ui.savesDirTitle') })).toBeTruthy();

    await keys.keyboard('{Escape}');

    expect(document.activeElement).toBe(link);
  });

  describe('the remove dialog', () => {
    const info = { gameId: 'g1', title: 'Hades', installDir: '', installType: '', method: 'none', owned: false, uninstallUnknown: false, sizeBytes: 0, sizeUnknown: true, dirMissing: false, quietUninstall: false, running: false, busy: false, downloadId: '' } as unknown as RemovalInfo;

    async function remove() {
      vi.mocked(inspectRemoval).mockResolvedValueOnce(info);
      vi.mocked(removeGame).mockImplementationOnce(async () => {
        libraryGames.update((list) => list.filter((game) => game.id !== 'g1'));
      });
      render(GameMenuHarness, { target: app });
      const picked = await pick('Hades', msg('games.actionRemoveLibrary'));
      const dialog = await screen.findByRole('dialog', { name: msg('modals.removeGameTitle') });
      return { ...picked, dialog };
    }

    it('cancelled: to the game', async () => {
      const { keys, link } = await remove();

      await keys.keyboard('{Escape}');

      expect(document.activeElement).toBe(link);
    });

    it('removed: to the next game in the list', async () => {
      libraryGames.set([hades, celeste]);
      const { keys, dialog } = await remove();

      const confirm = (await within(dialog).findAllByRole('button', { name: msg('modals.removeGameModeLibraryLabel') })).at(-1) as HTMLElement;
      await keys.click(confirm);

      await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
      expect(document.activeElement).toBe(screen.getByRole('link', { name: 'Celeste' }));
    });

    it('removed, and it was the only game: to the page heading', async () => {
      const { keys, dialog } = await remove();

      const confirm = (await within(dialog).findAllByRole('button', { name: msg('modals.removeGameModeLibraryLabel') })).at(-1) as HTMLElement;
      await keys.click(confirm);

      await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
      expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' }));
    });
  });
});
