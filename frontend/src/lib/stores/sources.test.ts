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
  // An answer one test queued and never used must not reach the next.
  for (const fn of Object.values(api)) fn.mockReset();
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

// R1: a first read that failed is kept as state, so the screen can say so instead of "no sources yet".
describe('the first read of the list', () => {
  it('R1: says the list is read once it arrived, empty included', async () => {
    const { store } = await load();

    expect(get(store.sourcesLoaded)).toBe(true);
    expect(get(store.sourcesLoadError)).toBeNull();
  });

  it('R1: keeps a failed first read as the reason, without rejecting, and with the events still heard', async () => {
    api.listSources.mockRejectedValueOnce(new Error('service not ready'));
    const { store } = await load();

    expect(get(store.sourcesLoaded)).toBe(false);
    expect(String(get(store.sourcesLoadError))).toContain('service not ready');
    expect(Object.keys(handlers)).toContain('source:updated');
  });

  it('R1: an event before any list arrived is kept, and the list is still not read', async () => {
    api.listSources.mockRejectedValueOnce(new Error('service not ready'));
    const { store } = await load();

    handlers['source:updated']({ data: source('a', 'Alpha') });

    expect(get(store.sources).map((item) => item.id)).toEqual(['a']);
    expect(get(store.sourcesLoaded)).toBe(false);
  });

  it('R1: a reload after the failure reads the whole list, which replaces what events left', async () => {
    api.listSources.mockRejectedValueOnce(new Error('service not ready'));
    const { store } = await load();
    handlers['source:updated']({ data: source('a', 'Alpha') });
    seeded = [source('b'), source('c')];

    expect(await store.reloadSources()).toBe(true);

    expect(get(store.sources).map((item) => item.id)).toEqual(['b', 'c']);
    expect(get(store.sourcesLoaded)).toBe(true);
    expect(get(store.sourcesLoadError)).toBeNull();
  });

  it('R1: a reload that fails again answers false and keeps the newest reason', async () => {
    api.listSources.mockRejectedValueOnce(new Error('service not ready'));
    const { store } = await load();
    api.listSources.mockRejectedValueOnce(new Error('still not ready'));

    expect(await store.reloadSources()).toBe(false);

    expect(get(store.sourcesLoaded)).toBe(false);
    expect(String(get(store.sourcesLoadError))).toContain('still not ready');
  });

  it('R1: an event that arrives while the reload reads gets the list read once more', async () => {
    api.listSources.mockRejectedValueOnce(new Error('service not ready'));
    const { store } = await load();
    api.listSources.mockImplementationOnce(async () => {
      handlers['source:updated']({ data: source('b') });
      return [source('a')];
    });
    seeded = [source('a'), source('b')];

    expect(await store.reloadSources()).toBe(true);

    expect(api.listSources).toHaveBeenCalledTimes(3);
    expect(get(store.sources).map((item) => item.id)).toEqual(['a', 'b']);
  });

  it('R1: a list an action read after the failure counts as read', async () => {
    api.listSources.mockRejectedValueOnce(new Error('service not ready'));
    api.refreshSource.mockResolvedValueOnce({ entries: 1, added: 0, review: 0 });
    const { store } = await load();
    seeded = [source('a')];

    await store.refresh('a');

    expect(get(store.sourcesLoaded)).toBe(true);
    expect(get(store.sourcesLoadError)).toBeNull();
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

  it('removes a source from the list only after the backend removed it, and hands a refusal to the caller', async () => {
    seeded = [source('a'), source('b')];
    api.removeSource.mockRejectedValueOnce(new Error('typhon:sources.source_busy: busy'));
    const { store, toast } = await load();

    await expect(store.remove('a')).rejects.toThrow('typhon:sources.source_busy');
    expect(get(store.sources).map((item) => item.id)).toEqual(['a', 'b']);
    expect(toast).not.toHaveBeenCalled();

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

    expect(added.source.id).toBe('f');
    expect(get(store.sources).map((item) => item.id)).toEqual(['f']);
  });
});

// Go's AddSource saves the source and then returns its first refresh's error: the store reads the list
// again to tell a saved source from a refusal that saved nothing.
describe('an add whose first refresh failed', () => {
  const FEED = 'https://feed.example/feed.json';
  const saved = (patch: Partial<Source> = {}) => ({ id: 'n', name: 'feed.example', type: 'url', url: FEED, status: 'error', ...patch }) as Source;
  const refreshFailed = new Error('typhon:sources.feed_bad_status: GET https://feed.example/feed.json: status 503');

  it('answers the saved source with the failure, from the list read again, and sends the add once', async () => {
    const { store } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [saved()];
      throw refreshFailed;
    });

    const result = await store.add(`  ${FEED}  `);

    expect(api.addSource.mock.calls).toEqual([[`  ${FEED}  `]]);
    expect(result.source.id).toBe('n');
    expect(result.failure).toBe(refreshFailed);
    expect(get(store.sources).map((item) => item.id)).toEqual(['n']);
  });

  it('finds a file source by its path, and a URL whatever the case of its scheme and host', async () => {
    const { store } = await load();
    api.addSourceFile.mockImplementationOnce(async () => {
      seeded = [saved({ id: 'f', type: 'file', url: '', path: 'C:\\Feeds\\mine.json' })];
      throw refreshFailed;
    });
    api.addSource.mockImplementationOnce(async () => {
      seeded = [...seeded, saved({ id: 'u', url: 'https://feed.example/Feed.json' })];
      throw refreshFailed;
    });

    expect((await store.addFile('C:\\Feeds\\mine.json')).source.id).toBe('f');
    expect((await store.add('HTTPS://FEED.EXAMPLE/Feed.json')).source.id).toBe('u');
  });

  it('rejects a refusal that saved nothing, after reading the list again', async () => {
    const { store } = await load();
    const reads = api.listSources.mock.calls.length;
    api.addSource.mockRejectedValueOnce(new Error('typhon:sources.feed_invalid_url: bad'));

    await expect(store.add('nonsense')).rejects.toThrow('sources.feed_invalid_url');
    expect(api.listSources.mock.calls.length).toBe(reads + 1);
    expect(api.addSource).toHaveBeenCalledTimes(1);
  });

  it('rejects "already added": the source in the list at that place is not this add’s', async () => {
    seeded = [saved({ id: 'old', status: 'active' })];
    const { store } = await load();
    api.addSource.mockRejectedValueOnce(new Error('typhon:sources.source_exists: GO'));

    await expect(store.add(FEED)).rejects.toThrow('sources.source_exists');
  });

  it('rejects "already added" also when the list was never read before the add', async () => {
    api.listSources.mockRejectedValueOnce(new Error('list failed'));
    const { store } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [saved({ id: 'old', status: 'active' })];
      throw new Error('typhon:sources.source_exists: GO');
    });

    await expect(store.add(FEED)).rejects.toThrow('sources.source_exists');
  });

  it('rejects with the add’s own reason when the list cannot be read again', async () => {
    const { store } = await load();
    api.addSource.mockRejectedValueOnce(refreshFailed);
    api.listSources.mockRejectedValueOnce(new Error('list failed'));

    await expect(store.add(FEED)).rejects.toBe(refreshFailed);
  });

  it('leaves the first refresh’s error to the add, so the player is told once', async () => {
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      handlers['source:updated']({ data: saved({ status: 'updating' }) });
      handlers['source:error']({ data: { sourceId: 'n', name: 'feed.example', message: refreshFailed.message, scheduled: false } });
      seeded = [saved()];
      throw refreshFailed;
    });

    await store.add(FEED);
    expect(toast).not.toHaveBeenCalled();

    // A later refresh, as Go sends it: the source updating, then failed.
    handlers['source:updated']({ data: saved({ status: 'updating' }) });
    handlers['source:updated']({ data: saved() });
    handlers['source:error']({ data: { sourceId: 'n', name: 'feed.example', message: refreshFailed.message, scheduled: false } });
    expect(toast).toHaveBeenCalledTimes(1);
  });
});

