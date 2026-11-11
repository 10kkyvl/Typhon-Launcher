import { describe, expect, it } from 'vitest';
import { computePosition, lengthPx, positionAtPoint, visiblePart, type Insets } from './position';

const viewport = { width: 390, height: 844 };
const menu = { width: 200, height: 160 };

function inside(r: { x: number; y: number }, size: { width: number; height: number }, pad: Insets | number = 8) {
  const p = typeof pad === 'number' ? { top: pad, right: pad, bottom: pad, left: pad } : pad;
  expect(r.x).toBeGreaterThanOrEqual(p.left);
  expect(r.y).toBeGreaterThanOrEqual(p.top);
  expect(r.x + size.width).toBeLessThanOrEqual(viewport.width - p.right);
  expect(r.y + size.height).toBeLessThanOrEqual(viewport.height - p.bottom);
}

describe('computePosition', () => {
  it('opens below the anchor, start-aligned, by default', () => {
    const r = computePosition({ left: 20, top: 100, width: 40, height: 36 }, menu, { viewport });
    expect(r).toMatchObject({ x: 20, y: 140, placement: 'bottom-start' });
  });

  it('lines up with the edge of the anchor that align names: start with its left, end with its right', () => {
    const anchor = { left: 150, top: 100, width: 100, height: 36 };
    expect(computePosition(anchor, menu, { viewport, placement: 'bottom-start' }).x).toBe(150);
    const end = computePosition(anchor, menu, { viewport, placement: 'bottom-end' });
    expect(end.x + menu.width).toBe(anchor.left + anchor.width);
    const up = computePosition({ ...anchor, top: 500 }, menu, { viewport, placement: 'top-end' });
    expect(up).toMatchObject({ placement: 'top-end', y: 500 - 4 - 160 });
    expect(up.x + menu.width).toBe(anchor.left + anchor.width);
  });

  it('flips to the top when there is no room below', () => {
    const r = computePosition({ left: 20, top: 760, width: 40, height: 36 }, menu, { viewport });
    expect(r.placement).toBe('top-start');
    expect(r.y).toBe(760 - 4 - 160);
    inside(r, menu);
  });

  it('flips an upward menu down when there is no room above', () => {
    const r = computePosition({ left: 20, top: 60, width: 40, height: 36 }, menu, { viewport, placement: 'top-end' });
    expect(r.placement).toBe('bottom-end');
    expect(r.y).toBe(60 + 36 + 4);
  });

  it('keeps the preferred side when the other side is even smaller', () => {
    const tall = { width: 200, height: 600 };
    const r = computePosition({ left: 20, top: 300, width: 40, height: 36 }, tall, { viewport });
    expect(r.placement).toBe('bottom-start');
    expect(r.maxHeight).toBe(844 - 336 - 4 - 8);
    expect(r.y + (r.maxHeight ?? 0)).toBeLessThanOrEqual(viewport.height - 8);
  });

  it('shifts horizontally so a start-aligned menu never leaves the right edge', () => {
    const r = computePosition({ left: 350, top: 100, width: 32, height: 32 }, menu, { viewport });
    expect(r.x).toBe(390 - 8 - 200);
    inside(r, menu);
  });

  it('shifts an end-aligned menu right when the anchor sits at the left edge', () => {
    const r = computePosition({ left: 0, top: 100, width: 32, height: 32 }, menu, { viewport, placement: 'bottom-end' });
    expect(r.x).toBe(8);
    inside(r, menu);
  });

  it('clamps a centred tooltip at the left edge', () => {
    const tip = { width: 180, height: 30 };
    const r = computePosition({ left: 0, top: 200, width: 24, height: 24 }, tip, {
      viewport,
      placement: 'top',
    });
    expect(r.x).toBe(8);
    inside(r, tip);
  });

  it('shrinks a floating box wider than the viewport', () => {
    const wide = { width: 600, height: 100 };
    const r = computePosition({ left: 10, top: 10, width: 20, height: 20 }, wide, { viewport });
    expect(r.maxWidth).toBe(390 - 16);
    expect(r.x).toBe(8);
  });

  it('flips a right-side popover to the left', () => {
    const pop = { width: 240, height: 120 };
    const r = computePosition({ left: 300, top: 200, width: 40, height: 40 }, pop, {
      viewport,
      placement: 'right-start',
    });
    expect(r.placement).toBe('left-start');
    expect(r.x).toBe(300 - 4 - 240);
  });
});

