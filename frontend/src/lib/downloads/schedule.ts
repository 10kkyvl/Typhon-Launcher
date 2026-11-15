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
