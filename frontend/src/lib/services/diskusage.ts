import { Service as DiskUsageService } from '../../../bindings/typhon/internal/diskusage';
import { inWails } from './backend';

export type LeftoverKind =
  | 'install_partial'
  | 'download_part'
  | 'relocate_staging'
  | 'update_staging'
  | 'update_previous'
  | 'update_replaced'
  | 'download_orphan'
  | 'relocate_copy_staging'
  | 'relocate_copy_target'
  | 'relocate_copy_source'
  | 'removal';

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
  /** Go's words, for logs: the window says `reasonCode`. */
  reason: string;
  reasonCode: string;
}

/** A folder or file the launcher left. `gameId` and `title` name the game a move's copy is of. */
export interface Leftover {
  path: string;
  kind: string;
  gameId?: string;
  title?: string;
  sizeBytes: number;
  sizeUnknown: boolean;
  deletable: boolean;
  /** Go's words, for logs: the window says `reasonCode`. */
  reason: string;
  reasonCode: string;
}

/** A game's screenshots or save snapshots; `titleUnknown` when nothing knows the game's name. */
export interface GroupUsage {
  gameId: string;
  title: string;
  titleUnknown: boolean;
  inLibrary: boolean;
  count: number;
  sizeBytes: number;
  sizeUnknown: boolean;
}

/** What deleting a whole group did with the items the last scan counted in it. */
export interface GroupDelete {
  deleted: number;
  gone: number;
  failed: number;
  code: '' | 'diskusage.delete_partial';
}

/** A part of the scan that failed. `message` is Go's cause, for logs: the window says `code`. */
export interface Warning {
  area: string;
  code: string;
  message: string;
}

/** The sections' sums; each byte is counted once, in the innermost row that holds it. */
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
  /** When the walk that measured this report started. */
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

function groupDelete(raw: { deleted: number; gone: number; failed: number; code: string }): GroupDelete {
  return { deleted: raw.deleted, gone: raw.gone, failed: raw.failed, code: raw.code === 'diskusage.delete_partial' ? raw.code : '' };
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

/** Stops the running scan; its requests answer diskusage.scan_cancelled. Nothing runs: nothing happens. */
export async function cancelScan(): Promise<void> {
  if (!inWails) throw unavailable();
  await DiskUsageService.CancelScan();
}

export async function deleteLeftover(path: string): Promise<void> {
  if (!inWails) throw unavailable();
  await DiskUsageService.DeleteLeftover(path);
}

export async function deleteDownloadData(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await DiskUsageService.DeleteDownloadData(id);
}

export async function deleteScreenshots(gameId: string): Promise<GroupDelete> {
  if (!inWails) throw unavailable();
  return groupDelete(await DiskUsageService.DeleteScreenshots(gameId));
}

export async function deleteSnapshots(gameId: string): Promise<GroupDelete> {
  if (!inWails) throw unavailable();
  return groupDelete(await DiskUsageService.DeleteSnapshots(gameId));
}
