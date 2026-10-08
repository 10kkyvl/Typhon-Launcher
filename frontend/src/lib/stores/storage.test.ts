import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get, writable } from 'svelte/store';

const getStorageInfo = vi.fn();
const settings = writable<{ libraryPath: string } | null>({ libraryPath: 'D:\Games' });

vi.mock('../services/system', () => ({ getStorageInfo }));
vi.mock('./settings', () => ({ settings }));

const info = { path: 'D:\Games', volume: 'D:', filesystem: 'NTFS', totalBytes: 100, freeBytes: 40, usedBytes: 60 };

async function load() {
  vi.resetModules();
  return await import('./storage');
}

beforeEach(() => {
  vi.clearAllMocks();
  settings.set({ libraryPath: 'D:\Games' });
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

  it('is not a failure when no library folder is chosen yet', async () => {
    const store = await load();
    settings.set({ libraryPath: '' });

    await store.refreshStorage();

    expect(getStorageInfo).not.toHaveBeenCalled();
    expect(get(store.storageInfo)).toBeNull();
    expect(get(store.storageFailed)).toBe(false);
  });
});
