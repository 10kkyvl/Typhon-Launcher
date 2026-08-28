import { get, writable } from 'svelte/store';
import { libraryGames } from './library';

export interface GameMenuState {
  gameId: string;
  x: number;
  y: number;
}

export const gameMenu = writable<GameMenuState | null>(null);

export function openGameMenu(event: MouseEvent, gameId: string) {
  if (!get(libraryGames).some((game) => game.id === gameId)) return;
  event.preventDefault();
  event.stopPropagation();
  gameMenu.set({ gameId, x: event.clientX, y: event.clientY });
}

export function closeGameMenu() {
  gameMenu.set(null);
}
