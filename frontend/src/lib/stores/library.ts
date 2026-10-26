import { derived, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { getGames, getRunningGames, type CrashEvent, type LibraryGame, type ProfileFallback, type SessionEvent } from '../services/library';
import { msg } from '../i18n';
import { clearCrashes, recordCrash, recordCrashDetail } from './crash';
import { toast } from './toasts';

export const libraryGames = writable<LibraryGame[]>([]);
export const installedGames = derived(libraryGames, (games) => games.filter((game) => !game.uninstalled));
export const runningGames = writable<Set<string>>(new Set());

export async function initLibrary() {
  libraryGames.set(await getGames());
  runningGames.set(new Set(await getRunningGames()));
  if (!inWails) return;

  Events.On('library:updated', (event) => {
    libraryGames.set((event.data as LibraryGame[]) ?? []);
  });
  Events.On('library:profile-fallback', (event) => {
    const { profileName } = event.data as ProfileFallback;
    toast(msg('games.profileFallback', { name: profileName }), 'danger');
  });
  Events.On('game:started', (event) => {
    const { gameId } = event.data as SessionEvent;
    runningGames.update((set) => new Set(set).add(gameId));
  });
  Events.On('game:crash', (event) => {
    recordCrashDetail(event.data as CrashEvent);
  });
  Events.On('game:stopped', (event) => {
    const session = event.data as SessionEvent;
    const { gameId, sessionSeconds } = session;
    runningGames.update((set) => {
      const next = new Set(set);
      next.delete(gameId);
      return next;
    });
    if (session.crashed) {
      recordCrash(session);
      return;
    }
    clearCrashes(gameId);
    if (sessionSeconds >= 60) {
      toast(msg('state.librarySessionEnded', { minutes: Math.round(sessionSeconds / 60) }));
    }
  });
}
