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

  it('is sent again after a failed first call', async () => {
    const { store, services } = await load([makeDownload()]);
    vi.mocked(services.pauseDownload).mockRejectedValueOnce(new Error('typhon:download.unavailable'));

    expect((await store.requestDownloadAction('d1', 'pause', { toast: false })).ok).toBe(false);
    expect(await store.requestDownloadAction('d1', 'pause')).toEqual({ ok: true });
    expect(services.pauseDownload).toHaveBeenCalledTimes(2);
  });
});

describe('reordering follows Go without overwriting the list from a snapshot', () => {
  it('keeps a download removed and one added while the order was asked for', async () => {
    const { store, services } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' }), makeDownload({ id: 'gone' })]);
    const listed = deferred<Download[]>();
    vi.mocked(services.listDownloads).mockReturnValueOnce(listed.promise);

    const move = store.moveUp('b');
    await vi.waitFor(() => expect(services.listDownloads).toHaveBeenCalledTimes(2));
    handlers['download:removed']({ data: { id: 'gone' } });
    handlers['download:added']({ data: makeDownload({ id: 'new' }) });
    listed.resolve([makeDownload({ id: 'b' }), makeDownload({ id: 'a' }), makeDownload({ id: 'gone' })]);
    await move;

    expect(ids(get(store.downloads))).toEqual(['b', 'a', 'new']);
  });

  it('keeps the records the events brought: the order is all it takes from the answer', async () => {
    const { store, services } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' })]);
    const listed = deferred<Download[]>();
    vi.mocked(services.listDownloads).mockReturnValueOnce(listed.promise);

    const move = store.moveDown('a');
    await vi.waitFor(() => expect(services.listDownloads).toHaveBeenCalledTimes(2));
    handlers['download:updated']({ data: makeDownload({ id: 'a', status: 'downloading', downloaded: 300 }) });
    listed.resolve([makeDownload({ id: 'b' }), makeDownload({ id: 'a', status: 'queued', downloaded: 0 })]);
    await move;

    expect(ids(get(store.downloads))).toEqual(['b', 'a']);
    expect(get(store.downloadsById).get('a')).toMatchObject({ status: 'downloading', downloaded: 300 });
  });

  it('applies only the newest order when two answers come back out of turn', async () => {
    const { store, services } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' }), makeDownload({ id: 'c' })]);
    const older = deferred<Download[]>();
    const newer = deferred<Download[]>();
    vi.mocked(services.listDownloads).mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);

    const first = store.moveUp('b');
    await vi.waitFor(() => expect(services.listDownloads).toHaveBeenCalledTimes(2));
    const second = store.moveUp('c');
    await vi.waitFor(() => expect(services.listDownloads).toHaveBeenCalledTimes(3));
    newer.resolve([makeDownload({ id: 'b' }), makeDownload({ id: 'c' }), makeDownload({ id: 'a' })]);
    await second;
    older.resolve([makeDownload({ id: 'b' }), makeDownload({ id: 'a' }), makeDownload({ id: 'c' })]);
    await first;

    expect(ids(get(store.downloads))).toEqual(['b', 'c', 'a']);
  });

  it('asks for no order after a move Go refused, and reports the refusal', async () => {
    const { store, services } = await load([makeDownload({ id: 'a', status: 'paused' })]);
    vi.mocked(services.moveDownloadUp).mockRejectedValueOnce(new Error('typhon:download.unavailable'));

    const outcome = await store.requestDownloadAction('a', 'moveUp', { toast: false });

    expect(outcome).toMatchObject({ ok: false, code: 'download.unavailable' });
    expect(services.listDownloads).toHaveBeenCalledTimes(1);
  });

  it('keeps the move as done when only reading the new order failed, and says so', async () => {
    const { store, services, toast } = await load([makeDownload({ id: 'a' }), makeDownload({ id: 'b' })]);
    vi.mocked(services.listDownloads).mockRejectedValueOnce(new Error('backend unavailable'));

    const outcome = await store.requestDownloadAction('b', 'moveUp', { toast: false });

    expect(outcome).toEqual({ ok: true });
    expect(ids(get(store.downloads))).toEqual(['a', 'b']);
    expect(toast).toHaveBeenCalledWith('text of Error: backend unavailable', 'danger');
  });
});
