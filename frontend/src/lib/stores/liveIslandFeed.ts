import { CircleAlert, CircleCheck, Info, TriangleAlert } from '@lucide/svelte';
import { get } from 'svelte/store';
import { liveIsland, liveIslandCurrent, liveIslandHeld, type LiveIslandMessage, type LiveIslandTone } from './liveIsland';
import { dismissToast, pauseToast, resumeToast, toasts, toastsInIsland, type Toast, type ToastKind } from './toasts';

/**
 * What the live island is told, wired once by LiveIsland.svelte while it is mounted (motion on):
 *
 * - toasts: the toast store stays their source and keeps their clocks, the queue and the repeats; the
 *   island is the second way to show what it holds, and the stack shows none of them meanwhile. A
 *   toast's clock runs only while the island shows it and nobody reads it. When the island goes
 *   (motion off) the toasts it held go back to the stack with their clocks running.
 */

const TOAST_ICON = { info: Info, success: CircleCheck, warning: TriangleAlert, danger: CircleAlert } as const;
const TOAST_TONE: Record<ToastKind, LiveIslandTone> = { info: 'accent', success: 'success', warning: 'warning', danger: 'danger' };

const toastId = (n: number) => `toast:${n}`;

function fromToast(t: Toast): LiveIslandMessage {
  return {
    id: toastId(t.id),
    text: t.message,
    tone: TOAST_TONE[t.kind],
    icon: TOAST_ICON[t.kind],
    count: t.count,
    actions: t.action ? [{ label: t.action.label, run: t.action.run }] : undefined,
    close: () => dismissToast(t.id),
  };
}

function trackToasts(): () => void {
  const mirrored = new Map<number, Toast>();

  const sync = () => {
    const list = get(toasts);
    const present = new Set(list.map((t) => t.id));
    for (const n of [...mirrored.keys()]) {
      if (present.has(n)) continue;
      mirrored.delete(n);
      liveIsland.hide(toastId(n));
    }
    for (const t of list) {
      const before = mirrored.get(t.id);
      mirrored.set(t.id, t);
      if (!before) liveIsland.show(fromToast(t));
      else if (before.count !== t.count) liveIsland.update(toastId(t.id), { count: t.count });
    }
    const shown = get(liveIslandCurrent)?.id;
    const reading = get(liveIslandHeld);
    for (const t of list) {
      if (shown === toastId(t.id) && !reading) resumeToast(t.id);
      else pauseToast(t.id);
    }
  };

  toastsInIsland(true);
  const offs = [toasts.subscribe(sync), liveIslandCurrent.subscribe(sync), liveIslandHeld.subscribe(sync)];
  return () => {
    for (const off of offs) off();
    for (const n of mirrored.keys()) {
      liveIsland.hide(toastId(n));
      resumeToast(n);
    }
    mirrored.clear();
    toastsInIsland(false);
  };
}

/** Starts every producer; returns the disposer. */
export function initLiveIslandFeed(): () => void {
  const offs = [trackToasts()];
  return () => {
    for (const off of offs) off();
  };
}
