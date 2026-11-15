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

vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../install/installErrors', () => ({ installErrorText: vi.fn((err: unknown) => `text of ${String(err)}`) }));
vi.mock('../i18n', async (actual) => ({ ...(await actual<typeof import('../i18n')>()), msg: vi.fn((key: string) => key) }));
vi.mock('./toasts', () => ({ toast: vi.fn(), ACTION_MS: 1 }));

vi.mock('../services/downloads', () => ({
  listDownloads: vi.fn(async () => []),
  cancelDownload: vi.fn(async () => {}),
  downloadNow: vi.fn(async () => {}),
  forceStartDownload: vi.fn(async () => {}),
  moveDownloadDown: vi.fn(async () => {}),
  moveDownloadUp: vi.fn(async () => {}),
  moveDownloadTo: vi.fn(async () => {}),
  pauseDownload: vi.fn(async () => {}),
  removeDownload: vi.fn(async () => {}),
  resumeDownload: vi.fn(async () => {}),
}));

function makeDownload(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: '',
    infoHash: 'hash1',
    destination: '/games/d1',
    status: 'queued',
    progress: 0,
    downloaded: 0,
    total: 1000,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-09-08T00:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...patch,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function load(seed: Download[] = []) {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const services = await import('../services/downloads');
  vi.mocked(services.listDownloads).mockResolvedValueOnce(seed);
  const { toast } = await import('./toasts');
  const store = await import('./downloads');
  await store.initDownloads();
  return { store, services, toast: vi.mocked(toast) };
}

const ids = (list: Download[]) => list.map((d) => d.id);

// A reset, not a clear: a `mockRejectedValueOnce` a test left unconsumed must not reach the next one.
beforeEach(() => {
  vi.resetAllMocks();
});

describe('an action reports its outcome', () => {
  it('tells the caller that a cancel failed and by which code, without a toast when the caller shows it', async () => {
    const { store, services, toast } = await load([makeDownload()]);
    const err = new Error('typhon:download.not_found');
    vi.mocked(services.cancelDownload).mockRejectedValueOnce(err);

    const outcome = await store.requestDownloadAction('d1', 'cancel', { toast: false });

    expect(outcome).toEqual({ ok: false, code: 'download.not_found', error: err });
    expect(toast).not.toHaveBeenCalled();
  });

  it('says ok when Go accepted the action', async () => {
    const { store, services } = await load([makeDownload()]);

    expect(await store.requestDownloadAction('d1', 'pause')).toEqual({ ok: true });
    expect(services.pauseDownload).toHaveBeenCalledWith('d1');
  });

  it('toasts a failure by default, as the old callers expect', async () => {
    const { store, services, toast } = await load([makeDownload()]);
    vi.mocked(services.resumeDownload).mockRejectedValueOnce(new Error('typhon:download.unavailable'));

    const outcome = await store.requestDownloadAction('d1', 'resume');

    expect(outcome.ok).toBe(false);
    expect(toast).toHaveBeenCalledWith('text of Error: typhon:download.unavailable', 'danger');
  });

  it.each([
    ['pause', 'pauseDownload'],
    ['resume', 'resumeDownload'],
    ['cancel', 'cancelDownload'],
    ['remove', 'removeDownload'],
    ['forceStart', 'forceStartDownload'],
    ['downloadNow', 'downloadNow'],
    ['moveUp', 'moveDownloadUp'],
    ['moveDown', 'moveDownloadDown'],
  ] as const)('the old %s(id) still resolves to nothing and toasts its failure', async (action, service) => {
    const { store, services, toast } = await load([makeDownload()]);
    vi.mocked(services[service]).mockRejectedValueOnce(new Error('typhon:download.unavailable'));

    await expect(store[action]('d1')).resolves.toBeUndefined();

    expect(services[service]).toHaveBeenCalledWith('d1');
    expect(toast).toHaveBeenCalledWith('text of Error: typhon:download.unavailable', 'danger');
  });
});

describe('moving a download to a place in the queue', () => {
  it('sends the place to Go and reports the outcome', async () => {
    const { store, services } = await load([makeDownload()]);

    expect(await store.requestDownloadAction('d1', 'moveTo', { position: 2 })).toEqual({ ok: true });

    expect(services.moveDownloadTo).toHaveBeenCalledWith('d1', 2);
  });

  it('reports a refusal by its code', async () => {
    const { store, services, toast } = await load([makeDownload({ status: 'paused' })]);
    const err = new Error('typhon:download.unavailable');
    vi.mocked(services.moveDownloadTo).mockRejectedValueOnce(err);

    const outcome = await store.requestDownloadAction('d1', 'moveTo', { position: 0, toast: false });

    expect(outcome).toEqual({ ok: false, code: 'download.unavailable', error: err });
    expect(toast).not.toHaveBeenCalled();
  });

  // Held by the type checker (npm run check); the run only keeps the cases in the suite.
  it('takes a place for moveTo only, and never without one', async () => {
    const { store } = await load([makeDownload()]);
    const calls = [
      // @ts-expect-error: a place means nothing to a pause
      () => store.requestDownloadAction('d1', 'pause', { position: 1 }),
      // @ts-expect-error: a move to a place needs the place
      () => store.requestDownloadAction('d1', 'moveTo'),
      // @ts-expect-error: a move to a place needs the place
      () => store.requestDownloadAction('d1', 'moveTo', { toast: false }),
    ];
    expect(calls).toHaveLength(3);
  });
});

describe('a second call while the first is in flight', () => {
  it('is not sent for the same download and action, and gets the first call\'s outcome', async () => {
    const { store, services } = await load([makeDownload()]);
    const call = deferred<void>();
    vi.mocked(services.pauseDownload).mockReturnValueOnce(call.promise);

    const first = store.requestDownloadAction('d1', 'pause');
    const second = store.requestDownloadAction('d1', 'pause');
    const old = store.pause('d1');
    call.resolve();

    expect(await first).toEqual({ ok: true });
    expect(await second).toEqual({ ok: true });
    await old;
    expect(services.pauseDownload).toHaveBeenCalledTimes(1);
  });

  it('is sent for another download or another action, and again once the first has answered', async () => {
    const { store, services } = await load([makeDownload(), makeDownload({ id: 'd2' })]);
    const call = deferred<void>();
    vi.mocked(services.pauseDownload).mockReturnValueOnce(call.promise);

    const first = store.requestDownloadAction('d1', 'pause');
    await store.requestDownloadAction('d2', 'pause');
    await store.requestDownloadAction('d1', 'cancel');
    call.resolve();
    await first;
    await store.requestDownloadAction('d1', 'pause');

    expect(vi.mocked(services.pauseDownload).mock.calls).toEqual([['d1'], ['d2'], ['d1']]);
    expect(services.cancelDownload).toHaveBeenCalledWith('d1');
  });

  it('toasts a shared failure once, when either caller asked for it', async () => {
    const { store, services, toast } = await load([makeDownload()]);
    const call = deferred<void>();
    vi.mocked(services.cancelDownload).mockReturnValueOnce(call.promise);

    const quiet = store.requestDownloadAction('d1', 'cancel', { toast: false });
    const loud = store.requestDownloadAction('d1', 'cancel');
    call.reject(new Error('typhon:download.not_found'));

    expect((await quiet).ok).toBe(false);
    expect((await loud).ok).toBe(false);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(services.cancelDownload).toHaveBeenCalledTimes(1);
  });

  it('is sent again after a failed first call', async () => {
    const { store, services } = await load([makeDownload()]);
    vi.mocked(services.pauseDownload).mockRejectedValueOnce(new Error('typhon:download.unavailable'));

    expect((await store.requestDownloadAction('d1', 'pause', { toast: false })).ok).toBe(false);
    expect(await store.requestDownloadAction('d1', 'pause')).toEqual({ ok: true });
    expect(services.pauseDownload).toHaveBeenCalledTimes(2);
  });
});

describe('moves are not shared: each is sent after the one before it', () => {
  it('sends two steps up as two calls, the second after the first answered, each with its outcome', async () => {
    const { store, services } = await load([makeDownload()]);
    const firstCall = deferred<void>();
    const secondCall = deferred<void>();
    vi.mocked(services.moveDownloadUp).mockReturnValueOnce(firstCall.promise).mockReturnValueOnce(secondCall.promise);

    const first = store.requestDownloadAction('d1', 'moveUp');
    const second = store.requestDownloadAction('d1', 'moveUp');
    await Promise.resolve();
    expect(services.moveDownloadUp).toHaveBeenCalledTimes(1);

    firstCall.resolve();
    expect(await first).toEqual({ ok: true });
    await vi.waitFor(() => expect(services.moveDownloadUp).toHaveBeenCalledTimes(2));
    secondCall.reject(new Error('typhon:download.unavailable'));

    expect(await second).toMatchObject({ ok: false, code: 'download.unavailable' });
  });

  it('sends a move to another place made during a move to a place, in order', async () => {
    const { store, services } = await load([makeDownload()]);
    const firstCall = deferred<void>();
    vi.mocked(services.moveDownloadTo).mockReturnValueOnce(firstCall.promise);

    const first = store.requestDownloadAction('d1', 'moveTo', { position: 3 });
    const second = store.requestDownloadAction('d1', 'moveTo', { position: 0 });
    const step = store.moveDown('d1');
    await Promise.resolve();
    expect(vi.mocked(services.moveDownloadTo).mock.calls).toEqual([['d1', 3]]);
    firstCall.resolve();

    expect(await first).toEqual({ ok: true });
    expect(await second).toEqual({ ok: true });
    await step;
    expect(vi.mocked(services.moveDownloadTo).mock.calls).toEqual([
      ['d1', 3],
      ['d1', 0],
    ]);
    expect(services.moveDownloadDown).toHaveBeenCalledWith('d1');
    expect(vi.mocked(services.moveDownloadDown).mock.invocationCallOrder[0]).toBeGreaterThan(
      vi.mocked(services.moveDownloadTo).mock.invocationCallOrder[1],
    );
  });

  it('a failed first move does not swallow the second, and each caller sees its own outcome and toast', async () => {
    const { store, services, toast } = await load([makeDownload()]);
    const firstCall = deferred<void>();
    vi.mocked(services.moveDownloadUp).mockReturnValueOnce(firstCall.promise);

    const first = store.requestDownloadAction('d1', 'moveUp', { toast: false });
    const second = store.requestDownloadAction('d1', 'moveUp');
    firstCall.reject(new Error('typhon:download.unavailable'));

    expect(await first).toMatchObject({ ok: false, code: 'download.unavailable' });
    expect(await second).toEqual({ ok: true });
    expect(services.moveDownloadUp).toHaveBeenCalledTimes(2);
    expect(toast).not.toHaveBeenCalled();
  });

  it('toasts the second move\'s failure when its caller asked, though the first did not', async () => {
    const { store, services, toast } = await load([makeDownload()]);
    vi.mocked(services.moveDownloadDown)
      .mockRejectedValueOnce(new Error('typhon:download.unavailable'))
      .mockRejectedValueOnce(new Error('typhon:download.not_found'));

    const first = store.requestDownloadAction('d1', 'moveDown', { toast: false });
    const second = store.requestDownloadAction('d1', 'moveDown');
    await first;
    await second;

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast).toHaveBeenCalledWith('text of Error: typhon:download.not_found', 'danger');
  });

  it('moves of two downloads do not wait for each other', async () => {
    const { store, services } = await load([makeDownload(), makeDownload({ id: 'd2' })]);
    const held = deferred<void>();
    vi.mocked(services.moveDownloadUp).mockReturnValueOnce(held.promise);

    const first = store.requestDownloadAction('d1', 'moveUp');
    expect(await store.requestDownloadAction('d2', 'moveUp')).toEqual({ ok: true });
    held.resolve();
    await first;

    expect(vi.mocked(services.moveDownloadUp).mock.calls).toEqual([['d1'], ['d2']]);
  });
});

