import { describe, expect, it, vi } from 'vitest';

vi.mock('../../../bindings/typhon/internal/download', () => ({ Manager: { PlayThrottle: vi.fn() } }));
vi.mock('./backend', () => ({ inWails: true }));

import { clockTime, weekday } from '../utils/format';
import { idlePlayThrottle, playThrottleBanner, scheduleOpenLabel } from './playThrottle';

describe('playThrottleBanner', () => {
  it('is empty when the throttle is not active', () => {
    expect(playThrottleBanner(idlePlayThrottle)).toBeNull();
    expect(playThrottleBanner({ active: false, mode: 'pause', downloadLimit: 0, uploadLimit: 0 })).toBeNull();
  });

  it('is empty for an active throttle with mode off', () => {
    expect(playThrottleBanner({ active: true, mode: 'off', downloadLimit: 0, uploadLimit: 0 })).toBeNull();
  });

  it('reports pause', () => {
    expect(playThrottleBanner({ active: true, mode: 'pause', downloadLimit: 0, uploadLimit: 0 })).toEqual({
      kind: 'pause',
    });
  });

  it('reports a closed schedule window on its own', () => {
    const state = {
      active: true,
      mode: 'off' as const,
      downloadLimit: 0,
      uploadLimit: 0,
      schedule: { blocked: true, nextOpenAt: '2026-10-22T01:00:00Z' },
    };
    expect(playThrottleBanner(state)).toEqual({ kind: 'schedule', nextOpenAt: '2026-10-22T01:00:00Z', game: false });
  });

  it('reports the schedule together with a running game', () => {
    const state = {
      active: true,
      mode: 'pause' as const,
      downloadLimit: 0,
      uploadLimit: 0,
      schedule: { blocked: true, nextOpenAt: '' },
    };
    expect(playThrottleBanner(state)).toEqual({ kind: 'schedule', nextOpenAt: '', game: true });
  });

  it('names the next window by its time today and by weekday and time later', () => {
    const now = new Date(2026, 9, 21, 20, 0);
    const today = new Date(2026, 9, 21, 23, 30);
    const later = new Date(2026, 9, 23, 1, 0);
    expect(scheduleOpenLabel(today.toISOString(), now)).toBe(clockTime(today));
    expect(scheduleOpenLabel(later.toISOString(), now)).toBe(`${weekday(later)} ${clockTime(later)}`);
    expect(scheduleOpenLabel('', now)).toBe('');
    expect(scheduleOpenLabel('not a date', now)).toBe('');
  });

  it('ignores an open schedule window', () => {
    const state = { ...idlePlayThrottle, schedule: { blocked: false, nextOpenAt: '' } };
    expect(playThrottleBanner(state)).toBeNull();
    expect(playThrottleBanner({ ...state, active: true, mode: 'pause' })).toEqual({ kind: 'pause' });
  });

  it('carries the effective limits in limit mode', () => {
    expect(playThrottleBanner({ active: true, mode: 'limit', downloadLimit: 5242880, uploadLimit: 1048576 })).toEqual({
      kind: 'limit',
      downloadLimit: 5242880,
      uploadLimit: 1048576,
    });
  });
});
