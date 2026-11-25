import { get, writable } from 'svelte/store';
import { listenFirst } from './listenFirst';
import { inWails } from '../services/backend';
import {
  cancel as cancelRequest,
  getOffers,
  getPeers,
  getShares,
  getStats,
  receive as receiveRequest,
  share as shareRequest,
  unshare as unshareRequest,
  type HashProgress,
  type Offer,
  type Peer,
  type Share,
  type Stats,
  type Transfer,
} from '../services/lan';
import { msg } from '../i18n';
import { sourceErrorText } from '../sources/sourceErrors';
import { settings } from './settings';
import { toast } from './toasts';

export const peers = writable<Peer[]>([]);
export const offers = writable<Offer[]>([]);
/** The games this computer shares now; one whose sharing was stopped is not among them. */
export const shares = writable<Share[]>([]);
export const transfers = writable<Transfer[]>([]);
export const hashing = writable<Map<string, HashProgress>>(new Map());

const emptyStats: Stats = {
  announcesSent: 0,
  announcesReceived: 0,
  rejected: null,
  peersKnown: 0,
  offersKnown: 0,
  sharesActive: 0,
};

export const lanStats = writable<Stats>(emptyStats);

const sayFailed = (item: Transfer) =>
  toast(msg('transfers.lanTransferFailedToast', { title: item.title, error: sourceErrorText(item.error, msg('transfers.lanUnknownError')) }), 'danger');

// While a download's answer is awaited, the transfers Go reported ended, and whether their failure was
// said: Go's event may come before its answer, which must not bring an ended transfer back.
let awaitingAnswers = 0;
const ended = new Map<string, { item: Transfer; said: boolean }>();

