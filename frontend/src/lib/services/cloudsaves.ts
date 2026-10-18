import { Events } from '@wailsio/runtime';
import { Service as CloudsavesService } from '../../../bindings/typhon/internal/cloudsaves';
import { inWails } from './backend';

export type CloudState = 'synced' | 'pending' | 'conflict' | 'failed' | 'skipped';

export interface CloudConflict {
  localDigest: string;
  localUpdatedAt: string;
  localSize: number;
  remoteDigest: string;
  remoteUpdatedAt: string;
  remoteSize: number;
  remoteDevice: string;
}

export interface CloudGameStatus {
  gameId: string;
  state: CloudState;
  code: string;
  message: string;
  at: string;
  syncedAt: string;
  conflict?: CloudConflict;
}

export interface CloudStatus {
  enabled: boolean;
  signedIn: boolean;
  usedBytes: number;
  quotaBytes: number;
  maxBytes: number;
  error: string;
  pending?: boolean;
  games: CloudGameStatus[];
}

export interface CloudEvent {
  gameId: string;
  status: CloudState;
  code: string;
  message: string;
  usedBytes: number;
  quotaBytes: number;
}

const unavailable = () => new Error('unavailable in browser');

export async function loadCloudStatus(): Promise<CloudStatus> {
  if (!inWails) throw unavailable();
  const status = (await CloudsavesService.Status()) as unknown as CloudStatus;
  return { ...status, games: status.games ?? [] };
}

export async function cloudSyncNow(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await CloudsavesService.SyncNow(gameId);
}

export async function cloudResolveConflict(gameId: string, keepLocal: boolean): Promise<void> {
  if (!inWails) throw unavailable();
  await CloudsavesService.ResolveConflict(gameId, keepLocal);
}

export async function cloudPullRemote(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await CloudsavesService.PullRemote(gameId);
}

export async function cloudDeleteRemote(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await CloudsavesService.DeleteRemote(gameId);
}

export async function cloudForget(): Promise<void> {
  if (!inWails) throw unavailable();
  await CloudsavesService.Forget();
}

export function onCloudEvent(handler: (event: CloudEvent) => void): () => void {
  if (!inWails) return () => {};
  return Events.On('saves:cloud', (event) => handler(event.data as CloudEvent));
}
