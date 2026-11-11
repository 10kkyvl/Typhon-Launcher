import { describe, expect, it } from 'vitest';
import { placePopover } from './playerCardPopover';

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
