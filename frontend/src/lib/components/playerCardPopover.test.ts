import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createHoverIntent, HOVER_DELAY, placePopover } from './playerCardPopover';

describe('hover intent', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  function make() {
    const changes: boolean[] = [];
    return { changes, intent: createHoverIntent((open) => changes.push(open)) };
  }

  it('opens only after the hover delay', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY - 1);
    expect(changes).toEqual([]);
    vi.advanceTimersByTime(1);
    expect(changes).toEqual([true]);
    expect(intent.open).toBe(true);
  });

  it('never opens when the pointer leaves early', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(100);
    intent.unhover();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([]);
  });

  it('closes when the pointer leaves', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.unhover();
    expect(changes).toEqual([true, false]);
  });

  it('opens at once on keyboard focus and closes on blur', () => {
    const { intent, changes } = make();
    intent.focus();
    expect(changes).toEqual([true]);
    intent.blur();
    expect(changes).toEqual([true, false]);
  });

  it('closes on Escape and stays closed until the next focus', () => {
    const { intent, changes } = make();
    intent.focus();
    intent.escape();
    expect(changes).toEqual([true, false]);
    intent.focus();
    expect(changes).toEqual([true, false]);
    intent.blur();
    intent.focus();
    expect(changes).toEqual([true, false, true]);
  });

  it('stays open on focus when the pointer leaves', () => {
    const { intent, changes } = make();
    intent.focus();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.unhover();
    expect(changes).toEqual([true]);
    expect(intent.open).toBe(true);
  });

  it('dismisses on scroll and can open again afterwards', () => {
    const { intent, changes } = make();
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    intent.dismiss();
    expect(changes).toEqual([true, false]);
    intent.hover();
    vi.advanceTimersByTime(HOVER_DELAY);
    expect(changes).toEqual([true, false, true]);
  });

  it('cancels the pending timer on destroy', () => {
    const { intent, changes } = make();
    intent.hover();
    intent.destroy();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([]);
  });
});

describe('placePopover', () => {
  const viewport = { width: 1000, height: 700 };
  const size = { width: 320, height: 200 };

  it('puts the card to the right of the anchor', () => {
    expect(placePopover({ left: 100, top: 50, width: 200, height: 40 }, size, viewport)).toEqual({ left: 308, top: 50 });
  });

  it('flips to the left when there is no room on the right', () => {
    expect(placePopover({ left: 700, top: 50, width: 280, height: 40 }, size, viewport)).toEqual({ left: 372, top: 50 });
  });

  it('keeps the card inside the viewport vertically', () => {
    expect(placePopover({ left: 100, top: 650, width: 200, height: 40 }, size, viewport).top).toBe(492);
  });

  it('goes below a wide anchor that leaves no side room', () => {
    expect(placePopover({ left: 10, top: 20, width: 980, height: 40 }, size, viewport)).toEqual({ left: 340, top: 68 });
  });

  it('goes above when there is no room below either', () => {
    expect(placePopover({ left: 10, top: 600, width: 980, height: 40 }, size, viewport)).toEqual({ left: 340, top: 392 });
  });

  it('never leaves the margin on a tiny viewport', () => {
    const place = placePopover({ left: 0, top: 0, width: 100, height: 20 }, size, { width: 200, height: 100 });
    expect(place.left).toBeGreaterThanOrEqual(8);
    expect(place.top).toBeGreaterThanOrEqual(8);
  });
});
