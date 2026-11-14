import { derived, get, writable } from 'svelte/store';
import { listenFirst } from './listenFirst';
import { inWails } from '../services/backend';
import {
  getGames,
  getRunningGames,
  onLibraryUpdated,
  type CrashEvent,
  type LibraryGame,
  type ProfileFallback,
  type SessionEvent,
} from '../services/library';
import { getCatalogGames } from '../services/sources';
import { msg } from '../i18n';
import type { CatalogTitle } from '../library/entries';
import { metadataErrorText } from '../metadata/metadataErrors';
import { clearCrashes, recordCrash, recordCrashDetail } from './crash';
import { navigate } from './router';
import { toast } from './toasts';

export const libraryGames = writable<LibraryGame[]>([]);
// Until the first list arrives, an empty libraryGames is not an empty library.
export const libraryLoaded = writable(false);
/** Why the first list could not be read; null once a list has arrived. */
export const libraryLoadError = writable<unknown>(null);
export const installedGames = derived(libraryGames, (games) => games.filter((game) => !game.uninstalled));
export const runningGames = writable<Set<string>>(new Set());
// When this launcher saw each running game start (ms since the epoch). A game that was already running
// when the launcher came up has no entry: how long it has been running is not known here.
export const sessionStarts = writable<Map<string, number>>(new Map());

function received(games: LibraryGame[]) {
  libraryGames.set(games);
  libraryLoaded.set(true);
  libraryLoadError.set(null);
}

type Loader = <T>(request: () => Promise<T>) => Promise<T>;

const direct: Loader = (request) => request();

// A list from library:updated is the newest; an answer requested before it must not replace it.
let listGeneration = 0;

/** Reads the list once; answers whether a list is there. A failure before any list is kept as state. */
export async function loadLibrary(load: Loader = direct): Promise<boolean> {
  const started = listGeneration;
  try {
    const games = await load(getGames);
    if (started === listGeneration) received(games);
  } catch (err) {
    if (started !== listGeneration) return true;
    if (!get(libraryLoaded)) libraryLoadError.set(err);
    return false;
  }
  try {
    runningGames.set(new Set(await load(getRunningGames)));
  } catch {
    // Sessions also arrive as game:started events; without this answer only a game that was already
    // running when the launcher came up shows as idle until it stops.
  }
  return true;
}

function onSessionStopped(session: SessionEvent) {
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
}

/** Subscribes first, then loads; never rejects. The answer is the disposer of the subscriptions. */
export async function initLibrary(): Promise<() => void> {
  if (!inWails) {
    await loadLibrary();
    return () => {};
  }
  const { on, load } = listenFirst();
  const offs = [
    onLibraryUpdated((games) => {
      listGeneration += 1;
      received(games);
    }),
    on('library:profile-fallback', (event) => {
      const { profileName } = event.data as ProfileFallback;
      toast(msg('games.profileFallback', { name: profileName }), 'danger');
    }),
    on('game:started', (event) => {
      const { gameId } = event.data as SessionEvent;
      runningGames.update((set) => new Set(set).add(gameId));
      sessionStarts.update((starts) => (starts.has(gameId) ? starts : new Map(starts).set(gameId, Date.now())));
    }),
    on('game:crash', (event) => {
      recordCrashDetail(event.data as CrashEvent);
    }),
    on('game:stopped', (event) => onSessionStopped(event.data as SessionEvent)),
  ];
  await loadLibrary(load);
  return () => {
    for (const off of offs) off();
  };
}

/** Catalog titles of downloads by the id they were asked under; null: the catalog does not know it. */
export const downloadTitles = writable<Record<string, CatalogTitle | null>>({});

const titlesAsked = new Set<string>();
const TITLES_RETRY_AFTER_MS = 60_000;
let titlesFailureShown = false;

function titleOf(game: { title: string; releaseYear?: number; developer?: string }): CatalogTitle {
  return { title: game.title, releaseYear: game.releaseYear, developer: game.developer };
}

/**
 * Asks the catalog for the titles of downloads, once per id: an id asked, answered or unknown is not
 * asked again, so progress ticks never re-request. A failure is shown once and its ids are asked
 * again only after a pause.
 */
export async function requestDownloadTitles(ids: readonly string[]): Promise<void> {
  const missing = [...new Set(ids)].filter((id) => id && !titlesAsked.has(id));
  if (missing.length === 0) return;
  for (const id of missing) titlesAsked.add(id);
  try {
    const games = await getCatalogGames(missing);
    const found: Record<string, CatalogTitle | null> = {};
    const claimed = new Set<unknown>();
    for (const id of missing) {
      const game = games.find((item) => item.id === id || (item.aliasIds ?? []).includes(id));
      if (game) claimed.add(game);
      found[id] = game ? titleOf(game) : null;
    }
    // The catalog answers an alias under its main id and skips ids it does not know: one id left
    // without a match and one game left unclaimed are the same game.
    const unmatched = missing.filter((id) => found[id] === null);
    const unclaimed = games.filter((item) => !claimed.has(item));
    if (unmatched.length === 1 && unclaimed.length === 1) found[unmatched[0]] = titleOf(unclaimed[0]);
    titlesFailureShown = false;
    downloadTitles.update((current) => ({ ...current, ...found }));
  } catch (err) {
    setTimeout(() => {
      for (const id of missing) titlesAsked.delete(id);
    }, TITLES_RETRY_AFTER_MS);
    if (titlesFailureShown) return;
    titlesFailureShown = true;
    toast(metadataErrorText(err, msg('games.libraryLoadGamesError')), 'danger');
  }
}
