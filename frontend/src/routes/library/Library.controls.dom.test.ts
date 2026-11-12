import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames } from '../../lib/stores/library';
import { libraryView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

const games = [
  { id: 'g-hades', title: 'Hades', lastPlayed: '2026-11-01T10:00:00Z', playtimeSeconds: 3600 },
  { id: 'g-celeste', title: 'Celeste', lastPlayed: null, playtimeSeconds: 0 },
].map((game, i) => ({ ...game, installDir: `C:\\Games\\${i}`, executable: 'game.exe', sizeBytes: 0, uninstalled: false, tags: [] }) as unknown as LibraryGame);

// The accessible name of the one button whose name holds `title`.
function nameOf(title: string): string {
  let found = '';
  screen.getByRole('button', {
    name: (name) => {
      if (!name.includes(title)) return false;
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
  libraryView.set('grid');
  document.body.replaceChildren();
});

describe('Library: a row of the list view', () => {
  it('is named by its title once: the cover says nothing of its own', () => {
    libraryView.set('list');
    renderDom(Library);

    const name = nameOf('Celeste');
    expect(name.startsWith('Celeste')).toBe(true);
    expect(times(name, 'Celeste')).toBe(1);
  });
});

describe('Library: hiding the "continue playing" block', () => {
  it('is the base icon button: the keyboard presses it and focus moves on, not to the page', async () => {
    const { user } = renderDom(Library);
    const block = screen.getByRole('region', { name: msg('games.libraryContinuePlayingTitle') });
    const hide = screen.getByRole('button', { name: msg('games.libraryHideLabel') });
    expect(hide.classList.contains('icon-btn')).toBe(true);
    hide.focus();

    await user.keyboard('{Enter}');

    expect(block.isConnected).toBe(false);
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement?.isConnected).toBe(true);
  });
});

describe('Library: "Show all" of the recent games', () => {
  it('is a text link the keyboard presses, and it lists the recent games', async () => {
    const played = { ...games[1], id: 'g-cells', title: 'Dead Cells', lastPlayed: '2026-10-01T10:00:00Z' } as LibraryGame;
    libraryGames.set([...games, played]);
    const { user } = renderDom(Library);
    const recent = msg('games.recentLabel');
    expect(screen.getAllByRole('heading', { level: 2, name: recent })).toHaveLength(1);

    const all = screen.getByRole('button', { name: msg('games.libraryShowAllButton') });
    expect(all.classList.contains('text-link')).toBe(true);
    all.focus();
    await user.keyboard('{Enter}');

    expect(screen.getAllByRole('heading', { level: 2, name: recent })).toHaveLength(2);
  });
});
