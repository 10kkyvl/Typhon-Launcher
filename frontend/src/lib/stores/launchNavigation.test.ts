import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CancelError } from '@wailsio/runtime';

const handlers: Array<() => void> = [];
const off = vi.fn();

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@wailsio/runtime')>()),
  Events: {
    On: vi.fn((_name: string, handler: () => void) => {
      handlers.push(handler);
      return off;
    }),
  },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./router', async () => {
  const { writable } = await import('svelte/store');
  return { navigate: vi.fn(), route: writable({ name: 'library', params: {}, key: 1 }) };
});
vi.mock('../bigpicture/mode', async () => {
  const { writable } = await import('svelte/store');
  return { bigPictureActive: writable(false) };
});
vi.mock('./toasts', () => ({ toast: vi.fn(() => 7), dismissToast: vi.fn() }));
vi.mock('../../../bindings/typhon/navigationservice', () => ({
  TakePending: vi.fn(),
  TakePendingLink: vi.fn(),
}));
vi.mock('../../../bindings/typhon/internal/deeplink/service', () => ({ Resolve: vi.fn() }));

const { TakePending, TakePendingLink } = await import('../../../bindings/typhon/navigationservice');
const { Resolve } = await import('../../../bindings/typhon/internal/deeplink/service');
const { navigate, route } = await import('./router');
const { bigPictureActive } = await import('../bigpicture/mode');
const { toast, dismissToast } = await import('./toasts');
const { initLaunchNavigation, setLinksReady, bigPictureGame, bigPictureSearch } = await import('./launchNavigation');
import { get, type Writable } from 'svelte/store';

interface Request {
  promise: Promise<unknown>;
  resolve: (value: unknown) => void;
  reject: (reason: unknown) => void;
  cancel: ReturnType<typeof vi.fn>;
}

function request(): Request {
  let resolve!: (value: unknown) => void;
  let reject!: (reason: unknown) => void;
  const base = new Promise<unknown>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  const cancel = vi.fn(() => reject(new CancelError('Promise cancelled.')));
  const promise = Object.assign(base, { cancel });
  return { promise, resolve, reject, cancel };
}

function mockResolve(...requests: Request[]) {
  const queue = [...requests];
  vi.mocked(Resolve).mockImplementation((() => queue.shift()!.promise) as never);
}

function link(provider: string, id: string, invalid = false) {
  return { provider, id, invalid };
}

async function settle() {
  await vi.advanceTimersByTimeAsync(0);
}

const NO_LINK = link('', '');

let stop: () => void;

beforeEach(() => {
  vi.useFakeTimers();
  handlers.length = 0;
  vi.mocked(TakePending).mockReset().mockResolvedValue('' as never);
  vi.mocked(TakePendingLink).mockReset().mockResolvedValue(NO_LINK as never);
  vi.mocked(Resolve).mockReset();
  vi.mocked(navigate).mockReset();
  vi.mocked(toast).mockClear();
  vi.mocked(dismissToast).mockClear();
  off.mockClear();
  (route as unknown as Writable<{ name: string; params: object; key: number }>).set({ name: 'library', params: {}, key: 1 });
  (bigPictureActive as unknown as Writable<boolean>).set(false);
  bigPictureGame.set(null);
  bigPictureSearch.set(null);
});

afterEach(() => {
  stop?.();
  vi.useRealTimers();
});

async function start() {
  stop = initLaunchNavigation();
  setLinksReady(true);
  await settle();
}

