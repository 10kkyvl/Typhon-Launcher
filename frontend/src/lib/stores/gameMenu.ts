import { get, writable } from 'svelte/store';
import { libraryGames } from './library';

export interface GameMenuState {
  gameId: string;
  x: number;
  y: number;
  /** The element the menu was opened on: focus goes back there after an action the menu started. */
  source: HTMLElement | null;
}

export const gameMenu = writable<GameMenuState | null>(null);

export function openGameMenu(event: MouseEvent, gameId: string) {
  if (!get(libraryGames).some((game) => game.id === gameId)) return;
  event.preventDefault();
  event.stopPropagation();
  const source = (event.currentTarget as HTMLElement | null) ?? null;
  gameMenu.set({ gameId, x: event.clientX, y: event.clientY, source });
}

export function closeGameMenu() {
  gameMenu.set(null);
}
