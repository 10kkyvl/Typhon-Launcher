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
  setNote: vi.fn(async () => {}),
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
    note: '',
    seconds: 0,
    achievements: [],
    review: null,
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

describe('подпись события', () => {
  it('сохраняет подпись и показывает её сразу', async () => {
    const { loadFeed, noteEvent, fetchFeed, feedEvents } = await load();
    fetchFeed.mockResolvedValueOnce({ events: [event(7)], next: 0 } as never);
    await loadFeed();

    const { setNote } = await import('../services/social');
    await noteEvent(7, '  Прошёл спустя три года  ');

    expect(vi.mocked(setNote)).toHaveBeenCalledWith('7', 'Прошёл спустя три года');
    expect(get(feedEvents)[0].note).toBe('Прошёл спустя три года');
  });

  it('возвращает прежнюю подпись, когда сервер отказал', async () => {
    const { loadFeed, noteEvent, fetchFeed, feedEvents } = await load();
    fetchFeed.mockResolvedValueOnce({ events: [{ ...event(7), note: 'старая' }], next: 0 } as never);
    await loadFeed();

    const { setNote } = await import('../services/social');
    vi.mocked(setNote).mockRejectedValueOnce(new Error('offline'));

    await noteEvent(7, 'новая');

    expect(get(feedEvents)[0].note).toBe('старая');
  });

  it('не шлёт запрос, когда подпись не изменилась', async () => {
    const { loadFeed, noteEvent, fetchFeed } = await load();
    fetchFeed.mockResolvedValueOnce({ events: [{ ...event(7), note: 'та же' }], next: 0 } as never);
    await loadFeed();

    const { setNote } = await import('../services/social');
    await noteEvent(7, '  та же  ');

    expect(vi.mocked(setNote)).not.toHaveBeenCalled();
  });

  it('обрезает подпись до лимита перед отправкой', async () => {
    const { loadFeed, noteEvent, fetchFeed } = await load();
    fetchFeed.mockResolvedValueOnce({ events: [event(7)], next: 0 } as never);
    await loadFeed();

    const { setNote } = await import('../services/social');
    await noteEvent(7, 'я'.repeat(1200));

    expect(vi.mocked(setNote)).toHaveBeenCalledWith('7', 'я'.repeat(1000));
  });

  it('молчит про неизвестное событие', async () => {
    const { noteEvent } = await load();
    const { setNote } = await import('../services/social');

    await noteEvent(99, 'что-то');

    expect(vi.mocked(setNote)).not.toHaveBeenCalled();
  });
});

describe('страницы без известных событий', () => {
  it('идёт на следующую страницу, пока не найдёт событие', async () => {
    const { loadFeed, feedEvents, feedCursor, fetchFeed } = await load();
    fetchFeed
      .mockResolvedValueOnce({ events: [], next: 5 })
      .mockResolvedValueOnce({ events: [], next: 4 })
      .mockResolvedValueOnce({ events: [event(1)], next: 3 });

    await loadFeed(true);

    expect(fetchFeed.mock.calls.map((c) => c[0])).toEqual(['', '5', '4']);
    expect(get(feedEvents).map((e) => e.id)).toEqual([1]);
    expect(get(feedCursor)).toBe(3);
  });

  it('останавливается после трёх дополнительных страниц', async () => {
    const { loadFeed, feedCursor, fetchFeed } = await load();
    fetchFeed.mockResolvedValue({ events: [], next: 9 });

    await loadFeed(true);

    expect(fetchFeed).toHaveBeenCalledTimes(4);
    expect(get(feedCursor)).toBe(9);
  });

  it('не ходит дальше, когда курсора нет', async () => {
    const { loadFeed, fetchFeed } = await load();
    fetchFeed.mockResolvedValue({ events: [], next: 0 });

    await loadFeed(true);

    expect(fetchFeed).toHaveBeenCalledTimes(1);
  });
});