describe('initLaunchNavigation', () => {
  it('без ссылки и маршрута ничего не делает', async () => {
    await start();

    expect(TakePending).toHaveBeenCalledTimes(1);
    expect(TakePendingLink).toHaveBeenCalledTimes(1);
    expect(navigate).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
    expect(Resolve).not.toHaveBeenCalled();
  });

  it('открывает известный маршрут запуска', async () => {
    vi.mocked(TakePending).mockResolvedValue('downloads' as never);

    await start();

    expect(navigate).toHaveBeenCalledWith('downloads');
  });

  it('найденная игра открывает карточку', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'found', gameId: 'game-9' });
    await settle();

    expect(Resolve).toHaveBeenCalledWith({ provider: 'steam', id: '620' });
    expect(navigate).toHaveBeenCalledWith('game', { id: 'game-9' });
    expect(toast).not.toHaveBeenCalled();
  });

  it('ненайденная игра говорит об этом и никуда не ведёт', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('igdb', '1877') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'not_found', gameId: '' });
    await settle();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Игра не найдена', 'danger');
  });

  it('found без id не открывает пустой экран', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('igdb', '1') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'found', gameId: '' });
    await settle();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Не удалось открыть ссылку', 'danger');
  });

  it('некорректная ссылка не доходит до Resolve', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('', '', true) as never);

    await start();

    expect(Resolve).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Некорректная ссылка', 'danger');
  });

  it('код link.unavailable говорит про соединение', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.reject(new Error('typhon:link.unavailable: dial tcp'));
    await settle();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Не удалось открыть ссылку, проверьте соединение', 'danger');
  });

  it('ошибка без кода — общий сбой, а не подсказка про соединение', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.reject(new Error('что-то про соединение'));
    await settle();

    expect(toast).toHaveBeenCalledWith('Не удалось открыть ссылку', 'danger');
  });

  it('сбой чтения ссылки показывается пользователю', async () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(TakePendingLink).mockRejectedValue(new Error('boom'));

    await start();

    expect(toast).toHaveBeenCalledWith('Не удалось открыть ссылку', 'danger');
    spy.mockRestore();
  });

  it('отмена молчит', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.promise.catch(() => {});
    (r.promise as unknown as { cancel: () => void }).cancel();
    await settle();

    expect(toast).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
  });

  it('новая ссылка отменяет прежнюю, поздний ответ игнорируется', async () => {
    vi.mocked(TakePendingLink)
      .mockResolvedValueOnce(link('steam', '1') as never)
      .mockResolvedValueOnce(link('steam', '2') as never);
    const first = request();
    const second = request();
    mockResolve(first, second);

    await start();
    handlers[0]!();
    await settle();

    expect(first.cancel).toHaveBeenCalledTimes(1);

    first.resolve({ status: 'found', gameId: 'old' });
    second.resolve({ status: 'found', gameId: 'new' });
    await settle();

    expect(navigate).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith('game', { id: 'new' });
  });

  it('значение выдаётся один раз: повторное событие без ссылки ничего не открывает', async () => {
    vi.mocked(TakePendingLink)
      .mockResolvedValueOnce(link('steam', '620') as never)
      .mockResolvedValue(NO_LINK as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'found', gameId: 'game-9' });
    await settle();
    handlers[0]!();
    await settle();

    expect(Resolve).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledTimes(1);
  });

  it('на каждое событие читаются и маршрут, и ссылка, сначала маршрут', async () => {
    const order: string[] = [];
    vi.mocked(TakePending).mockImplementation((async () => {
      order.push('route');
      return 'downloads';
    }) as never);
    vi.mocked(TakePendingLink).mockImplementation((async () => {
      order.push('link');
      return link('steam', '620');
    }) as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'found', gameId: 'game-9' });
    await settle();

    expect(order).toEqual(['route', 'link']);
    expect(vi.mocked(navigate).mock.calls).toEqual([['downloads'], ['game', { id: 'game-9' }]]);
  });

  it('медленный Resolve показывает состояние загрузки и снимает его', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    expect(toast).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(500);
    expect(toast).toHaveBeenCalledWith('Открываем игру…', 'info');

    r.resolve({ status: 'found', gameId: 'game-9' });
    await settle();

    expect(dismissToast).toHaveBeenCalledWith(7);
  });

  it('быстрый ответ не показывает загрузку', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'found', gameId: 'game-9' });
    await settle();
    await vi.advanceTimersByTimeAsync(1000);

    expect(toast).not.toHaveBeenCalled();
    expect(dismissToast).not.toHaveBeenCalled();
  });

  it('остановка снимает подписку и отменяет идущий Resolve', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.promise.catch(() => {});
    stop();
    await settle();

    expect(off).toHaveBeenCalledTimes(1);
    expect(r.cancel).toHaveBeenCalledTimes(1);
    expect(navigate).not.toHaveBeenCalled();
  });
});

describe('готовность интерфейса', () => {
  it('до готовности ничего не забирается, после перехода в готовность — один раз', async () => {
    stop = initLaunchNavigation();
    handlers[0]!();
    await settle();

    expect(TakePending).not.toHaveBeenCalled();
    expect(TakePendingLink).not.toHaveBeenCalled();

    setLinksReady(true);
    await settle();

    expect(TakePending).toHaveBeenCalledTimes(1);
    expect(TakePendingLink).toHaveBeenCalledTimes(1);

    setLinksReady(true);
    await settle();

    expect(TakePendingLink).toHaveBeenCalledTimes(1);
  });

  it('потеря готовности отменяет идущий Resolve', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.promise.catch(() => {});
    setLinksReady(false);
    await settle();

    expect(r.cancel).toHaveBeenCalledTimes(1);
    expect(navigate).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });
});

