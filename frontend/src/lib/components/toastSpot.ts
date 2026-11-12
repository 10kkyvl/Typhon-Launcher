import type { Rect, Size } from '../utils/position';

export type ToastEnd = 'bottom' | 'top';

function overlap(a: Rect, b: Rect): boolean {
  return a.left < b.left + b.width && b.left < a.left + a.width && a.top < b.top + b.height && b.top < a.top + a.height;
}

/**
 * The corner the toast stack stands in while a dialog may be open. Its place is the bottom right; it
 * moves to the top right only while the dialog's panel reaches into the bottom right and leaves the
 * top right clear. If the panel takes both, the stack covers something either way and stays where
 * people look for it.
 *
 * `panel` is the open dialog's panel, or null when there is none (no dialog, or a layer without a
 * panel such as the lightbox). `lane` is the column the stack can stand in: its right edge, from the
 * top line to the bottom line. `stack` is the size of the stack now.
 */
export function toastSpot(panel: Rect | null, lane: Rect, stack: Size): ToastEnd {
  if (!panel || stack.width <= 0 || stack.height <= 0) return 'bottom';
  // Field by field: a measured box (DOMRect) keeps its numbers on the prototype, and a spread copies none.
  const { width, height } = stack;
  const at = (top: number): Rect => ({ left: lane.left + lane.width - width, top, width, height });
  const atBottom = at(lane.top + lane.height - height);
  const atTop = at(lane.top);
  return overlap(panel, atBottom) && !overlap(panel, atTop) ? 'top' : 'bottom';
}
