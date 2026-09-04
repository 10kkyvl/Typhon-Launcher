import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
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
import { sourceErrorText } from '../sources/sourceErrors';
import { toast } from './toasts';

export const peers = writable<Peer[]>([]);
export const offers = writable<Offer[]>([]);
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

function upsertTransfer(item: Transfer) {
  transfers.update((list) => {
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

export async function initLan() {
  if (!inWails) return;

  try {
    const [peersList, offersList, sharesList, statsResult] = await Promise.all([
      getPeers(),
      getOffers(),
      getShares(),
      getStats(),
    ]);
    peers.set(peersList);
    offers.set(offersList);
    shares.set(sharesList);
    lanStats.set(statsResult);
  } catch (err) {
    toast(sourceErrorText(err), 'danger');
  }

  Events.On('lan:peers', (event) => {
    peers.set((event.data as Peer[]) ?? []);
  });
  Events.On('lan:shares', (event) => {
    shares.set((event.data as Share[]) ?? []);
  });
  Events.On('lan:hashing', (event) => {
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
  Events.On('lan:transfer', (event) => {
    upsertTransfer(event.data as Transfer);
  });
  Events.On('lan:stats', (event) => {
    lanStats.set(event.data as Stats);
  });

  setInterval(async () => {
    try {
      const offersList = await getOffers();
      offers.set(offersList);
    } catch (err) {
      toast(sourceErrorText(err), 'danger');
    }
  }, 10000);
}

async function run(action: () => Promise<void>) {
  try {
    await action();
  } catch (err) {
    toast(sourceErrorText(err), 'danger');
  }
}

export function share(gameId: string) {
  hashing.update((map) => {
    const next = new Map(map);
    next.set(gameId, { gameId, processedBytes: 0, totalBytes: 0, currentFile: '', done: false });
    return next;
  });
  shareRequest(gameId)
    .then((result) => {
      upsertShare(result);
      clearHashing(gameId);
    })
    .catch((err) => {
      toast(sourceErrorText(err), 'danger');
      clearHashing(gameId);
    });
}

export function unshare(gameId: string) {
  return run(async () => {
    await unshareRequest(gameId);
    shares.update((list) => list.filter((s) => s.gameId !== gameId));
  });
}

export function receive(infoHash: string, peerId: string) {
  return run(async () => {
    const transfer = await receiveRequest(infoHash, peerId);
    upsertTransfer(transfer);
  });
}

export function cancel(id: string) {
  return run(() => cancelRequest(id));
}
