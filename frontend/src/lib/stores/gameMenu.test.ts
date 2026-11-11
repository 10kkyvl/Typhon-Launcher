import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { LibraryGame } from '../services/library';

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
vi.mock('../services/library', () => ({
  getGames: vi.fn(async () => []),
  getRunningGames: vi.fn(async () => []),
}));

import { closeGameMenu, gameMenu, gameMenuButton, openGameMenu, type GameMenuState } from './gameMenu';
import { libraryGames } from './library';

const atPointer = (x: number, y: number): Omit<GameMenuState, 'gameId'> => ({ at: { x, y }, focus: 'menu', trigger: null, source: null });

beforeEach(() => {
  gameMenu.set(null);
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false } as LibraryGame]);
});

describe('game menu', () => {
  it('opens for a game in the library, where and how it was asked for', () => {
    expect(openGameMenu('g1', atPointer(120, 80))).toBe(true);

    expect(get(gameMenu)).toEqual({ gameId: 'g1', at: { x: 120, y: 80 }, focus: 'menu', trigger: null, source: null });
  });

  it('does not open for a game the library does not have', () => {
    expect(openGameMenu('catalog-only', atPointer(10, 10))).toBe(false);

    expect(get(gameMenu)).toBeNull();
  });

  it('each opening is a new one, also at the same spot', () => {
    openGameMenu('g1', atPointer(1, 2));
    const first = get(gameMenu);
    openGameMenu('g1', atPointer(1, 2));

    expect(get(gameMenu)).not.toBe(first);
    expect(get(gameMenu)).toEqual(first);
  });

  it('closes', () => {
    openGameMenu('g1', atPointer(1, 2));

    closeGameMenu();

    expect(get(gameMenu)).toBeNull();
  });

  it('a menu button says its menu is open only while the menu is open from that button', () => {
    expect(gameMenuButton('g1', 'more-g1', null)).toMatchObject({ id: 'more-g1', 'aria-haspopup': 'menu', 'aria-expanded': false, 'aria-controls': undefined });

    openGameMenu('g1', { at: { anchor: () => null }, focus: 'first', trigger: 'more-g1', source: null });
    expect(gameMenuButton('g1', 'more-g1', get(gameMenu))).toMatchObject({ 'aria-expanded': true, 'aria-controls': 'game-menu' });
    expect(gameMenuButton('g1', 'other', get(gameMenu))).toMatchObject({ 'aria-expanded': false, 'aria-controls': undefined });
  });
});
