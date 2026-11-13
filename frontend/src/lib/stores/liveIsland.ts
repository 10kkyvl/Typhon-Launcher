import type { LucideIcon } from '@lucide/svelte';
import { derived, get, writable, type Readable } from 'svelte/store';
import { RESUME_FLOOR_MS } from './toasts';

/**
 * The live island: one pill at the top centre of the window that says what just happened (a toast's
 * message, a failed download, a friend who started playing) while motion is on. This store holds its
 * messages; LiveIsland.svelte draws the current one and liveIslandFeed.ts posts them.
 *
 * - One message is shown at a time, the rest wait: by rank (an error, then a friend, then a plain
 *   note), and of one rank in the order they came, so a message on screen gives way only to a more
 *   important one, and comes back after it;
 * - `ttl` (ms) hides a message by itself, counted only while it is on screen and not being read
 *   (`hold`: the pill open, hovered or holding focus); after a hold it gets at least
 *   `RESUME_FLOOR_MS`, as a toast does. Without `ttl` it stays until `hide` (a toast: the toast store
 *   keeps its clock);
 * - while a dialog is open (`dialog`, told by the island) a message with an action does not run out,
 *   since the dialog's Tab trap keeps the keyboard from it; it gets its full time when the dialog
 *   closes (the toast store's rule for a toast with an action).
 *
 * `show` with an id already there replaces that message; `update` patches it in place.
 */

export type LiveIslandKind = 'error' | 'friend' | 'info';
export type LiveIslandTone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger';

export interface LiveIslandAction {
  label: string;
  run: () => void;
}

export interface LiveIslandMessage {
  id: string;
  /** Already translated and short: the pill's line. */
  text: string;
  /** Rank, see the header. Default `info`. */
  kind?: LiveIslandKind;
  tone?: LiveIslandTone;
  icon?: LucideIcon;
  /** A person's picture in place of the icon. */
  avatar?: string;
  /** The open card's title (a game's or a person's name); the pill's line when absent. */
  title?: string;
  /** The open card's line under the title. */
  detail?: string;
  /** The open card's buttons, at most two are drawn; the first is the primary one. */
  actions?: LiveIslandAction[];
  /** How many times a toast was raised, once it is more than one. */
  count?: number;
  ttl?: number;
}

const RANK: Record<LiveIslandKind, number> = { error: 0, friend: 1, info: 2 };

export function liveIslandRank(m: Pick<LiveIslandMessage, 'kind' | 'tone'>): number {
  const kind = m.kind ?? 'info';
  if (kind === 'info' && (m.tone === 'danger' || m.tone === 'warning')) return RANK.error;
  return RANK[kind];
}

const items = writable<LiveIslandMessage[]>([]);
const heldFlag = writable(false);
const claims = writable(0);

interface Life {
  ttl: number;
  remaining: number;
  startedAt: number;
  handle?: ReturnType<typeof setTimeout>;
}
const lives = new Map<string, Life>();
let held = false;
let dialogOpen = false;

/** Most important first; of one rank, the first that came. */
function order(list: readonly LiveIslandMessage[]): LiveIslandMessage[] {
  return list
    .map((m, i) => ({ m, i }))
    .sort((a, b) => liveIslandRank(a.m) - liveIslandRank(b.m) || a.i - b.i)
    .map((x) => x.m);
}

// `get`, not a subscription: a message posted from inside another store's subscriber would see a stale list.
const snapshot = () => get(items);

const waitsOutDialog = (m: LiveIslandMessage | undefined) => dialogOpen && (m?.actions?.length ?? 0) > 0;

/** Runs the clock of the message on screen, unless it is being read or waits out a dialog; stops the rest. */
function sync(): void {
  const top = order(snapshot())[0];
  for (const [id, life] of lives) {
    const running = id === top?.id && !held && !waitsOutDialog(top);
    if (running && life.handle === undefined) {
      life.startedAt = Date.now();
      life.handle = setTimeout(() => hide(id), life.remaining);
    } else if (!running && life.handle !== undefined) {
      clearTimeout(life.handle);
      life.handle = undefined;
      life.remaining = Math.max(0, life.remaining - (Date.now() - life.startedAt));
    }
  }
}

function forget(id: string): void {
  const life = lives.get(id);
  if (life?.handle !== undefined) clearTimeout(life.handle);
  lives.delete(id);
}

function arm(id: string, ttl: number | undefined): void {
  forget(id);
  if (ttl && ttl > 0) lives.set(id, { ttl, remaining: ttl, startedAt: 0 });
}

function show(message: LiveIslandMessage): void {
  items.update((list) => [...list.filter((m) => m.id !== message.id), message]);
  arm(message.id, message.ttl);
  sync();
}

/** A patch with `ttl` restarts its timer. Nothing for an unknown id. */
function update(id: string, patch: Partial<Omit<LiveIslandMessage, 'id'>>): void {
  if (!has(id)) return;
  items.update((list) => list.map((m) => (m.id === id ? { ...m, ...patch } : m)));
  if ('ttl' in patch) arm(id, patch.ttl);
  sync();
}

function hide(id: string): void {
  forget(id);
  items.update((list) => (list.some((m) => m.id === id) ? list.filter((m) => m.id !== id) : list));
  sync();
}

function has(id: string): boolean {
  return snapshot().some((m) => m.id === id);
}

function hold(on: boolean): void {
  if (held === on) return;
  held = on;
  heldFlag.set(on);
  if (!on) for (const life of lives.values()) life.remaining = Math.max(life.remaining, RESUME_FLOOR_MS);
  sync();
}

function dialog(open: boolean): void {
  if (dialogOpen === open) return;
  dialogOpen = open;
  if (!open) {
    for (const message of snapshot()) {
      const life = lives.get(message.id);
      if (life && message.actions?.length && life.handle === undefined) life.remaining = life.ttl;
    }
  }
  sync();
}

function clear(): void {
  for (const id of [...lives.keys()]) forget(id);
  held = false;
  dialogOpen = false;
  heldFlag.set(false);
  items.set([]);
}

export const liveIsland = { show, update, hide, has, hold, dialog, clear };

/** Every message the island holds, in the order they came. */
export const liveIslandMessages: Readable<LiveIslandMessage[]> = { subscribe: items.subscribe };
/** The message on screen. */
export const liveIslandCurrent: Readable<LiveIslandMessage | undefined> = derived(items, (list) => order(list)[0]);
/** The reader holds the island: open, hovered or holding focus. */
export const liveIslandHeld: Readable<boolean> = { subscribe: heldFlag.subscribe };

/** True while the island is mounted to show messages: what would otherwise be a toast goes to it. */
export const liveIslandOn: Readable<boolean> = derived(claims, (n) => n > 0);

/** The island is showing messages from now on; the returned function lets go. */
export function claimLiveIsland(): () => void {
  claims.update((n) => n + 1);
  let released = false;
  return () => {
    if (released) return;
    released = true;
    claims.update((n) => n - 1);
  };
}
