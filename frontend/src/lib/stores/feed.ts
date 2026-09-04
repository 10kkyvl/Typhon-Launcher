import { get, writable } from 'svelte/store';
import { AccountError } from '../services/account';
import { accountErrorText } from '../services/accountMessages';
import {
  feed as fetchFeed,
  react as sendReact,
  unreact as sendUnreact,
  type FeedEvent,
} from '../services/social';
import { hasReacted, toggleReaction } from '../social/feed';
import { toast } from './toasts';

export const feedEvents = writable<FeedEvent[]>([]);
export const feedCursor = writable(0);
export const feedLoading = writable(false);
export const feedReady = writable(false);

const pending = new Set<string>();

function report(err: unknown, fallback: string) {
  if (err instanceof AccountError && err.code === 'unauthenticated') return;
  toast(accountErrorText(err, fallback), 'danger');
}

function replace(id: number, apply: (event: FeedEvent) => FeedEvent) {
  feedEvents.update((events) => events.map((event) => (event.id === id ? apply(event) : event)));
}

async function page(cursor: number, append: boolean): Promise<void> {
  if (get(feedLoading)) return;
  feedLoading.set(true);
  try {
    const loaded = await fetchFeed(cursor > 0 ? String(cursor) : '');
    feedEvents.update((events) => {
      if (!append) return loaded.events;
      const known = new Set(events.map((event) => event.id));
      return [...events, ...loaded.events.filter((event) => !known.has(event.id))];
    });
    feedCursor.set(loaded.next);
    feedReady.set(true);
  } catch (err) {
    report(err, 'Не удалось загрузить ленту');
  } finally {
    feedLoading.set(false);
  }
}

export async function loadFeed(force = false): Promise<void> {
  if (get(feedReady) && !force) return;
  await page(0, false);
}

export async function moreFeed(): Promise<void> {
  const cursor = get(feedCursor);
  if (cursor <= 0) return;
  await page(cursor, true);
}

export function resetFeed(): void {
  pending.clear();
  feedEvents.set([]);
  feedCursor.set(0);
  feedReady.set(false);
}

export async function reactToEvent(id: number, emoji: string): Promise<void> {
  const key = `${id}:${emoji}`;
  if (pending.has(key)) return;
  const current = get(feedEvents).find((event) => event.id === id);
  if (!current) return;
  const optimistic = toggleReaction(current, emoji);
  if (optimistic === current) return;
  const remove = hasReacted(current, emoji);
  pending.add(key);
  replace(id, () => optimistic);
  try {
    await (remove ? sendUnreact : sendReact)(String(id), emoji);
  } catch (err) {
    replace(id, (event) => (hasReacted(event, emoji) === remove ? event : toggleReaction(event, emoji)));
    report(err, 'Не удалось отправить реакцию');
  } finally {
    pending.delete(key);
  }
}
