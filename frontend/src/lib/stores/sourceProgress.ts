import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { isRefreshStage, type RefreshStage, type SourceProgress } from '../sources/progress';

export const sourceProgress = writable<Record<string, SourceProgress>>({});

export const clearDelayMs = 10_000;

interface ProgressEvent {
  sourceId: string;
  stage: string;
  done: number;
  total: number;
}

interface SourceEvent {
  id: string;
  name?: string;
  status?: string;
}

interface SourceErrorEvent {
  sourceId: string;
}

const timers = new Map<string, ReturnType<typeof setTimeout>>();

function cancelTimer(id: string) {
  const timer = timers.get(id);
  if (timer === undefined) return;
  clearTimeout(timer);
  timers.delete(id);
}

export function clearSourceProgress(id: string) {
  cancelTimer(id);
  sourceProgress.update((map) => {
    if (!(id in map)) return map;
    const next = { ...map };
    delete next[id];
    return next;
  });
}

export function applyProgress(event: ProgressEvent, now: number = Date.now()) {
  if (!event.sourceId || !isRefreshStage(event.stage)) return;
  const stage: RefreshStage = event.stage;
  cancelTimer(event.sourceId);
  sourceProgress.update((map) => {
    const prev = map[event.sourceId];
    return {
      ...map,
      [event.sourceId]: {
        stage,
        done: event.done,
        total: event.total,
        startedAt: prev?.startedAt ?? now,
        sawMatch: stage === 'match' || (prev?.sawMatch ?? false),
      },
    };
  });
  if (stage === 'save' && event.done >= event.total) {
    timers.set(
      event.sourceId,
      setTimeout(() => clearSourceProgress(event.sourceId), clearDelayMs),
    );
  }
}

export function initSourceProgress() {
  if (!inWails) return;
  Events.On('sources:progress', (event) => applyProgress(event.data as ProgressEvent));
  Events.On('source:updated', (event) => {
    const item = event.data as SourceEvent;
    if (!item.name || item.status !== 'updating') clearSourceProgress(item.id);
  });
  Events.On('source:error', (event) => clearSourceProgress((event.data as SourceErrorEvent).sourceId));
}
