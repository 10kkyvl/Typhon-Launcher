import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Download } from '../services/downloads';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

const backend = vi.hoisted(() => ({ inWails: true }));
vi.mock('../services/backend', () => backend);
vi.mock('../install/installErrors', () => ({ installErrorText: vi.fn((err: unknown) => String(err)) }));
vi.mock('../i18n', () => ({ msg: vi.fn((key: string) => key) }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/downloads', () => ({ listDownloads: vi.fn() }));

function makeDownload(id: string): Download {
  return { id, name: id, status: 'queued', files: [], origin: {} } as unknown as Download;
}

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

async function fresh() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const services = await import('../services/downloads');
  const store = await import('./downloads');
  return { store, list: vi.mocked(services.listDownloads) };
}

beforeEach(() => {
  vi.clearAllMocks();
  backend.inWails = true;
});

describe('the downloads list’s first read', () => {
  it('counts as loaded only once the first list has arrived', async () => {
    const { store, list } = await fresh();
    const first = deferred<Download[]>();
    list.mockReturnValueOnce(first.promise);

    const init = store.initDownloads();
    expect(get(store.downloadsLoaded)).toBe(false);
    expect(get(store.downloadsLoadError)).toBeNull();

    first.resolve([makeDownload('a')]);
    await init;
    expect(get(store.downloadsLoaded)).toBe(true);
    expect(get(store.downloadsLoadError)).toBeNull();
    expect(get(store.downloads).map((d) => d.id)).toEqual(['a']);
  });

  it('a failed first read is kept as state, not thrown: not loaded, the error kept', async () => {
    const { store, list } = await fresh();
    const failure = new Error('typhon:download.state_unread: bad json');
    list.mockRejectedValueOnce(failure);

    await expect(store.initDownloads()).resolves.toBeUndefined();

    expect(get(store.downloadsLoaded)).toBe(false);
    expect(get(store.downloadsLoadError)).toBe(failure);
  });

  it('a reload that succeeds loads the list, clears the error and answers true', async () => {
    const { store, list } = await fresh();
    list.mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce([makeDownload('a')]);
    await store.initDownloads();

    await expect(store.reloadDownloads()).resolves.toBe(true);

    expect(get(store.downloadsLoaded)).toBe(true);
    expect(get(store.downloadsLoadError)).toBeNull();
    expect(get(store.downloads).map((d) => d.id)).toEqual(['a']);
  });

  it('a reload that fails again keeps the failure, with its new error, and answers false', async () => {
    const { store, list } = await fresh();
    const again = new Error('boom again');
    list.mockRejectedValueOnce(new Error('boom')).mockRejectedValueOnce(again);
    await store.initDownloads();

    await expect(store.reloadDownloads()).resolves.toBe(false);

    expect(get(store.downloadsLoaded)).toBe(false);
    expect(get(store.downloadsLoadError)).toBe(again);
  });

  it('keeps what events brought before a failed first read, and what they bring after it', async () => {
    const { store, list } = await fresh();
    const first = deferred<Download[]>();
    list.mockReturnValueOnce(first.promise);

    const init = store.initDownloads();
    await Promise.resolve();
    handlers['download:added']({ data: makeDownload('early') });
    first.reject(new Error('boom'));
    await init;
    handlers['download:added']({ data: makeDownload('late') });

    expect(get(store.downloads).map((d) => d.id)).toEqual(['early', 'late']);
    expect(get(store.downloadsLoaded)).toBe(false);
  });

  it('a reload answered after an event asks again, so the list is at least as new as the event', async () => {
    const { store, list } = await fresh();
    const reread = deferred<Download[]>();
    list.mockRejectedValueOnce(new Error('boom')).mockReturnValueOnce(reread.promise).mockResolvedValueOnce([makeDownload('a'), makeDownload('b')]);
    await store.initDownloads();

    const reload = store.reloadDownloads();
    handlers['download:added']({ data: makeDownload('b') });
    reread.resolve([makeDownload('a')]);

    await expect(reload).resolves.toBe(true);
    expect(list).toHaveBeenCalledTimes(3);
    expect(get(store.downloads).map((d) => d.id)).toEqual(['a', 'b']);
  });

  it('outside Wails the empty list counts as read', async () => {
    backend.inWails = false;
    const { store, list } = await fresh();
    list.mockResolvedValueOnce([]);

    await store.initDownloads();

    expect(get(store.downloadsLoaded)).toBe(true);
  });
});
