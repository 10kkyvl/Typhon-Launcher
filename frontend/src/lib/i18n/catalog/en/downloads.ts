import type { Message } from '../../types';
import type { DownloadsKey } from '../ru/downloads';

export const downloads: Record<DownloadsKey, Message> = {
  'downloads.selected': { one: 'Selected: {count} file, {size}', other: 'Selected: {count} files, {size}' },
  'downloads.active': { one: '{count} active', other: '{count} active' },
  'downloads.speedGraph.caption': 'Speed, last {seconds} s',
  'downloads.speedGraph.peak': 'peak {speed}',
  'downloads.speedGraph.ago': '−{seconds} s',

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
  'downloads.panel.cancelTitle': 'Cancel downloading “{name}”?',
  'downloads.panel.cancelDeletes': "The download stops and the {size} downloaded so far are deleted from disk. This can't be undone.",
  'downloads.panel.cancelKeeps': 'The download stops and leaves the list. The files it has already written stay on disk.',
  'downloads.panel.cancelConfirm': 'Cancel download',
  'downloads.panel.cancelBusy': 'Cancelling…',
  'downloads.panel.cancelKeep': 'Keep downloading',
  'downloads.panel.cancelled': '“{name}” cancelled',
  // ---- end 6-3
};
