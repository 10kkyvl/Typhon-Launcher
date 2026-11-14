import { derived, get, writable, type Readable } from 'svelte/store';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { inWails } from '../services/backend';
import {
  cloudAllowLaunchOnce,
  loadCloudStatus,
  onCloudEvent,
  type CloudConflict,
  type CloudEvent,
  type CloudGameStatus,
  type CloudStatus,
} from '../services/cloudsaves';
import { libraryGames } from './library';
import { toast } from './toasts';

export interface CloudConflictTarget {
  gameId: string;
  title: string;
}

export const cloudStatus = writable<CloudStatus | null>(null);
export const cloudConflictTarget = writable<CloudConflictTarget | null>(null);

// Status loads: those started, and those still running.
let loadsStarted = 0;
const loadsRunning = writable(0);

/** Whether the status is loading: a conflict listed without its details then gets them shortly. */
export const cloudStatusLoading: Readable<boolean> = derived(loadsRunning, (n) => n > 0);

/** The recorded copies a conflict is about; '' while the status has not listed them. */
const conflictCopies = (g: CloudGameStatus) => (g.conflict ? `${g.conflict.localDigest}:${g.conflict.remoteDigest}` : '');

// The conflicts whose choice was applied, by game: their copies, and the loads started by then. The
// status still lists such a conflict until a load started after the choice answers, or the game
// leaves the conflict state; a load that started before may still list it.
interface Settled {
  copies: string;
  loads: number;
}

const settled = writable<ReadonlyMap<string, Settled>>(new Map());

function unsettle(keep: (gameId: string, mark: Settled) => boolean) {
  settled.update((marks) => {
    const next = new Map([...marks].filter(([gameId, mark]) => keep(gameId, mark)));
    return next.size === marks.size ? marks : next;
  });
}

/**
 * The games in a save conflict the player has not settled, with the conflict's details when the status
 * has them. A conflict whose choice was applied leaves at once, before the status is loaded again.
 */
export const unsettledConflicts: Readable<ReadonlyMap<string, CloudConflict | undefined>> = derived(
  [cloudStatus, settled],
  ([status, marks]) => {
    const open = new Map<string, CloudConflict | undefined>();
    for (const g of status?.games ?? []) {
      if (g.state !== 'conflict') continue;
      const mark = marks.get(g.gameId);
      const copies = conflictCopies(g);
      if (mark && (mark.copies === '' || copies === '' || mark.copies === copies)) continue;
      open.set(g.gameId, g.conflict);
    }
    return open;
  },
);

// How many parts of the game page show a game's conflict.
const onPage = writable<ReadonlyMap<string, number>>(new Map());

/** The games whose conflict the game page shows: the island and the sidebar's status line leave them to it. */
export const conflictsOnPage: Readable<ReadonlySet<string>> = derived(onPage, (counts) => new Set(counts.keys()));

/** The page shows `gameId`'s conflict until the returned function is called. */
export function showConflictOnPage(gameId: string): () => void {
  const change = (by: number) =>
    onPage.update((counts) => {
      const next = new Map(counts);
      const n = (next.get(gameId) ?? 0) + by;
      if (n > 0) next.set(gameId, n);
      else next.delete(gameId);
      return next;
    });
  change(1);
  let shown = true;
  return () => {
    if (!shown) return;
    shown = false;
    change(-1);
  };
}

// A launch the saves held back, waiting on the open dialog. Its answer is given once: "resolved"
// after the choice was applied, "given up" when the dialog closes or goes to another game.
interface WaitingLaunch {
  target: CloudConflictTarget;
  answer: (resolved: boolean) => void;
}

let waiting: WaitingLaunch | null = null;
const launchWaiting = writable(false);

/** Whether a launch waits on the open dialog: only such a launch can be let through on this computer's saves. */
export const cloudConflictLaunch: Readable<boolean> = { subscribe: launchWaiting.subscribe };

function waitFor(target: CloudConflictTarget, resolve: (resolved: boolean) => void): WaitingLaunch {
  let done = false;
  return {
    target,
    answer(resolved) {
      if (done) return;
      done = true;
      resolve(resolved);
    },
  };
}

function show(next: CloudConflictTarget | null, resolved = false) {
  const left = waiting;
  waiting = null;
  launchWaiting.set(false);
  cloudConflictTarget.set(next);
  left?.answer(resolved);
}

