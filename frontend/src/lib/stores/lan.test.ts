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

vi.mock('./settings', async () => {
  const { writable } = await import('svelte/store');
  return { settings: writable(null) };
});

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

// Stage 1: what Go's events and answers do to the store today, pinned before the screen is rebuilt.
describe('lan store pins: events', () => {
  it('lan:peers, lan:shares and lan:stats replace what the store holds', async () => {
    const { store } = await load();
    const stats = { announcesSent: 1, announcesReceived: 2, rejected: null, peersKnown: 3, offersKnown: 4, sharesActive: 5 };

    handlers['lan:peers']({ data: [{ id: 'p1', host: 'PC-1', addr: '', port: 0, lastSeen: '' }] });
    handlers['lan:shares']({ data: [{ gameId: 'g1', title: 'Игра', enabled: true }] });
    handlers['lan:stats']({ data: stats });

    expect(get(store.peers).map((p) => p.id)).toEqual(['p1']);
    expect(get(store.shares).map((s) => s.gameId)).toEqual(['g1']);
    expect(get(store.lanStats)).toEqual(stats);
  });

  it('lan:hashing adds a game in preparation and drops it when Go says it is done', async () => {
    const { store } = await load();

    handlers['lan:hashing']({ data: { gameId: 'g1', processedBytes: 5, totalBytes: 10, currentFile: '', done: false } });
    expect([...get(store.hashing).keys()]).toEqual(['g1']);

    handlers['lan:hashing']({ data: { gameId: 'g1', processedBytes: 10, totalBytes: 10, currentFile: '', done: true } });
    expect(get(store.hashing).size).toBe(0);
  });

  it('lan:transfer updates a receiving transfer in place', async () => {
    const { store } = await load();

    handlers['lan:transfer']({ data: makeTransfer({ downloaded: 10 }) });
    handlers['lan:transfer']({ data: makeTransfer({ downloaded: 60 }) });

    expect(get(store.transfers).map((t) => t.downloaded)).toEqual([60]);
  });
});

// Go's Shares and lan:shares list every game it ever prepared, a stopped one with `enabled: false`
// (internal/lan store.list): only the games still shared are the store's.
describe('lan store: games still shared', () => {
  const listed = [
    { gameId: 'g1', title: 'Игра', enabled: true },
    { gameId: 'g2', title: 'Другая', enabled: false },
  ];

  it('the first read keeps only the games still shared', async () => {
    vi.resetModules();
    const service = await import('../services/lan');
    vi.mocked(service.getShares).mockResolvedValueOnce(listed as never);
    const store = await import('./lan');

    await store.initLan();

    expect(get(store.shares).map((s) => s.gameId)).toEqual(['g1']);
  });

  it('lan:shares keeps only the games still shared: a stopped one leaves', async () => {
    const { store } = await load();

    handlers['lan:shares']({ data: listed });

    expect(get(store.shares).map((s) => s.gameId)).toEqual(['g1']);
  });
});

describe('lan store pins: answers', () => {
  it('a transfer Go starts joins the list', async () => {
    const { service, store } = await load();
    vi.mocked(service.receive).mockResolvedValue(makeTransfer({ id: 'new' }) as never);

    await store.receive('hash', 'p1');

    expect(service.receive).toHaveBeenCalledWith('hash', 'p1');
    expect(get(store.transfers).map((t) => t.id)).toEqual(['new']);
  });

  it('a refused download is said in the words of its code, never in Go’s text', async () => {
    const { service, store } = await load();
    const { msg } = await import('../i18n');
    vi.mocked(service.receive).mockRejectedValue(new Error('typhon:lan.already_have: lan: this game is already shared or being received'));
    const { toast } = await import('./toasts');

    await store.receive('hash', 'p1');

    expect(toast).toHaveBeenCalledExactlyOnceWith(msg('errSources.lanAlreadyHave'), 'danger');
    expect(get(store.transfers)).toEqual([]);
  });

  it('a stopped share leaves the list once Go answers', async () => {
    const { service, store } = await load();
    handlers['lan:shares']({ data: [{ gameId: 'g1', title: 'Игра', enabled: true }] });
    vi.mocked(service.unshare).mockResolvedValue(undefined as never);

    await store.unshare('g1');

    expect(service.unshare).toHaveBeenCalledWith('g1');
    expect(get(store.shares)).toEqual([]);
  });

  it('a cancel goes to Go by the transfer’s id; the transfer leaves when Go reports it cancelled', async () => {
    const { service, store } = await load();
    handlers['lan:transfer']({ data: makeTransfer() });
    vi.mocked(service.cancel).mockResolvedValue(undefined as never);

    await store.cancel('t1');
    expect(service.cancel).toHaveBeenCalledWith('t1');
    expect(get(store.transfers)).toHaveLength(1);

    handlers['lan:transfer']({ data: makeTransfer({ status: 'cancelled' }) });
    expect(get(store.transfers)).toHaveLength(0);
  });

  it('a cancel asked through a confirmation rejects with Go’s refusal and leaves the saying to it', async () => {
    const { service, store } = await load();
    const refusal = new Error('typhon:lan.unknown_transfer: lan: unknown transfer');
    vi.mocked(service.cancel).mockRejectedValue(refusal);
    const { toast } = await import('./toasts');

    await expect(store.cancelTransfer('t1')).rejects.toBe(refusal);

    expect(toast).not.toHaveBeenCalled();
  });
});
