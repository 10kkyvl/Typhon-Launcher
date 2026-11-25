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

    expect(added.id).toBe('f');
    expect(get(store.sources).map((item) => item.id)).toEqual(['f']);
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