describe('неудачное чтение ленты', () => {
  it('провал первой страницы остаётся состоянием ленты, без тоста', async () => {
    const { loadFeed, fetchFeed, feedReady, feedLoadError } = await load();
    const { toast } = await import('./toasts');
    const failure = new Error('offline');
    fetchFeed.mockRejectedValueOnce(failure);

    await loadFeed(true);

    expect(get(feedReady)).toBe(false);
    expect(get(feedLoadError)).toBe(failure);
    expect(vi.mocked(toast)).not.toHaveBeenCalled();
  });

  it('провал фонового обновления уже прочитанной ленты говорит тостом и не трогает состояние', async () => {
    const { loadFeed, fetchFeed, feedReady, feedLoadError, feedEvents } = await load();
    const { toast } = await import('./toasts');
    fetchFeed.mockResolvedValueOnce({ events: [event(1)], next: 0 } as never).mockRejectedValueOnce(new Error('offline'));
    await loadFeed(true);

    await loadFeed(true);

    expect(get(feedReady)).toBe(true);
    expect(get(feedLoadError)).toBeNull();
    expect(get(feedEvents).map((e) => e.id)).toEqual([1]);
    expect(vi.mocked(toast)).toHaveBeenCalledTimes(1);
  });

  it('повторное чтение отвечает, пришла ли лента, и снимает провал', async () => {
    const { reloadFeed, fetchFeed, feedReady, feedLoadError } = await load();
    fetchFeed.mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ events: [], next: 0 });

    expect(await reloadFeed()).toBe(false);
    expect(await reloadFeed()).toBe(false);
    expect(get(feedLoadError)).not.toBeNull();
    expect(await reloadFeed()).toBe(true);
    expect(get(feedReady)).toBe(true);
    expect(get(feedLoadError)).toBeNull();
  });

  it('провал «Показать ещё» сохраняет список и остаётся состоянием до новой попытки, без тоста', async () => {
    const { loadFeed, moreFeed, fetchFeed, feedEvents, feedMoreError, feedLoadError } = await load();
    const { toast } = await import('./toasts');
    const failure = new Error('offline');
    let release = () => {};
    fetchFeed
      .mockResolvedValueOnce({ events: [event(1)], next: 5 } as never)
      .mockRejectedValueOnce(failure)
      .mockReturnValueOnce(new Promise((resolve) => (release = () => resolve({ events: [event(2)], next: 0 }))) as never);
    await loadFeed(true);

    await moreFeed();

    expect(get(feedEvents).map((e) => e.id)).toEqual([1]);
    expect(get(feedMoreError)).toBe(failure);
    expect(get(feedLoadError)).toBeNull();
    expect(vi.mocked(toast)).not.toHaveBeenCalled();

    const next = moreFeed();
    await vi.waitFor(() => expect(get(feedMoreError)).toBeNull());
    release();
    await next;
    expect(get(feedEvents).map((e) => e.id)).toEqual([1, 2]);
  });

  it('повторное чтение, прерванное сбросом, отвечает тем, что принесло чтение после сброса', async () => {
    const { reloadFeed, loadFeed, resetFeed, fetchFeed } = await load();
    let cut = () => {};
    let bring = () => {};
    fetchFeed
      .mockReturnValueOnce(new Promise((_resolve, reject) => (cut = () => reject(new Error('offline')))) as never)
      .mockReturnValueOnce(new Promise((resolve) => (bring = () => resolve({ events: [event(1)], next: 0 }))) as never);

    const answer = reloadFeed();
    resetFeed();
    const after = loadFeed(true);
    cut();
    await vi.advanceTimersByTimeAsync(0);
    bring();

    expect(await answer).toBe(true);
    await after;
    expect(fetchFeed).toHaveBeenCalledTimes(2);
  });

  it('каждый сброс ленты виден экрану, чтобы тот перечитал первую страницу', async () => {
    const { resetFeed, feedResets } = await load();
    const seen: number[] = [];
    const stop = feedResets.subscribe((count) => seen.push(count));

    resetFeed();
    resetFeed();
    stop();

    expect(seen).toEqual([seen[0], seen[0] + 1, seen[0] + 2]);
  });

  it('удачное чтение первой страницы снимает провал «Показать ещё»: список уже другой', async () => {
    const { loadFeed, moreFeed, fetchFeed, feedMoreError } = await load();
    fetchFeed
      .mockResolvedValueOnce({ events: [event(1)], next: 5 } as never)
      .mockRejectedValueOnce(new Error('offline'))
      .mockRejectedValueOnce(new Error('still offline'))
      .mockResolvedValueOnce({ events: [event(1)], next: 5 } as never);
    await loadFeed(true);
    await moreFeed();
    expect(get(feedMoreError)).not.toBeNull();

    await loadFeed(true);
    expect(get(feedMoreError)).not.toBeNull();

    await loadFeed(true);
    expect(get(feedMoreError)).toBeNull();
  });

  it('сброс ленты снимает оба провала', async () => {
    const { loadFeed, moreFeed, resetFeed, fetchFeed, feedMoreError, feedLoadError } = await load();
    fetchFeed.mockRejectedValueOnce(new Error('offline'));
    await loadFeed(true);
    expect(get(feedLoadError)).not.toBeNull();
    fetchFeed.mockResolvedValueOnce({ events: [event(1)], next: 5 } as never).mockRejectedValueOnce(new Error('offline'));
    await loadFeed(true);
    await moreFeed();
    expect(get(feedMoreError)).not.toBeNull();

    resetFeed();

    expect(get(feedLoadError)).toBeNull();
    expect(get(feedMoreError)).toBeNull();
  });
});
