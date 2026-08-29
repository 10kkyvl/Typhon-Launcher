import { Service as LanService } from '../../../bindings/typhon/internal/lan';
import { inWails } from './backend';

export interface Peer {
  id: string;
  host: string;
  addr: string;
  port: number;
  lastSeen: string;
}

export interface Offer {
  peerId: string;
  host: string;
  addr: string;
  port: number;
  gameId: string;
  title: string;
  version: string;
  exe: string;
  sizeBytes: number;
  infoHash: string;
  lastSeen: string;
}

export interface Share {
  gameId: string;
  title: string;
  version: string;
  exe: string;
  root: string;
  infoHash: string;
  sizeBytes: number;
  fingerprint: string;
  builtAt: string;
  enabled: boolean;
}

export interface Stats {
  announcesSent: number;
  announcesReceived: number;
  rejected: Record<string, number> | null;
  peersKnown: number;
  offersKnown: number;
  sharesActive: number;
}

export type TransferStatus = 'receiving' | 'completed' | 'failed' | 'cancelled';

export interface Transfer {
  id: string;
  infoHash: string;
  peerId: string;
  gameId: string;
  title: string;
  downloaded: number;
  total: number;
  status: TransferStatus;
  error?: string;
  startedAt: string;
  updatedAt: string;
}

export interface HashProgress {
  gameId: string;
  processedBytes: number;
  totalBytes: number;
  currentFile: string;
  done: boolean;
  error?: string;
}

const unavailable = () => new Error('unavailable in browser');

const emptyStats: Stats = {
  announcesSent: 0,
  announcesReceived: 0,
  rejected: null,
  peersKnown: 0,
  offersKnown: 0,
  sharesActive: 0,
};

export async function getPeers(): Promise<Peer[]> {
  if (!inWails) return [];
  return ((await LanService.Peers()) ?? []) as unknown as Peer[];
}

export async function getOffers(): Promise<Offer[]> {
  if (!inWails) return [];
  return ((await LanService.Available()) ?? []) as unknown as Offer[];
}

export async function getShares(): Promise<Share[]> {
  if (!inWails) return [];
  return ((await LanService.Shares()) ?? []) as unknown as Share[];
}

export async function getStats(): Promise<Stats> {
  if (!inWails) return { ...emptyStats };
  return (await LanService.StatsOf()) as unknown as Stats;
}

export async function share(gameId: string): Promise<Share> {
  if (!inWails) throw unavailable();
  return (await LanService.Share(gameId)) as unknown as Share;
}

export async function unshare(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LanService.Unshare(gameId);
}

export async function receive(infoHash: string, peerId: string): Promise<Transfer> {
  if (!inWails) throw unavailable();
  return (await LanService.Receive(infoHash, peerId)) as unknown as Transfer;
}

export async function cancel(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await LanService.Cancel(id);
}