// Go answered with the source: a list that cannot be read after it is the list's trouble, not the add's.
describe('an add whose list read after it fails', () => {
  const FEED = 'https://feed.example/feed.json';
  const saved = (patch: Partial<Source> = {}) => ({ id: 'n', name: 'feed.example', type: 'url', url: FEED, status: 'active', ...patch }) as Source;
  const refreshFailed = new Error('typhon:sources.feed_bad_status: GET https://feed.example/feed.json: status 503');

  it('answers the source Go added, listed from its events', async () => {
    const { store } = await load();
    api.addSource.mockImplementationOnce(async () => {
      handlers['source:updated']({ data: saved() });
      return saved();
    });
    api.listSources.mockRejectedValueOnce(new Error('list failed'));

    const result = await store.add(FEED);

    expect(result).toEqual({ source: saved() });
    expect(get(store.sources).map((s) => s.id)).toEqual(['n']);
  });

  it('answers the source Go added and lists it from the answer when no event came', async () => {
    seeded = [saved({ id: 'o', url: 'https://other.example/feed.json' })];
    const { store } = await load();
    api.addSource.mockResolvedValueOnce(saved());
    api.listSources.mockRejectedValueOnce(new Error('list failed'));

    expect((await store.add(FEED)).source.id).toBe('n');
    expect(get(store.sources).map((s) => s.id)).toEqual(['o', 'n']);
  });

  it('keeps what a newer event said of the source over the answer', async () => {
    const { store } = await load();
    api.addSource.mockImplementationOnce(async () => {
      handlers['source:updated']({ data: saved({ status: 'updating' }) });
      return saved();
    });
    api.listSources.mockRejectedValueOnce(new Error('list failed'));

    await store.add(FEED);

    expect(get(store.sources).map((s) => s.status)).toEqual(['updating']);
  });

  it('keeps a failed first read as the list’s reason, not the add’s', async () => {
    api.listSources.mockRejectedValueOnce(new Error('service not ready'));
    const { store } = await load();
    api.addSource.mockResolvedValueOnce(saved());
    api.listSources.mockRejectedValueOnce(new Error('still not ready'));

    expect((await store.add(FEED)).source.id).toBe('n');
    expect(get(store.sourcesLoaded)).toBe(false);
    expect(String(get(store.sourcesLoadError))).toContain('still not ready');
  });

  it('answers the saved source with its failure from the list its events left, and says the failure once', async () => {
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      handlers['source:updated']({ data: saved({ status: 'updating' }) });
      handlers['source:updated']({ data: saved({ status: 'error' }) });
      handlers['source:error']({ data: { sourceId: 'n', name: 'feed.example', message: refreshFailed.message, scheduled: false } });
      throw refreshFailed;
    });
    api.listSources.mockRejectedValueOnce(new Error('list failed'));

    const result = await store.add(FEED);

    expect(result.source.id).toBe('n');
    expect(result.failure).toBe(refreshFailed);
    expect(toast).not.toHaveBeenCalled();
  });
});

