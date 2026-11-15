import { derived, get, readable, writable, type Readable } from 'svelte/store';
import { listenFirst } from './listenFirst';
import { inWails } from '../services/backend';
import type { DownloadAction } from '../downloads/actions';
import { followOrder } from '../downloads/order';
import { createSpeedHistory, emptySpeedSeries, type SpeedSeries } from '../downloads/speedHistory';
import {
  cancelDownload as cancelRequest,
  downloadNow as downloadNowRequest,
  forceStartDownload,
  listDownloads,
  moveDownloadDown,
  moveDownloadTo,
  moveDownloadUp,
  pauseDownload,
  removeDownload,
  resumeDownload,
  type Download,
  type DownloadStatus,
} from '../services/downloads';
import { installErrorText } from '../install/installErrors';
import { withFiles } from '../utils/downloadFiles';
import { msg } from '../i18n';
import { errorCode } from '../i18n/errors';
import { liveIsland, liveIslandOn, type LiveIslandMessage } from './liveIsland';
import { navigate } from './router';
import { ACTION_MS, toast } from './toasts';

export const downloads = writable<Download[]>([]);
/** Until the first list arrives, an empty `downloads` is not an empty list. */
export const downloadsLoaded = writable(false);
/** Why the first list could not be read; null once a list has arrived. */
export const downloadsLoadError = writable<unknown>(null);

// DownloadProgress mirrors internal/download.ProgressUpdate: the fields a
// running download changes every 250ms tick, without Files or anything else
// that only changes on add, on a file-list change, or on completion — those
// still arrive as a full Download through download:added/download:updated/
// download:completed. Declared here rather than in bindings/ because the
// generated bindings only know about wails-bound methods, not plain event
// payload shapes.
export interface DownloadProgress {
  id: string;
  status: DownloadStatus;
  progress: number;
  downloaded: number;
  downloadSpeed: number;
  uploadSpeed: number;
  etaSeconds: number;
  seeders: number;
  peers: number;
  stalled: boolean;
  stalledSince?: string | null;
}

/** The events Go's download Manager sends (the event* constants of internal/download/manager.go). */
export const downloadEvents = {
  added: 'download:added',
  updated: 'download:updated',
  progress: 'download:progress',
  completed: 'download:completed',
  failed: 'download:failed',
  removed: 'download:removed',
  reordered: 'download:reordered',
} as const;

/** The payload of download:removed: internal/download.RemovedEvent. */
export interface DownloadRemoved {
  id: string;
}

/** The payload of download:reordered: internal/download.ReorderedEvent, every download id in Go's order. */
export interface DownloadReordered {
  ids: string[];
}

export interface DownloadEventPayloads {
  added: Download;
  updated: Download;
  progress: DownloadProgress;
  completed: Download;
  failed: Download;
  removed: DownloadRemoved;
  reordered: DownloadReordered;
}

export const downloadsById = derived(downloads, ($downloads) => {
  const map = new Map<string, Download>();
  for (const d of $downloads) map.set(d.id, d);
  return map;
});

const activeStatuses: DownloadStatus[] = ['downloading', 'metadata', 'verifying', 'paused'];

export const active = derived(downloads, ($downloads) =>
  $downloads.filter((d) => activeStatuses.includes(d.status)),
);

export const queue = derived(downloads, ($downloads) => $downloads.filter((d) => d.status === 'queued'));

export const failed = derived(downloads, ($downloads) => $downloads.filter((d) => d.status === 'failed'));

export const completed = derived(downloads, ($downloads) =>
  $downloads
    .filter((d) => d.status === 'completed')
    .slice()
    .sort((a, b) => Date.parse(b.completedAt ?? b.addedAt) - Date.parse(a.completedAt ?? a.addedAt)),
);

// Only a download that moves data adds to a total: downloading ones to the download total,
// downloading or seeding ones to the upload total, whatever speeds any other status carries.
const takesData = (d: Download) => d.status === 'downloading';
const givesData = (d: Download) => d.status === 'downloading' || d.seeding;

