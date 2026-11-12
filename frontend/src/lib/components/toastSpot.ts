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
 * The least width a toast's message is read in: about 24 characters at `--font-sm`, three or four
 * words a line. The toast's own styles use the same number (`--toast-measure` in Toasts.svelte):
 * an action that would leave the message less than this moves to a row of its own under it.
 */
export const TOAST_MEASURE_REM = 16;
/**
 * What a toast takes besides its message: the paddings (1.2 and 0.4), the icon (1.6), the close
 * button (3.2), the two gaps between the three (0.8 each) and the border (0.1 a side).
 */
export const TOAST_FIXED_REM = 8.2;
/**
 * The least a toast is narrowed to: its fixed parts and one measure of message. At that width a
 * plain toast has exactly the measure, and one with an action has it too, with the action on its
 * own row (toast-tokens.test.ts holds the sum against the styles).
 */
export const TOAST_MIN_WIDTH_REM = TOAST_FIXED_REM + TOAST_MEASURE_REM;
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
