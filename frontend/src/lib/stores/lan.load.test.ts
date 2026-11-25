import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
// The settings store as App holds it; only `lanSharing` is read here.
vi.mock('./settings', async () => {
  const { writable } = await import('svelte/store');
  return { settings: writable<{ lanSharing: boolean } | null>(null) };
});

const emptyStats = { announcesSent: 0, announcesReceived: 0, rejected: null, peersKnown: 0, offersKnown: 0, sharesActive: 0 };

vi.mock('../services/lan', () => ({
  getPeers: vi.fn(async () => []),
  getOffers: vi.fn(async () => []),
  getShares: vi.fn(async () => []),
  getStats: vi.fn(async () => ({ ...emptyStats })),
  share: vi.fn(),
  unshare: vi.fn(),
  receive: vi.fn(),
  cancel: vi.fn(),
}));

const offer = (title: string) => ({ peerId: 'p1', host: 'PC-1', addr: '', port: 0, gameId: 'g', title, version: '', exe: '', sizeBytes: 1, infoHash: title, lastSeen: '' });

async function fresh() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const service = await import('../services/lan');
  const store = await import('./lan');
  const { settings } = await import('./settings');
  const { toast } = await import('./toasts');
  return { service, store, settings: settings as unknown as { set: (value: { lanSharing: boolean } | null) => void }, toast };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe('lan store: the first read', () => {
  it('until the first read answers, the store says it has not read anything', async () => {
    const { service, store } = await fresh();
    vi.mocked(service.getShares).mockReturnValueOnce(new Promise(() => {}));

    void store.initLan();
    await Promise.resolve();

    expect(get(store.lanLoaded)).toBe(false);
    expect(get(store.lanLoadError)).toBeNull();
  });

  it('a failed first read never rejects: it is kept as the load error, with no toast', async () => {
    const { service, store, toast } = await fresh();
    const cause = new Error('typhon:lan.library_unavailable: lan: library service unavailable');
    vi.mocked(service.getShares).mockRejectedValueOnce(cause);

    await expect(store.initLan()).resolves.toBeUndefined();

    expect(get(store.lanLoaded)).toBe(false);
    expect(get(store.lanLoadError)).toBe(cause);
    expect(toast).not.toHaveBeenCalled();
  });

  it('a read again after the failure fills the store, clears the error and answers true', async () => {
    const { service, store } = await fresh();
    vi.mocked(service.getShares).mockRejectedValueOnce(new Error('not ready'));
    await store.initLan();
    vi.mocked(service.getOffers).mockResolvedValueOnce([offer('Portal 2')] as never);

    await expect(store.reloadLan()).resolves.toBe(true);

    expect(get(store.lanLoaded)).toBe(true);
    expect(get(store.lanLoadError)).toBeNull();
    expect(get(store.offers).map((o) => o.title)).toEqual(['Portal 2']);
  });

  it('a read again that fails too keeps the error and answers false', async () => {
    const { service, store } = await fresh();
    vi.mocked(service.getShares).mockRejectedValueOnce(new Error('not ready'));
    await store.initLan();
    vi.mocked(service.getShares).mockRejectedValueOnce(new Error('still not ready'));

    await expect(store.reloadLan()).resolves.toBe(false);

    expect(get(store.lanLoaded)).toBe(false);
    expect((get(store.lanLoadError) as Error).message).toBe('still not ready');
  });

  it('an event during a read again gets the snapshot asked once more', async () => {
    const { service, store } = await fresh();
    vi.mocked(service.getShares).mockRejectedValueOnce(new Error('not ready'));
    await store.initLan();
    vi.mocked(service.getOffers).mockImplementationOnce(async () => {
      handlers['lan:stats']({ data: { ...emptyStats, peersKnown: 1 } });
      return [offer('old')] as never;
    });
    vi.mocked(service.getOffers).mockResolvedValueOnce([offer('new')] as never);

    await store.reloadLan();

    expect(get(store.offers).map((o) => o.title)).toEqual(['new']);
  });

  it('starts no timer: nothing is read in the background until a screen asks for it', async () => {
    const { service, store } = await fresh();

    await store.initLan();
    vi.mocked(service.getOffers).mockClear();
    await vi.advanceTimersByTimeAsync(store.OFFERS_POLL_MS * 3);

    expect(vi.getTimerCount()).toBe(0);
    expect(service.getOffers).not.toHaveBeenCalled();
  });
});

