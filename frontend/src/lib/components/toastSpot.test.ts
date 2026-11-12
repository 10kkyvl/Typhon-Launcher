import { describe, expect, it } from 'vitest';
import type { Rect, Size } from '../utils/position';
import { TOAST_GAP_REM, TOAST_MIN_WIDTH_REM, toastSpot, type ToastSpot } from './toastSpot';

// Boxes as [left, top, right, bottom], the way they are read off a screenshot.
const box = (left: number, top: number, right: number, bottom: number): Rect => ({ left, top, width: right - left, height: bottom - top });

// The smallest window the launcher allows, 1100 × 750 with a 10px rem: the title bar is 40 high, the
// stack's column ends 24 from the right and from the bottom. The column is as wide as the stack in it.
const REM = 10;
const WINDOW = { width: 1100, height: 750 };
const column = (stack: Size, window = WINDOW): Rect => box(window.width - 24 - stack.width, 48, window.width - 24, window.height - 24);
const space = (window = WINDOW) => ({ minWidth: TOAST_MIN_WIDTH_REM * REM, gap: TOAST_GAP_REM * REM, floor: window.height, reserved: false });

// Measured in the running app.
const PLAIN = { width: 282, height: 44 };
const ACTION = { width: 380, height: 99 };
const TWO = { width: 380, height: 151 };
const STACKS = { 'one plain toast': PLAIN, 'one wrapped toast with an action': ACTION, 'two toasts': TWO };

const spot = (panel: Rect | null, stack: Size, window = WINDOW) => toastSpot(panel, column(stack, window), stack, space(window));
const BOTTOM: ToastSpot = { at: 'bottom', width: null, room: 0 };
const TOP: ToastSpot = { at: 'top', width: null, room: 0 };
const gutter = (width: number): ToastSpot => ({ at: 'bottom', width, room: 0 });
const room = (px: number): ToastSpot => ({ at: 'bottom', width: null, room: px });

/** A dialog `rem` wide, centred, as tall as a dialog gets in this window. */
const tall = (rem: number, window = WINDOW): Rect => {
  const width = Math.min(rem * REM, window.width - 48);
  return box((window.width - width) / 2, 80, (window.width + width) / 2, window.height - 40);
};

/** Where the stack and the dialog end up: the stack's box, and the lowest the dialog's panel may reach. */
function outcome(panel: Rect, stack: Size, window = WINDOW) {
  const result = spot(panel, stack, window);
  const lane = column(stack, window);
  const right = lane.left + lane.width;
  const width = result.width ?? stack.width;
  const top = result.at === 'top' ? lane.top : lane.top + lane.height - stack.height;
  const panelBottom = Math.min(panel.top + panel.height, window.height - result.room);
  return { result, stackBox: box(right - width, top, right, top + stack.height), rest: box(panel.left, panel.top, panel.left + panel.width, panelBottom) };
}

const overlap = (a: Rect, b: Rect) =>
  a.left < b.left + b.width && b.left < a.left + a.width && a.top < b.top + b.height && b.top < a.top + a.height;

describe('where the toast stack stands', () => {
  it('in the bottom right while no dialog is open, or over a layer without a panel (the lightbox)', () => {
    expect(spot(null, TWO)).toEqual(BOTTOM);
  });

  it('in the bottom right under a small dialog in the middle of the window', () => {
    expect(spot(box(330, 250, 770, 550), TWO)).toEqual(BOTTOM);
  });

  it('in the top right while a dialog reaches into the bottom right and leaves the top clear', () => {
    expect(spot(box(300, 200, 800, 710), ACTION)).toEqual(TOP);
  });

  it('a dialog that only touches the stack edge to edge does not take its corner', () => {
    expect(spot(box(196, 200, 696, 710), ACTION)).toEqual(BOTTOM);
    expect(spot(box(300, 100, 800, 627), ACTION)).toEqual(BOTTOM);
  });

  it('an empty stack takes no room: nothing moves and nothing is asked before there is a toast', () => {
    expect(spot(tall(104), { width: 0, height: 0 })).toEqual(BOTTOM);
  });

  it('reads a measured box as it reads a plain one', () => {
    // As a DOMRect: the numbers are getters on the prototype, the object has no fields of its own.
    const measured = Object.create({
      get width() {
        return ACTION.width;
      },
      get height() {
        return ACTION.height;
      },
    }) as Size;

    expect(toastSpot(box(300, 200, 800, 710), column(ACTION), measured, space())).toEqual(TOP);
  });
});