// Wails orders events and call answers independently: the first refresh's error event may come
// before or after the add's answer, and the add may end without saying it.
describe('the first refresh’s error, said once whatever the order', () => {
  const FEED = 'https://feed.example/feed.json';
  const saved = (patch: Partial<Source> = {}) => ({ id: 'n', name: 'feed.example', type: 'url', url: FEED, status: 'error', ...patch }) as Source;
  const refreshFailed = new Error('typhon:sources.feed_bad_status: GET https://feed.example/feed.json: status 503');
  const errorOf = (sourceId: string, message = refreshFailed.message) => ({ data: { sourceId, name: sourceId, message, scheduled: false } });

  it('says a held error once when the add ends without saying it: the list unreadable, the dialog gone', async () => {
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      handlers['source:error'](errorOf('n'));
      throw refreshFailed;
    });
    api.listSources.mockRejectedValueOnce(new Error('list failed'));

    // What the dialog does with a rejection once it was dismissed.
    const mark = store.errorMark();
    await store.add(FEED).catch((reason: unknown) => store.toastUnannounced(reason, mark));

    expect(toast.mock.calls.filter((call) => call[1] === 'danger')).toHaveLength(1);
  });

  it('says a held error of a source the add did not save once the add ends', async () => {
    const { store, toast } = await load();
    let finish: (s: Source) => void = () => {};
    api.addSource.mockImplementationOnce(() => new Promise<Source>((resolve) => (finish = resolve)));
    const pending = store.add(FEED);

    handlers['source:error'](errorOf('z', 'status 500'));
    expect(toast).not.toHaveBeenCalled();
    seeded = [saved({ status: 'active' })];
    finish(saved({ status: 'active' }));
    await pending;

    expect(toast).toHaveBeenCalledTimes(1);
    expect(String(toast.mock.calls[0][0])).toContain('z');
  });

  it('keeps an error held while another add that did not list the source is still out', async () => {
    const { store, toast } = await load();
    let finishFirst: (s: Source) => void = () => {};
    let finishSecond: (s: Source) => void = () => {};
    api.addSource.mockImplementationOnce(() => new Promise<Source>((resolve) => (finishFirst = resolve)));
    api.addSource.mockImplementationOnce(() => new Promise<Source>((resolve) => (finishSecond = resolve)));
    const first = store.add(FEED);
    const second = store.add('https://second.example/feed.json');

    handlers['source:error'](errorOf('z', 'status 500'));
    finishFirst(saved({ status: 'active' }));
    await first;
    expect(toast).not.toHaveBeenCalled();

    finishSecond(saved({ id: 'm', url: 'https://second.example/feed.json', status: 'active' }));
    await second;
    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('swallows the error event of the failure the add already said when it comes after the answer', async () => {
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [saved()];
      throw refreshFailed;
    });

    const result = await store.add(FEED);
    expect(result.failure).toBe(refreshFailed);

    handlers['source:updated']({ data: saved({ status: 'updating' }) });
    handlers['source:updated']({ data: saved() });
    handlers['source:error'](errorOf('n'));

    expect(toast).not.toHaveBeenCalled();
  });

  it('says a later refresh’s error of that source after its late event was swallowed', async () => {
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [saved()];
      throw refreshFailed;
    });
    await store.add(FEED);
    handlers['source:updated']({ data: saved() });
    handlers['source:error'](errorOf('n'));

    handlers['source:updated']({ data: saved({ status: 'updating' }) });
    handlers['source:updated']({ data: saved() });
    handlers['source:error'](errorOf('n'));

    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('says a later refresh’s error when the add’s refresh sent no error event (its state could not be saved)', async () => {
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [saved({ status: 'active' })];
      throw refreshFailed;
    });
    await store.add(FEED);
    // Go settles the source without an error event, then a later refresh fails the same way.
    handlers['source:updated']({ data: saved({ status: 'active' }) });
    handlers['source:updated']({ data: saved({ status: 'updating' }) });
    handlers['source:updated']({ data: saved() });
    handlers['source:error'](errorOf('n'));

    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('says a different failure of that source that comes after the answer', async () => {
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [saved()];
      throw refreshFailed;
    });
    await store.add(FEED);

    handlers['source:error'](errorOf('n', 'typhon:sources.source_busy: refresh already running'));

    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('swallows no error of another source after the answer', async () => {
    seeded = [saved({ id: 'o', url: 'https://other.example/feed.json', status: 'active' })];
    const { store, toast } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [...seeded, saved()];
      throw refreshFailed;
    });
    await store.add(FEED);

    handlers['source:error'](errorOf('o'));

    expect(toast).toHaveBeenCalledTimes(1);
  });
});

