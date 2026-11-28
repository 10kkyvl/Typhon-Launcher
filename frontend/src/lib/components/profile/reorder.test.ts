import { describe, expect, it } from 'vitest';
import { pickTarget, type Box } from './reorder';

const box = (index: number, left: number, top: number): Box => ({ index, left, top, right: left + 100, bottom: top + 50 });

describe('where a dragged block lands', () => {
  const boxes = [box(0, 0, 0), box(1, 120, 0), box(2, 0, 70)];

  it('on the block under the pointer', () => {
    expect(pickTarget(150, 20, boxes)).toBe(1);
    expect(pickTarget(10, 100, boxes)).toBe(2);
  });

  it('on the nearest block by its centre when the pointer is between blocks or outside them', () => {
    expect(pickTarget(115, 25, boxes)).toBe(1);
    expect(pickTarget(105, 25, boxes)).toBe(0);
    expect(pickTarget(-500, 95, boxes)).toBe(2);
    expect(pickTarget(1000, -40, boxes)).toBe(1);
  });

  it('nowhere when there is no block', () => {
    expect(pickTarget(0, 0, [])).toBeNull();
  });
});
