import { get, writable } from 'svelte/store';
import { libraryGames } from './library';

export interface RedistTarget {
  gameId: string;
  title: string;
}

export const redistTarget = writable<RedistTarget | null>(null);

export function openRedist(gameId: string) {
  const game = get(libraryGames).find((g) => g.id === gameId);
  if (!game) return;
  redistTarget.set({ gameId, title: game.title });
}

export function closeRedist() {
  redistTarget.set(null);
}