// The source an add saved is the one whose id was not in the list at the press: Go saves the URL in its
// own spelling (feed.ValidateURL: url.Parse + String), so no string the frontend derives from the typed
// one can find it.
describe('an add finds the source it saved by id', () => {
  const failedAt = (url: string) => new Error(`typhon:sources.feed_bad_status: GET ${url}: status 503`);
  const urlSource = (id: string, url: string, patch: Partial<Source> = {}) => ({ id, name: id, type: 'url', url, status: 'error', ...patch }) as Source;
  const fileSource = (id: string, path: string, patch: Partial<Source> = {}) =>
    ({ id, name: id, type: 'file', url: '', path, status: 'active', ...patch }) as Source;

  // The URL as typed and as Go saves it (output/redesign-2.0/work/rv-7m6r1/gonorm.out).
  const spellings: [string, string, string][] = [
    ['a Cyrillic host', 'https://раздачи.рф/feed.json', 'https://%D1%80%D0%B0%D0%B7%D0%B4%D0%B0%D1%87%D0%B8.%D1%80%D1%84/feed.json'],
    ['a Cyrillic path', 'https://feed.example/фид.json', 'https://feed.example/%D1%84%D0%B8%D0%B4.json'],
    ['a space in the path', 'https://feed.example/my feed.json', 'https://feed.example/my%20feed.json'],
    ['an empty fragment', 'https://feed.example/feed.json#', 'https://feed.example/feed.json'],
  ];

  for (const [label, typed, stored] of spellings) {
    it(`answers the saved source for ${label}, and leaves its refresh error to the add`, async () => {
      const { store, toast } = await load();
      const err = failedAt(stored);
      api.addSource.mockImplementationOnce(async () => {
        handlers['source:updated']({ data: urlSource('n', stored, { status: 'updating' }) });
        handlers['source:error']({ data: { sourceId: 'n', name: 'n', message: err.message, scheduled: false } });
        seeded = [urlSource('n', stored)];
        throw err;
      });

      const outcome = await store.add(typed).then(
        (value) => ({ resolved: value.source.id, failure: value.failure }),
        (reason: unknown) => ({ rejected: String(reason) }),
      );

      expect(outcome).toEqual({ resolved: 'n', failure: err });
      expect(get(store.sources).map((s) => s.id)).toEqual(['n']);
      expect(toast).not.toHaveBeenCalled();
    });
  }

  it('says the error of a source that was listed at the press while an add waits', async () => {
    seeded = [urlSource('o', 'https://other.example/f.json', { status: 'active' })];
    const { store, toast } = await load();
    let finish: (s: Source) => void = () => {};
    api.addSource.mockImplementationOnce(() => new Promise<Source>((resolve) => (finish = resolve)));
    const pending = store.add('https://feed.example/f.json');

    handlers['source:error']({ data: { sourceId: 'o', name: 'Other', message: 'status 500', scheduled: false } });
    expect(toast).toHaveBeenCalledTimes(1);

    finish(urlSource('n', 'https://feed.example/f.json', { status: 'active' }));
    await pending;
  });

  it('says the error of a listed file source whose path differs from the added one only by case', async () => {
    seeded = [fileSource('o', '/feeds/Feed.json')];
    const { store, toast } = await load();
    let finish: (s: Source) => void = () => {};
    api.addSourceFile.mockImplementationOnce(() => new Promise<Source>((resolve) => (finish = resolve)));
    const pending = store.addFile('/feeds/feed.json');

    handlers['source:error']({ data: { sourceId: 'o', name: 'Feed.json', message: 'status 500', scheduled: false } });
    expect(toast).toHaveBeenCalledTimes(1);

    finish(fileSource('n', '/feeds/feed.json'));
    await pending;
  });

  it('does not take a listed file source whose path differs only by case for the one it saved', async () => {
    seeded = [fileSource('o', '/feeds/Feed.json')];
    const { store } = await load();
    api.addSourceFile.mockImplementationOnce(async () => {
      seeded = [...seeded, fileSource('n', '/feeds/feed.json', { status: 'error' })];
      throw new Error('typhon:sources.feed_invalid_json: bad');
    });

    expect((await store.addFile('/feeds/feed.json')).source.id).toBe('n');
  });

  it('takes, of two new sources, the one at the place Go named for the check', async () => {
    const goPlace = 'https://%D1%80%D0%B0%D0%B7%D0%B4%D0%B0%D1%87%D0%B8.%D1%80%D1%84/feed.json';
    const { store } = await load();
    api.addSource.mockImplementationOnce(async () => {
      seeded = [urlSource('x', 'https://elsewhere.example/feed.json'), urlSource('n', goPlace)];
      throw failedAt(goPlace);
    });

    expect((await store.add('https://раздачи.рф/feed.json', goPlace)).source.id).toBe('n');
  });

  it('rejects when the one new source is at another place than Go named: it is another add’s', async () => {
    const { store } = await load();
    const refused = new Error('typhon:sources.feed_invalid_url: bad');
    api.addSource.mockImplementationOnce(async () => {
      seeded = [urlSource('x', 'https://elsewhere.example/feed.json')];
      throw refused;
    });

    await expect(store.add('https://feed.example/feed.json', 'https://feed.example/feed.json')).rejects.toBe(refused);
  });

  it('does not take a new source of the other kind for the one it saved', async () => {
    const { store } = await load();
    const refused = new Error('typhon:sources.feed_invalid_url: bad');
    api.addSource.mockImplementationOnce(async () => {
      seeded = [fileSource('f', '/feeds/feed.json')];
      throw refused;
    });

    await expect(store.add('https://feed.example/feed.json')).rejects.toBe(refused);
  });

  it('rejects a refusal when the list was never read before the add, whatever the list holds after', async () => {
    api.listSources.mockRejectedValueOnce(new Error('list failed'));
    const { store } = await load();
    const refused = new Error('typhon:sources.feed_invalid_url: bad');
    api.addSource.mockImplementationOnce(async () => {
      seeded = [urlSource('o', 'https://other.example/feed.json', { status: 'active' })];
      throw refused;
    });

    await expect(store.add('https://feed.example/feed.json')).rejects.toBe(refused);
  });

  it('holds no error of a source listed at the press, whatever its place', async () => {
    seeded = [urlSource('o', 'https://feed.example/feed.json', { status: 'active' })];
    const { store, toast } = await load();
    let finish: (s: Source) => void = () => {};
    api.addSource.mockImplementationOnce(() => new Promise<Source>((resolve) => (finish = resolve)));
    const pending = store.add('HTTPS://FEED.EXAMPLE/feed.json');

    handlers['source:error']({ data: { sourceId: 'o', name: 'o', message: 'status 500', scheduled: false } });
    expect(toast).toHaveBeenCalledTimes(1);

    finish(urlSource('n', 'https://feed.example/feed.json', { status: 'active' }));
    await pending;
  });
});

