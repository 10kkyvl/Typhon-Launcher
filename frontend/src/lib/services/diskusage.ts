import { Service as DiskUsageService } from '../../../bindings/typhon/internal/diskusage';
import { inWails } from './backend';

export type LeftoverKind =
  | 'install_partial'
  | 'download_part'
  | 'relocate_staging'
  | 'update_staging'
  | 'update_previous'
  | 'update_replaced'
  | 'download_orphan';

export interface GameUsage {
  id: string;
  title: string;
  installDir: string;
  sizeBytes: number;
  sizeUnknown: boolean;
  missing: boolean;
  external: boolean;
  linked: boolean;
  lastPlayed: string | null;
  installedAt: string;
  running: boolean;
  busy: boolean;
}

export interface DownloadUsage {
  id: string;
  name: string;
  root: string;
  sizeBytes: number;
  sizeUnknown: boolean;
  missing: boolean;
  seeding: boolean;
  installed: boolean;
  gameId: string;
  referenced: boolean;
  reason: string;
  reasonCode: string;
}

export interface Leftover {
  path: string;
  kind: string;
  sizeBytes: number;
  sizeUnknown: boolean;
  deletable: boolean;
  reason: string;
  reasonCode: string;
}

export interface GroupUsage {
  gameId: string;
  title: string;
  inLibrary: boolean;
  count: number;
  sizeBytes: number;
  sizeUnknown: boolean;
}

export interface Warning {
  area: string;
  message: string;
}

export interface Totals {
  gamesBytes: number;
  downloadsBytes: number;
  leftoversBytes: number;
  screenshotsBytes: number;
  snapshotsBytes: number;
  totalBytes: number;
  reclaimableBytes: number;
  unknownSizes: boolean;
}

export interface Report {
  games: GameUsage[];
  downloads: DownloadUsage[];
  leftovers: Leftover[];
  screenshots: GroupUsage[];
  snapshots: GroupUsage[];
  warnings: Warning[];
  totals: Totals;
  scannedAt: string;
}

const unavailable = () => new Error('unavailable in browser');

type Raw = {
  games: unknown[] | null;
  downloads: unknown[] | null;
  leftovers: unknown[] | null;
  screenshots: unknown[] | null;
  snapshots: unknown[] | null;
  warnings: unknown[] | null;
  totals: unknown;
  scannedAt: string;
};

function normalize(raw: Raw): Report {
  return {
    games: (raw.games ?? []) as GameUsage[],
    downloads: (raw.downloads ?? []) as DownloadUsage[],
    leftovers: (raw.leftovers ?? []) as Leftover[],
    screenshots: (raw.screenshots ?? []) as GroupUsage[],
    snapshots: (raw.snapshots ?? []) as GroupUsage[],
    warnings: (raw.warnings ?? []) as Warning[],
    totals: raw.totals as Totals,
    scannedAt: raw.scannedAt,
  };
}

export async function cachedUsage(): Promise<Report | null> {
  if (!inWails) throw unavailable();
  const raw = await DiskUsageService.Cached();
  return raw ? normalize(raw as unknown as Raw) : null;
}

export async function scanUsage(): Promise<Report> {
  if (!inWails) throw unavailable();
  return normalize((await DiskUsageService.Scan()) as unknown as Raw);
}

export async function deleteLeftover(path: string): Promise<void> {
  if (!inWails) throw unavailable();
  await DiskUsageService.DeleteLeftover(path);
}

export async function deleteDownloadData(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await DiskUsageService.DeleteDownloadData(id);
}

export async function deleteScreenshots(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await DiskUsageService.DeleteScreenshots(gameId);
}

export async function deleteSnapshots(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await DiskUsageService.DeleteSnapshots(gameId);
}
