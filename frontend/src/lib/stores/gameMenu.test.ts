import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { LibraryGame } from '../services/library';

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
vi.mock('../services/library', () => ({
  getGames: vi.fn(async () => []),
  getRunningGames: vi.fn(async () => []),
}));

import { closeGameMenu, gameMenu, openGameMenu } from './gameMenu';
import { libraryGames } from './library';

function mouse(x: number, y: number) {
  return { clientX: x, clientY: y, preventDefault: vi.fn(), stopPropagation: vi.fn() };
}

beforeEach(() => {
  gameMenu.set(null);
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false } as LibraryGame]);
});

describe('game context menu', () => {
  it('opens at the pointer for a game in the library and swallows the browser menu', () => {
    const event = mouse(120, 80);

    openGameMenu(event as unknown as MouseEvent, 'g1');

    expect(get(gameMenu)).toEqual({ gameId: 'g1', x: 120, y: 80 });
    expect(event.preventDefault).toHaveBeenCalledTimes(1);
    expect(event.stopPropagation).toHaveBeenCalledTimes(1);
  });

  it('leaves the browser menu alone for a game the library does not have', () => {
    const event = mouse(10, 10);

    openGameMenu(event as unknown as MouseEvent, 'catalog-only');

    expect(get(gameMenu)).toBeNull();
    expect(event.preventDefault).not.toHaveBeenCalled();
  });

  it('moves to the new spot when it is opened again', () => {
    openGameMenu(mouse(1, 2) as unknown as MouseEvent, 'g1');
    openGameMenu(mouse(30, 40) as unknown as MouseEvent, 'g1');

    expect(get(gameMenu)).toEqual({ gameId: 'g1', x: 30, y: 40 });
  });

  it('closes', () => {
    openGameMenu(mouse(1, 2) as unknown as MouseEvent, 'g1');

    closeGameMenu();

    expect(get(gameMenu)).toBeNull();
  });
});
