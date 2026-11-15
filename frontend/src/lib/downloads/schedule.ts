import type { Download } from '../services/downloads';
import type { PlayThrottle } from '../services/playThrottle';

/**
 * Whether the schedule holds this download back right now: Go gates a queued download and a
 * running one alike while the schedule blocks (`pausedLocked`), unless "Download now" set
 * `ignoreSchedule` on it.
 */
export function waitsForSchedule(
  download: Pick<Download, 'status' | 'ignoreSchedule'>,
  throttle: Pick<PlayThrottle, 'schedule'>,
): boolean {
  if (download.status !== 'queued' && download.status !== 'downloading') return false;
  return Boolean(throttle.schedule?.blocked) && !download.ignoreSchedule;
}

/**
 * Whether Go would hold this download back if it started now: a running game in the pause mode, or the
 * schedule, unless "Download now" lifted it (internal/download `pausedLocked`). A forced start under a
 * hold sets the status to downloading with the engine gated, which would read as a stalled download.
 */
export function heldBack(download: Pick<Download, 'ignoreSchedule'>, throttle: Pick<PlayThrottle, 'mode' | 'schedule'>): boolean {
  if (throttle.mode === 'pause') return true;
  return Boolean(throttle.schedule?.blocked) && !download.ignoreSchedule;
}