describe('computePosition beside the anchor (a context menu next to a control)', () => {
  const wide = { width: 1024, height: 768 };
  const tall = { width: 200, height: 300 };
  const pad = { top: 48, right: 8, bottom: 8, left: 8 };

  it('opens to the right, top-aligned with the anchor', () => {
    const r = computePosition({ left: 300, top: 200, width: 80, height: 20 }, tall, { viewport: wide, placement: 'right-start', padding: pad });
    expect(r).toEqual({ x: 384, y: 200, placement: 'right-start' });
  });

  it('slides up along the window to show the whole menu next to an anchor near the bottom, without a max height', () => {
    const r = computePosition({ left: 300, top: 700, width: 80, height: 20 }, tall, { viewport: wide, placement: 'right-start', padding: pad });
    expect(r).toEqual({ x: 384, y: 768 - 8 - 300, placement: 'right-start' });
  });

  it('slides down below the title bar next to an anchor at the top', () => {
    const r = computePosition({ left: 300, top: 10, width: 80, height: 20 }, tall, { viewport: wide, placement: 'right-start', padding: pad });
    expect(r).toEqual({ x: 384, y: 48, placement: 'right-start' });
  });

  it('flips to the left when there is no room on the right', () => {
    const r = computePosition({ left: 900, top: 200, width: 80, height: 20 }, tall, { viewport: wide, placement: 'right-start', padding: pad });
    expect(r).toEqual({ x: 900 - 4 - 200, y: 200, placement: 'left-start' });
  });

  it('limits the height only when the window itself is shorter than the menu', () => {
    const r = computePosition({ left: 300, top: 100, width: 80, height: 20 }, tall, {
      viewport: { width: 1024, height: 250 },
      placement: 'right-start',
      padding: 8,
    });
    expect(r).toEqual({ x: 384, y: 8, placement: 'right-start', maxHeight: 234 });
  });

  it('falls back to below an anchor too wide to leave room on either side (a list row)', () => {
    const row = { left: 100, top: 300, width: 900, height: 58 };
    const r = computePosition(row, tall, { viewport: wide, placement: 'right-start', fallback: 'bottom-start', padding: pad });
    expect(r).toEqual({ x: 100, y: 362, placement: 'bottom-start' });
  });

  it('keeps the side when either one has room, whatever the fallback', () => {
    const r = computePosition({ left: 900, top: 200, width: 80, height: 20 }, tall, {
      viewport: wide,
      placement: 'right-start',
      fallback: 'bottom-start',
      padding: pad,
    });
    expect(r.placement).toBe('left-start');
    expect(r.maxWidth).toBeUndefined();
  });

  it('without a fallback it squeezes into the wider side, as before', () => {
    const row = { left: 100, top: 300, width: 900, height: 58 };
    const r = computePosition(row, tall, { viewport: wide, placement: 'right-start', padding: pad });
    expect(r.placement).toBe('left-start');
    expect(r.maxWidth).toBe(88);
  });
});

describe('computePosition under the title bar', () => {
  const pad: Insets = { top: 40 + 8, right: 8, bottom: 8, left: 8 };

  it('never puts a menu under the title bar: an upward menu without room below the bar flips down', () => {
    // 200 px above the anchor would hold the menu, but not once the bar is taken off; below there is plenty.
    const r = computePosition({ left: 20, top: 200, width: 40, height: 36 }, menu, { viewport, padding: pad, placement: 'top-start' });
    expect(r.placement).toBe('bottom-start');
    inside(r, menu, pad);
  });

  it('counts the bar as lost space when choosing the side, and shrinks a menu to what is left under it', () => {
    const tall = { width: 200, height: 760 };
    const r = computePosition({ left: 20, top: 780, width: 40, height: 36 }, tall, { viewport, padding: pad });
    expect(r.placement).toBe('top-start');
    expect(r.maxHeight).toBe(780 - 4 - 48);
    expect(r.y).toBe(48);
  });

  it('clamps a point menu below the bar', () => {
    const r = positionAtPoint(50, 10, menu, { viewport, padding: pad });
    expect(r.y).toBe(48);
    inside(r, menu, pad);
  });
});

describe('positionAtPoint', () => {
  it('puts the menu corner at the pointer', () => {
    expect(positionAtPoint(50, 60, menu, { viewport })).toMatchObject({ x: 50, y: 60 });
  });

  it('opens leftwards and upwards near the bottom-right corner', () => {
    const r = positionAtPoint(380, 830, menu, { viewport });
    expect(r).toMatchObject({ x: 180, y: 670, placement: 'top-start' });
    inside(r, menu);
  });

  it('clamps when neither direction fits', () => {
    const big = { width: 300, height: 160 };
    const r = positionAtPoint(200, 100, big, { viewport });
    expect(r.x).toBe(390 - 8 - 300);
    inside(r, big);
  });
});

describe('lengthPx', () => {
  it('reads rem against the root font size and px as is, and falls back on anything else', () => {
    expect(lengthPx(' 4rem', 10, 0)).toBe(40);
    expect(lengthPx('8px', 10, 0)).toBe(8);
    expect(lengthPx('', 10, 7)).toBe(7);
    expect(lengthPx('calc(1rem + 2px)', 10, 7)).toBe(7);
    expect(lengthPx('2em', 10, 7)).toBe(7);
  });
});

describe('visiblePart', () => {
  const anchor = { left: 100, top: 100, width: 40, height: 30 };

  it('is the whole anchor when the window and its scroll containers show all of it', () => {
    expect(visiblePart(anchor, [{ left: 0, top: 50, width: 300, height: 300 }], viewport)).toEqual(anchor);
  });

  it('is the part a scroll container and the window still show', () => {
    expect(visiblePart(anchor, [{ left: 0, top: 115, width: 300, height: 300 }], viewport)).toEqual({ left: 100, top: 115, width: 40, height: 15 });
  });

  it('is nothing once the anchor has left the window', () => {
    expect(visiblePart({ ...anchor, top: -40 }, [], viewport)).toBeNull();
    expect(visiblePart({ ...anchor, top: 900 }, [], viewport)).toBeNull();
    expect(visiblePart({ ...anchor, left: -50 }, [], viewport)).toBeNull();
  });

  it('is nothing once a scroll container has clipped it away, though it is still in the window', () => {
    expect(visiblePart(anchor, [{ left: 0, top: 200, width: 300, height: 300 }], viewport)).toBeNull();
  });

  it('keeps an empty rect inside the window (no layout yet, a zero-size anchor)', () => {
    expect(visiblePart({ left: 0, top: 0, width: 0, height: 0 }, [], viewport)).not.toBeNull();
  });
});
