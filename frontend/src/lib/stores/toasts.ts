import { writable, type Readable } from 'svelte/store';

/**
 * Toasts say that something happened ("Saved", "Could not start the game"). The rules:
 *
 * - a plain one stays `PLAIN_MS`, a warning or an error `URGENT_MS`, one with an action `ACTION_MS`;
 * - the clock stops while the toast is being read (`pauseToast`: the pointer over it, focus inside it)
 *   and gives at least `RESUME_FLOOR_MS` after `resumeToast`;
 * - the same message raised again does not stack: the one already shown or waiting takes it;
 * - at most `MAX_VISIBLE` are on screen, the rest wait in the order raised;
 * - while a dialog is open (`holdToasts(true)`, told by the host: this store knows nothing of layers)
 *   a toast with an action does not run out, since its action cannot be reached by Tab from inside
 *   the dialog. Its clock starts when the dialog closes. While held it takes none of the
 *   `MAX_VISIBLE` slots, so what the dialog itself raises is still shown at once.
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
}

export const PLAIN_MS = 4000;
export const URGENT_MS = 6000;
export const ACTION_MS = 9000;
export const RESUME_FLOOR_MS = 1500;
export const MAX_VISIBLE = 2;

export function toastDuration(item: Pick<Toast, 'kind' | 'action'>): number {
  if (item.action) return ACTION_MS;
  return item.kind === 'danger' || item.kind === 'warning' ? URGENT_MS : PLAIN_MS;
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
const clocks = new Map<number, Clock>();

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

// A toast held by the dialog has no clock, so it takes no slot: two of them would otherwise keep an
// error raised by the dialog's own action waiting until the dialog closed.
function promote(): void {
  let room = MAX_VISIBLE - shown.filter((item) => !heldByDialog(item, dialogOpen)).length;
  const next: Toast[] = [];
  while (queue.length > 0) {
    const held = heldByDialog(queue[0], dialogOpen);
    if (!held && room <= 0) break;
    const item = queue.shift() as Toast;
    if (!held) room -= 1;
    next.push(item);
  }
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
  const same = (item: Toast) => item.message === message && item.kind === kind && item.action?.label === action?.label;
  const repeated = shown.find(same);
  if (repeated) {
    const clock = clocks.get(repeated.id);
    if (clock) {
      stop(clock);
      clock.remaining = toastDuration(repeated);
      run(repeated, clock);
    }
    return repeated.id;
  }
  const waiting = queue.find(same);
  if (waiting) return waiting.id;
  const id = nextId++;
  queue.push(action ? { id, message, kind, action } : { id, message, kind });
  promote();
  return id;
}

export function dismissToast(id: number): void {
  const clock = clocks.get(id);
  if (clock) clearTimeout(clock.handle);
  clocks.delete(id);
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

/** The toasts waiting for room on screen, in order. */
export function queuedToasts(): readonly Toast[] {
  return queue;
}
