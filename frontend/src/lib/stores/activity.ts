import { derived } from 'svelte/store';
import type { Download, DownloadStatus } from '../services/downloads';
import type { Installation } from '../services/install';
import { bytesSize, etaLabel, speedBytes, truncateMiddle } from '../utils/format';
import { msg } from '../i18n';
import { downloads, statusLabels } from './downloads';
import { installActive, installStatusLabels, installations } from './install';

export type ActivityKind = 'download' | 'install';
export type ActivityTone = 'accent' | 'muted' | 'warning' | 'danger';

export interface ActivityItem {
  key: string;
  kind: ActivityKind;
  downloadId: string;
  name: string;
  status: string;
  detail: string;
  progress: number;
  tone: ActivityTone;
  attention: boolean;
  pausable: boolean;
  resumable: boolean;
}

const dockStatuses: DownloadStatus[] = ['queued', 'metadata', 'downloading', 'verifying', 'paused'];

function downloadDetail(item: Download) {
  if (item.status === 'downloading') {
    return msg('state.activityDownloadDetail', {
      speed: speedBytes(item.downloadSpeed),
      eta: etaLabel(item.etaSeconds),
    });
  }
  if (item.total > 0) return `${bytesSize(item.downloaded)} / ${bytesSize(item.total)}`;
  return '';
}

function installDetail(item: Installation) {
  if (item.status === 'waiting_for_user') return msg('state.activityWaitingForUser');
  if (item.currentFile) return truncateMiddle(item.currentFile, 44);
  if (item.bytesTotal > 0) return `${bytesSize(item.bytesDone)} / ${bytesSize(item.bytesTotal)}`;
  return '';
}

function fromDownload(item: Download): ActivityItem {
  return {
    key: `download:${item.id}`,
    kind: 'download',
    downloadId: item.id,
    name: item.name,
    status: statusLabels(item.status),
    detail: downloadDetail(item),
    progress: item.progress,
    tone: item.status === 'paused' || item.status === 'queued' ? 'muted' : 'accent',
    attention: false,
    pausable: item.status === 'downloading' || item.status === 'metadata',
    resumable: item.status === 'paused' || item.status === 'queued',
  };
}

function fromInstall(item: Installation): ActivityItem {
  const waiting = item.status === 'waiting_for_user';
  return {
    key: `install:${item.id}`,
    kind: 'install',
    downloadId: item.downloadId,
    name: item.name,
    status: installStatusLabels(item.status),
    detail: installDetail(item),
    progress: item.progress,
    tone: waiting ? 'warning' : 'accent',
    attention: waiting,
    pausable: false,
    resumable: false,
  };
}

export const activity = derived([downloads, installations], ([$downloads, $installations]) => {
  const installItems = $installations
    .filter((i) => installActive(i.status) || i.status === 'waiting_for_user')
    .map(fromInstall);
  const installedDownloads = new Set(installItems.map((i) => i.downloadId));
  const downloadItems = $downloads
    .filter((d) => dockStatuses.includes(d.status) && !installedDownloads.has(d.id))
    .map(fromDownload);
  return [...installItems, ...downloadItems].sort(
    (a, b) => Number(b.attention) - Number(a.attention),
  );
});

export const activitySummary = derived(activity, ($items) => ({
  count: $items.length,
  primary: $items[0] ?? null,
  progress: $items.length === 0 ? 0 : $items.reduce((sum, i) => sum + i.progress, 0) / $items.length,
  attention: $items.some((i) => i.attention),
}));