function upsertTransfer(item: Transfer) {
  // Go's progress event can land after its report that the transfer ended.
  if (item.status === 'receiving' && ended.has(item.id)) return;
  // A failure is said by the transfer the list held: the row leaves, and nothing else would say why.
  const held = get(transfers).some((t) => t.id === item.id);
  if (item.status === 'failed' && held) sayFailed(item);
  if (item.status !== 'receiving' && awaitingAnswers > 0) ended.set(item.id, { item, said: item.status !== 'failed' || held });
  transfers.update((list) => {
    if (item.status !== 'receiving') return list.filter((t) => t.id !== item.id);
    const index = list.findIndex((t) => t.id === item.id);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

function upsertShare(item: Share) {
  shares.update((list) => {
    const index = list.findIndex((s) => s.gameId === item.gameId);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

function clearHashing(gameId: string) {
  hashing.update((map) => {
    if (!map.has(gameId)) return map;
    const next = new Map(map);
    next.delete(gameId);
    return next;
  });
}

/** Until a whole first read arrives, the lists hold only what events said: not what Go holds. */
export const lanLoaded = writable(false);
/** Why the first read failed; null once a read has arrived. */
export const lanLoadError = writable<unknown>(null);

type Snapshot = [Peer[], Offer[], Share[], Stats];

const readSnapshot = (): Promise<Snapshot> => Promise.all([getPeers(), getOffers(), getShares(), getStats()]);

// Go lists every game it ever prepared, a stopped one with `enabled: false`: the store holds the shared ones.
const stillShared = (list: Share[] | null | undefined) => (list ?? []).filter((share) => share.enabled);

function received([peersList, offersList, sharesList, statsResult]: Snapshot) {
  settledOff = false;
  peers.set(peersList);
  offers.set(offersList);
  shares.set(stillShared(sharesList));
  lanStats.set(statsResult);
  lanLoaded.set(true);
  lanLoadError.set(null);
}

/** Reads everything; answers whether a read is there. A failure before any read is kept as state. */
async function readAll(read: () => Promise<Snapshot>): Promise<boolean> {
  try {
    received(await read());
  } catch (err) {
    if (!get(lanLoaded)) lanLoadError.set(err);
    return false;
  }
  return true;
}

// Every LAN event Go sent: a read asked for before one of them may predate it.
let heard = 0;

// Whether the offers and figures the store holds are what Go has while sharing is off. Go says nothing
// when sharing is switched (in Settings, where no screen watches the offers), so whatever may have come
// from a running sharing (a read, a lan:stats event, a poll) clears it, and the next watch that finds
// sharing off settles again.
let settledOff = false;

/**
 * Reads everything again, as the screen's Retry after a failed first read does; answers whether a read
 * is there. An event that arrives meanwhile gets the snapshot asked once more, as the first read does.
 */
export function reloadLan(): Promise<boolean> {
  return readAll(async () => {
    const before = heard;
    const snapshot = await readSnapshot();
    return heard === before ? snapshot : readSnapshot();
  });
}

/**
 * Subscribes first, then reads; never rejects: a failed first read is `lanLoadError`. Nothing is read
 * in the background from here: the offers are read again only while a screen watches them.
 */
export async function initLan(): Promise<void> {
  const first = listenFirst();
  if (!inWails) {
    await readAll(readSnapshot);
    return;
  }
  const on: typeof first.on = (name, handler) =>
    first.on(name, (event) => {
      heard += 1;
      handler(event);
    });

  on('lan:peers', (event) => {
    peers.set((event.data as Peer[]) ?? []);
  });
  on('lan:shares', (event) => {
    shares.set(stillShared(event.data as Share[] | null));
  });
  on('lan:hashing', (event) => {
    const progress = event.data as HashProgress;
    if (progress.done) {
      clearHashing(progress.gameId);
      return;
    }
    hashing.update((map) => {
      const next = new Map(map);
      next.set(progress.gameId, progress);
      return next;
    });
  });
  on('lan:transfer', (event) => {
    upsertTransfer(event.data as Transfer);
  });
  on('lan:stats', (event) => {
    settledOff = false;
    lanStats.set(event.data as Stats);
  });

  await readAll(() => first.load(readSnapshot));
}

/** How often the offers are read again while watched: Go sends no event when they change. */
export const OFFERS_POLL_MS = 10_000;

let watchers = 0;
let poll: ReturnType<typeof setInterval> | null = null;
let stopFollowing: (() => void) | null = null;
// Bumped whenever the poll starts or stops: an answer asked for before then is dropped.
let pollRun = 0;

async function pollOffers(run: number) {
  try {
    const list = await getOffers();
    if (run === pollRun) offers.set(list);
  } catch {
    // A failed background read keeps the list on screen and the next tick asks again; the screen's
    // first read is where a failure is said.
  }
}

async function readStats(run: number) {
  try {
    const result = await getStats();
    if (run === pollRun) lanStats.set(result);
  } catch {
    // The figures stay as Go last sent them; its next lan:stats event replaces them.
  }
}

function stopPoll() {
  if (poll === null) return;
  clearInterval(poll);
  poll = null;
  pollRun += 1;
}

/**
 * Sharing as the settings say: while on, the offers are read at once and then on every tick; found off,
 * the offers are emptied and the figures read once, unless the store has settled for off since.
 */
function follow(sharing: boolean) {
  if (!sharing) {
    stopPoll();
    if (settledOff) return;
    settledOff = true;
    // Go dropped its table of offers with the sharing: what was nearby is no longer known.
    offers.set([]);
    void readStats(pollRun);
    return;
  }
  settledOff = false;
  if (poll !== null) return;
  pollRun += 1;
  const run = pollRun;
  void pollOffers(run);
  poll = setInterval(() => void pollOffers(run), OFFERS_POLL_MS);
}

/**
 * Keeps the offers fresh while the caller shows them and LAN sharing is on (Go has none to give while it
 * is off). One poll serves every watcher; the returned stop ends this watch, and the poll with the last.
 */
export function watchOffers(): () => void {
  watchers += 1;
  if (watchers === 1) stopFollowing = settings.subscribe((value) => follow(Boolean(value?.lanSharing)));
  let stopped = false;
  return () => {
    if (stopped) return;
    stopped = true;
    watchers -= 1;
    if (watchers > 0) return;
    stopFollowing?.();
    stopFollowing = null;
    stopPoll();
  };
}

async function run(action: () => Promise<void>) {
  try {
    await action();
  } catch (err) {
    toast(sourceErrorText(err), 'danger');
  }
}

const shareGeneration = new Map<string, number>();

function nextShareGeneration(gameId: string): number {
  const next = (shareGeneration.get(gameId) ?? 0) + 1;
  shareGeneration.set(gameId, next);
  return next;
}

export function share(gameId: string) {
  const generation = nextShareGeneration(gameId);
  hashing.update((map) => {
    const next = new Map(map);
    next.set(gameId, { gameId, processedBytes: 0, totalBytes: 0, currentFile: '', done: false });
    return next;
  });
  shareRequest(gameId)
    .then((result) => {
      clearHashing(gameId);
      if (shareGeneration.get(gameId) !== generation) return;
      upsertShare(result);
    })
    .catch((err) => {
      clearHashing(gameId);
      if (shareGeneration.get(gameId) !== generation) return;
      toast(sourceErrorText(err), 'danger');
    });
}

export function unshare(gameId: string) {
  const generation = nextShareGeneration(gameId);
  return run(async () => {
    await unshareRequest(gameId);
    if (shareGeneration.get(gameId) !== generation) return;
    shares.update((list) => list.filter((s) => s.gameId !== gameId));
  });
}

export function receive(infoHash: string, peerId: string) {
  return run(async () => {
    awaitingAnswers += 1;
    try {
      const transfer = await receiveRequest(infoHash, peerId);
      const end = ended.get(transfer.id);
      if (!end) {
        upsertTransfer(transfer);
        return;
      }
      // It failed before the list ever held it: nothing else says so.
      if (!end.said) sayFailed(end.item);
    } finally {
      awaitingAnswers -= 1;
      if (awaitingAnswers === 0) ended.clear();
    }
  });
}

/** Rejects when Go refused: the confirmation that asks for it shows the reason. */
export async function cancelTransfer(id: string): Promise<void> {
  await cancelRequest(id);
}

export function cancel(id: string) {
  return run(() => cancelTransfer(id));
}
