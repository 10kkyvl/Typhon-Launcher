import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Source } from '../services/sources';

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

let seeded: Source[] = [];

const api = {
  listSources: vi.fn(async () => seeded),
  addSource: vi.fn(),
  addSourceFile: vi.fn(),
  refreshAllSources: vi.fn(),
  refreshSource: vi.fn(),
  removeSource: vi.fn(),
  setSourceEnabled: vi.fn(),
};

vi.mock('../services/sources', () => api);

function source(id: string, name = id): Source {
  return { id, name } as Source;
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./sources');
  const toasts = await import('./toasts');
  await store.initSources();
  return { store, toast: vi.mocked(toasts.toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  seeded = [];
  api.listSources.mockImplementation(async () => seeded);
});

describe('source events', () => {
  it('adds a source it has not seen and replaces one it has', async () => {
    seeded = [source('a', 'Alpha')];
    const { store } = await load();

    handlers['source:updated']({ data: source('b', 'Beta') });
    handlers['source:updated']({ data: source('a', 'Alpha 2') });

    expect(get(store.sources).map((item) => item.name)).toEqual(['Alpha 2', 'Beta']);
  });

  it('removes a source that arrives without a name', async () => {
    seeded = [source('a'), source('b')];
    const { store } = await load();

    handlers['source:updated']({ data: { id: 'a', name: '' } });

    expect(get(store.sources).map((item) => item.id)).toEqual(['b']);
  });

  it('shows an error the player triggered', async () => {
    const { toast } = await load();

    handlers['source:error']({ data: { sourceId: 'a', name: 'Alpha', message: 'timeout', scheduled: false } });

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
    expect(String(toast.mock.calls[0][0])).toContain('Alpha');
  });

  it('keeps quiet about a failure of a scheduled refresh the player never asked for', async () => {
    const { toast } = await load();

    handlers['source:error']({ data: { sourceId: 'a', name: 'Alpha', message: 'timeout', scheduled: true } });

    expect(toast).not.toHaveBeenCalled();
  });

  it('announces new entries only when there are some', async () => {
    const { toast } = await load();

    handlers['release:added']({ data: { sourceId: 'a', count: 0 } });
    expect(toast).not.toHaveBeenCalled();

    handlers['release:added']({ data: { sourceId: 'a', count: 7 } });
    expect(toast).toHaveBeenCalledTimes(1);
    expect(String(toast.mock.calls[0][0])).toContain('7');
  });
});

describe('source actions', () => {
  it('reloads the list and reports the result of a refresh', async () => {
    seeded = [source('a')];
    api.refreshSource.mockResolvedValueOnce({ entries: 120, added: 4, review: 2 });
    const { store, toast } = await load();
    seeded = [source('a'), source('b')];

    await store.refresh('a');

    expect(get(store.sources).map((item) => item.id)).toEqual(['a', 'b']);
    expect(toast.mock.calls[0][1]).toBe('success');
    const text = String(toast.mock.calls[0][0]);
    expect(text).toContain('120');
    expect(text).toContain('4');
  });

  it('tells the player why a refresh failed instead of showing a success', async () => {
    api.refreshSource.mockRejectedValueOnce(new Error('typhon:sources.source_busy: refresh already running'));
    const { store, toast } = await load();

    await store.refresh('a');

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
    expect(String(toast.mock.calls[0][0])).not.toContain('typhon:');
  });

  it('turns the refreshing flag off even when refresh-all fails', async () => {
    api.refreshAllSources.mockRejectedValueOnce(new Error('offline'));
    const { store, toast } = await load();

    const pending = store.refreshAll();
    expect(get(store.refreshingAll)).toBe(true);
    await pending;

    expect(get(store.refreshingAll)).toBe(false);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });

  it('removes a source from the list only after the backend removed it', async () => {
    seeded = [source('a'), source('b')];
    api.removeSource.mockRejectedValueOnce(new Error('typhon:sources.source_busy: busy'));
    const { store, toast } = await load();

    await store.remove('a');
    expect(get(store.sources).map((item) => item.id)).toEqual(['a', 'b']);
    expect(toast).toHaveBeenCalledTimes(1);

    api.removeSource.mockResolvedValueOnce(undefined);
    await store.remove('a');
    expect(get(store.sources).map((item) => item.id)).toEqual(['b']);
  });

  it('reloads after toggling a source and reports a failed toggle', async () => {
    seeded = [source('a')];
    const { store, toast } = await load();
    api.setSourceEnabled.mockRejectedValueOnce(new Error('typhon:sources.source_not_found: gone'));

    await store.toggle('a', false);

    expect(api.setSourceEnabled).toHaveBeenCalledWith('a', false);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });

  it('lets an add failure reach the dialog that asked for it', async () => {
    api.addSource.mockRejectedValueOnce(new Error('typhon:sources.feed_bad_scheme: ftp'));
    const { store } = await load();

    await expect(store.add('ftp://example.invalid/feed.json')).rejects.toThrow('sources.feed_bad_scheme');
  });

  it('adds a source from a file and reloads the list', async () => {
    api.addSourceFile.mockResolvedValueOnce(source('f'));
    const { store } = await load();
    seeded = [source('f')];

    const added = await store.addFile('C:\\feeds\\mine.json');

    expect(added.id).toBe('f');
    expect(get(store.sources).map((item) => item.id)).toEqual(['f']);
  });
});
