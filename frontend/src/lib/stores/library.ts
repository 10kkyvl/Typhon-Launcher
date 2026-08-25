import { derived, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { getGames, getRunningGames, type LibraryGame } from '../services/library';
import { toast } from './toasts';

export const libraryGames = writable<LibraryGame[]>([]);
export const installedGames = derived(libraryGames, (games) => games.filter((game) => !game.uninstalled));
export const runningGames = writable<Set<string>>(new Set());

interface SessionEvent {
  gameId: string;
  sessionSeconds: number;
  playtimeSeconds: number;
}

export async function initLibrary() {
  libraryGames.set(await getGames());
  runningGames.set(new Set(await getRunningGames()));
  if (!inWails) return;

  Events.On('library:updated', (event) => {
    libraryGames.set((event.data as LibraryGame[]) ?? []);
  });
  Events.On('game:started', (event) => {
    const { gameId } = event.data as SessionEvent;
    runningGames.update((set) => new Set(set).add(gameId));
  });
  Events.On('game:stopped', (event) => {
    const { gameId, sessionSeconds } = event.data as SessionEvent;
    runningGames.update((set) => {
      const next = new Set(set);
      next.delete(gameId);
      return next;
    });
    if (sessionSeconds >= 60) {
      toast(`Сессия завершена: ${Math.round(sessionSeconds / 60)} мин`);
    }
  });
}
