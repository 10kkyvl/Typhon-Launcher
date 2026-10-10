import { describe, expect, it } from 'vitest';
import { unreadGrew } from './unread';

describe('unreadGrew', () => {
  it('is false for the first value, there is nothing to compare with', () => {
    expect(unreadGrew(null, 0)).toBe(false);
    expect(unreadGrew(null, 4)).toBe(false);
  });

  it('is true only when the count went up', () => {
    expect(unreadGrew(0, 1)).toBe(true);
    expect(unreadGrew(2, 5)).toBe(true);
    expect(unreadGrew(3, 3)).toBe(false);
    expect(unreadGrew(3, 2)).toBe(false);
    expect(unreadGrew(1, 0)).toBe(false);
  });
});
