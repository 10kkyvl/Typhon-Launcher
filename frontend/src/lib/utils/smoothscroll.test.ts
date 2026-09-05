import { describe, expect, it } from 'vitest';
import { canScroll, coarseWheel, settle, wheelPixels, type ScrollMetrics } from './smoothscroll';

function metrics(patch: Partial<ScrollMetrics> = {}): ScrollMetrics {
  return { scrollTop: 100, scrollHeight: 1000, clientHeight: 400, ...patch };
}

describe('wheelPixels', () => {
  it('keeps pixel deltas', () => {
    expect(wheelPixels({ deltaY: 100, deltaMode: 0 }, 400)).toBe(100);
  });

  it('turns lines into pixels', () => {
    expect(wheelPixels({ deltaY: 3, deltaMode: 1 }, 400)).toBe(120);
  });

  it('turns pages into most of the viewport', () => {
    expect(wheelPixels({ deltaY: -1, deltaMode: 2 }, 400)).toBe(-360);
  });
});

describe('coarseWheel', () => {
  it.each([
    [-120, 100],
    [120, -100],
    [-360, 300],
    [240, -200],
  ])('treats wheelDeltaY %i as a notched wheel', (wheelDeltaY, deltaY) => {
    expect(coarseWheel({ deltaY, deltaMode: 0, wheelDeltaY })).toBe(true);
  });

  it.each([
    [-12, 4],
    [-48.72, 16.24],
    [119, 99.2],
    [-121.5, 101.25],
  ])('treats wheelDeltaY %s as a touchpad', (wheelDeltaY, deltaY) => {
    expect(coarseWheel({ deltaY, deltaMode: 0, wheelDeltaY })).toBe(false);
  });

  it('treats line and page deltas as a wheel', () => {
    expect(coarseWheel({ deltaY: 3, deltaMode: 1 })).toBe(true);
    expect(coarseWheel({ deltaY: 1, deltaMode: 2 })).toBe(true);
  });

  it('falls back to the delta size when wheelDeltaY is missing', () => {
    expect(coarseWheel({ deltaY: 57, deltaMode: 0 })).toBe(true);
    expect(coarseWheel({ deltaY: 12, deltaMode: 0 })).toBe(false);
    expect(coarseWheel({ deltaY: 57.5, deltaMode: 0 })).toBe(false);
  });
});

describe('canScroll', () => {
  it('scrolls both ways in the middle', () => {
    expect(canScroll(metrics(), -1)).toBe(true);
    expect(canScroll(metrics(), 1)).toBe(true);
  });

  it('stops at the top', () => {
    expect(canScroll(metrics({ scrollTop: 0 }), -1)).toBe(false);
    expect(canScroll(metrics({ scrollTop: 0 }), 1)).toBe(true);
  });

  it('stops at the bottom', () => {
    expect(canScroll(metrics({ scrollTop: 600 }), 1)).toBe(false);
    expect(canScroll(metrics({ scrollTop: 600 }), -1)).toBe(true);
  });

  it('ignores containers that do not overflow', () => {
    expect(canScroll(metrics({ scrollTop: 0, scrollHeight: 400 }), 1)).toBe(false);
  });
});

describe('settle', () => {
  it('stays put without elapsed time', () => {
    expect(settle(0, 300, 0)).toBe(0);
  });

  it('moves toward the target without overshooting', () => {
    const next = settle(0, 300, 16);
    expect(next).toBeGreaterThan(0);
    expect(next).toBeLessThan(300);
  });

  it('covers more ground in a longer frame', () => {
    expect(settle(0, 300, 32)).toBeGreaterThan(settle(0, 300, 16));
  });

  it('snaps once the remainder is under half a pixel', () => {
    expect(settle(299.9, 300, 16)).toBe(300);
    expect(settle(0, 300, 10_000)).toBe(300);
  });

  it('runs backwards too', () => {
    expect(settle(300, 0, 16)).toBeLessThan(300);
    expect(settle(300, 0, 16)).toBeGreaterThan(0);
  });
});
