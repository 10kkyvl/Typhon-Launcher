import { describe, expect, it } from 'vitest';
import { sessionClock } from './activityWidget';

describe('sessionClock', () => {
  it.each([
    [0, '0:00'],
    [999, '0:00'],
    [65_000, '1:05'],
    [3_599_000, '59:59'],
    [3_729_000, '1:02:09'],
    [-5_000, '0:00'],
  ])('%i ms reads %s', (ms, text) => {
    expect(sessionClock(ms)).toBe(text);
  });
});
