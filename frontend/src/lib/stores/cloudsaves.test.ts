import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { CloudEvent, CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';

vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));

let load: () => Promise<CloudStatus> = async () => status();
let emit: (event: CloudEvent) => void = () => {};

vi.mock('../services/cloudsaves', () => ({
  loadCloudStatus: vi.fn(() => load()),
  onCloudEvent: vi.fn((handler: (event: CloudEvent) => void) => {
    emit = handler;
    return vi.fn();
  }),
}));

vi.mock('../services/library', () => ({
  getGames: vi.fn(async () => []),
  getRunningGames: vi.fn(async () => []),
}));

function status(patch: Partial<CloudStatus> = {}): CloudStatus {
  return {
    enabled: true,
    signedIn: true,
    usedBytes: 100,
    quotaBytes: 1000,
    maxBytes: 500,
    error: '',
    games: [{ gameId: 'g1', state: 'synced', code: '', message: '', at: '', syncedAt: '' }],
    ...patch,
  };
}

function event(patch: Partial<CloudEvent> = {}): CloudEvent {
  return { gameId: 'g1', status: 'synced', code: '', message: '', usedBytes: 100, quotaBytes: 1000, ...patch };
}

async function init() {
  vi.resetModules();
  const store = await import('./cloudsaves');
  const library = await import('./library');
  const toasts = await import('./toasts');
  library.libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false } as LibraryGame]);
  store.initCloudSaves();
  await Promise.resolve();
  await Promise.resolve();
  return { store, library, toast: vi.mocked(toasts.toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  load = async () => status();
});

describe('cloud saves store', () => {
  it('loads the status at start', async () => {
    const { store } = await init();
    expect(get(store.cloudStatus)?.usedBytes).toBe(100);
  });

  it('reports a failed first load instead of showing an empty status', async () => {
    load = async () => {
      throw new Error('typhon:cloud.network: нет связи');
    };
    const { store, toast } = await init();
    expect(get(store.cloudStatus)).toBeNull();
    expect(toast).toHaveBeenCalledWith('Нет связи с сервером Typhon', 'danger');
  });

  it('updates the game state and usage from an event', async () => {
    const { store } = await init();
    emit(event({ status: 'failed', code: 'cloud.quota', usedBytes: 900 }));
    const current = get(store.cloudStatus);
    expect(current?.usedBytes).toBe(900);
    expect(current?.games.find((g) => g.gameId === 'g1')).toMatchObject({ state: 'failed', code: 'cloud.quota' });
  });

  it('opens the conflict dialog when a game enters conflict', async () => {
    const { store } = await init();
    expect(get(store.cloudConflictTarget)).toBeNull();
    emit(event({ status: 'conflict', code: 'cloud.conflict' }));
    expect(get(store.cloudConflictTarget)).toEqual({ gameId: 'g1', title: 'Hades' });
  });

  it('does not reopen the dialog for a game that is already in conflict', async () => {
    load = async () => status({ games: [{ gameId: 'g1', state: 'conflict', code: '', message: '', at: '', syncedAt: '' }] });
    const { store } = await init();
    emit(event({ status: 'conflict' }));
    expect(get(store.cloudConflictTarget)).toBeNull();
  });

  it('keeps the usage when an event carries no quota', async () => {
    const { store } = await init();
    emit(event({ status: 'failed', usedBytes: 0, quotaBytes: 0 }));
    expect(get(store.cloudStatus)?.usedBytes).toBe(100);
  });

  it('marks the account as pending on a global event without touching any game', async () => {
    const { store } = await init();
    emit(event({ gameId: '', status: 'pending', code: 'cloud.account_unknown', quotaBytes: 0, usedBytes: 0 }));
    const current = get(store.cloudStatus);
    expect(current?.pending).toBe(true);
    expect(current?.games).toHaveLength(1);
    expect(current?.games[0].gameId).toBe('g1');
  });

  it('keeps a failed pass as the global error, as the backend status does', async () => {
    const { store } = await init();
    emit(event({ gameId: '', status: 'failed', code: 'cloud.server', quotaBytes: 0, usedBytes: 0 }));
    const current = get(store.cloudStatus);
    expect(current?.error).toBe('cloud.server');
    expect(current?.games).toEqual(status().games);
  });

  it('loads the status for a caller that reports a failure itself, without a toast', async () => {
    const { store, toast } = await init();
    load = async () => {
      throw new Error('typhon:cloud.network: нет связи');
    };
    await expect(store.fetchCloudStatus()).rejects.toThrow('cloud.network');
    expect(toast).not.toHaveBeenCalled();
    load = async () => status({ usedBytes: 300 });
    await store.fetchCloudStatus();
    expect(get(store.cloudStatus)?.usedBytes).toBe(300);
  });

  it('clears the global error once a game syncs again, and only then', async () => {
    load = async () => status({ error: 'cloud.network' });
    const { store } = await init();
    emit(event({ gameId: '', status: 'pending', code: 'cloud.account_unknown' }));
    emit(event({ gameId: 'g1', status: 'failed', code: 'cloud.quota' }));
    expect(get(store.cloudStatus)?.error).toBe('cloud.network');
    emit(event({ gameId: 'g1', status: 'synced' }));
    expect(get(store.cloudStatus)?.error).toBe('');
  });
});

