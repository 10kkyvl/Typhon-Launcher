import { describe, expect, it } from 'vitest';
import { stageDrift } from './stageDrift';

// The prototype's numbers: the stage drifts up at half the page's speed and is gone by 220 px, before the
// grid that follows the header covers it.
describe('the stage as the page scrolls', () => {
  it('stands whole at the top of the page', () => {
    expect(stageDrift(0)).toEqual({ shift: 0, opacity: 1 });
  });

  it('drifts up at half the page speed and fades as it goes', () => {
    const at = stageDrift(100);
    expect(at.shift).toBe(-50);
    expect(at.opacity).toBeCloseTo(1 - 100 / 220, 6);
    expect(stageDrift(110)).toEqual({ shift: -55, opacity: 0.5 });
  });

  it('is gone by 220 px and stays gone further down, still drifting', () => {
    expect(stageDrift(220)).toEqual({ shift: -110, opacity: 0 });
    expect(stageDrift(400)).toEqual({ shift: -200, opacity: 0 });
  });

  it('treats an overscroll above the top and a value that is not a number as the top', () => {
    expect(stageDrift(-40)).toEqual({ shift: 0, opacity: 1 });
    expect(stageDrift(Number.NaN)).toEqual({ shift: 0, opacity: 1 });
  });
});
