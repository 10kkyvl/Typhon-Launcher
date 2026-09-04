import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
  Call: { ByID: vi.fn() },
  CancellablePromise: class {},
}));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/account', () => {
  class AccountError extends Error {
    code: string;
    field: string;
    constructor(code: string, field = '') {
      super(code);
      this.name = 'AccountError';
      this.code = code;
      this.field = field;
    }
  }
  return { AccountError };
});
vi.mock('../services/social', () => ({
  feed: vi.fn(async () => ({ events: [], next: 0 })),
  react: vi.fn(async () => {}),
  unreact: vi.fn(async () => {}),
}));

function event(id: number, emoji = 'fire', count = 0) {
  return {
    id,
    user: { id: `u${id}`, username: `u${id}`, displayName: `u${id}`, avatarUrl: '' },
    kind: 'completed',
    game: { igdbId: 1, title: 'Game', coverUrl: '' },
    createdAt: '2026-09-03T10:00:00Z',
    reactions: count > 0 ? [{ emoji, count }] : [],
    mine: [] as string[],
  };
}

async function load() {
  vi.resetModules();
  const feed = await import('./feed');
  const { feed: fetchFeed } = await import('../services/social');
  return { ...feed, fetchFeed: vi.mocked(fetchFeed) };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-09-03T12:00:00Z'));
});

afterEach(() => {
  vi.useRealTimers();
});

describe('свежесть ленты', () => {
  it('первый вход загружает ленту', async () => {
    const { loadFeed, fetchFeed, feedReady } = await load();

    await loadFeed();

    expect(fetchFeed).toHaveBeenCalledTimes(1);
    expect(get(feedReady)).toBe(true);
  });

  it('повторный вход до истечения срока не ходит в сеть', async () => {
    const { loadFeed, fetchFeed, staleAfter } = await load();
    await loadFeed();

    vi.advanceTimersByTime(staleAfter - 1);
    await loadFeed();

    expect(fetchFeed).toHaveBeenCalledTimes(1);
  });

  it('вход после истечения срока перечитывает ленту', async () => {
    const { loadFeed, fetchFeed, staleAfter } = await load();
    await loadFeed();

    vi.advanceTimersByTime(staleAfter);
    await loadFeed();

    expect(fetchFeed).toHaveBeenCalledTimes(2);
  });

  it('принудительная загрузка перечитывает ленту сразу', async () => {
    const { loadFeed, fetchFeed } = await load();
    await loadFeed();

    await loadFeed(true);

    expect(fetchFeed).toHaveBeenCalledTimes(2);
  });

  it('неудачная загрузка не считается свежей', async () => {
    const { loadFeed, fetchFeed } = await load();
    fetchFeed.mockRejectedValueOnce(new Error('offline'));

    await loadFeed();
    await loadFeed();

    expect(fetchFeed).toHaveBeenCalledTimes(2);
  });

  it('сброс ленты заставляет перечитать её', async () => {
    const { loadFeed, resetFeed, fetchFeed } = await load();
    await loadFeed();

    resetFeed();
    await loadFeed();

    expect(fetchFeed).toHaveBeenCalledTimes(2);
  });
});

describe('очередь загрузок', () => {
  it('принудительная загрузка во время запроса выполняется после него', async () => {
    const { loadFeed, fetchFeed, feedEvents } = await load();
    let release = () => {};
    const first = new Promise<{ events: ReturnType<typeof event>[]; next: number }>((resolve) => {
      release = () => resolve({ events: [event(1)], next: 0 });
    });
    fetchFeed.mockReturnValueOnce(first as never);
    fetchFeed.mockResolvedValueOnce({ events: [event(2)], next: 0 } as never);

    const pendingLoad = loadFeed();
    const forced = loadFeed(true);
    release();
    await Promise.all([pendingLoad, forced]);

    expect(fetchFeed).toHaveBeenCalledTimes(2);
    expect(get(feedEvents).map((e) => e.id)).toEqual([2]);
  });

  it('обычная загрузка во время запроса не дублирует его', async () => {
    const { loadFeed, fetchFeed } = await load();

    await Promise.all([loadFeed(), loadFeed()]);

    expect(fetchFeed).toHaveBeenCalledTimes(1);
  });
});

describe('реакции', () => {
  it('событие помечено занятым, пока реакция в полёте', async () => {
    const { loadFeed, reactToEvent, fetchFeed, feedPending } = await load();
    fetchFeed.mockResolvedValueOnce({ events: [event(7)], next: 0 } as never);
    await loadFeed();

    const { react } = await import('../services/social');
    let release = () => {};
    vi.mocked(react).mockReturnValueOnce(
      new Promise<void>((resolve) => {
        release = resolve;
      }),
    );

    const sending = reactToEvent(7, 'fire');
    expect(get(feedPending).has(7)).toBe(true);

    release();
    await sending;

    expect(get(feedPending).has(7)).toBe(false);
  });
});
