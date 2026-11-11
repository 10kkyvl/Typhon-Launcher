import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { addToSteam, type LibraryGame } from '../services/library';
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
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false, executable: 'Hades.exe' } as LibraryGame]);
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
