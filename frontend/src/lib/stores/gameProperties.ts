import { get, writable } from 'svelte/store';
import { libraryGames } from './library';

export interface GamePropertiesTarget {
  gameId: string;
}

export const gamePropertiesTarget = writable<GamePropertiesTarget | null>(null);

export function openGameProperties(gameId: string) {
  const game = get(libraryGames).find((g) => g.id === gameId);
  if (!game || game.uninstalled || !game.installDir) return;
  gamePropertiesTarget.set({ gameId });
}

export function closeGameProperties() {
  gamePropertiesTarget.set(null);
}
