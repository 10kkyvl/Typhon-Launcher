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
 * The start after an update: the show opens held and plays its finish. With no hand-over the moment
 * it began is unknown and long past, so it is drawn from its hold.
 */
export function resumeFinishedShow(now = Date.now(), startedAt = 0): void {
  state.set({ startedAt, phase: 'finish', since: now, version: '', from: '' });
}

/**
 * A successor of a seamless restart goes on with the show the old launcher began: the same clock, so
 * the same frame, held until the old launcher is gone (`releaseShow`).
 */
export function takeOverShow(startedAt: number, version: string): void {
  state.set({ startedAt, phase: 'run', since: startedAt, version, from: '', handoff: true, held: true });
}

/** The successor owns the launcher: the show plays its finish from where it is. */
export function releaseShow(now = Date.now()): void {
  const show = get(state);
  if (!show?.held) return;
  state.set({ startedAt: show.startedAt, phase: 'finish', since: now, version: show.version, from: show.from, handoff: true });
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