export const stats = derived(downloads, ($downloads) => ({
  downSpeed: $downloads.reduce((sum, d) => sum + (takesData(d) ? d.downloadSpeed : 0), 0),
  upSpeed: $downloads.reduce((sum, d) => sum + (givesData(d) ? d.uploadSpeed : 0), 0),
  activeCount: $downloads.filter((d) => activeStatuses.includes(d.status)).length,
  queuedCount: $downloads.filter((d) => d.status === 'queued').length,
}));

const history = createSpeedHistory();

downloads.subscribe((list) => {
  const now = Date.now();
  for (const d of list) history.record(d.id, d.status, d.downloadSpeed, now);
  history.keep(new Set(list.map((d) => d.id)));
});

/**
 * The download speed of one download over the last two minutes, a sample a second, kept since this
 * window loaded; nothing is stored. A reader gets a fresh series every second; the timer runs only
 * while somebody reads.
 */
export function speedHistory(id: string): Readable<SpeedSeries> {
  return readable(emptySpeedSeries(), (set) => {
    set(history.series(id, Date.now()));
    const timer = setInterval(() => set(history.series(id, Date.now())), 1000);
    return () => clearInterval(timer);
  });
}

export function statusLabels(status: DownloadStatus): string {
  const labels: Record<DownloadStatus, string> = {
    queued: msg('state.downloadsStatusQueued'),
    metadata: msg('state.downloadsStatusMetadata'),
    downloading: msg('state.downloadsStatusDownloading'),
    paused: msg('state.downloadsStatusPaused'),
    verifying: msg('state.downloadsStatusVerifying'),
    completed: msg('state.downloadsStatusCompleted'),
    failed: msg('common.error'),
  };
  return labels[status];
}

