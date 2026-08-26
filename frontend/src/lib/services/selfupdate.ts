import { Service as SelfUpdateService } from '../../../bindings/typhon/internal/selfupdate';
import { inWails } from './backend';

export type SelfUpdateState = 'idle' | 'checking' | 'available' | 'downloading' | 'ready' | 'applying' | 'failed';

export interface SelfUpdateStatus {
  state: SelfUpdateState;
  currentVersion: string;
  availableVersion?: string;
  notes?: string;
  publishedAt?: string;
  totalBytes?: number;
  downloadedBytes?: number;
  checkedAt?: string;
  error?: string;
  errorCode?: string;
}

export interface SelfUpdateProgress {
  version: string;
  totalBytes: number;
  downloadedBytes: number;
}

export interface SelfUpdateOutcome {
  version: string;
  ok: boolean;
  error?: string;
  finishedAt: string;
}

const SENTINEL_CODES: Record<string, string> = {
  'selfupdate: another update operation is in progress': 'busy',
  'selfupdate: no verified update is ready to apply': 'not_ready',
  'selfupdate: check for an update before downloading': 'not_checked',
};

export class SelfUpdateError extends Error {
  code: string;

  constructor(message: string, code = SENTINEL_CODES[message] ?? 'unknown') {
    super(message);
    this.name = 'SelfUpdateError';
    this.code = code;
  }
}

function toSelfUpdateError(err: unknown): SelfUpdateError {
  if (err instanceof SelfUpdateError) return err;
  const raw = err instanceof Error ? err.message : String(err);
  return new SelfUpdateError(raw);
}

const unavailable = () => new SelfUpdateError('unavailable in browser', 'unavailable');

function idleStatus(): SelfUpdateStatus {
  return { state: 'idle', currentVersion: '' };
}

export async function getStatus(): Promise<SelfUpdateStatus> {
  if (!inWails) return idleStatus();
  try {
    return (await SelfUpdateService.GetStatus()) as unknown as SelfUpdateStatus;
  } catch (err) {
    throw toSelfUpdateError(err);
  }
}

export async function getOutcome(): Promise<SelfUpdateOutcome | null> {
  if (!inWails) return null;
  try {
    const outcome = (await SelfUpdateService.GetOutcome()) as unknown as SelfUpdateOutcome;
    return outcome?.version ? outcome : null;
  } catch (err) {
    throw toSelfUpdateError(err);
  }
}

export async function checkForUpdate(): Promise<SelfUpdateStatus> {
  if (!inWails) throw unavailable();
  try {
    return (await SelfUpdateService.CheckForUpdate()) as unknown as SelfUpdateStatus;
  } catch (err) {
    throw toSelfUpdateError(err);
  }
}

export async function downloadUpdate(): Promise<SelfUpdateStatus> {
  if (!inWails) throw unavailable();
  try {
    return (await SelfUpdateService.DownloadUpdate()) as unknown as SelfUpdateStatus;
  } catch (err) {
    throw toSelfUpdateError(err);
  }
}

export async function applyUpdate(): Promise<void> {
  if (!inWails) throw unavailable();
  try {
    await SelfUpdateService.ApplyUpdate();
  } catch (err) {
    throw toSelfUpdateError(err);
  }
}

export async function dismissUpdate(): Promise<void> {
  if (!inWails) throw unavailable();
  try {
    await SelfUpdateService.DismissUpdate();
  } catch (err) {
    throw toSelfUpdateError(err);
  }
}
