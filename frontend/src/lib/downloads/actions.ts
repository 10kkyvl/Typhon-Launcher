import type { Download, DownloadStatus } from '../services/downloads';
import type { PlayThrottle } from '../services/playThrottle';
import { waitsForSchedule } from './schedule';

/** What the user can ask of one download; each is one Manager method in Go. */
export type DownloadAction = 'pause' | 'resume' | 'forceStart' | 'downloadNow' | 'moveUp' | 'moveDown' | 'cancel' | 'remove';

export const downloadActionNames: readonly DownloadAction[] = [
  'pause',
  'resume',
  'forceStart',
  'downloadNow',
  'moveUp',
  'moveDown',
  'cancel',
  'remove',
];

const anyStatus: readonly DownloadAction[] = ['cancel', 'remove'];

// The guards of internal/download (Pause, Resume, ForceStart, move, DownloadNow, discard); every
// other status gets download.unavailable. actions.test.ts reads those guards from the Go source.
const accepted: Readonly<Record<DownloadStatus, readonly DownloadAction[]>> = {
  queued: ['pause', 'forceStart', 'downloadNow', 'moveUp', 'moveDown', ...anyStatus],
  metadata: ['downloadNow', ...anyStatus],
  downloading: ['pause', 'downloadNow', ...anyStatus],
  paused: ['resume', 'forceStart', 'downloadNow', ...anyStatus],
  verifying: ['downloadNow', ...anyStatus],
  completed: anyStatus,
  failed: ['resume', 'forceStart', 'downloadNow', ...anyStatus],
};

/** Whether Go lets `action` through for a download in `status`. */
export function goAccepts(status: DownloadStatus, action: DownloadAction): boolean {
  return (accepted[status] ?? anyStatus).includes(action);
}

/**
 * The actions a download offers, in `downloadActionNames` order: what Go accepts for its status, less
 * "Download now" where it would start nothing (it only lifts the schedule, so a paused download would
 * stay paused). Up and down are offered by status; the ends of the queue are the screen's to judge.
 */
export function downloadActions(
  download: Pick<Download, 'status' | 'ignoreSchedule'>,
  throttle: Pick<PlayThrottle, 'schedule'>,
): DownloadAction[] {
  return downloadActionNames.filter((action) => {
    if (!goAccepts(download.status, action)) return false;
    return action !== 'downloadNow' || waitsForSchedule(download, throttle);
  });
}