function upsert(item: Download) {
  downloads.update((list) => {
    const index = list.findIndex((d) => d.id === item.id);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

// mergeProgress folds a lightweight tick into the existing full record. An
// id the store has no record for yet (webview reload, a race at startup
// before the initial listDownloads() resolves) is dropped rather than
// turned into a half-filled card: every field but the ones below would be
// missing, and there is nothing here to fill them with.
function mergeProgress(list: Download[], patch: DownloadProgress): Download[] {
  const index = list.findIndex((d) => d.id === patch.id);
  if (index < 0) return list;
  const next = [...list];
  next[index] = {
    ...next[index],
    status: patch.status,
    progress: patch.progress,
    downloaded: patch.downloaded,
    downloadSpeed: patch.downloadSpeed,
    uploadSpeed: patch.uploadSpeed,
    etaSeconds: patch.etaSeconds,
    seeders: patch.seeders,
    peers: patch.peers,
    stalled: patch.stalled,
    stalledSince: patch.stalledSince ?? null,
  };
  return next;
}

// Every download event Go sent: a list asked for before one of them may predate it.
let heard = 0;

/** Reads the list; answers whether a list is there. A failure before any list is kept as state. */
async function readList(read: () => Promise<Download[]>): Promise<boolean> {
  try {
    downloads.set(await read());
  } catch (err) {
    if (!get(downloadsLoaded)) downloadsLoadError.set(err);
    return false;
  }
  downloadsLoaded.set(true);
  downloadsLoadError.set(null);
  return true;
}

/**
 * Reads the list again, as the page's Retry after a failed first read does; answers whether a list is
 * there. An event that arrives meanwhile gets the list asked once more, as the first read does.
 */
export function reloadDownloads(): Promise<boolean> {
  return readList(async () => {
    const before = heard;
    const list = await listDownloads();
    return heard === before ? list : listDownloads();
  });
}

function hasId(data: unknown): boolean {
  return typeof data === 'object' && data !== null && typeof (data as { id?: unknown }).id === 'string';
}

function isOrder(data: unknown): data is DownloadReordered {
  if (typeof data !== 'object' || data === null) return false;
  const ids = (data as { ids?: unknown }).ids;
  return Array.isArray(ids) && ids.every((id) => typeof id === 'string');
}

/** Subscribes first, then reads the list; never rejects: a failed first read is `downloadsLoadError`. */
export async function initDownloads(): Promise<void> {
  const first = listenFirst();
  if (!inWails) {
    await readList(listDownloads);
    return;
  }
  const on: typeof first.on = (name, handler) =>
    first.on(name, (event) => {
      heard += 1;
      handler(event);
    });

  // Every download event names its download; one that does not is dropped rather than stored.
  const listen = <K extends Exclude<keyof DownloadEventPayloads, 'reordered'>>(
    kind: K,
    handler: (payload: DownloadEventPayloads[K]) => void,
  ) =>
    on(downloadEvents[kind], (event) => {
      if (hasId(event.data)) handler(event.data as DownloadEventPayloads[K]);
    });

  listen('added', (payload) => {
    const item = withFiles(payload);
    upsert(item);
    toast(msg('state.downloadsAddedToast', { name: item.name }));
  });
  listen('updated', (payload) => {
    upsert(withFiles(payload));
  });
  listen('progress', (patch) => {
    downloads.update((list) => mergeProgress(list, patch));
  });
  listen('completed', (payload) => {
    const item = withFiles(payload);
    upsert(item);
    toast(msg('state.downloadsCompletedToast', { name: item.name }), 'success');
  });
  listen('failed', (payload) => {
    const item = withFiles(payload);
    upsert(item);
    if (get(liveIslandOn)) liveIsland.show(failedMessage(item));
    else failedToast(item);
  });
  listen('removed', ({ id }) => {
    downloads.update((list) => list.filter((d) => d.id !== id));
  });
  // Go sends its whole order after every move. The records and the set of downloads stay as the
  // other events left them: an id the store no longer holds is ignored, and a download the event
  // does not name (added after Go sent it) keeps its index, which for an added one is the end.
  on(downloadEvents.reordered, (event) => {
    if (isOrder(event.data)) {
      const { ids } = event.data;
      downloads.update((list) => followOrder(list, ids));
    }
  });

  await readList(() => first.load(listDownloads));
}

function failedToast(item: Download): void {
  toast(msg('state.downloadsFailedToast', { name: item.name, error: installErrorText(item.error) }), 'danger');
}

/** The id of the live island's message for a failed download. */
export const failedMessageId = (id: string) => `download-failed:${id}`;

/** With the live island showing messages, a failure is one message with Retry and "Open downloads". */
function failedMessage(item: Download): LiveIslandMessage {
  return {
    id: failedMessageId(item.id),
    kind: 'error',
    tone: 'danger',
    icon: 'danger',
    text: msg('ui.liveIslandDownloadFailed'),
    title: item.name,
    detail: item.error ? installErrorText(item.error) : undefined,
    ttl: ACTION_MS,
    actions: [
      { label: msg('common.retry'), run: () => void resume(item.id) },
      { label: msg('ui.liveIslandOpenDownloads'), run: () => navigate('downloads') },
    ],
    fallback: () => failedToast(item),
  };
}

/** How an action ended: `code` is the `typhon:<code>` Go answered with, '' when it gave none. */
export type DownloadActionOutcome = { ok: true } | { ok: false; code: string; error: unknown };

/**
 * What a caller adds to a request: `toast: false` when it shows a failure itself, and for `moveTo`
 * the place among the queued downloads, which no other action takes.
 */
export type DownloadActionOptions<A extends DownloadAction> = { toast?: boolean } & (A extends 'moveTo'
  ? { position: number }
  : { position?: never });

const requests: Record<DownloadAction, (id: string, position: number | undefined) => Promise<void>> = {
  pause: (id) => pauseDownload(id),
  resume: (id) => resumeDownload(id),
  forceStart: (id) => forceStartDownload(id),
  downloadNow: (id) => downloadNowRequest(id),
  moveUp: (id) => moveDownloadUp(id),
  moveDown: (id) => moveDownloadDown(id),
  moveTo: (id, position) =>
    position === undefined ? Promise.reject(new Error('moveTo without a place')) : moveDownloadTo(id, position),
  cancel: (id) => cancelRequest(id),
  remove: (id) => removeDownload(id),
};

const moves: ReadonlySet<DownloadAction> = new Set<DownloadAction>(['moveUp', 'moveDown', 'moveTo']);

/** A call shared by every caller asking the same while it runs; `toast` is whether any of them wants one. */
interface SharedCall {
  outcome: Promise<DownloadActionOutcome>;
  toast: boolean;
}

const inFlight = new Map<string, SharedCall>();

/** The last move asked for each download, which the next one waits for. */
const lastMove = new Map<string, Promise<DownloadActionOutcome>>();

/**
 * Asks Go for `action` on download `id` and says how it ended. A failure is toasted unless the caller
 * shows it itself (`toast: false`, such as a confirm dialog with its own error line).
 *
 * While a call for the same download and action runs, a second one is not sent: it gets the first
 * one's outcome, and the failure is toasted once if either caller asked for it. Moves are the
 * exception, since two steps are two places: every move is sent, after the download's move before it
 * has answered, and each caller gets the outcome and the toast of its own call.
 */
export function requestDownloadAction(
  id: string,
  action: 'moveTo',
  options: DownloadActionOptions<'moveTo'>,
): Promise<DownloadActionOutcome>;
export function requestDownloadAction(
  id: string,
  action: Exclude<DownloadAction, 'moveTo'>,
  options?: DownloadActionOptions<Exclude<DownloadAction, 'moveTo'>>,
): Promise<DownloadActionOutcome>;
export function requestDownloadAction(
  id: string,
  action: DownloadAction,
  options: { toast?: boolean; position?: number } = {},
): Promise<DownloadActionOutcome> {
  const wantsToast = options.toast ?? true;
  const send = () => perform(id, action, options.position);
  if (moves.has(action)) {
    const before = lastMove.get(id);
    const call = (before ? before.then(send, send) : send()).then((outcome) => {
      if (lastMove.get(id) === call) lastMove.delete(id);
      return reported(outcome, wantsToast);
    });
    lastMove.set(id, call);
    return call;
  }
  const key = `${action}:${id}`;
  const running = inFlight.get(key);
  if (running) {
    running.toast ||= wantsToast;
    return running.outcome;
  }
  const shared: SharedCall = {
    toast: wantsToast,
    outcome: send().then((outcome) => {
      inFlight.delete(key);
      return reported(outcome, shared.toast);
    }),
  };
  inFlight.set(key, shared);
  return shared.outcome;
}

async function perform(id: string, action: DownloadAction, position: number | undefined): Promise<DownloadActionOutcome> {
  try {
    await requests[action](id, position);
  } catch (error) {
    return { ok: false, code: errorCode(error), error };
  }
  return { ok: true };
}

function reported(outcome: DownloadActionOutcome, wantsToast: boolean): DownloadActionOutcome {
  if (!outcome.ok && wantsToast) toast(installErrorText(outcome.error), 'danger');
  return outcome;
}

export async function pause(id: string): Promise<void> {
  await requestDownloadAction(id, 'pause');
}

export async function resume(id: string): Promise<void> {
  await requestDownloadAction(id, 'resume');
}

export async function cancel(id: string): Promise<void> {
  await requestDownloadAction(id, 'cancel');
}

export async function remove(id: string): Promise<void> {
  await requestDownloadAction(id, 'remove');
}

export async function forceStart(id: string): Promise<void> {
  await requestDownloadAction(id, 'forceStart');
}

export async function downloadNow(id: string): Promise<void> {
  await requestDownloadAction(id, 'downloadNow');
}

export async function moveUp(id: string): Promise<void> {
  await requestDownloadAction(id, 'moveUp');
}

export async function moveDown(id: string): Promise<void> {
  await requestDownloadAction(id, 'moveDown');
}