describe('отмена идущего Resolve', () => {
  async function pendingLink() {
    vi.mocked(TakePendingLink).mockResolvedValueOnce(link('steam', '620') as never);
    const r = request();
    mockResolve(r);
    await start();
    r.promise.catch(() => {});
    return r;
  }

  it('маршрут запуска отменяет Resolve, поздний ответ не перекидывает', async () => {
    const r = await pendingLink();
    vi.mocked(TakePending).mockResolvedValueOnce('downloads' as never);
    handlers[0]!();
    await settle();

    expect(r.cancel).toHaveBeenCalledTimes(1);
    r.resolve({ status: 'found', gameId: 'late' });
    await settle();

    expect(vi.mocked(navigate).mock.calls).toEqual([['downloads']]);
  });

  it('некорректная ссылка отменяет Resolve', async () => {
    const r = await pendingLink();
    vi.mocked(TakePendingLink).mockResolvedValueOnce(link('', '', true) as never);
    handlers[0]!();
    await settle();

    expect(r.cancel).toHaveBeenCalledTimes(1);
    r.resolve({ status: 'found', gameId: 'late' });
    await settle();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Некорректная ссылка', 'danger');
  });

  it('ручная смена маршрута отменяет Resolve', async () => {
    const r = await pendingLink();
    (route as unknown as Writable<{ name: string; params: object; key: number }>).set({ name: 'catalog', params: {}, key: 2 });
    await settle();

    expect(r.cancel).toHaveBeenCalledTimes(1);
    r.resolve({ status: 'found', gameId: 'late' });
    await settle();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });
});

describe('Big Picture', () => {
  it('найденная игра уходит в Big Picture, а не в основной роутер', async () => {
    (bigPictureActive as unknown as Writable<boolean>).set(true);
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'found', gameId: 'game-9' });
    await settle();

    expect(get(bigPictureGame)).toBe('game-9');
    expect(navigate).not.toHaveBeenCalled();
  });
});

describe('ссылка поиска', () => {
  function search(query: string) {
    return { provider: '', id: '', query, invalid: false };
  }

  it('открывает каталог с запросом и не вызывает Resolve', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(search('Ведьмак 3') as never);

    await start();

    expect(navigate).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith('catalog', { q: 'Ведьмак 3' });
    expect(Resolve).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });

  it('разметка и кавычки доходят до каталога текстом, без правок', async () => {
    const text = '<img src=x onerror=alert(1)> "--play" g1';
    vi.mocked(TakePendingLink).mockResolvedValue(search(text) as never);

    await start();

    expect(navigate).toHaveBeenCalledWith('catalog', { q: text });
  });

  it('до готовности интерфейса ждёт, после неё открывается один раз', async () => {
    vi.mocked(TakePendingLink).mockResolvedValueOnce(search('Hades') as never).mockResolvedValue(NO_LINK as never);
    stop = initLaunchNavigation();
    handlers[0]!();
    await settle();

    expect(navigate).not.toHaveBeenCalled();

    setLinksReady(true);
    await settle();
    handlers[0]!();
    await settle();

    expect(vi.mocked(navigate).mock.calls).toEqual([['catalog', { q: 'Hades' }]]);
  });

  it('отменяет идущий Resolve, его поздний ответ не перекидывает', async () => {
    vi.mocked(TakePendingLink)
      .mockResolvedValueOnce(link('steam', '620') as never)
      .mockResolvedValueOnce(search('Hades') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.promise.catch(() => {});
    handlers[0]!();
    await settle();

    expect(r.cancel).toHaveBeenCalledTimes(1);
    r.resolve({ status: 'found', gameId: 'late' });
    await settle();

    expect(vi.mocked(navigate).mock.calls).toEqual([['catalog', { q: 'Hades' }]]);
    expect(toast).not.toHaveBeenCalled();
  });

  it('в Big Picture уходит в его роутер, а не в основной', async () => {
    (bigPictureActive as unknown as Writable<boolean>).set(true);
    vi.mocked(TakePendingLink).mockResolvedValue(search('Hades') as never);

    await start();

    expect(get(bigPictureSearch)).toBe('Hades');
    expect(navigate).not.toHaveBeenCalled();
    expect(Resolve).not.toHaveBeenCalled();
  });

  it('некорректная ссылка важнее запроса', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue({ provider: '', id: '', query: 'Hades', invalid: true } as never);

    await start();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Некорректная ссылка', 'danger');
  });
});

describe('статусы и коды ошибок', () => {
  it('not_in_catalog говорит, что игры нет в каталоге', async () => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.resolve({ status: 'not_in_catalog', gameId: '' });
    await settle();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Этой игры пока нет в каталоге', 'danger');
  });

  it.each([
    ['link.outdated', 'Обновите Typhon, чтобы открыть эту ссылку'],
    ['link.bad_request', 'Сервер не принял ссылку'],
    ['link.bad_response', 'Сервер ответил неожиданно, попробуйте позже'],
    ['link.too_many', 'Слишком много запросов, попробуйте чуть позже'],
    ['metadata.rate_limited', 'сервер метаданных ограничил частоту запросов'],
    ['metadata.not_configured', 'провайдер метаданных не настроен'],
    ['catalog.save_failed', 'не удалось сохранить изменения в каталоге игр'],
    ['nothing.registered_here', 'Не удалось открыть ссылку'],
  ])('код %s показывает свой текст', async (code, text) => {
    vi.mocked(TakePendingLink).mockResolvedValue(link('steam', '620') as never);
    const r = request();
    mockResolve(r);

    await start();
    r.reject(new Error(`typhon:${code}: detail`));
    await settle();

    expect(toast).toHaveBeenCalledWith(text, 'danger');
  });
});
