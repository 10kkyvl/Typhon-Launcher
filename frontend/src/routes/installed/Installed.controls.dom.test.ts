import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { playGame, type LibraryGame } from '../../lib/services/library';
import { libraryGames } from '../../lib/stores/library';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  playGame: vi.fn(async () => {}),
}));

const games = [
  { id: 'g-hades', title: 'Hades', lastPlayed: '2026-11-01T10:00:00Z' },
  { id: 'g-celeste', title: 'Celeste', lastPlayed: null },
].map((game, i) => ({ ...game, installDir: `C:\\Games\\${i}`, executable: 'game.exe', sizeBytes: 0, uninstalled: false, tags: [] }) as unknown as LibraryGame);

// The accessible name of the one button whose name holds `title`, other than the row's menu button.
function nameOf(title: string): string {
  let found = '';
  const menu = msg('games.installedMenuFor', { title });
  screen.getByRole('button', {
    name: (name) => {
      if (!name.includes(title) || name === menu) return false;
      found = name;
      return true;
    },
  });
  return found;
}

const times = (text: string, word: string) => text.split(word).length - 1;

beforeEach(() => {
  libraryGames.set(games);
});

afterEach(() => {
  libraryGames.set([]);
  installedView.set('list');
  document.body.replaceChildren();
});

describe('Installed: a row of the list view', () => {
  it('is named by its title once: the picture says nothing of its own', () => {
    installedView.set('list');
    renderDom(Installed);

    const name = nameOf('Celeste');
    expect(name.startsWith('Celeste')).toBe(true);
    expect(times(name, 'Celeste')).toBe(1);
  });
});

describe('Installed: the play button on a cover', () => {
  afterEach(() => vi.mocked(playGame).mockReset());

  it('is the base icon button, says which game it starts, and the keyboard presses it', async () => {
    installedView.set('grid');
    const { user } = renderDom(Installed);

    const plays = screen.getAllByRole('button', { name: msg('games.play') });
    expect(plays.map((button) => button.classList.contains('icon-btn'))).toEqual([true, true]);
    const celeste = screen.getByRole('button', { name: msg('games.play'), description: 'Celeste' });
    celeste.focus();
    await user.keyboard('{Enter}');

    expect(playGame).toHaveBeenCalledWith('g-celeste');
  });
});
