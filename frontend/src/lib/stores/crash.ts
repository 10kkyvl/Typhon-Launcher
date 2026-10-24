import { get, writable } from 'svelte/store';
import { CRASH_DETAIL_WINDOW_MS, mergeCrash, recordFromCrash, recordFromStopped, type CrashRecord } from '../redist/crashView';
import type { CrashEvent, SessionEvent } from '../services/library';

export type { CrashRecord };

export const CRASH_QUIET_MS = 5 * 60 * 1000;

export const crashTarget = writable<CrashRecord | null>(null);

const counts = new Map<string, number>();
const shownAt = new Map<string, number>();
const stoppedAt = new Map<string, number>();

export function recordCrash(event: SessionEvent, now: number = Date.now()): CrashRecord {
  const count = (counts.get(event.gameId) ?? 0) + 1;
  counts.set(event.gameId, count);
  const record = recordFromStopped(event, count, now);
  stoppedAt.set(event.gameId, now);
  const open = get(crashTarget);
  if (open && open.gameId === event.gameId) {
    crashTarget.set(record);
    shownAt.set(event.gameId, now);
    return record;
  }
  const last = shownAt.get(event.gameId);
  if (last !== undefined && now - last < CRASH_QUIET_MS) return record;
  shownAt.set(event.gameId, now);
  crashTarget.set(record);
  return record;
}

export function recordCrashDetail(event: CrashEvent, now: number = Date.now()): CrashRecord | null {
  const open = get(crashTarget);
  if (open && open.gameId === event.gameId) {
    const merged = mergeCrash(open, event);
    crashTarget.set(merged);
    return merged;
  }
  const stopped = stoppedAt.get(event.gameId);
  if (stopped !== undefined && now - stopped < CRASH_DETAIL_WINDOW_MS) return null;
  const count = (counts.get(event.gameId) ?? 0) + 1;
  counts.set(event.gameId, count);
  const record = recordFromCrash(event, count, now);
  const last = shownAt.get(event.gameId);
  if (last !== undefined && now - last < CRASH_QUIET_MS) return record;
  shownAt.set(event.gameId, now);
  crashTarget.set(record);
  return record;
}

export function clearCrashes(gameId: string) {
  counts.delete(gameId);
}

export function closeCrash() {
  crashTarget.set(null);
}

export function resetCrashes() {
  counts.clear();
  shownAt.clear();
  stoppedAt.clear();
  crashTarget.set(null);
}