// The dialog already open for this game stays as it is: the event and a launch error may both
// report the same conflict, in either order, and the player sees one question.
export function openCloudConflict(gameId: string): boolean {
  if (get(cloudConflictTarget)?.gameId === gameId) return true;
  const game = get(libraryGames).find((g) => g.id === gameId);
  if (!game) return false;
  show({ gameId, title: game.title });
  return true;
}

/**
 * Opens the dialog for a launch the conflict held back and answers whether the conflict was resolved.
 * Null when the game is not in the library: there is no dialog to show, the caller shows the error.
 */
export function askToResolveConflict(gameId: string): Promise<boolean> | null {
  if (!openCloudConflict(gameId)) return null;
  const target = get(cloudConflictTarget) as CloudConflictTarget;
  const older = waiting;
  return new Promise((resolve) => {
    waiting = waitFor(target, resolve);
    launchWaiting.set(true);
    older?.answer(false);
  });
}

export function closeCloudConflict() {
  show(null);
}

/** The choice for `target` was applied. Nothing happens when the dialog has moved on to another game. */
export function finishCloudConflict(target: CloudConflictTarget) {
  if (get(cloudConflictTarget) !== target) return;
  const game = get(cloudStatus)?.games.find((g) => g.gameId === target.gameId);
  settled.update((marks) => new Map(marks).set(target.gameId, { copies: game ? conflictCopies(game) : '', loads: loadsStarted }));
  show(null, true);
}

/**
 * Lets the launch waiting on the dialog for `target` start once on this computer's saves, the conflict
 * left recorded, and answers it "resolved" so that it is made again. Rejects with the refusal, and the
 * launch then still waits.
 */
export async function allowConflictLaunch(target: CloudConflictTarget): Promise<void> {
  await cloudAllowLaunchOnce(target.gameId);
  if (get(cloudConflictTarget) !== target) return;
  show(null, true);
}

/** Loads the status for a caller that reports a failure itself; rejects when it cannot be loaded. */
export async function fetchCloudStatus(): Promise<void> {
  const started = ++loadsStarted;
  loadsRunning.update((n) => n + 1);
  try {
    cloudStatus.set(await loadCloudStatus());
    unsettle((_, mark) => mark.loads >= started);
  } finally {
    loadsRunning.update((n) => n - 1);
  }
}

export async function refreshCloudStatus(): Promise<boolean> {
  if (!inWails) return false;
  try {
    await fetchCloudStatus();
    return true;
  } catch (err) {
    toast(cloudErrorText(err, msg('cloudsaves.statusFailed')), 'danger');
    return false;
  }
}

function applyEvent(event: CloudEvent) {
  const now = new Date().toISOString();
  cloudStatus.update((status) => {
    if (!status) return status;
    const next: CloudStatus = { ...status };
    if (event.quotaBytes > 0) {
      next.usedBytes = event.usedBytes;
      next.quotaBytes = event.quotaBytes;
    }
    if (!event.gameId) {
      next.pending = event.status === 'pending';
      if (event.status === 'failed') next.error = event.code;
      return next;
    }
    // The backend clears a failed pass and a pending account without an event of its own; a game
    // syncing again proves both are over.
    if (event.status === 'synced') {
      next.error = '';
      next.pending = false;
    }
    const games = status.games.filter((g) => g.gameId !== event.gameId);
    const previous = status.games.find((g) => g.gameId === event.gameId);
    games.push({
      gameId: event.gameId,
      state: event.status,
      code: event.code,
      message: event.message,
      at: now,
      syncedAt: event.status === 'synced' ? now : (previous?.syncedAt ?? ''),
      conflict: event.status === 'conflict' ? previous?.conflict : undefined,
    });
    next.games = games;
    return next;
  });
  // The game left the conflict: a later one with the same copies is the player's to settle again.
  if (event.gameId && event.status !== 'conflict') unsettle((gameId) => gameId !== event.gameId);
}


// A conflict found in the background opens no dialog over the page: the live island or a toast says it
// (liveIslandFeed.ts). The status is loaded again for the conflict's details, which the event lacks.
export function initCloudSaves() {
  if (!inWails) return () => {};
  void refreshCloudStatus();
  return onCloudEvent((event) => {
    applyEvent(event);
    if (event.status !== 'conflict' || !event.gameId) return;
    void refreshCloudStatus();
  });
}
