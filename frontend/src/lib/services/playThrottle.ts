import { Manager } from '../../../bindings/typhon/internal/download';
import { clockTime, weekday } from '../utils/format';
import { inWails } from './backend';

export type PlayDownloadMode = 'off' | 'pause' | 'limit';

export interface ScheduleState {
  blocked: boolean;
  nextOpenAt: string;
}

export interface PlayThrottle {
  active: boolean;
  mode: PlayDownloadMode;
  downloadLimit: number;
  uploadLimit: number;
  schedule?: ScheduleState;
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
  | { kind: 'limit'; downloadLimit: number; uploadLimit: number }
  | { kind: 'schedule'; nextOpenAt: string; game: boolean };

export function playThrottleBanner(state: PlayThrottle): PlayThrottleBanner | null {
  const game = state.active && (state.mode === 'pause' || state.mode === 'limit');
  if (state.schedule?.blocked) return { kind: 'schedule', nextOpenAt: state.schedule.nextOpenAt, game };
  if (!state.active) return null;
  if (state.mode === 'pause') return { kind: 'pause' };
  if (state.mode === 'limit') {
    return { kind: 'limit', downloadLimit: state.downloadLimit, uploadLimit: state.uploadLimit };
  }
  return null;
}

export function scheduleOpenLabel(nextOpenAt: string, now: Date = new Date()): string {
  if (!nextOpenAt) return '';
  const at = new Date(nextOpenAt);
  if (Number.isNaN(at.getTime())) return '';
  const sameDay =
    at.getFullYear() === now.getFullYear() && at.getMonth() === now.getMonth() && at.getDate() === now.getDate();
  return sameDay ? clockTime(at) : `${weekday(at)} ${clockTime(at)}`;
}
