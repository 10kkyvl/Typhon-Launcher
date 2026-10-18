import { get, writable } from 'svelte/store';
import type { SessionEvent } from '../services/library';

export const CRASH_QUIET_MS = 5 * 60 * 1000;

export interface CrashRecord {
  gameId: string;
  exitCode: number;
  exitKnown: boolean;
  reasonCode: string;
  sessionSeconds: number;
  at: number;
  count: number;
}

export const crashTarget = writable<CrashRecord | null>(null);

const counts = new Map<string, number>();
const shownAt = new Map<string, number>();

export function recordCrash(event: SessionEvent, now: number = Date.now()): CrashRecord {
  const count = (counts.get(event.gameId) ?? 0) + 1;
  counts.set(event.gameId, count);
  const record: CrashRecord = {
    gameId: event.gameId,
    exitCode: event.exitCode,
    exitKnown: event.exitKnown,
    reasonCode: event.reasonCode,
    sessionSeconds: event.sessionSeconds,
    at: now,
    count,
  };
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

export function clearCrashes(gameId: string) {
  counts.delete(gameId);
}

export function closeCrash() {
  crashTarget.set(null);
}

export function resetCrashes() {
  counts.clear();
  shownAt.clear();
  crashTarget.set(null);
}
