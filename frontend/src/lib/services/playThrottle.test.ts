import { describe, expect, it, vi } from 'vitest';

vi.mock('../../../bindings/typhon/internal/download', () => ({ Manager: { PlayThrottle: vi.fn() } }));
vi.mock('./backend', () => ({ inWails: true }));

import { idlePlayThrottle, playThrottleBanner } from './playThrottle';

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

  it('carries the effective limits in limit mode', () => {
    expect(playThrottleBanner({ active: true, mode: 'limit', downloadLimit: 5242880, uploadLimit: 1048576 })).toEqual({
      kind: 'limit',
      downloadLimit: 5242880,
      uploadLimit: 1048576,
    });
  });
});
