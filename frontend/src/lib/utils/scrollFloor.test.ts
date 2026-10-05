import { describe, expect, it } from 'vitest';
import { scrollFloor } from './scrollFloor';

describe('scrollFloor', () => {
  const cases = [
    {
      title: 'scrolled to the bottom keeps the whole visible part of the block',
      input: { scrollTop: 1200, viewport: 800, scrollHeight: 2000, top: 1400, height: 560 },
      want: 560,
    },
    {
      title: 'block partly above the viewport is held down to the viewport bottom',
      input: { scrollTop: 1000, viewport: 800, scrollHeight: 2000, top: 1400, height: 560 },
      want: 360,
    },
    {
      title: 'block entirely below the viewport needs no floor',
      input: { scrollTop: 0, viewport: 800, scrollHeight: 2000, top: 1400, height: 560 },
      want: 0,
    },
    {
      title: 'content after the block already covers the viewport',
      input: { scrollTop: 300, viewport: 800, scrollHeight: 3000, top: 400, height: 500 },
      want: 0,
    },
    {
      title: 'fractional positions round up so the scroll offset never clamps',
      input: { scrollTop: 100.4, viewport: 700, scrollHeight: 800.4, top: 500, height: 260.4 },
      want: 261,
    },
    {
      title: 'empty block at the bottom',
      input: { scrollTop: 400, viewport: 800, scrollHeight: 1200, top: 1160, height: 0 },
      want: 0,
    },
  ];

  for (const c of cases) {
    it(c.title, () => {
      const floor = scrollFloor(c.input);
      expect(floor).toBe(c.want);
      const trailing = Math.max(0, c.input.scrollHeight - c.input.top - c.input.height);
      const heightAfter = c.input.top + floor + trailing;
      if (c.input.top < c.input.scrollTop + c.input.viewport) {
        expect(heightAfter).toBeGreaterThanOrEqual(c.input.scrollTop + c.input.viewport - 1);
      }
    });
  }
});