describe('a dialog that takes both corners', () => {
  it('the stack stands beside it, narrowed to the gutter, when the gutter is wide enough for a toast', () => {
    // "Collection" at 1100 × 750 with two toasts: both corners meet the panel, 266 are left beside it.
    expect(spot(box(290, 117, 810, 673), TWO)).toEqual(gutter(258));
    expect(spot(box(290, 117, 810, 673), ACTION)).toEqual(gutter(258));
  });

  it('one plain toast under the same dialog fits below it and stays as it is', () => {
    expect(spot(box(290, 117, 810, 673), PLAIN)).toEqual(BOTTOM);
  });

  it('the gutter counts from the least a toast can be read in, and not below', () => {
    const least = TOAST_MIN_WIDTH_REM * REM;
    const reaching = (gutterWidth: number) => box(200, 80, 1076 - TOAST_GAP_REM * REM - gutterWidth, 710);

    expect(spot(reaching(least), TWO)).toEqual(gutter(least));
    expect(spot(reaching(least - 1), TWO).width).toBeNull();
  });

  it('with no gutter the dialog is asked for room: from the floor up to the stack and a gap', () => {
    // "Game properties" at 1100 × 750: 238 beside it, two short of a toast.
    expect(spot(box(270, 80, 830, 710), PLAIN)).toEqual(room(76));
    expect(spot(box(270, 80, 830, 710), ACTION)).toEqual(room(131));
    expect(spot(box(270, 80, 830, 710), TWO)).toEqual(room(183));
  });

  it('while the dialog is making room its measured panel is the shortened one: the room stays, sized to the stack now', () => {
    const shortened = box(270, 80, 830, 567);
    const making = { ...space(), reserved: true };

    expect(toastSpot(shortened, column(TWO), TWO, making)).toEqual(room(183));
    expect(toastSpot(shortened, column(PLAIN), PLAIN, making)).toEqual(room(76));
    expect(toastSpot(shortened, column(PLAIN), { width: 0, height: 0 }, making)).toEqual(BOTTOM);
    expect(toastSpot(null, column(PLAIN), PLAIN, making)).toEqual(BOTTOM);
  });
});

describe('every dialog width at the smallest window', () => {
  it.each([
    [46, 'one plain toast', BOTTOM],
    [46, 'one wrapped toast with an action', gutter(288)],
    [46, 'two toasts', gutter(288)],
    [52, 'one plain toast', gutter(258)],
    [52, 'one wrapped toast with an action', gutter(258)],
    [52, 'two toasts', gutter(258)],
    [64, 'one plain toast', room(76)],
    [64, 'one wrapped toast with an action', room(131)],
    [64, 'two toasts', room(183)],
    [86, 'one plain toast', room(76)],
    [86, 'one wrapped toast with an action', room(131)],
    [86, 'two toasts', room(183)],
    [104, 'one plain toast', room(76)],
    [104, 'one wrapped toast with an action', room(131)],
    [104, 'two toasts', room(183)],
  ] as const)('a tall %irem dialog with %s', (rem, stack, expected) => {
    expect(spot(tall(rem), STACKS[stack])).toEqual(expected);
  });

  const windows = [WINDOW, { width: 1280, height: 800 }, { width: 1920, height: 1080 }];
  const cases = windows.flatMap((window) =>
    [46, 52, 56, 64, 86, 104].flatMap((rem) => Object.entries(STACKS).map(([name, stack]) => ({ window, rem, name, stack }))),
  );

  it.each(cases)('$rem rem at $window.width × $window.height with $name: the stack is clear of the dialog', ({ window, rem, stack }) => {
    const { result, stackBox, rest } = outcome(tall(rem, window), stack, window);

    expect(overlap(stackBox, rest)).toBe(false);
    // Narrowed, the stack stands wholly beside the panel, however tall it wraps to.
    if (result.width !== null) expect(stackBox.left).toBeGreaterThanOrEqual(rest.left + rest.width + TOAST_GAP_REM * REM);
    // Making room, the panel ends a gap above the stack.
    if (result.room > 0) expect(rest.top + rest.height).toBe(stackBox.top - TOAST_GAP_REM * REM);
  });
});