describe('the order follows download:reordered', () => {
  const reordered = (data: unknown) => handlers['download:reordered']({ data });

  it('puts the list in the order Go sends', async () => {
    const { store } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' }), makeDownload({ id: 'c' })]);

    reordered({ ids: ['c', 'a', 'b'] });

    expect(ids(get(store.downloads))).toEqual(['c', 'a', 'b']);
  });

  it('does not bring back a download removed meanwhile, and keeps one added since at the end', async () => {
    const { store } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' }), makeDownload({ id: 'gone' })]);
    handlers['download:removed']({ data: { id: 'gone' } });
    handlers['download:added']({ data: makeDownload({ id: 'new' }) });

    reordered({ ids: ['gone', 'b', 'a'] });

    expect(ids(get(store.downloads))).toEqual(['b', 'a', 'new']);
  });

  it('keeps the records the events brought: the order is all it takes', async () => {
    const { store } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' })]);
    handlers['download:updated']({ data: makeDownload({ id: 'a', status: 'downloading', downloaded: 300 }) });

    reordered({ ids: ['b', 'a'] });

    expect(ids(get(store.downloads))).toEqual(['b', 'a']);
    expect(get(store.downloadsById).get('a')).toMatchObject({ status: 'downloading', downloaded: 300 });
  });

  it('changes nothing for a payload that is not a list of ids', async () => {
    const { store } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' })]);
    const before = get(store.downloads);

    for (const data of [null, undefined, 'b,a', {}, { id: 'a' }, { ids: null }, { ids: 'ba' }, { ids: [2, 1] }, { ids: ['b', 7] }]) {
      reordered(data);
    }

    expect(get(store.downloads)).toBe(before);
  });

  it('a move reads no list: the order arrives with the event', async () => {
    const { store, services } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' })]);

    expect(await store.requestDownloadAction('b', 'moveUp')).toEqual({ ok: true });
    await store.moveDown('a');

    expect(services.listDownloads).toHaveBeenCalledTimes(1);
    expect(ids(get(store.downloads))).toEqual(['a', 'b']);
  });

  it('reports a move Go refused', async () => {
    const { store, services } = await load([makeDownload({ id: 'a', status: 'paused' })]);
    vi.mocked(services.moveDownloadUp).mockRejectedValueOnce(new Error('typhon:download.unavailable'));

    const outcome = await store.requestDownloadAction('a', 'moveUp', { toast: false });

    expect(outcome).toMatchObject({ ok: false, code: 'download.unavailable' });
  });
});
