import { describe, expect, it } from 'vitest';
import { codePoints, counterStage, grownHeight } from './textArea';

describe('counterStage', () => {
  it.each([
    [0, 5000, 'under'],
    [4499, 5000, 'under'],
    [4500, 5000, 'near'],
    [4999, 5000, 'near'],
    [5000, 5000, 'at'],
    [5001, 5000, 'over'],
    [8, 10, 'under'],
    [9, 10, 'near'],
    [3, 5, 'under'],
    [4, 5, 'near'],
    [0, 1, 'near'],
    [1, 1, 'at'],
  ] as const)('%i of %i is %s', (count, max, stage) => {
    expect(counterStage(count, max)).toBe(stage);
  });
});

describe('codePoints', () => {
  it('counts a letter outside the basic plane as one', () => {
    expect(codePoints('a😀б')).toBe(3);
    expect(codePoints('')).toBe(0);
  });
});

describe('grownHeight', () => {
  const box = { lineHeight: 20, padding: 18, border: 2, rows: 3, grow: 6 };

  it('keeps the rows it opens with for a short text', () => {
    expect(grownHeight({ ...box, scrollHeight: 38 })).toEqual({ height: 3 * 20 + 20, scrolls: false });
  });

  it('follows a longer text, its border included', () => {
    expect(grownHeight({ ...box, scrollHeight: 5 * 20 + 18 })).toEqual({ height: 5 * 20 + 20, scrolls: false });
  });

  it('stops at the rows it grows to and scrolls past them', () => {
    expect(grownHeight({ ...box, scrollHeight: 6 * 20 + 18 })).toEqual({ height: 6 * 20 + 20, scrolls: false });
    expect(grownHeight({ ...box, scrollHeight: 9 * 20 + 18 })).toEqual({ height: 6 * 20 + 20, scrolls: true });
  });

  it('never grows to fewer rows than it opens with', () => {
    expect(grownHeight({ ...box, grow: 2, scrollHeight: 9 * 20 + 18 })).toEqual({ height: 3 * 20 + 20, scrolls: true });
  });

  it('answers nothing without a layout', () => {
    expect(grownHeight({ ...box, lineHeight: Number.NaN, scrollHeight: 0 })).toBeNull();
    expect(grownHeight({ ...box, lineHeight: 0, scrollHeight: 0 })).toBeNull();
  });
});
