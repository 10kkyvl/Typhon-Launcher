import { derived } from 'svelte/store';
import type { Download, DownloadStatus } from '../services/downloads';
import type { Installation } from '../services/install';
import type { MoveJob } from '../services/relocate';
import type { VerifyState } from '../services/updates';
import type { PlayThrottle } from '../services/playThrottle';
import { bytesSize, etaLabel, speedBytes, truncateMiddle } from '../utils/format';
import { locale, msg } from '../i18n';
import { downloadActions } from '../downloads/actions';
import { downloads, statusLabels } from './downloads';
import { playThrottle } from './playThrottle';
import { installActive, installStatusLabels, installations } from './install';
import { installIndeterminate, installTotalUnknown } from '../install/progress';
import { libraryGames } from './library';
import { activeMove } from './relocate';
import { verifications } from './updates';
import { movePercent, stageLabel } from '../relocate/moveText';

export type ActivityKind = 'download' | 'install' | 'verify' | 'move';
export type ActivityTone = 'accent' | 'muted' | 'warning' | 'danger';

export interface ActivityItem {
  key: string;
  kind: ActivityKind;
  downloadId: string;
  name: string;
  status: string;
  detail: string;
  progress: number;
  indeterminate?: boolean;
  tone: ActivityTone;
  attention: boolean;
  /** A download that stands still (queued or paused): it reads its state, not a percent. */
  idle: boolean;
  /** The download offers Pause (`downloadActions`). */
  pausable: boolean;
  /** The download offers Continue (`downloadActions`). */
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
  if (item.status === 'verifying') return '';
  if (item.status === 'waiting_for_user') return msg('state.activityWaitingForUser');
  if (item.currentFile) return truncateMiddle(item.currentFile, 44);
  if (item.bytesTotal > 0) return `${bytesSize(item.bytesDone)} / ${bytesSize(item.bytesTotal)}`;
  if (installTotalUnknown(item)) return msg('modals.installWritten', { size: bytesSize(item.bytesDone) });
  return '';
}

function verifyDetail(item: VerifyState) {
  if (item.currentFile) return truncateMiddle(item.currentFile, 44);
  if (item.totalBytes > 0) return `${bytesSize(item.processedBytes)} / ${bytesSize(item.totalBytes)}`;
  return '';
}

function fromDownload(item: Download, throttle: PlayThrottle): ActivityItem {
  const idle = item.status === 'paused' || item.status === 'queued';
  const offers = downloadActions(item, throttle);
  return {
    key: `download:${item.id}`,
    kind: 'download',
    downloadId: item.id,
    name: item.name,
    status: statusLabels(item.status),
    detail: downloadDetail(item),
    progress: item.progress,
    tone: idle ? 'muted' : 'accent',
    attention: false,
    idle,
    pausable: offers.includes('pause'),
    resumable: offers.includes('resume'),
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
    indeterminate: installIndeterminate(item),
    progress: item.progress,
    tone: waiting ? 'warning' : 'accent',
    attention: waiting,
    idle: false,
    pausable: false,
    resumable: false,
  };
}

function fromVerify(item: VerifyState, title: string): ActivityItem {
  return {
    key: `verify:${item.gameId}`,
    kind: 'verify',
    downloadId: item.gameId,
    name: title,
    status: msg('state.activityVerifying'),
    detail: verifyDetail(item),
    progress: item.progress,
    tone: 'accent',
    attention: false,
    idle: false,
    pausable: false,
    resumable: false,
  };
}

function moveDetail(job: MoveJob) {
  if (job.currentFile) return truncateMiddle(job.currentFile, 44);
  if (job.totalBytes > 0) return `${bytesSize(job.copiedBytes)} / ${bytesSize(job.totalBytes)}`;
  return '';
}

function fromMove(job: MoveJob): ActivityItem {
  return {
    key: `move:${job.id}`,
    kind: 'move',
    downloadId: job.id,
    name: job.title || msg('ui.library'),
    status: stageLabel(job.stage),
    detail: moveDetail(job),
    progress: movePercent(job) / 100,
    tone: 'accent',
    attention: false,
    idle: false,
    pausable: false,
    resumable: false,
  };
}

export const activity = derived(
  [downloads, installations, verifications, activeMove, libraryGames, playThrottle, locale],
  ([$downloads, $installations, $verifications, $activeMove, $libraryGames, $playThrottle]) => {
    const installItems = $installations
      .filter((i) => installActive(i.status) || i.status === 'waiting_for_user')
      .map(fromInstall);
    const installedDownloads = new Set(installItems.map((i) => i.downloadId));
    const downloadItems = $downloads
      .filter((d) => dockStatuses.includes(d.status) && !installedDownloads.has(d.id))
      .map((d) => fromDownload(d, $playThrottle));
    const verifyItems = Object.values($verifications)
      .filter((v) => v.running)
      .map((v) =>
        fromVerify(v, $libraryGames.find((g) => g.id === v.gameId)?.title || msg('ui.game')),
      );
    const moveItems = $activeMove ? [fromMove($activeMove)] : [];
    return [...installItems, ...verifyItems, ...downloadItems, ...moveItems].sort(
      (a, b) => Number(b.attention) - Number(a.attention),
    );
  },
);

export const activitySummary = derived(activity, ($items) => ({
  count: $items.length,
  primary: $items[0] ?? null,
  progress: $items.length === 0 ? 0 : $items.reduce((sum, i) => sum + i.progress, 0) / $items.length,
  attention: $items.some((i) => i.attention),
}));
