import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get, writable } from 'svelte/store';
import type { MoveJob } from '../services/relocate';

const getStorageInfo = vi.fn();
const settings = writable<{ libraryPath: string; gamesPath?: string } | null>({ libraryPath: 'D:\Games' });
type Game = { id: string; installDir: string; sizeBytes: number; sizeUnknown?: boolean; title?: string };
const installedGames = writable<Game[]>([]);

vi.mock('../services/system', () => ({ getStorageInfo }));
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/relocate', () => ({ listMoves: vi.fn(async () => []) }));
vi.mock('./settings', () => ({ settings }));
vi.mock('./library', () => ({ installedGames }));

const info = { path: 'D:\Games', volume: 'D:', filesystem: 'NTFS', totalBytes: 100, freeBytes: 40, usedBytes: 60 };

async function load() {
  vi.resetModules();
  return await import('./storage');
}

// The move list the storage store reads, from the same module instance `load` gave it.
async function moveList() {
  return (await import('./relocate')).moves;
}

const move = (id: string, gameId: string, stage: MoveJob['stage'] = 'copy') => ({ id, scope: 'game', stage, gameId }) as MoveJob;

beforeEach(() => {
  vi.clearAllMocks();
  settings.set({ libraryPath: 'D:\Games' });
  installedGames.set([]);
});

describe('library storage', () => {
  it('holds what the backend reported', async () => {
    getStorageInfo.mockResolvedValueOnce(info);
    const store = await load();

    await store.refreshStorage();

    expect(get(store.storageInfo)).toEqual(info);
    expect(get(store.storageFailed)).toBe(false);
  });

  it('marks the failure instead of leaving an empty state that looks like nothing happened', async () => {
    getStorageInfo.mockRejectedValueOnce(new Error('disk offline'));
    const store = await load();

    await store.refreshStorage();

    expect(get(store.storageInfo)).toBeNull();
    expect(get(store.storageFailed)).toBe(true);
  });

  it('drops a stale reading when the next one fails', async () => {
    getStorageInfo.mockResolvedValueOnce(info).mockRejectedValueOnce(new Error('disk offline'));
    const store = await load();
    await store.refreshStorage();

    await store.refreshStorage();

    expect(get(store.storageInfo)).toBeNull();
    expect(get(store.storageFailed)).toBe(true);
  });

  it('clears the failure once a refresh succeeds', async () => {
    getStorageInfo.mockRejectedValueOnce(new Error('disk offline')).mockResolvedValueOnce(info);
    const store = await load();
    await store.refreshStorage();

    await store.refreshStorage();

    expect(get(store.storageInfo)).toEqual(info);
    expect(get(store.storageFailed)).toBe(false);
  });

  it('ignores an older reading that fails after a newer one succeeded', async () => {
    let failOld: (err: Error) => void = () => {};
    getStorageInfo
      .mockReturnValueOnce(new Promise((_, reject) => { failOld = reject; }))
      .mockResolvedValueOnce(info);
    const store = await load();

    const old = store.refreshStorage();
    await store.refreshStorage();
    failOld(new Error('disk offline'));
    await old;

    expect(get(store.storageInfo)).toEqual(info);
    expect(get(store.storageFailed)).toBe(false);
  });

  it('is not a failure when no library folder is chosen yet', async () => {
    const store = await load();
    settings.set({ libraryPath: '' });

    await store.refreshStorage();

    expect(getStorageInfo).not.toHaveBeenCalled();
    expect(get(store.storageInfo)).toBeNull();
    expect(get(store.storageFailed)).toBe(false);
  });
});

describe('following the library storage', () => {
  const game: Game = { id: 'g1', installDir: 'D:\\Games\\One', sizeBytes: 10 };

  it('reads now, and again on each change that moves the numbers, until it is disposed', async () => {
    getStorageInfo.mockResolvedValue(info);
    installedGames.set([game]);
    const store = await load();
    const moves = await moveList();

    const stop = store.followStorage();
    expect(getStorageInfo).toHaveBeenCalledTimes(1);

    installedGames.set([{ ...game, title: 'renamed' }]);
    settings.set({ libraryPath: get(settings)?.libraryPath ?? '', gamesPath: 'E:\\Other' });
    expect(getStorageInfo).toHaveBeenCalledTimes(1);

    installedGames.set([{ ...game, sizeUnknown: true }]);
    expect(getStorageInfo).toHaveBeenCalledTimes(2);
    installedGames.set([]);
    expect(getStorageInfo).toHaveBeenCalledTimes(3);
    settings.set({ libraryPath: 'F:\\Library' });
    expect(getStorageInfo).toHaveBeenCalledTimes(4);
    moves.set([move('m1', 'g1')]);
    expect(getStorageInfo).toHaveBeenCalledTimes(4);
    moves.set([move('m1', 'g1', 'done')]);
    expect(getStorageInfo).toHaveBeenCalledTimes(5);

    stop();
    installedGames.set([game]);
    moves.set([move('m2', 'g1')]);
    moves.set([move('m2', 'g1', 'done')]);
    expect(getStorageInfo).toHaveBeenCalledTimes(5);
    await vi.waitFor(() => expect(get(store.storageInfo)).toEqual(info));
  });

  it('reads again when any of two moves ends, not only the first listed, and not on their progress', async () => {
    getStorageInfo.mockResolvedValue(info);
    const store = await load();
    const moves = await moveList();
    moves.set([move('a', 'g1'), move('b', 'g2')]);
    const stop = store.followStorage();
    expect(getStorageInfo).toHaveBeenCalledTimes(1);

    moves.set([move('a', 'g1', 'verify'), move('b', 'g2', 'verify')]);
    expect(getStorageInfo).toHaveBeenCalledTimes(1);
    moves.set([move('a', 'g1'), move('b', 'g2', 'done')]);
    expect(getStorageInfo).toHaveBeenCalledTimes(2);
    moves.set([move('a', 'g1', 'cancelled'), move('b', 'g2', 'done')]);
    expect(getStorageInfo).toHaveBeenCalledTimes(3);

    stop();
    await vi.waitFor(() => expect(get(store.storageInfo)).toEqual(info));
  });
});
