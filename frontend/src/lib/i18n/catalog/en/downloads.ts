import type { Message } from '../../types';
import type { DownloadsKey } from '../ru/downloads';

export const downloads: Record<DownloadsKey, Message> = {
  'downloads.selected': { one: 'Selected: {count} file, {size}', other: 'Selected: {count} files, {size}' },
  'downloads.active': { one: '{count} active', other: '{count} active' },
  'downloads.speedGraph.caption': 'Speed, last {seconds} s',
  'downloads.speedGraph.peak': 'peak {speed}',
  'downloads.speedGraph.ago': '−{seconds} s',

  // ---- Cancelling a download: one wording for the panel, the queue and the failed list (lib/confirm/prompts.ts)
  'downloads.cancel.title': 'Cancel downloading “{name}”?',
  'downloads.cancel.deletes': "The download stops and leaves the list, and the {size} downloaded so far is deleted from disk. This can't be undone.",
  'downloads.cancel.keeps': 'The download stops and leaves the list. The {size} downloaded so far stays on disk, in “{path}”.',
  'downloads.cancel.busy': 'Cancelling…',
  'downloads.cancel.done': 'Download “{name}” cancelled',
  // ---- end of cancelling a download
  // ---- 6-3: the panel of a download under way (lib/components/DownloadItem.svelte)
  'downloads.panel.pause': 'Pause',
  'downloads.panel.pauseNamed': 'Pause “{name}”',
  'downloads.panel.resume': 'Resume',
  'downloads.panel.resumeNamed': 'Resume “{name}”',
  'downloads.panel.downloadNow': 'Download now',
  'downloads.panel.downloadNowNamed': 'Download now “{name}”',
  'downloads.panel.more': 'More actions for “{name}”',
  'downloads.panel.details': 'Details',
  'downloads.panel.startNow': 'Start now',
  'downloads.panel.openFolder': 'Open folder',
  'downloads.panel.cancel': 'Cancel download',
  'downloads.panel.progressOf': 'Progress of “{name}”',
  'downloads.panel.amount': '{done} of {total} · {percent}%',
  'downloads.panel.amountPaused': '{done} of {total} · {percent}%, paused',
  'downloads.panel.amountChecking': '{done} of {total} · {percent}%, checking files',
  'downloads.panel.speed': 'Speed',
  'downloads.panel.upload': 'upload {speed}',
  'downloads.panel.eta': '{eta} left',
  'downloads.panel.peers': 'peers {peers}, seeds {seeders}',
  'downloads.panel.limit': 'limit {limit}',
  'downloads.panel.scheduleNote': 'Outside the schedule window: the download goes on when the window opens.',
  // ---- end 6-3
  // Stage 6-4: the queue and the failed list (routes/downloads/Queue*, FailedList, downloadCancel).
  'downloads.queue.grip': '“{title}”, position {position} of {total}',
  'downloads.queue.gripHint': 'Drag to change the order',
  'downloads.queue.help':
    'The up and down arrows move the download one place, Home and End to the start and the end of the queue. Escape cancels a drag.',
  'downloads.queue.position': '#{n} in the queue',
  'downloads.queue.stateWaiting': 'Waiting',
  'downloads.queue.stateSchedule': 'Waiting for the schedule',
  'downloads.queue.stateNext': 'Starts next',
  'downloads.queue.downloaded': '{percent}% downloaded',
  'downloads.queue.downloadedFor': '“{title}” downloaded',
  'downloads.queue.downloadNowFor': 'Download now: “{title}”',
  'downloads.queue.toTop': 'To top',
  'downloads.queue.toTopFor': 'To top of the queue: “{title}”',
  'downloads.queue.details': 'Details',
  'downloads.queue.cancelFor': 'Cancel downloading “{title}”',
  'downloads.queue.moved': '“{title}” moved: position {position} of {total}',
  'downloads.queue.moveFailed': 'Could not move “{title}”: {reason}',
  'downloads.failed.retryFor': 'Retry: “{title}”',
  'downloads.failed.detailsFor': 'Details: “{title}”',
  'downloads.failed.downloaded': '{done} of {total} downloaded',
  'downloads.failed.nothing': 'Nothing downloaded',
  'downloads.failed.retried': 'Retrying “{title}”',
  // End of stage 6-4.
};
