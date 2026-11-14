import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { LibraryGame } from '../services/library';

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

let games: LibraryGame[] = [];
let running: string[] = [];
let gamesError: Error | null = null;

// The real service over a fake binding: the payload check of its subscriptions runs as in the app.
vi.mock('../../../bindings/typhon/internal/library', () => ({
  Service: {
    GetGames: vi.fn(async () => {
      if (gamesError) throw gamesError;
      return games;
    }),
    GetRunningGames: vi.fn(async () => running),
  },
}));

const catalog = vi.hoisted(() => ({ answer: vi.fn<(ids: string[]) => Promise<unknown[]>>() }));
vi.mock('../services/sources', () => ({ getCatalogGames: catalog.answer }));

function game(patch: Partial<LibraryGame> = {}): LibraryGame {
  return { id: 'g1', title: 'Game One', uninstalled: false, ...patch } as LibraryGame;
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./library');
  const toasts = await import('./toasts');
  await store.initLibrary();
  return { store, toast: vi.mocked(toasts.toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  games = [];
  running = [];
  gamesError = null;
});

describe('library store', () => {
  it('starts from the games and the running sessions the backend reports', async () => {
    games = [game({ id: 'a' }), game({ id: 'b' })];
    running = ['b'];
    const { store } = await load();

    expect(get(store.libraryGames).map((item) => item.id)).toEqual(['a', 'b']);
    expect([...get(store.runningGames)]).toEqual(['b']);
  });

  it('leaves games that were uninstalled out of the installed list but keeps them in the library', async () => {
    games = [game({ id: 'a' }), game({ id: 'b', uninstalled: true })];
    const { store } = await load();

    expect(get(store.installedGames).map((item) => item.id)).toEqual(['a']);
    expect(get(store.libraryGames).map((item) => item.id)).toEqual(['a', 'b']);
  });

  it('replaces the list from library:updated', async () => {
    games = [game({ id: 'a' })];
    const { store } = await load();

    handlers['library:updated']({ data: [game({ id: 'x' }), game({ id: 'y' })] });

    expect(get(store.libraryGames).map((item) => item.id)).toEqual(['x', 'y']);
  });

  it('counts as loaded once the first list arrives, an empty one too', async () => {
    vi.resetModules();
    const fresh = await import('./library');
    expect(get(fresh.libraryLoaded)).toBe(false);

    const { store } = await load();

    expect(get(store.libraryGames)).toEqual([]);
    expect(get(store.libraryLoaded)).toBe(true);
  });

  it('counts as loaded from library:updated when the first request failed', async () => {
    gamesError = new Error('boom');
    vi.resetModules();
    for (const key of Object.keys(handlers)) delete handlers[key];
    const store = await import('./library');
    await store.initLibrary();
    expect(get(store.libraryLoaded)).toBe(false);

    handlers['library:updated']({ data: [game({ id: 'a' })] });

    expect(get(store.libraryLoaded)).toBe(true);
    expect(get(store.libraryLoadError)).toBeNull();
  });

  it('keeps a failed first load as state, never as a rejection, and a retry loads', async () => {
    gamesError = new Error('typhon:library.game_not_found: boom');
    vi.resetModules();
    for (const key of Object.keys(handlers)) delete handlers[key];
    const store = await import('./library');

    await expect(store.initLibrary()).resolves.toBeTypeOf('function');

    expect(get(store.libraryLoadError)).toBe(gamesError);
    expect(get(store.libraryLoaded)).toBe(false);

    expect(await store.loadLibrary()).toBe(false);
    expect(get(store.libraryLoadError)).toBe(gamesError);

    gamesError = null;
    games = [game({ id: 'a' })];
    expect(await store.loadLibrary()).toBe(true);
    expect(get(store.libraryLoadError)).toBeNull();
    expect(get(store.libraryLoaded)).toBe(true);
    expect(get(store.libraryGames).map((item) => item.id)).toEqual(['a']);
  });

  it('lets a list from library:updated that arrives during the load win over the older answer', async () => {
    let answer: (value: LibraryGame[]) => void = () => {};
    vi.resetModules();
    for (const key of Object.keys(handlers)) delete handlers[key];
    const binding = await import('../../../bindings/typhon/internal/library');
    vi.mocked(binding.Service.GetGames).mockReturnValueOnce(
      new Promise((resolve) => (answer = resolve)) as unknown as ReturnType<typeof binding.Service.GetGames>,
    );
    const store = await import('./library');
    const started = store.initLibrary();

    handlers['library:updated']({ data: [game({ id: 'new' })] });
    answer([game({ id: 'old' })]);
    await started;

    expect(get(store.libraryGames).map((item) => item.id)).toEqual(['new']);
  });

  it('takes only a list from library:updated', async () => {
    games = [game({ id: 'a' })];
    const { store } = await load();

    handlers['library:updated']({ data: { not: 'a list' } });

    expect(get(store.libraryGames)).toEqual([]);
    expect(get(store.libraryLoaded)).toBe(true);
  });

  it('answers a disposer that takes every listener away', async () => {
    vi.resetModules();
    for (const key of Object.keys(handlers)) delete handlers[key];
    const runtime = await import('@wailsio/runtime');
    const store = await import('./library');

    const dispose = await store.initLibrary();
    const offs = vi.mocked(runtime.Events.On).mock.results.map((result) => result.value as ReturnType<typeof vi.fn>);
    dispose();

    expect(offs.length).toBeGreaterThan(0);
    for (const off of offs) expect(off).toHaveBeenCalledTimes(1);
  });

  it('shows an empty library when library:updated carries nothing', async () => {
    games = [game({ id: 'a' })];
    const { store } = await load();

    handlers['library:updated']({ data: null });

    expect(get(store.libraryGames)).toEqual([]);
  });
});

describe('running sessions', () => {
  it('marks a game as running when it starts', async () => {
    const { store } = await load();

    handlers['game:started']({ data: { gameId: 'a', sessionSeconds: 0, playtimeSeconds: 0 } });

    expect(get(store.runningGames).has('a')).toBe(true);
  });

  it('replaces the set instead of changing it in place, so subscribers see the change', async () => {
    const { store } = await load();
    const before = get(store.runningGames);

    handlers['game:started']({ data: { gameId: 'a', sessionSeconds: 0, playtimeSeconds: 0 } });

    expect(get(store.runningGames)).not.toBe(before);
    expect(before.size).toBe(0);
  });

  it('notes a stopped game until the next list, in stopping order, whichever comes first', async () => {
    running = ['a', 'b'];
    const { store } = await load();

    handlers['game:stopped']({ data: { gameId: 'a', sessionSeconds: 5, playtimeSeconds: 5 } });
    handlers['game:stopped']({ data: { gameId: 'b', sessionSeconds: 5, playtimeSeconds: 5 } });
    const order = [...get(store.stoppedSinceList)].sort((x, y) => y[1] - x[1]).map(([id]) => id);
    expect(order).toEqual(['b', 'a']);

    handlers['library:updated']({ data: [game({ id: 'a' }), game({ id: 'b' })] });
    expect(get(store.stoppedSinceList).size).toBe(0);

    handlers['library:updated']({ data: [game({ id: 'a' })] });
    handlers['game:stopped']({ data: { gameId: 'a', sessionSeconds: 5, playtimeSeconds: 5 } });
    expect([...get(store.stoppedSinceList).keys()]).toEqual(['a']);
  });

  it('unmarks only the game that stopped', async () => {
    running = ['a', 'b'];
    const { store } = await load();

    handlers['game:stopped']({ data: { gameId: 'a', sessionSeconds: 5, playtimeSeconds: 5 } });

    expect([...get(store.runningGames)]).toEqual(['b']);
  });

  it('stays quiet about a session shorter than a minute', async () => {
    running = ['a'];
    const { toast } = await load();

    handlers['game:stopped']({ data: { gameId: 'a', sessionSeconds: 59, playtimeSeconds: 59 } });

    expect(toast).not.toHaveBeenCalled();
  });

  it('reports a session of a minute or more, rounded to whole minutes', async () => {
    running = ['a', 'b'];
    const { toast } = await load();

    handlers['game:stopped']({ data: { gameId: 'a', sessionSeconds: 60, playtimeSeconds: 60 } });
    handlers['game:stopped']({ data: { gameId: 'b', sessionSeconds: 150, playtimeSeconds: 150 } });

    expect(toast).toHaveBeenCalledTimes(2);
    expect(toast.mock.calls[0][0]).toContain('1');
    expect(toast.mock.calls[1][0]).toContain('3');
  });

  it.each([
    { name: 'quick emulator exit', quick: true, crashed: false, toasts: 1 },
    { name: 'plain short session', quick: false, crashed: false, toasts: 0 },
    { name: 'crash', quick: false, crashed: true, toasts: 0 },
  ])('tells about an emulator that closed at once: $name', async ({ quick, crashed, toasts }) => {
    running = ['a'];
    const { store, toast } = await load();
    const { crashTarget } = await import('./crash');

    handlers['game:stopped']({
      data: { gameId: 'a', sessionSeconds: 1, playtimeSeconds: 1, crashed, emulatorQuickExit: quick, emulator: quick ? 'DuckStation' : '' },
    });

    expect(toast).toHaveBeenCalledTimes(toasts);
    expect(get(store.runningGames).has('a')).toBe(false);
    expect(get(crashTarget) !== null).toBe(crashed);
    if (quick) {
      expect(toast.mock.calls[0][0]).toContain('DuckStation');
      expect(toast.mock.calls[0][1]).toBe('info');
    }
  });

  // The sidebar's activity widget counts a session from the moment this launcher saw it start; a game
  // already running when the launcher came up has no such moment.
  it('notes when this launcher saw a game start, none for one already running, and forgets it when it stops', async () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date('2026-11-13T10:00:00Z'));
      running = ['old'];
      const { store } = await load();
      expect([...get(store.sessionStarts)]).toEqual([]);

      handlers['game:started']({ data: { gameId: 'a', sessionSeconds: 0, playtimeSeconds: 0 } });
      vi.setSystemTime(new Date('2026-11-13T10:00:30Z'));
      handlers['game:started']({ data: { gameId: 'a', sessionSeconds: 0, playtimeSeconds: 0 } });

      expect(get(store.sessionStarts).get('a')).toBe(Date.parse('2026-11-13T10:00:00Z'));
      expect(get(store.sessionStarts).has('old')).toBe(false);

      handlers['game:stopped']({ data: { gameId: 'a', sessionSeconds: 30, playtimeSeconds: 30 } });
      expect(get(store.sessionStarts).has('a')).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('catalog titles of downloads', () => {
  async function fresh() {
    vi.resetModules();
    const store = await import('./library');
    const toasts = await import('./toasts');
    return { store, toast: vi.mocked(toasts.toast) };
  }

  beforeEach(() => {
    catalog.answer.mockReset();
  });

  it('asks once per id, however often it is asked, and remembers an id the catalog does not know', async () => {
    catalog.answer.mockResolvedValue([]);
    const { store } = await fresh();

    await Promise.all(Array.from({ length: 12 }, () => store.requestDownloadTitles(['c-unknown'])));
    await store.requestDownloadTitles(['c-unknown']);

    expect(catalog.answer).toHaveBeenCalledTimes(1);
    expect(get(store.downloadTitles)).toEqual({ 'c-unknown': null });
  });

  it('files a game the catalog answers under an alias by the id it was asked for', async () => {
    catalog.answer.mockResolvedValue([{ id: 'c-main', title: 'Main', aliasIds: ['c-alias'], releaseYear: 2020, developer: 'Dev' }]);
    const { store } = await fresh();

    await store.requestDownloadTitles(['c-alias', 'c-gone']);

    expect(get(store.downloadTitles)).toEqual({ 'c-alias': { title: 'Main', releaseYear: 2020, developer: 'Dev' }, 'c-gone': null });
  });

  it('shows a failure once and does not ask again at once', async () => {
    catalog.answer.mockRejectedValue(new Error('typhon:metadata.offline: down'));
    const { store, toast } = await fresh();

    for (let i = 0; i < 5; i += 1) await store.requestDownloadTitles(['c-a']);
    await store.requestDownloadTitles(['c-b']);

    expect(catalog.answer).toHaveBeenCalledTimes(2);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });
});
