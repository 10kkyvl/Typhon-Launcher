import { writable, type Readable } from 'svelte/store';

/**
 * Toasts say that something happened ("Saved", "Could not start the game"). The rules:
 *
 * - a plain one stays `PLAIN_MS`, a warning or an error `URGENT_MS`, one with an action `ACTION_MS`;
 * - the clock stops while the toast is being read (`pauseToast`: the pointer over it, focus inside it)
 *   and gives at least `RESUME_FLOOR_MS` after `resumeToast`;
 * - the same plain message raised again does not stack: the one already shown or waiting takes it
 *   and counts the repeats (`count`), so a second failure is not mistaken for none.
 *   A toast with an action is always its own: the same words can offer different things;
 * - at most `MAX_VISIBLE` are on screen, the rest wait: errors and warnings ahead of plain ones, each
 *   group in the order raised. A plain one that has waited `QUEUE_STALE_MS` is dropped at its turn
 *   (a burst of twenty would otherwise show old news for a minute); an error, a warning or a toast
 *   with an action is never dropped;
 * - while a dialog is open (`holdToasts(true)`, told by the host: this store knows nothing of layers)
 *   a toast with an action does not run out, since its action cannot be reached by Tab from inside
 *   the dialog. Its clock starts when the dialog closes. While held it takes none of the
 *   `MAX_VISIBLE` slots, so what the dialog itself raises is still shown at once; but only `MAX_HELD`
 *   held one is shown at a time and never more than `MAX_VISIBLE + MAX_HELD` toasts in all: ten
 *   offers raised under a long-open dialog wait their turn (none is dropped or folded) and do not
 *   hold up the plain ones and the errors raised after them;
 * - while the live island shows the toasts (`toastsInIsland(true)`, motion on in the main window) it
 *   shows one at a time and ranks them itself, so every toast is on the list at once: none waits
 *   for a slot and none goes stale. The island keeps the clocks of the ones it does not show paused.
 */

export type ToastKind = 'info' | 'success' | 'warning' | 'danger';

export interface ToastAction {
  label: string;
  run: () => void;
}

export interface Toast {
  id: number;
  message: string;
  kind: ToastKind;
  action?: ToastAction;
  /** How many times a plain toast was raised, once it is more than one. */
  count?: number;
}

export const PLAIN_MS = 4000;
export const URGENT_MS = 6000;
export const ACTION_MS = 9000;
export const RESUME_FLOOR_MS = 1500;
export const MAX_VISIBLE = 2;
/** How many toasts held by a dialog are shown past the `MAX_VISIBLE` slots. */
export const MAX_HELD = 1;
/** A plain toast that has waited this long for a slot is no longer news and is dropped at its turn. */
export const QUEUE_STALE_MS = 12_000;

const urgent = (item: Pick<Toast, 'kind'>) => item.kind === 'danger' || item.kind === 'warning';

export function toastDuration(item: Pick<Toast, 'kind' | 'action'>): number {
  if (item.action) return ACTION_MS;
  return urgent(item) ? URGENT_MS : PLAIN_MS;
}

/** A toast with an action waits out a dialog: the dialog's Tab trap keeps the keyboard away from it. */
export function heldByDialog(item: Pick<Toast, 'action'>, dialogOpen: boolean): boolean {
  return dialogOpen && item.action !== undefined;
}

interface Clock {
  handle: ReturnType<typeof setTimeout> | undefined;
  remaining: number;
  startedAt: number;
  /** Being read: the pointer is over the toast or focus is inside it. */
  paused: boolean;
}

const visible = writable<Toast[]>([]);
let shown: Toast[] = [];
let queue: Toast[] = [];
let nextId = 1;
let dialogOpen = false;
let slots = MAX_VISIBLE;
const clocks = new Map<number, Clock>();
/** When each waiting toast joined the queue, or was last raised again. */
const queuedAt = new Map<number, number>();

function publish(list: Toast[]): void {
  shown = list;
  visible.set(list);
}

function stop(clock: Clock): void {
  if (clock.handle === undefined) return;
  clearTimeout(clock.handle);
  clock.handle = undefined;
  clock.remaining = Math.max(0, clock.remaining - (Date.now() - clock.startedAt));
}

function run(item: Toast, clock: Clock): void {
  if (clock.handle !== undefined || clock.paused || heldByDialog(item, dialogOpen)) return;
  clock.startedAt = Date.now();
  clock.handle = setTimeout(() => dismissToast(item.id), clock.remaining);
}

// A toast held by the dialog has no clock, so it takes no slot: it would otherwise keep an error
// raised by the dialog's own action waiting until the dialog closed. A held one that has to wait
// does not hold up the ones behind it either.
function promote(): void {
  let held = shown.filter((item) => heldByDialog(item, dialogOpen)).length;
  let timed = shown.length - held;
  const next: Toast[] = [];
  const stay: Toast[] = [];
  for (const item of queue) {
    const holds = heldByDialog(item, dialogOpen);
    const fits = held + timed < slots + MAX_HELD && (holds ? held < MAX_HELD : timed < slots);
    if (!fits) {
      stay.push(item);
      continue;
    }
    const waited = Date.now() - (queuedAt.get(item.id) ?? Date.now());
    queuedAt.delete(item.id);
    // Only a plain message goes stale: an error must be seen, and an offer is not made twice.
    if (!urgent(item) && !item.action && waited > QUEUE_STALE_MS) continue;
    if (holds) held += 1;
    else timed += 1;
    next.push(item);
  }
  queue = stay;
  if (next.length === 0) return;
  for (const item of next) {
    const clock: Clock = { handle: undefined, remaining: toastDuration(item), startedAt: 0, paused: false };
    clocks.set(item.id, clock);
    run(item, clock);
  }
  publish([...shown, ...next]);
}

