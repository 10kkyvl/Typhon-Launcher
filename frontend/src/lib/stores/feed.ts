import { get, writable } from 'svelte/store';
import { AccountError } from '../services/account';
import { accountErrorText } from '../services/accountMessages';
import {
  feed as fetchFeed,
  react as sendReact,
  setNote as sendNote,
  unreact as sendUnreact,
  type FeedEvent,
} from '../services/social';
import { hasReacted, toggleReaction } from '../social/feed';
import { NOTE_LIMIT, trimNote } from '../social/note';
import { msg } from '../i18n';
import { toast } from './toasts';

export const feedEvents = writable<FeedEvent[]>([]);
export const feedCursor = writable(0);
export const feedLoading = writable(false);
export const feedReady = writable(false);
export const feedLoadedAt = writable(0);
export const feedPending = writable<ReadonlySet<number>>(new Set());

export const staleAfter = 60_000;

const pending = new Set<string>();

let generation = 0;

let inflight: Promise<void> | null = null;

function report(err: unknown, fallback: string) {
  if (err instanceof AccountError && err.code === 'unauthenticated') return;
  toast(accountErrorText(err, fallback), 'danger');
}

function replace(id: number, apply: (event: FeedEvent) => FeedEvent) {
  feedEvents.update((events) => events.map((event) => (event.id === id ? apply(event) : event)));
}

function publishPending() {
  const ids = new Set<number>();
  for (const key of pending) ids.add(Number(key.slice(0, key.indexOf(':'))));
  feedPending.set(ids);
}

function fresh(): boolean {
  return get(feedReady) && Date.now() - get(feedLoadedAt) < staleAfter;
}

async function page(cursor: number, append: boolean, owner: number): Promise<void> {
  feedLoading.set(true);
  try {
    const loaded = await fetchFeed(cursor > 0 ? String(cursor) : '');
    if (owner !== generation) return;
    feedEvents.update((events) => {
      if (!append) return loaded.events;
      const known = new Set(events.map((event) => event.id));
      return [...events, ...loaded.events.filter((event) => !known.has(event.id))];
    });
    feedCursor.set(loaded.next);
    feedLoadedAt.set(Date.now());
    feedReady.set(true);
  } catch (err) {
    if (owner !== generation) return;
    report(err, msg('state.feedLoadFailed'));
  } finally {
    if (owner === generation) feedLoading.set(false);
  }
}

function queue(cursor: number, append: boolean): Promise<void> {
  const owner = generation;
  const previous = inflight;
  const task = (async () => {
    if (previous) await previous;
    if (owner === generation) await page(cursor, append, owner);
  })();
  inflight = task;
  void task.finally(() => {
    if (inflight === task) inflight = null;
  });
  return task;
}

export async function loadFeed(force = false): Promise<void> {
  if (!force) {
    if (fresh()) return;
    if (inflight) return inflight;
  }
  await queue(0, false);
}

export async function moreFeed(): Promise<void> {
  const cursor = get(feedCursor);
  if (cursor <= 0) return;
  if (inflight) return inflight;
  await queue(cursor, true);
}

export function resetFeed(): void {
  generation++;
  inflight = null;
  feedLoading.set(false);
  pending.clear();
  publishPending();
  feedEvents.set([]);
  feedCursor.set(0);
  feedLoadedAt.set(0);
  feedReady.set(false);
}

export async function reactToEvent(id: number, emoji: string): Promise<void> {
  const owner = generation;
  const key = `${id}:${emoji}`;
  if (pending.has(key)) return;
  const current = get(feedEvents).find((event) => event.id === id);
  if (!current) return;
  const optimistic = toggleReaction(current, emoji);
  if (optimistic === current) return;
  const remove = hasReacted(current, emoji);
  pending.add(key);
  publishPending();
  replace(id, () => optimistic);
  try {
    await (remove ? sendUnreact : sendReact)(String(id), emoji);
  } catch (err) {
    if (owner !== generation) return;
    replace(id, (event) => (hasReacted(event, emoji) === remove ? event : toggleReaction(event, emoji)));
    report(err, msg('state.feedReactFailed'));
  } finally {
    if (owner === generation) {
      pending.delete(key);
      publishPending();
    }
  }
}

export async function noteEvent(id: number, note: string): Promise<void> {
  const current = get(feedEvents).find((event) => event.id === id);
  if (!current) return;
  const next = [...trimNote(note)].slice(0, NOTE_LIMIT).join('');
  if (next === current.note) return;
  const previous = current.note;
  const owner = generation;
  const key = `${id}:note`;
  if (pending.has(key)) return;
  pending.add(key);
  publishPending();
  replace(id, (event) => ({ ...event, note: next }));
  try {
    await sendNote(String(id), next);
  } catch (err) {
    if (owner !== generation) return;
    replace(id, (event) => ({ ...event, note: previous }));
    report(err, msg('state.feedNoteFailed'));
  } finally {
    if (owner === generation) {
      pending.delete(key);
      publishPending();
    }
  }
}
