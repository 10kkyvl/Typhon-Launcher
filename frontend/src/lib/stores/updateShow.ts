import { get, writable, type Readable } from 'svelte/store';
import type { SelfUpdateStatus } from '../services/selfupdate';

/**
 * The launcher-update show (UpdateShow.svelte): when it began and which part of it runs. The status
 * alone starts and ends it (stores/selfupdate.ts feeds every status here): it starts when an update is
 * being applied, never from a downloaded one; it finishes when the running version changed, or at the
 * start after an update (the restart cuts the show in two: the new process opens on the held funnel
 * and plays the rest); it goes home when applying ends without a new version. Its own timers are for
 * motion only: a stuck `applying` ends when the backend gives up and says so.
 *
 * `startedAt` is in ms since the epoch, so a successor process given it draws the same frame.
 */
export interface UpdateShowState {
  startedAt: number;
  phase: 'run' | 'finish' | 'home';
  /** When the phase began. */
  since: number;
  /** The version being installed: the words of the show; empty when it has nothing to say. */
  version: string;
  /** The version that was running when the show began. */
  from: string;
  /**
   * A seamless restart: while the funnel holds, the curtain under it deepens to the page colour, so
   * the successor's window can take this one's place unseen.
   */
  handoff?: boolean;
  /** A successor holds the show it took over: no status moves it until `releaseShow`. */
  held?: boolean;
  /**
   * A successor's curtain: the page colour the old window handed over (`#rrggbb`), which this build's
   * theme may not resolve `--bg` to. Kept to the show's end, so the curtain lifts without a step.
   */
  background?: string;
  /** A finish owed at a start after an update: `since` is the first frame the show draws (`beginOwedFinish`). */
  owed?: boolean;
}

/** A restart the player confirmed: the show began at the press, not when the status said so. */
export interface ConfirmedStart {
  startedAt: number;
  handoff: boolean;
}

const state = writable<UpdateShowState | null>(null);
export const updateShow: Readable<UpdateShowState | null> = { subscribe: state.subscribe };

/** Which of the logo's arcs are flying (hidden on the logo), as the show draws them; null at rest. */
export const logoArcsAway = writable<readonly boolean[] | null>(null);

export function showFollowsStatus(status: SelfUpdateStatus, now = Date.now(), confirmed?: ConfirmedStart): void {
  const show = get(state);
  if (show?.held) return;
  if (status.state === 'applying') {
    if (show?.phase === 'run') {
      // A refused hand-over goes on as an ordinary restart: same show, no curtain.
      if (confirmed && Boolean(show.handoff) !== confirmed.handoff) state.set({ ...show, handoff: confirmed.handoff });
      return;
    }
    const startedAt = confirmed?.startedAt ?? now;
    state.set({
      startedAt,
      phase: 'run',
      since: startedAt,
      version: status.availableVersion ?? '',
      from: status.currentVersion,
      ...(confirmed?.handoff ? { handoff: true } : {}),
    });
    return;
  }
  if (show?.phase !== 'run') return;
  const changed = Boolean(status.currentVersion && show.from && status.currentVersion !== show.from);
  state.set({ ...show, phase: changed ? 'finish' : 'home', since: now });
}

/**
 * How long a shell that has just appeared still takes the finish after an update. The sign-in and
 * the read of the update's outcome are both answered at the start, and which comes first is a race:
 * a guest, an offline start or a saved session settles in one local call, the outcome comes after
 * three, milliseconds later. Half a second is about the least a person needs to take in a screen that
 * has just appeared and act on it: an outcome read sooner finishes over the start, one read later
 * would dim a page in use. A time bound, measured on the steady clock.
 */
export const FRESH_SHELL_MS = 500;

let hosts = 0;
let shellSince = 0;

/**
 * UpdateShow is mounted: from now on the shell (or the screen the sign-in led to) is on screen.
 * Returns the disposer for its unmount.
 */
export function hostUpdateShow(): () => void {
  if (hosts === 0) shellSince = performance.now();
  hosts += 1;
  let gone = false;
  return () => {
    if (gone) return;
    gone = true;
    hosts -= 1;
  };
}

/**
 * The start after an update: the show is owed its finish, drawn from its hold (the moment it began is
 * unknown and long past). Its clock starts at the first frame the show draws, however long the
 * sign-in keeps the shell away, and a shell that has just appeared (FRESH_SHELL_MS) still takes it.
 * Once the shell has been up for the player to use it is too late: a dim over a page in use says
 * nothing the card does not, so nothing plays and the answer is false. `overShell` plays it anyway
 * (the showcase's play). A show already up (a successor's) is never replaced.
 */
export function resumeFinishedShow(overShell = false): boolean {
  const inUse = hosts > 0 && performance.now() - shellSince > FRESH_SHELL_MS;
  if (get(state) || (inUse && !overShell)) return false;
  state.set({ startedAt: 0, phase: 'finish', since: 0, version: '', from: '', owed: true });
  return true;
}

/** The first frame of an owed finish is drawn at `now`: the finish is timed from it. */
export function beginOwedFinish(now: number): void {
  const show = get(state);
  if (!show?.owed) return;
  state.set({ startedAt: show.startedAt, phase: show.phase, since: now, version: show.version, from: show.from });
}

/**
 * A successor of a seamless restart goes on with the show the old launcher began: the same clock, so
 * the same frame, held until the old launcher is gone (`releaseShow`).
 */
export function takeOverShow(startedAt: number, version: string, background = ''): void {
  const handed = /^#[0-9a-f]{6}$/i.test(background) ? { background } : {};
  state.set({ startedAt, phase: 'run', since: startedAt, version, from: '', handoff: true, held: true, ...handed });
}

/** The successor owns the launcher: the show plays its finish from where it is. */
export function releaseShow(now = Date.now()): void {
  const show = get(state);
  if (!show?.held) return;
  const handed = show.background ? { background: show.background } : {};
  state.set({ startedAt: show.startedAt, phase: 'finish', since: now, version: show.version, from: show.from, handoff: true, ...handed });
}

/** Called by the show once its last frame is drawn, or when it cannot be drawn at all. */
export function endUpdateShow(): void {
  state.set(null);
  logoArcsAway.set(null);
}

/** What a successor process is handed to go on with the running show. */
export function updateShowHandoff(): { startedAt: number } | null {
  const show = get(state);
  return show?.phase === 'run' ? { startedAt: show.startedAt } : null;
}
