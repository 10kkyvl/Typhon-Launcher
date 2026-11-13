import { get, writable, type Readable } from 'svelte/store';
import { cloudErrorText } from '../cloudsaves/messages';
import { msg } from '../i18n';
import { inWails } from '../services/backend';
import { cloudAllowLaunchOnce, loadCloudStatus, onCloudEvent, type CloudEvent, type CloudStatus } from '../services/cloudsaves';
import { libraryGames } from './library';
import { toast } from './toasts';

export interface CloudConflictTarget {
  gameId: string;
  title: string;
}

export const cloudStatus = writable<CloudStatus | null>(null);
export const cloudConflictTarget = writable<CloudConflictTarget | null>(null);

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
  show(null, true);
}

/**
 * Lets the launch waiting on the dialog for `target` start once on this computer's saves, the conflict
 * left recorded, and answers it "resolved" so that it is made again. Rejects with the refusal, and the
 * launch then still waits.
 */
export async function allowConflictLaunch(target: CloudConflictTarget): Promise<void> {
  await cloudAllowLaunchOnce(target.gameId);
  finishCloudConflict(target);
}

/** Loads the status for a caller that reports a failure itself; rejects when it cannot be loaded. */
export async function fetchCloudStatus(): Promise<void> {
  cloudStatus.set(await loadCloudStatus());
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
