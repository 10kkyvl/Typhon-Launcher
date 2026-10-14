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
  return { store, toast: vi.mocked(toasts.toast) };
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
});
