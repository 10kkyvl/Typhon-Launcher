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

vi.mock('../services/library', () => ({
  getGames: vi.fn(async () => games),
  getRunningGames: vi.fn(async () => running),
}));

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
});