describe('lan store: the offers poll', () => {
  it('while watched with sharing on, reads the offers at once and then on every tick', async () => {
    const { service, store, settings } = await fresh();
    await store.initLan();
    settings.set({ lanSharing: true });
    vi.mocked(service.getOffers).mockClear();

    const stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);
    expect(service.getOffers).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(store.OFFERS_POLL_MS * 2);
    expect(service.getOffers).toHaveBeenCalledTimes(3);

    stop();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('two watchers share one timer; it stops with the last of them', async () => {
    const { store, settings } = await fresh();
    settings.set({ lanSharing: true });

    const first = store.watchOffers();
    const second = store.watchOffers();
    expect(vi.getTimerCount()).toBe(1);

    first();
    first();
    expect(vi.getTimerCount()).toBe(1);
    second();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('with sharing off, reads nothing: Go has no offers to give then', async () => {
    const { service, store, settings } = await fresh();
    settings.set({ lanSharing: false });

    const stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(store.OFFERS_POLL_MS * 2);

    expect(service.getOffers).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
    stop();
  });

  it('sharing turned on while watched starts the poll; turned off stops it, empties the offers and reads the figures again', async () => {
    const { service, store, settings } = await fresh();
    settings.set({ lanSharing: false });
    const stop = store.watchOffers();
    vi.mocked(service.getOffers).mockResolvedValue([offer('Portal 2')] as never);

    settings.set({ lanSharing: true });
    await vi.advanceTimersByTimeAsync(0);
    expect(get(store.offers)).toHaveLength(1);
    expect(vi.getTimerCount()).toBe(1);

    vi.mocked(service.getStats).mockResolvedValueOnce({ ...emptyStats, sharesActive: 2 } as never);
    settings.set({ lanSharing: false });
    await vi.advanceTimersByTimeAsync(0);
    expect(vi.getTimerCount()).toBe(0);
    expect(get(store.offers)).toEqual([]);
    expect(get(store.lanStats).sharesActive).toBe(2);
    stop();
  });

  it('sharing turned off in Settings while no screen watched: the first watch empties the offers and reads the figures once', async () => {
    const { service, store, settings } = await fresh();
    settings.set({ lanSharing: true });
    vi.mocked(service.getStats).mockResolvedValueOnce({ ...emptyStats, peersKnown: 3, offersKnown: 5 } as never);
    vi.mocked(service.getOffers).mockResolvedValueOnce([offer('Portal 2')] as never);
    await store.initLan();
    settings.set({ lanSharing: false });
    vi.mocked(service.getStats).mockClear();

    const stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);
    settings.set({ lanSharing: false });
    await vi.advanceTimersByTimeAsync(0);

    expect(service.getStats).toHaveBeenCalledTimes(1);
    expect(get(store.lanStats).peersKnown).toBe(0);
    expect(get(store.lanStats).offersKnown).toBe(0);
    expect(get(store.offers)).toEqual([]);
    stop();
  });

  it('off, on and off again across watches: each watch that finds sharing off after it was on settles once', async () => {
    const { service, store, settings } = await fresh();
    settings.set({ lanSharing: false });
    let stop = store.watchOffers();
    stop();
    settings.set({ lanSharing: true });
    vi.mocked(service.getOffers).mockResolvedValueOnce([offer('Portal 2')] as never);
    stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);
    expect(get(store.offers)).toHaveLength(1);
    stop();

    settings.set({ lanSharing: false });
    vi.mocked(service.getStats).mockClear();
    stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);

    expect(service.getStats).toHaveBeenCalledTimes(1);
    expect(get(store.offers)).toEqual([]);
    stop();
  });

  it('figures Go sent while sharing was on and no screen watched are read again by the next watch that finds it off', async () => {
    const { service, store, settings } = await fresh();
    await store.initLan();
    settings.set({ lanSharing: false });
    let stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);
    stop();
    settings.set({ lanSharing: true });
    handlers['lan:stats']({ data: { ...emptyStats, peersKnown: 4 } });
    settings.set({ lanSharing: false });
    vi.mocked(service.getStats).mockClear();

    stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);

    expect(service.getStats).toHaveBeenCalledTimes(1);
    expect(get(store.lanStats).peersKnown).toBe(0);
    stop();
  });

  it('a read again while sharing was on is settled again by the next watch that finds it off', async () => {
    const { service, store, settings } = await fresh();
    settings.set({ lanSharing: false });
    let stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);
    stop();
    settings.set({ lanSharing: true });
    vi.mocked(service.getStats).mockResolvedValueOnce({ ...emptyStats, peersKnown: 3 } as never);
    await store.reloadLan();
    settings.set({ lanSharing: false });
    vi.mocked(service.getStats).mockClear();

    stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);

    expect(service.getStats).toHaveBeenCalledTimes(1);
    expect(get(store.lanStats).peersKnown).toBe(0);
    stop();
  });

  it('a poll answer that arrives after sharing went off does not bring the offers back', async () => {
    const { service, store, settings } = await fresh();
    let answer!: (value: never) => void;
    vi.mocked(service.getOffers).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)) as never);
    settings.set({ lanSharing: true });
    const stop = store.watchOffers();

    settings.set({ lanSharing: false });
    answer([offer('late')] as never);
    await vi.advanceTimersByTimeAsync(0);

    expect(get(store.offers)).toEqual([]);
    stop();
  });

  it('a failed poll keeps the list it has and says nothing', async () => {
    const { service, store, settings, toast } = await fresh();
    vi.mocked(service.getOffers).mockResolvedValueOnce([offer('Portal 2')] as never);
    settings.set({ lanSharing: true });
    const stop = store.watchOffers();
    await vi.advanceTimersByTimeAsync(0);
    vi.mocked(service.getOffers).mockRejectedValue(new Error('dial tcp: connection refused'));

    await vi.advanceTimersByTimeAsync(store.OFFERS_POLL_MS * 3);

    expect(get(store.offers).map((o) => o.title)).toEqual(['Portal 2']);
    expect(toast).not.toHaveBeenCalled();
    stop();
  });
});
