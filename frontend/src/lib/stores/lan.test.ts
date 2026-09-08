import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
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

vi.mock('../services/lan', () => ({
  getPeers: vi.fn(async () => []),
  getOffers: vi.fn(async () => []),
  getShares: vi.fn(async () => []),
  getStats: vi.fn(async () => ({
    announcesSent: 0,
    announcesReceived: 0,
    rejected: null,
    peersKnown: 0,
    offersKnown: 0,
    sharesActive: 0,
  })),
  share: vi.fn(),
  unshare: vi.fn(),
  receive: vi.fn(),
  cancel: vi.fn(),
}));

function makeTransfer(patch: Partial<Record<string, unknown>> = {}) {
  return {
    id: 't1',
    infoHash: 'hash',
    peerId: 'p1',
    gameId: 'g1',
    title: 'Игра',
    downloaded: 0,
    total: 100,
    status: 'receiving',
    startedAt: '',
    updatedAt: '',
    ...patch,
  };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const service = await import('../services/lan');
  const store = await import('./lan');
  await store.initLan();
  return { service, store };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('transfers store', () => {
  it('drops a transfer once it reaches a terminal status instead of keeping it forever', async () => {
    const { store } = await load();

    handlers['lan:transfer']({ data: makeTransfer({ status: 'receiving' }) });
    expect(get(store.transfers)).toHaveLength(1);

    handlers['lan:transfer']({ data: makeTransfer({ status: 'completed' }) });

    expect(get(store.transfers)).toHaveLength(0);
  });
});

describe('share / unshare race', () => {
  function makeShare(patch: Partial<Record<string, unknown>> = {}) {
    return {
      gameId: 'g1',
      title: 'Игра',
      version: '1.0',
      exe: 'game.exe',
      root: 'C:/games/g1',
      infoHash: 'hash',
      sizeBytes: 1000,
      fingerprint: 'fp',
      builtAt: '',
      enabled: true,
      ...patch,
    };
  }

  it('does not let a slow share() response resurrect a game the user already unshared', async () => {
    const { service, store } = await load();

    let resolveShare!: (value: unknown) => void;
    vi.mocked(service.share).mockReturnValue(
      new Promise((resolve) => {
        resolveShare = resolve;
      }) as never,
    );
    vi.mocked(service.unshare).mockResolvedValue(undefined as never);

    store.share('g1');
    await store.unshare('g1');
    expect(get(store.shares)).toHaveLength(0);

    resolveShare(makeShare());
    await Promise.resolve();
    await Promise.resolve();

    expect(get(store.shares)).toHaveLength(0);
  });
});

describe('offers polling', () => {
  it('toasts once when the backend goes unreachable, not on every 10s tick', async () => {
    const { service } = await load();
    vi.mocked(service.getOffers).mockRejectedValue(new Error('dial tcp: connection refused'));

    const { toast } = await import('./toasts');
    vi.mocked(toast).mockClear();

    await vi.advanceTimersByTimeAsync(10000);
    await vi.advanceTimersByTimeAsync(10000);
    await vi.advanceTimersByTimeAsync(10000);

    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('allows a new toast once polling recovers and fails again', async () => {
    const { service } = await load();
    vi.mocked(service.getOffers).mockRejectedValue(new Error('dial tcp: connection refused'));

    const { toast } = await import('./toasts');
    vi.mocked(toast).mockClear();

    await vi.advanceTimersByTimeAsync(10000);
    expect(toast).toHaveBeenCalledTimes(1);

    vi.mocked(service.getOffers).mockResolvedValueOnce([]);
    await vi.advanceTimersByTimeAsync(10000);

    vi.mocked(service.getOffers).mockRejectedValue(new Error('dial tcp: connection refused'));
    await vi.advanceTimersByTimeAsync(10000);

    expect(toast).toHaveBeenCalledTimes(2);
  });
});