describe('background add failures', () => {
  const failed = new Error('typhon:sources.feed_bad_status: GET https://feed.example/x: 404');

  it('toasts an error nobody announced, like a sources.json write that failed after the source was created', async () => {
    const { store, toast } = await load();
    const mark = store.errorMark();

    store.toastUnannounced(new Error('save sources: access denied'), mark);

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });

  it('stays quiet when source:error already announced the same failure after the mark', async () => {
    const { store, toast } = await load();
    const mark = store.errorMark();
    handlers['source:error']({
      data: { sourceId: 'a', name: 'Alpha', message: 'typhon:sources.feed_bad_status: GET https://feed.example/x: 404', scheduled: false },
    });
    toast.mockClear();

    store.toastUnannounced(failed, mark);

    expect(toast).not.toHaveBeenCalled();
  });

  it('still toasts when the matching announcement came before the mark', async () => {
    const { store, toast } = await load();
    handlers['source:error']({
      data: { sourceId: 'a', name: 'Alpha', message: 'typhon:sources.feed_bad_status: GET https://feed.example/x: 404', scheduled: false },
    });
    const mark = store.errorMark();
    toast.mockClear();

    store.toastUnannounced(failed, mark);

    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('does not count scheduled errors, an empty message or a different failure as announced', async () => {
    const { store, toast } = await load();
    const mark = store.errorMark();
    handlers['source:error']({ data: { sourceId: 'a', name: 'Alpha', message: 'typhon:sources.feed_bad_status: GET https://feed.example/x: 404', scheduled: true } });
    handlers['source:error']({ data: { sourceId: 'b', name: 'Beta', message: '', scheduled: false } });
    handlers['source:error']({ data: { sourceId: 'c', name: 'Gamma', message: 'typhon:sources.feed_empty', scheduled: false } });
    toast.mockClear();

    store.toastUnannounced(failed, mark);

    expect(toast).toHaveBeenCalledTimes(1);
  });
});

describe('announcements that already told the player', () => {
  const saveFailure = 'replace C:/data/sources.json: rename C:/data/.tmp-1 C:/data/sources.json: Access is denied.';

  it('treats a source:degraded save failure as announced, so a minimized add does not toast it twice', async () => {
    const { store, toast } = await load();
    const mark = store.errorMark();
    handlers['source:degraded']({ data: { degraded: true, message: saveFailure } });

    store.toastUnannounced(new Error(`save sources: ${saveFailure}`), mark);

    expect(toast).not.toHaveBeenCalled();
  });

  it('does not count a recovery notice as an announcement', async () => {
    const { store, toast } = await load();
    const mark = store.errorMark();
    handlers['source:degraded']({ data: { degraded: false, message: saveFailure } });

    store.toastUnannounced(new Error(`save sources: ${saveFailure}`), mark);

    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('shows a translated reason instead of the raw error code', async () => {
    const { toast } = await load();

    handlers['source:error']({
      data: { sourceId: 'a', name: 'Alpha', message: 'typhon:sources.feed_invalid_json: некорректный JSON фида', scheduled: false },
    });

    const text = String(toast.mock.calls[0][0]);
    expect(text).toContain('Alpha');
    expect(text).not.toContain('typhon:');
  });

  it('says the catalogue\'s plain failure, not the backend\'s text, when the error carries no known code', async () => {
    const { toast } = await load();

    handlers['source:error']({ data: { sourceId: 'a', name: 'Alpha', message: 'не удалось сопоставить 3 из 10 названий', scheduled: false } });

    const { msg } = await import('../i18n');
    const text = String(toast.mock.calls[0][0]);
    expect(text).toBe(msg('state.sourcesErrorToast', { name: 'Alpha', message: msg('errSources.fallback') }));
    expect(text).not.toContain('не удалось сопоставить');
  });
});

describe('manual refresh failures', () => {
  it('shows one toast when the backend already announced the failure', async () => {
    const { store, toast } = await load();
    api.refreshSource.mockImplementationOnce(async () => {
      handlers['source:error']({
        data: { sourceId: 'a', name: 'Alpha', message: 'typhon:sources.feed_invalid_json: некорректный JSON фида', scheduled: false },
      });
      throw new Error('typhon:sources.feed_invalid_json: некорректный JSON фида');
    });

    await store.refresh('a');

    expect(toast).toHaveBeenCalledTimes(1);
    expect(String(toast.mock.calls[0][0])).toContain('Alpha');
  });

  it('still toasts a failure the backend did not announce', async () => {
    const { store, toast } = await load();
    api.refreshSource.mockRejectedValueOnce(new Error('typhon:sources.source_busy'));

    await store.refresh('a');

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });
});
