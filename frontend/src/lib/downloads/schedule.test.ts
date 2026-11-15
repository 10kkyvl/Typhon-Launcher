import { describe, expect, it } from 'vitest';
import type { PlayThrottle } from '../services/playThrottle';
import { heldBack } from './schedule';

type Throttle = Pick<PlayThrottle, 'mode' | 'schedule'>;

const idle: Throttle = { mode: 'off' };
const blocked: Throttle = { mode: 'off', schedule: { blocked: true, nextOpenAt: '' } };
const open: Throttle = { mode: 'off', schedule: { blocked: false, nextOpenAt: '' } };

describe('heldBack: whether Go would hold a download back if it started now', () => {
  it.each([
    ['a game running in the pause mode', { mode: 'pause' } as Throttle, {}, true],
    ['the pause mode, even for a download let outside the schedule', { mode: 'pause' } as Throttle, { ignoreSchedule: true }, true],
    ['the schedule blocking', blocked, {}, true],
    ['the schedule blocking a download let outside it', blocked, { ignoreSchedule: true }, false],
    ['the limit mode, which slows and holds nothing', { mode: 'limit' } as Throttle, {}, false],
    ['an open schedule', open, {}, false],
    ['nothing at all', idle, {}, false],
  ])('%s', (_, throttle, download, held) => {
    expect(heldBack(download, throttle)).toBe(held);
  });
});
