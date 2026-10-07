import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { MoveJob, MoveStage } from '../services/relocate';
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

let seeded: MoveJob[] = [];

vi.mock('../services/relocate', () => ({
  listMoves: vi.fn(async () => seeded),
}));

vi.mock('../services/library', () => ({
  getGames: vi.fn(async () => []),
  getRunningGames: vi.fn(async () => []),
}));

function job(patch: Partial<MoveJob> = {}): MoveJob {
  return {
    id: 'm1',
    scope: 'game',
    stage: 'copy',
    gameId: 'g1',
    title: 'Hades',
    source: 'C:\\Games\\Hades',
    target: 'D:\\Games\\Hades',
    staging: '',
    renamed: false,
    totalBytes: 100,
    copiedBytes: 10,
    phase: '',
    currentFile: '',
    startedAt: '2026-09-01T10:00:00Z',
    updatedAt: '2026-09-01T10:00:00Z',
    ...patch,
  };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./relocate');
  const library = await import('./library');
  const toasts = await import('./toasts');
  await store.initMoves();
  return { store, library, toast: vi.mocked(toasts.toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  seeded = [];
});

describe('move jobs', () => {
  it('reports no active move when nothing is running', async () => {
    const { store } = await load();

    expect(get(store.activeMove)).toBeNull();
  });

  it.each<MoveStage>(['prepare', 'copy', 'verify', 'commit', 'repoint', 'cleanup'])(
    'treats a job in the %s stage as active',
    async (stage) => {
      seeded = [job({ stage })];
      const { store } = await load();

      expect(get(store.activeMove)?.id).toBe('m1');
    },
  );

  it.each<MoveStage>(['done', 'failed', 'cancelled'])('lets go of a job that is %s', async (stage) => {
    seeded = [job({ stage })];
    const { store } = await load();

    expect(get(store.activeMove)).toBeNull();
  });

  it('picks the running job over finished ones listed before it', async () => {
    seeded = [job({ id: 'old', stage: 'done' }), job({ id: 'now', stage: 'verify' })];
    const { store } = await load();

    expect(get(store.activeMove)?.id).toBe('now');
  });

  it('follows progress events for the same job', async () => {
    seeded = [job({ copiedBytes: 10 })];
    const { store } = await load();

    handlers['move:progress']({ data: job({ copiedBytes: 60 }) });

    expect(get(store.moves)).toHaveLength(1);
    expect(get(store.moves)[0].copiedBytes).toBe(60);
  });

  it('adds a job that has just started', async () => {
    const { store } = await load();

    handlers['move:started']({ data: job({ id: 'fresh', stage: 'prepare' }) });

    expect(get(store.activeMove)?.id).toBe('fresh');
  });

  it('keeps a cancelled job in the list and stays silent', async () => {
    seeded = [job()];
    const { store, toast } = await load();

    handlers['move:cancelled']({ data: job({ stage: 'cancelled' }) });

    expect(get(store.moves)[0].stage).toBe('cancelled');
    expect(get(store.activeMove)).toBeNull();
    expect(toast).not.toHaveBeenCalled();
  });
});

describe('move notifications', () => {
  it('names the game when a move to another drive finishes', async () => {
    const { toast } = await load();

    handlers['move:completed']({ data: job({ stage: 'done', title: 'Hades' }) });

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('success');
    expect(String(toast.mock.calls[0][0])).toContain('Hades');
  });

  it('says it was the library when the job has no title', async () => {
    const { toast } = await load();

    handlers['move:completed']({ data: job({ stage: 'done', title: '', scope: 'library' }) });

    expect(toast).toHaveBeenCalledTimes(1);
    expect(String(toast.mock.calls[0][0])).not.toContain('Hades');
  });

  it('explains a coded failure in words', async () => {
    const { toast } = await load();

    handlers['move:failed']({
      data: job({ stage: 'failed', error: 'typhon:relocate.not_enough_space: need 5 GB' }),
    });

    expect(toast.mock.calls[0][1]).toBe('danger');
    const text = String(toast.mock.calls[0][0]);
    expect(text).not.toContain('typhon:');
    expect(text).not.toContain('not_enough_space');
  });

  it('falls back to a message that names the game when the failure has no code', async () => {
    const { toast } = await load();

    handlers['move:failed']({ data: job({ stage: 'failed', error: 'boom', title: 'Hades' }) });

    const text = String(toast.mock.calls[0][0]);
    expect(text).toContain('Hades');
    expect(text).not.toContain('boom');
  });
});

describe('move dialog target', () => {
  function game(id: string, title: string): LibraryGame {
    return { id, title, uninstalled: false } as LibraryGame;
  }

  it('opens for a game the library knows and carries its title', async () => {
    const { store, library } = await load();
    library.libraryGames.set([game('g1', 'Hades')]);

    store.openMoveGame('g1');

    expect(get(store.moveTarget)).toEqual({ gameId: 'g1', title: 'Hades' });
  });

  it('stays closed for a game the library does not know', async () => {
    const { store, library } = await load();
    library.libraryGames.set([game('g1', 'Hades')]);

    store.openMoveGame('missing');

    expect(get(store.moveTarget)).toBeNull();
  });

  it('closes again', async () => {
    const { store, library } = await load();
    library.libraryGames.set([game('g1', 'Hades')]);
    store.openMoveGame('g1');

    store.closeMoveGame();

    expect(get(store.moveTarget)).toBeNull();
  });
});
