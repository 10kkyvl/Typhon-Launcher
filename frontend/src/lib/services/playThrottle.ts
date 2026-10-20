import { Manager } from '../../../bindings/typhon/internal/download';
import { inWails } from './backend';

export type PlayDownloadMode = 'off' | 'pause' | 'limit';

export interface PlayThrottle {
  active: boolean;
  mode: PlayDownloadMode;
  downloadLimit: number;
  uploadLimit: number;
}

export const idlePlayThrottle: PlayThrottle = { active: false, mode: 'off', downloadLimit: 0, uploadLimit: 0 };

export const defaultPlayDownloadLimit = 5 * 1024 * 1024;
export const defaultPlayUploadLimit = 1024 * 1024;

export async function fetchPlayThrottle(): Promise<PlayThrottle> {
  if (!inWails) return idlePlayThrottle;
  return (await Manager.PlayThrottle()) as unknown as PlayThrottle;
}

export type PlayThrottleBanner =
  | { kind: 'pause' }
  | { kind: 'limit'; downloadLimit: number; uploadLimit: number };

export function playThrottleBanner(state: PlayThrottle): PlayThrottleBanner | null {
  if (!state.active) return null;
  if (state.mode === 'pause') return { kind: 'pause' };
  if (state.mode === 'limit') {
    return { kind: 'limit', downloadLimit: state.downloadLimit, uploadLimit: state.uploadLimit };
  }
  return null;
}
