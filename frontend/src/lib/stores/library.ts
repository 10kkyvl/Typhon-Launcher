import { derived, writable } from 'svelte/store';
import { listenFirst } from './listenFirst';
import { inWails } from '../services/backend';
import { getGames, getRunningGames, type CrashEvent, type LibraryGame, type ProfileFallback, type SessionEvent } from '../services/library';
import { msg } from '../i18n';
import { clearCrashes, recordCrash, recordCrashDetail } from './crash';
import { navigate } from './router';
import { toast } from './toasts';

export const libraryGames = writable<LibraryGame[]>([]);
export const installedGames = derived(libraryGames, (games) => games.filter((game) => !game.uninstalled));
export const runningGames = writable<Set<string>>(new Set());

export async function initLibrary() {
  const { on, load } = listenFirst();
  if (!inWails) {
    libraryGames.set(await getGames());
    runningGames.set(new Set(await getRunningGames()));
    return;
  }

  on('library:updated', (event) => {
    libraryGames.set((event.data as LibraryGame[]) ?? []);
  });
  on('library:profile-fallback', (event) => {
    const { profileName } = event.data as ProfileFallback;
    toast(msg('games.profileFallback', { name: profileName }), 'danger');
  });
  on('game:started', (event) => {
    const { gameId } = event.data as SessionEvent;
    runningGames.update((set) => new Set(set).add(gameId));
  });
  on('game:crash', (event) => {
    recordCrashDetail(event.data as CrashEvent);
  });
  on('game:stopped', (event) => {
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
    if (session.emulatorQuickExit) {
      toast(msg('emul.quickExit', { emulator: session.emulator }), 'info', {
        label: msg('emul.quickExitAction'),
        run: () => navigate('settings', { tab: 'downloads' }),
      });
      return;
    }
    if (sessionSeconds >= 60) {
      toast(msg('state.librarySessionEnded', { minutes: Math.round(sessionSeconds / 60) }));
    }
  });
  libraryGames.set(await load(getGames));
  runningGames.set(new Set(await load(getRunningGames)));
}
