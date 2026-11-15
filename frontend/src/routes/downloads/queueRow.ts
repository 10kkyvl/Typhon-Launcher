import { msg } from '../../lib/i18n';

/** A queued download's state in words: held by the schedule, first in line, or waiting its turn. */
export function queueState(index: number, waitingForSchedule: boolean): string {
  if (waitingForSchedule) return msg('downloads.queue.stateSchedule');
  return index === 0 ? msg('downloads.queue.stateNext') : msg('downloads.queue.stateWaiting');
}
