import { describe, expect, it } from 'vitest';
import type { Rect } from '../utils/position';
import { toastCorner } from './toastCorner';

// A 1280 × 800 window: the column the stack can stand in runs down the right edge, from under the
// title bar to above the corner's inset. One toast is 380 × 60, so the stack would take
// 876…1256 × 716…776 at the bottom and 876…1256 × 48…108 at the top.
const lane: Rect = { left: 876, top: 48, width: 380, height: 728 };
const stack = { width: 380, height: 60 };

describe('the corner the toast stack stands in', () => {
  it('is the bottom right while no dialog is open', () => {
    expect(toastCorner(null, lane, stack)).toBe('bottom');
  });

  it('stays the bottom right under a small dialog in the middle of the window', () => {
    expect(toastCorner({ left: 420, top: 250, width: 440, height: 300 }, lane, stack)).toBe('bottom');
  });

  it('is the top right while a tall dialog reaches into the bottom right', () => {
    expect(toastCorner({ left: 340, top: 150, width: 600, height: 640 }, lane, stack)).toBe('top');
  });

  it('stays the bottom right when a tall, wide dialog reaches into both corners', () => {
    expect(toastCorner({ left: 40, top: 56, width: 1200, height: 736 }, lane, stack)).toBe('bottom');
  });

  it('stays the bottom right over a layer without a panel (the lightbox)', () => {
    expect(toastCorner(null, lane, stack)).toBe('bottom');
  });

  it('a dialog that only touches the stack edge to edge does not take the corner', () => {
    expect(toastCorner({ left: 276, top: 150, width: 600, height: 640 }, lane, stack)).toBe('bottom');
    expect(toastCorner({ left: 340, top: 76, width: 600, height: 640 }, lane, stack)).toBe('bottom');
  });

  it('an empty stack takes no room, so nothing moves before there is a toast', () => {
    expect(toastCorner({ left: 340, top: 150, width: 600, height: 640 }, lane, { width: 0, height: 0 })).toBe('bottom');
  });

  it('reads a measured box as it reads a plain one', () => {
    // As a DOMRect: the numbers are getters on the prototype, the object has no fields of its own.
    const box = Object.create({
      get width() {
        return 380;
      },
      get height() {
        return 60;
      },
    }) as Rect;

    expect(toastCorner({ left: 340, top: 150, width: 600, height: 640 }, lane, box)).toBe('top');
  });

  it('a taller stack meets a dialog that one toast would clear', () => {
    const panel = { left: 340, top: 200, width: 600, height: 450 };

    expect(toastCorner(panel, lane, stack)).toBe('bottom');
    expect(toastCorner(panel, lane, { width: 380, height: 136 })).toBe('top');
  });
});
