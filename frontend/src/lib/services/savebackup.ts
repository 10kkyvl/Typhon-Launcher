import { Events } from '@wailsio/runtime';
import { Service as SaveBackupService } from '../../../bindings/typhon/internal/savebackup';
import { inWails } from './backend';

export type SnapshotKind = 'manual' | 'session' | 'update' | 'pre-restore';
export type BackupStatus = 'created' | 'skipped' | 'failed' | 'deleted' | 'restored';

export interface Snapshot {
  id: string;
  gameId: string;
  kind: SnapshotKind;
  createdAt: string;
  sourcePath: string;
  sizeBytes: number;
  files: number;
  digest: string;
  path?: string;
  broken?: boolean;
  problem?: string;
}

export interface BackupEvent {
  gameId: string;
  kind: SnapshotKind;
  snapshot: Snapshot | null;
  status: BackupStatus;
  code: string;
  error: string;
}

const unavailable = () => new Error('unavailable in browser');

export async function backupsEnabled(): Promise<boolean> {
  if (!inWails) return false;
  return await SaveBackupService.Enabled();
}

export async function listBackups(gameId: string): Promise<Snapshot[]> {
  if (!inWails) throw unavailable();
  return ((await SaveBackupService.List(gameId)) ?? []) as unknown as Snapshot[];
}

export async function createBackup(gameId: string): Promise<Snapshot> {
  if (!inWails) throw unavailable();
  return (await SaveBackupService.Create(gameId)) as unknown as Snapshot;
}

export async function restoreBackup(gameId: string, snapshotId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await SaveBackupService.Restore(gameId, snapshotId);
}

export async function deleteBackup(gameId: string, snapshotId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await SaveBackupService.Delete(gameId, snapshotId);
}

export function onBackupEvent(handler: (event: BackupEvent) => void): () => void {
  if (!inWails) return () => {};
  return Events.On('saves:backups', (event) => handler(event.data as BackupEvent));
}
