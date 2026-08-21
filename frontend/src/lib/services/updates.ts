import { Service as UpdatesService } from '../../../bindings/typhon/internal/updates';
import { inWails } from './backend';

export type UpdateState =
  | 'idle'
  | 'update_available'
  | 'update_downloading'
  | 'update_ready'
  | 'updating'
  | 'update_failed'
  | 'update_rollback';

export type AvailabilityKind = 'none' | 'update' | 'new_release';
export type StrategyType = '' | 'full_release' | 'torrent_reuse' | 'patch_chain';
export type VerifyMethod = 'torrent' | 'manifest' | 'unavailable';
export type StepKind =
  | 'download'
  | 'recheck'
  | 'apply_patch'
  | 'extract'
  | 'install'
  | 'verify'
  | 'swap'
  | 'cleanup';

export interface UpdateAvailability {
  available: boolean;
  kind: AvailabilityKind;
  gameId: string;
  installedReleaseId: string;
  targetReleaseId: string;
  installedVersion: string;
  targetVersion: string;
  confidence: number;
  strategy: StrategyType;
  estimatedDownloadBytes: number;
  requiresFullInstall: boolean;
  patchCount: number;
  reason?: string;
  targetSize: number;
}

export interface UpdateStep {
  kind: StepKind;
  label: string;
  releaseId?: string;
  patchId?: string;
  bytes?: number;
  fromVersion?: string;
  toVersion?: string;
}

export interface UpdatePlan {
  id: string;
  gameId: string;
  installedReleaseId: string;
  targetReleaseId: string;
  installedVersion: string;
  targetVersion: string;
  strategy: StrategyType;
  steps: UpdateStep[];
  downloadBytes: number;
  reusedBytes: number;
  requiredDiskBytes: number;
  backupRecommended: boolean;
  backupAvailable: boolean;
  backupCreated: boolean;
  requiresRestart: boolean;
  rollbackAvailable: boolean;
  confidence: number;
  createdAt: string;
}

export interface Update {
  gameId: string;
  title: string;
  state: UpdateState;
  availability: UpdateAvailability;
  plan?: UpdatePlan | null;
  planning: boolean;
  downloadId?: string;
  installId?: string;
  step?: StepKind;
  progress: number;
  message?: string;
  error?: string;
  canRollback: boolean;
  checkedAt: string;
}

export interface ManifestIssue {
  path: string;
  kind: 'missing' | 'corrupted' | 'size';
}

export interface VerifyState {
  gameId: string;
  method: VerifyMethod;
  running: boolean;
  repairing: boolean;
  progress: number;
  currentFile?: string;
  ratio: number;
  totalBytes: number;
  okBytes: number;
  missingFiles: number;
  corruptedPieces: number;
  issues?: ManifestIssue[];
  extra?: string[];
  repairable: boolean;
  flat?: boolean;
  infoHash?: string;
  checkedAt: string | null;
  error?: string;
}

export interface UpdateHistory {
  id: string;
  gameId: string;
  fromVersion: string;
  toVersion: string;
  strategy: string;
  downloadBytes: number;
  startedAt: string;
  completedAt: string | null;
  status: string;
  error?: string;
}

const unavailable = () => new Error('unavailable in browser');

export async function listUpdates(): Promise<Update[]> {
  if (!inWails) return [];
  return ((await UpdatesService.GetUpdates()) ?? []) as unknown as Update[];
}

export async function checkUpdates(): Promise<Update[]> {
  if (!inWails) return [];
  return ((await UpdatesService.CheckUpdates()) ?? []) as unknown as Update[];
}

export async function checkGameUpdate(gameId: string): Promise<Update> {
  if (!inWails) throw unavailable();
  return (await UpdatesService.CheckGame(gameId)) as unknown as Update;
}

export async function prepareUpdatePlan(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.PreparePlan(gameId);
}

export async function startUpdate(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.StartUpdate(gameId);
}

export async function prefetchUpdate(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.PrefetchUpdate(gameId);
}

export async function cancelUpdate(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.CancelUpdate(gameId);
}

export async function rollbackUpdate(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.Rollback(gameId);
}

export async function verifyGame(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.VerifyGame(gameId);
}

export async function repairGame(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.RepairGame(gameId);
}

export async function buildManifest(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await UpdatesService.BuildManifest(gameId);
}

export async function getVerifyState(gameId: string): Promise<VerifyState | null> {
  if (!inWails) return null;
  return (await UpdatesService.GetVerifyState(gameId)) as unknown as VerifyState;
}

export async function getUpdateHistory(gameId: string): Promise<UpdateHistory[]> {
  if (!inWails) return [];
  return ((await UpdatesService.GetHistory(gameId)) ?? []) as unknown as UpdateHistory[];
}
