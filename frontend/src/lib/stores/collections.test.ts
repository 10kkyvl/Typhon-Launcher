import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { LibraryCollection } from '../services/library';

const handlers: Record<string, (event: { data: unknown }) => void> = {};
const offs: Array<() => void> = [];

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      const off = vi.fn();
      offs.push(off);
      return off;
    }),
  },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));

const service = vi.hoisted(() => ({ getCollections: vi.fn<() => Promise<LibraryCollection[]>>() }));
// The real service over a fake binding: the payload check of its subscription runs as in the app.
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { GetCollections: service.getCollections } }));

const collection = (id: string): LibraryCollection => ({ id, name: id, createdAt: '', updatedAt: '' });

async function fresh() {
  vi.resetModules();
  vi.mocked((await import('./toasts')).toast).mockClear();
  for (const key of Object.keys(handlers)) delete handlers[key];
  offs.length = 0;
  return import('./collections');
}

beforeEach(() => {
  service.getCollections.mockReset();
});

describe('collections store', () => {
  it('loads the collections and says they are ready', async () => {
    service.getCollections.mockResolvedValue([collection('a')]);
    const store = await fresh();
    expect(get(store.collectionsState)).toBe('loading');

    await store.initCollections();

    expect(get(store.collections).map((item) => item.id)).toEqual(['a']);
    expect(get(store.collectionsState)).toBe('ready');
  });

  it('keeps a failed load as state, not as "no collections", and a retry loads', async () => {
    service.getCollections.mockRejectedValue(new Error('typhon:library.collections_unreadable: x'));
    const store = await fresh();

    await expect(store.initCollections()).resolves.toBeTypeOf('function');

    expect(get(store.collectionsState)).toBe('failed');
    expect(await store.loadCollections()).toBe(false);
    expect(get(store.collectionsState)).toBe('failed');

    service.getCollections.mockResolvedValue([collection('b')]);
    expect(await store.loadCollections()).toBe(true);
    expect(get(store.collectionsState)).toBe('ready');
    expect(get(store.collections).map((item) => item.id)).toEqual(['b']);
  });

  it('tells the player once when the first load fails; a failed Retry is left to the notice', async () => {
    const err = new Error('typhon:library.game_not_found: raw');
    service.getCollections.mockRejectedValue(err);
    const store = await fresh();
    const { toast } = await import('./toasts');
    const { libraryErrorText } = await import('../library/errors');
    const { msg } = await import('../i18n');

    await store.initCollections();
    await store.loadCollections();

    expect(vi.mocked(toast).mock.calls).toEqual([[libraryErrorText(err, msg('games.collectionsLoadFailed')), 'danger']]);
    expect(vi.mocked(toast).mock.calls[0][0]).not.toContain('raw');
  });

  it('lets an event that arrives during the load win over the older answer', async () => {
    let answer: (value: LibraryCollection[]) => void = () => {};
    service.getCollections.mockReturnValue(new Promise((resolve) => (answer = resolve)));
    const store = await fresh();
    const started = store.initCollections();

    handlers['library:collections']({ data: [collection('new')] });
    answer([collection('old')]);
    await started;

    expect(get(store.collections).map((item) => item.id)).toEqual(['new']);
    expect(get(store.collectionsState)).toBe('ready');
  });

  it('takes an event after a failed load as ready, and only a list from it', async () => {
    service.getCollections.mockRejectedValue(new Error('x'));
    const store = await fresh();
    await store.initCollections();

    handlers['library:collections']({ data: 'nonsense' });

    expect(get(store.collections)).toEqual([]);
    expect(get(store.collectionsState)).toBe('ready');
  });

  it('answers a disposer that takes its listener away', async () => {
    service.getCollections.mockResolvedValue([]);
    const store = await fresh();

    const dispose = await store.initCollections();
    dispose();

    expect(offs).toHaveLength(1);
    expect(offs[0]).toHaveBeenCalledTimes(1);
  });
});
