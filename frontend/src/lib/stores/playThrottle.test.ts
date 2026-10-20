import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));

const fetchPlayThrottle = vi.fn();
vi.mock('../services/playThrottle', () => ({
  fetchPlayThrottle: () => fetchPlayThrottle(),
  idlePlayThrottle: { active: false, mode: 'off', downloadLimit: 0, uploadLimit: 0 },
}));

import { toast } from './toasts';
import { initPlayThrottle, playThrottle } from './playThrottle';

describe('playThrottle store', () => {
  beforeEach(() => {
    fetchPlayThrottle.mockReset();
    vi.mocked(toast).mockReset();
  });

  it('loads the initial state and follows the event', async () => {
    fetchPlayThrottle.mockResolvedValue({ active: true, mode: 'pause', downloadLimit: 0, uploadLimit: 0 });
    await initPlayThrottle();
    expect(get(playThrottle).mode).toBe('pause');
    handlers['downloads:play-throttle']({ data: { active: false, mode: 'off', downloadLimit: 0, uploadLimit: 0 } });
    expect(get(playThrottle).active).toBe(false);
  });

  it('shows a toast when the initial fetch fails', async () => {
    fetchPlayThrottle.mockRejectedValue(new Error('boom'));
    await initPlayThrottle();
    expect(toast).toHaveBeenCalledWith(expect.any(String), 'danger');
  });
});

describe('playThrottle store ordering', () => {
  it('keeps an event that arrives while the initial fetch is in flight', async () => {
    let resolve!: (v: unknown) => void;
    fetchPlayThrottle.mockReturnValue(new Promise((r) => (resolve = r)));
    const init = initPlayThrottle();
    handlers['downloads:play-throttle']({ data: { active: true, mode: 'limit', downloadLimit: 1, uploadLimit: 1 } });
    resolve({ active: false, mode: 'off', downloadLimit: 0, uploadLimit: 0 });
    await init;
    expect(get(playThrottle).mode).toBe('limit');
  });
});
