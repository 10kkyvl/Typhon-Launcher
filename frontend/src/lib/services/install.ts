import { Service as InstallService } from '../../../bindings/typhon/internal/install';
import { inWails } from './backend';

export type InstallType =
  | 'portable'
  | 'archive_zip'
  | 'archive_7z'
  | 'archive_rar'
  | 'exe_installer'
  | 'msi_installer'
  | 'unknown';

export type InstallStatus =
  | 'pending'
  | 'preparing'
  | 'installing'
  | 'extracting'
  | 'verifying'
  | 'waiting_for_user'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted';

export type InstallMode = 'move' | 'copy';

export interface Candidate {
  path: string;
  score: number;
}

export interface Plan {
  type: InstallType;
  sourcePath: string;
  contentRoot: string;
  destination: string;
  installerPath: string;
  workingDir: string;
  archivePath: string;
  compressedSize: number;
  estimatedSize: number;
  requiresUserInteraction: boolean;
  canAutoInstall: boolean;
  candidates: Candidate[] | null;
}

export interface PlanInfo {
  plan: Plan;
  downloadId: string;
  name: string;
  requiredBytes: number;
  freeBytes: number;
  seeding: boolean;
}

export interface Installation {
  id: string;
  downloadId: string;
  gameId: string;
  name: string;
  type: InstallType;
  status: InstallStatus;
  mode: string;
  sourcePath: string;
  contentRoot: string;
  destination: string;
  installerPath: string;
  workingDir: string;
  archivePath: string;
  progress: number;
  currentFile: string;
  bytesDone: number;
  bytesTotal: number;
  executable: string;
  candidates: Candidate[] | null;
  detectedVersion: string;
  versionSource: string;
  startedAt: string;
  completedAt: string | null;
  error: string;
}

export interface StartOptions {
  destination?: string;
  mode?: string;
  type?: InstallType;
  installerPath?: string;
}

const unavailable = () => new Error('unavailable in browser');

export const controlledTypes: InstallType[] = ['portable', 'archive_zip', 'archive_7z', 'archive_rar'];
export const externalTypes: InstallType[] = ['exe_installer', 'msi_installer'];

export function isControlled(type: InstallType) {
  return controlledTypes.includes(type);
}

export function isExternal(type: InstallType) {
  return externalTypes.includes(type);
}

export async function listInstallations(): Promise<Installation[]> {
  if (!inWails) return [];
  return ((await InstallService.List()) ?? []) as unknown as Installation[];
}

export async function inspectDownload(downloadID: string): Promise<PlanInfo> {
  if (!inWails) throw unavailable();
  return (await InstallService.InspectDownload(downloadID)) as unknown as PlanInfo;
}

export async function startInstall(downloadID: string, opts: StartOptions): Promise<Installation> {
  if (!inWails) throw unavailable();
  const payload = {
    destination: opts.destination ?? '',
    mode: opts.mode ?? '',
    type: opts.type ?? '',
    installerPath: opts.installerPath ?? '',
  };
  return (await InstallService.Start(downloadID, payload as never)) as unknown as Installation;
}

export async function cancelInstall(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await InstallService.Cancel(id);
}

export async function confirmExecutable(id: string, executable: string): Promise<void> {
  if (!inWails) throw unavailable();
  await InstallService.ConfirmExecutable(id, executable);
}

export async function retryInstall(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await InstallService.Retry(id);
}

export async function dismissInstall(id: string): Promise<void> {
  if (!inWails) throw unavailable();
  await InstallService.Dismiss(id);
}

export async function deleteDownloadData(downloadID: string): Promise<void> {
  if (!inWails) throw unavailable();
  await InstallService.DeleteDownloadData(downloadID);
}