// What a launch waiting on the dialog has been told so far: still waiting, or its answer.
async function answer(question: Promise<boolean> | null): Promise<boolean | 'waiting'> {
  expect(question).not.toBeNull();
  return await Promise.race([question as Promise<boolean>, new Promise<'waiting'>((resolve) => setTimeout(() => resolve('waiting'), 0))]);
}

describe('a launch waiting on the conflict dialog', () => {
  it('opens the dialog for the game and is answered "resolved" once the choice is applied', async () => {
    const { store } = await init();
    const question = store.askToResolveConflict('g1');
    const target = get(store.cloudConflictTarget);
    expect(target).toEqual({ gameId: 'g1', title: 'Hades' });
    expect(await answer(question)).toBe('waiting');

    store.finishCloudConflict(target!);

    expect(await answer(question)).toBe(true);
    expect(get(store.cloudConflictTarget)).toBeNull();
  });

  it('is answered "given up" when the dialog is closed without a choice', async () => {
    const { store } = await init();
    const question = store.askToResolveConflict('g1');
    store.closeCloudConflict();
    expect(await answer(question)).toBe(false);
    expect(get(store.cloudConflictTarget)).toBeNull();
  });

  it('is answered once: a late close or a second choice changes nothing', async () => {
    const { store } = await init();
    const question = store.askToResolveConflict('g1');
    const target = get(store.cloudConflictTarget)!;
    store.finishCloudConflict(target);
    store.closeCloudConflict();
    store.finishCloudConflict(target);
    expect(await answer(question)).toBe(true);
  });

  it('joins the dialog an earlier event opened for the same game: one dialog, and the launch goes on after the choice', async () => {
    const { store } = await init();
    emit(event({ status: 'conflict' }));
    const opened = get(store.cloudConflictTarget);
    expect(opened).toEqual({ gameId: 'g1', title: 'Hades' });

    const question = store.askToResolveConflict('g1');

    expect(get(store.cloudConflictTarget)).toBe(opened);
    store.finishCloudConflict(opened!);
    expect(await answer(question)).toBe(true);
  });

  it('keeps its dialog when the event for the same game arrives after the launch error', async () => {
    const { store } = await init();
    const question = store.askToResolveConflict('g1');
    const asked = get(store.cloudConflictTarget);

    emit(event({ status: 'conflict' }));

    expect(await answer(question)).toBe('waiting');
    expect(get(store.cloudConflictTarget)).toBe(asked);
    store.finishCloudConflict(asked!);
    expect(await answer(question)).toBe(true);
  });

  it('is given up when an event replaces the dialog with another game, and a late choice for it changes nothing', async () => {
    const { store, library } = await init();
    library.libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const question = store.askToResolveConflict('g1');
    const first = get(store.cloudConflictTarget)!;

    emit(event({ gameId: 'g2', status: 'conflict' }));

    expect(await answer(question)).toBe(false);
    expect(get(store.cloudConflictTarget)).toEqual({ gameId: 'g2', title: 'Celeste' });
    store.finishCloudConflict(first);
    expect(get(store.cloudConflictTarget)).toEqual({ gameId: 'g2', title: 'Celeste' });
  });

  it('is given up when a newer launch asks about another game', async () => {
    const { store, library } = await init();
    library.libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const older = store.askToResolveConflict('g1');
    const newer = store.askToResolveConflict('g2');

    expect(await answer(older)).toBe(false);
    expect(await answer(newer)).toBe('waiting');
    store.finishCloudConflict(get(store.cloudConflictTarget)!);
    expect(await answer(newer)).toBe(true);
  });

  it('gives up the older launch when a newer one asks about the same game; the choice answers the newer one', async () => {
    const { store } = await init();
    const older = store.askToResolveConflict('g1');
    const opened = get(store.cloudConflictTarget);
    const newer = store.askToResolveConflict('g1');

    expect(await answer(older)).toBe(false);
    expect(await answer(newer)).toBe('waiting');
    expect(get(store.cloudConflictTarget)).toBe(opened);

    store.finishCloudConflict(opened!);
    expect(await answer(newer)).toBe(true);
    expect(await answer(older)).toBe(false);
  });

  it('opens nothing for a game the library does not hold, and says so to the launch', async () => {
    const { store } = await init();
    expect(store.askToResolveConflict('missing')).toBeNull();
    expect(get(store.cloudConflictTarget)).toBeNull();
  });
});
