import { msg } from '../../lib/i18n';
import type { PlayThrottle } from '../../lib/services/playThrottle';

/** A queued download's state in words: held by the schedule, first in line, or waiting its turn. */
export function queueState(index: number, waitingForSchedule: boolean): string {
  if (waitingForSchedule) return msg('downloads.queue.stateSchedule');
  return index === 0 ? msg('downloads.queue.stateNext') : msg('downloads.queue.stateWaiting');
}

/**
 * Whether "Start now" (Go's ForceStart) would start anything. It passes the limit of downloads at once,
 * not a hold: while the schedule holds this download or a running game pauses every download, Go would
 * mark it downloading and move no data (hunter D-9). "Download now" is the action for the schedule.
 */
export function startNowHelps(waitingForSchedule: boolean, throttle: Pick<PlayThrottle, 'active' | 'mode'>): boolean {
  return !waitingForSchedule && !(throttle.active && throttle.mode === 'pause');
}
