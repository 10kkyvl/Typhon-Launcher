import type { Rect, Size } from '../utils/position';

export type ToastEnd = 'bottom' | 'top';

export interface ToastSpot {
  /** The end of its column the stack stands at. */
  at: ToastEnd;
  /** The widest the stack may be, px, while it stands in the gutter beside the panel; null: as wide as it likes. */
  width: number | null;
  /** How far above `floor` the dialog's panel must end to stay clear of the stack, px; 0: nothing is asked of it. */
  room: number;
}

export interface ToastSpace {
  /** The least width a toast can be read in, px (`TOAST_MIN_WIDTH_REM`). */
  minWidth: number;
  /** What is kept between the stack and the panel when one of them gives way, px (`TOAST_GAP_REM`). */
  gap: number;
  /** The bottom of the area the dialog lies in: the window's bottom edge. */
  floor: number;
  /** The dialog is making room now: its measured panel is the shortened one, not the one at rest. */
  reserved: boolean;
}

/**
 * The least a toast is narrowed to. Its fixed parts take 7.6rem (the paddings, the icon, the close
 * button and the gaps between them), a short action with its gap about 9rem more; 24rem leaves the
 * message 7 to 8rem beside an action and 16rem without one, two or three short words a line at
 * `--font-sm`. Narrower than that the message starts breaking inside words (`overflow-wrap: anywhere`).
 */
export const TOAST_MIN_WIDTH_REM = 24;
/** `--space-2`, the gap between two toasts. */
export const TOAST_GAP_REM = 0.8;

function overlap(a: Rect, b: Rect): boolean {
  return a.left < b.left + b.width && b.left < a.left + a.width && a.top < b.top + b.height && b.top < a.top + a.height;
}

/**
 * Where the toast stack stands while a dialog may be open, so that it never lies over the dialog's
 * panel. The first that is clear of the panel:
 *
 * 1. the bottom right, its place;
 * 2. the top right;
 * 3. the bottom right, narrowed to the gutter between the panel and the column's edge, if a toast can
 *    still be read in it (`minWidth`);
 * 4. the bottom right, with the dialog asked to end above the stack (`room`).
 *
 * `panel` is the open dialog's panel as it rests, or null when there is none (no dialog, or a layer
 * without a panel such as the lightbox). `lane` is the column the stack can stand in: its right
 * edge, from the top line to the bottom line. `stack` is the size the stack likes to be now.
 *
 * A panel that is already making room cannot be measured as it rests without taking the room away,
 * so while `reserved` the answer stays 4, with the room the stack needs now, until the stack is empty.
 */
export function toastSpot(panel: Rect | null, lane: Rect, stack: Size, space: ToastSpace): ToastSpot {
  const free: ToastSpot = { at: 'bottom', width: null, room: 0 };
  // Field by field: a measured box (DOMRect) keeps its numbers on the prototype, and a spread copies none.
  const { width, height } = stack;
  if (!panel || width <= 0 || height <= 0) return free;

  const right = lane.left + lane.width;
  const bottom = lane.top + lane.height;
  const making: ToastSpot = { ...free, room: space.floor - (bottom - height) + space.gap };
  if (space.reserved) return making;

  const at = (top: number): Rect => ({ left: right - width, top, width, height });
  if (!overlap(panel, at(bottom - height))) return free;
  if (!overlap(panel, at(lane.top))) return { ...free, at: 'top' };
  const gutter = right - (panel.left + panel.width) - space.gap;
  if (gutter >= space.minWidth) return { ...free, width: gutter };
  return making;
}
