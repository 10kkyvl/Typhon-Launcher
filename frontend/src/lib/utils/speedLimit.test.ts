import { describe, expect, it } from 'vitest';
import { makeFormat } from './format';
import { translator } from '../i18n';
import { SPEED_LIMIT_NO_LIMIT, SPEED_LIMIT_STEPS_MB, speedLimitAt, speedLimitPosition } from './speedLimit';

const MB = 1024 ** 2;

describe('the speed limit steps', () => {
  it('are the prototype’s steps in MB/s, ascending, with "no limit" one past the last', () => {
    expect([...SPEED_LIMIT_STEPS_MB]).toEqual([1, 2, 3, 5, 8, 10, 15, 20, 30, 50, 75, 100, 150, 200, 300, 500, 750, 1000]);
    expect(SPEED_LIMIT_NO_LIMIT).toBe(SPEED_LIMIT_STEPS_MB.length);
  });
});

describe('a stored limit in bytes/s to its place on the track', () => {
  it.each([
    [1 * MB, 0],
    [2 * MB, 1],
    [20 * MB, 7],
    [1000 * MB, 17],
  ])('a value on a step (%i bytes/s) sits on that step, %i', (bytes, position) => {
    expect(speedLimitPosition(bytes)).toBe(position);
  });

  it.each([
    [12 * MB, 5.4],
    [4 * MB, 2.5],
    [875 * MB, 16.5],
    [1.5 * MB, 0.5],
  ])('a value between steps (%i bytes/s) sits between them, at %d, not snapped', (bytes, position) => {
    expect(speedLimitPosition(bytes)).toBeCloseTo(position, 10);
  });

  it('zero is no limit, past the last step', () => {
    expect(speedLimitPosition(0)).toBe(SPEED_LIMIT_NO_LIMIT);
  });

  it('a negative or broken value is no limit, as Go reads one below zero', () => {
    expect(speedLimitPosition(-1)).toBe(SPEED_LIMIT_NO_LIMIT);
    expect(speedLimitPosition(Number.NaN)).toBe(SPEED_LIMIT_NO_LIMIT);
  });

  it('a value under the first step sits at the start; one over the last at the last step, never at "no limit"', () => {
    expect(speedLimitPosition(0.5 * MB)).toBe(0);
    expect(speedLimitPosition(1)).toBe(0);
    expect(speedLimitPosition(2000 * MB)).toBe(17);
    expect(speedLimitPosition(Number.POSITIVE_INFINITY)).toBe(17);
  });
});

describe('a place on the track to the limit in bytes/s', () => {
  it.each([
    [0, 1 * MB],
    [7, 20 * MB],
    [17, 1000 * MB],
  ])('step %i is %i bytes/s, binary megabytes', (position, bytes) => {
    expect(speedLimitAt(position)).toBe(bytes);
  });

  it('the place past the last step is 0, no limit', () => {
    expect(speedLimitAt(SPEED_LIMIT_NO_LIMIT)).toBe(0);
  });

  it('a place between steps takes the nearest step', () => {
    expect(speedLimitAt(5.4)).toBe(10 * MB);
    expect(speedLimitAt(5.6)).toBe(15 * MB);
    expect(speedLimitAt(17.6)).toBe(0);
  });

  it('a place outside the track is held at its ends', () => {
    expect(speedLimitAt(-3)).toBe(1 * MB);
    expect(speedLimitAt(40)).toBe(0);
    expect(speedLimitAt(Number.NaN)).toBe(0);
  });

  it('every step goes there and back unchanged', () => {
    for (let position = 0; position <= SPEED_LIMIT_NO_LIMIT; position++) {
      expect(speedLimitPosition(speedLimitAt(position))).toBe(position);
    }
  });
});

describe('the words for a limit', () => {
  // The steps carry no text of their own: the launcher's formatter says every limit, on a step or not.
  const ru = makeFormat('ru', translator('ru'));
  const en = makeFormat('en', translator('en'));

  it('reads a step, a value off the steps and no limit through rateLimitLabel', () => {
    expect(ru.rateLimitLabel(speedLimitAt(7))).toBe('20 МБ/с');
    expect(en.rateLimitLabel(speedLimitAt(SPEED_LIMIT_NO_LIMIT))).toBe('Unlimited');
    expect(ru.rateLimitLabel(Math.round(0.5 * MB))).toBe('0,5 МБ/с');
  });
});
