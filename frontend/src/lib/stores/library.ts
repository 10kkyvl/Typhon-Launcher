import { derived, writable } from 'svelte/store';
import { listenFirst } from './listenFirst';
import { inWails } from '../services/backend';
import { getGames, getRunningGames, type CrashEvent, type LibraryGame, type ProfileFallback, type SessionEvent } from '../services/library';
import { msg } from '../i18n';
import { clearCrashes, recordCrash, recordCrashDetail } from './crash';
import { navigate } from './router';
import { toast } from './toasts';

export const libraryGames = writable<LibraryGame[]>([]);
// Until the first list arrives, an empty libraryGames is not an empty library.
export const libraryLoaded = writable(false);
export const installedGames = derived(libraryGames, (games) => games.filter((game) => !game.uninstalled));
export const runningGames = writable<Set<string>>(new Set());
// When this launcher saw each running game start (ms since the epoch). A game that was already running
// when the launcher came up has no entry: how long it has been running is not known here.
export const sessionStarts = writable<Map<string, number>>(new Map());

export async function initLibrary() {
  const { on, load } = listenFirst();
  if (!inWails) {
    libraryGames.set(await getGames());
    libraryLoaded.set(true);
    runningGames.set(new Set(await getRunningGames()));
    return;
  }

  on('library:updated', (event) => {
    libraryGames.set((event.data as LibraryGame[]) ?? []);
    libraryLoaded.set(true);
  });
  on('library:profile-fallback', (event) => {
    const { profileName } = event.data as ProfileFallback;
    toast(msg('games.profileFallback', { name: profileName }), 'danger');
  });
  on('game:started', (event) => {
    const { gameId } = event.data as SessionEvent;
    runningGames.update((set) => new Set(set).add(gameId));
    sessionStarts.update((starts) => (starts.has(gameId) ? starts : new Map(starts).set(gameId, Date.now())));
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
    sessionStarts.update((starts) => {
      if (!starts.has(gameId)) return starts;
      const next = new Map(starts);
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
  libraryLoaded.set(true);
  runningGames.set(new Set(await load(getRunningGames)));
}