function reset(list: Toast[]): void {
  for (const clock of clocks.values()) clearTimeout(clock.handle);
  clocks.clear();
  queuedAt.clear();
  queue = [];
  publish(list);
}

/**
 * The toasts on screen, oldest first. Read it; raise and remove toasts with `toast` and
 * `dismissToast`. `set` exists for a clean slate (tests, a reset): it drops the waiting ones and every
 * clock, and a list given to it is shown as it is, with no clocks.
 */
export const toasts: Readable<Toast[]> & { set(list: Toast[]): void } = {
  subscribe: visible.subscribe,
  set: reset,
};

/** Shows a message. Returns the id `dismissToast` takes: of the new toast, or of the one that took a repeat. */
export function toast(message: string, kind: ToastKind = 'info', action?: ToastAction): number {
  // A toast with an action is never folded: the same words can offer different things (two games
  // that can each be marked as completed), and the second offer would be lost.
  const same = (item: Toast) => !action && !item.action && item.message === message && item.kind === kind;
  // A repeat is counted on the toast that takes it: folding must not hide that it happened again.
  const again = (item: Toast): Toast => (same(item) ? { ...item, count: (item.count ?? 1) + 1 } : item);
  const repeated = shown.find(same);
  if (repeated) {
    publish(shown.map(again));
    const clock = clocks.get(repeated.id);
    if (clock) {
      stop(clock);
      clock.remaining = toastDuration(repeated);
      run(repeated, clock);
    }
    return repeated.id;
  }
  const waiting = queue.find(same);
  if (waiting) {
    queue = queue.map(again);
    queuedAt.set(waiting.id, Date.now());
    return waiting.id;
  }
  const id = nextId++;
  const item: Toast = action ? { id, message, kind, action } : { id, message, kind };
  // An error or a warning goes ahead of the plain ones still waiting, behind the urgent ones.
  const behind = urgent(item) ? queue.findIndex((other) => !urgent(other)) : -1;
  queue.splice(behind < 0 ? queue.length : behind, 0, item);
  queuedAt.set(id, Date.now());
  promote();
  return id;
}

export function dismissToast(id: number): void {
  const clock = clocks.get(id);
  if (clock) clearTimeout(clock.handle);
  clocks.delete(id);
  queuedAt.delete(id);
  queue = queue.filter((item) => item.id !== id);
  if (shown.some((item) => item.id === id)) publish(shown.filter((item) => item.id !== id));
  promote();
}

/** The toast is being read: its clock stops. */
export function pauseToast(id: number): void {
  const clock = clocks.get(id);
  if (!clock || clock.paused) return;
  clock.paused = true;
  stop(clock);
}

/** The reader has left it: the clock goes on, with at least a moment left. */
export function resumeToast(id: number): void {
  const clock = clocks.get(id);
  const item = shown.find((candidate) => candidate.id === id);
  if (!clock || !item || !clock.paused) return;
  clock.paused = false;
  clock.remaining = Math.max(clock.remaining, RESUME_FLOOR_MS);
  run(item, clock);
}

/** The host says whether a dialog is open. Toasts with an action wait it out and then get their full time. */
export function holdToasts(open: boolean): void {
  if (open === dialogOpen) return;
  dialogOpen = open;
  for (const item of shown) {
    if (!item.action) continue;
    const clock = clocks.get(item.id);
    if (!clock) continue;
    if (open) {
      stop(clock);
    } else {
      clock.remaining = toastDuration(item);
      run(item, clock);
    }
  }
  promote();
}

/**
 * The live island takes the toasts over (true) or gives them back to the stack (false). Given back,
 * those that fit the stack's slots stay on screen, errors and warnings first, and the rest go back to
 * the front of the queue, as if they had been waiting there from now.
 */
export function toastsInIsland(on: boolean): void {
  slots = on ? Infinity : MAX_VISIBLE;
  if (!on) {
    let held = 0;
    let timed = 0;
    const keep = new Set<number>();
    for (const item of [...shown.filter(urgent), ...shown.filter((other) => !urgent(other))]) {
      const holds = heldByDialog(item, dialogOpen);
      if (holds ? held >= MAX_HELD : timed >= MAX_VISIBLE) continue;
      if (holds) held += 1;
      else timed += 1;
      keep.add(item.id);
    }
    const back = shown.filter((item) => !keep.has(item.id));
    if (back.length > 0) {
      for (const item of back) {
        clearTimeout(clocks.get(item.id)?.handle);
        clocks.delete(item.id);
        queuedAt.set(item.id, Date.now());
      }
      const all = [...back, ...queue];
      queue = [...all.filter(urgent), ...all.filter((other) => !urgent(other))];
      publish(shown.filter((item) => keep.has(item.id)));
    }
  }
  promote();
}

/** The toasts waiting for room on screen, in order. */
export function queuedToasts(): readonly Toast[] {
  return queue;
}
