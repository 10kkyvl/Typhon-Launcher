import { Manager } from '../../../bindings/typhon/internal/download';
import { inWails } from './backend';

export type DownloadStatus =
  | 'queued'
  | 'metadata'
  | 'downloading'
  | 'paused'
  | 'verifying'
  | 'completed'
  | 'failed';

export interface FileState {
  path: string;
  size: number;
  selected: boolean;
  bytesDone: number;
}

export type DownloadPurpose = 'update' | 'repair';

export interface DownloadOrigin {
  releaseId?: string;
  sourceId?: string;
  gameId?: string;
  version?: string;
  purpose?: DownloadPurpose;
  updatePlanId?: string;
  libraryId?: string;
}

export interface Download {
  id: string;
  name: string;
  type: string;
  source: string;
  infoHash: string;
  destination: string;
  status: DownloadStatus;
  progress: number;
  downloaded: number;
  total: number;
  downloadSpeed: number;
  uploadSpeed: number;
  etaSeconds: number;
  seeders: number;
  peers: number;
  files: FileState[];
  seeding: boolean;
  stalled?: boolean;
  stalledSince?: string | null;
  addedAt: string;
  completedAt: string | null;
  error: string;
  origin: DownloadOrigin;
}

export interface TorrentInfo {
  infoHash: string;
  name: string;
  totalBytes: number;
  files: FileState[];
}

const unavailable = () => new Error('unavailable in browser');

export async function listDownloads(): Promise<Download[]> {
  if (!inWails) return [];
  return (await Manager.List()) as unknown as Download[];
}

export async function getDownload(id: string): Promise<Download> {
  if (!inWails) throw unavailable();
  return (await Manager.Get(id)) as unknown as Download;
}

export async function fetchMetadata(source: string): Promise<TorrentInfo> {
  if (!inWails) throw unavailable();
  return (await Manager.FetchMetadata(source)) as unknown as TorrentInfo;
}

export async function discardMetadata(infoHash: string): Promise<void> {
  if (!inWails) return;
  await Manager.DiscardMetadata(infoHash);
}

export async function startDownload(
  infoHash: string,
  destination: string,
  selectedIndices: number[],
): Promise<Download> {
  if (!inWails) throw unavailable();
  return (await Manager.StartDownload(infoHash, destination, selectedIndices)) as unknown as Download;
}

export async function startDownloadFrom(
  infoHash: string,
  destination: string,
  selectedIndices: number[],
  origin: DownloadOrigin,
): Promise<Download> {
  if (!inWails) throw unavailable();
  return (await Manager.StartDownloadFrom(
    infoHash,
    destination,
    selectedIndices,
    origin as never,
  )) as unknown as Download;
}

export async function selectTorrentFile(): Promise<string> {
  if (!inWails) return '';
  return await Manager.AddTorrentSelectFile();
}

export async function pauseDownload(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.Pause(id);
}

export async function resumeDownload(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.Resume(id);
}

export async function cancelDownload(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.Cancel(id);
}

export async function removeDownload(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.Remove(id);
}

export async function deleteDownloadData(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.DeleteData(id);
}

export async function moveDownloadUp(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.MoveUp(id);
}

export async function moveDownloadDown(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.MoveDown(id);
}

export async function forceStartDownload(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.ForceStart(id);
}
