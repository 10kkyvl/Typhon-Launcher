import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
vi.mock('../install/installErrors', () => ({ installErrorText: vi.fn((err: unknown) => String(err)) }));
vi.mock('./toasts', () => ({ toast: vi.fn(), ACTION_MS: 1 }));

let seeded: Download[] = [];

vi.mock('../services/downloads', () => ({
  listDownloads: vi.fn(async () => seeded),
  cancelDownload: vi.fn(),
  downloadNow: vi.fn(),
  forceStartDownload: vi.fn(),
  moveDownloadDown: vi.fn(),
  moveDownloadUp: vi.fn(),
  pauseDownload: vi.fn(),
  removeDownload: vi.fn(),
  resumeDownload: vi.fn(),
}));

function makeDownload(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: '',
    infoHash: 'hash1',
    destination: '/games/d1',
    status: 'downloading',
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

const T0 = Date.parse('2026-11-15T12:00:00.000Z');

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./downloads');
  await store.initDownloads();
  return { store };
}

function tick(id: string, downloadSpeed: number, status: Download['status'] = 'downloading') {
  handlers['download:progress']({
    data: { id, status, progress: 0.5, downloaded: 500, downloadSpeed, uploadSpeed: 0, etaSeconds: 10, seeders: 1, peers: 1, stalled: false },
  });
}

/** Four ticks a second, as Go sends them, each second's speeds given by `speedAt(second)`. */
function run(id: string, seconds: number, speedAt: (second: number) => number) {
  for (let s = 0; s < seconds; s++) {
    for (let q = 0; q < 4; q++) {
      tick(id, speedAt(s) + q);
      vi.advanceTimersByTime(250);
    }
  }
}

function last<T>(list: readonly T[], count: number): T[] {
  return list.slice(list.length - count);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  vi.setSystemTime(T0);
  seeded = [makeDownload({ downloadSpeed: 0 })];
});

afterEach(() => {
  vi.useRealTimers();
});

describe('speed history', () => {
  it('keeps one sample a second, the last tick of that second, for the last 120 seconds', async () => {
    const { store } = await load();
    run('d1', 3, (s) => (s + 1) * 1000);
    vi.advanceTimersByTime(1);

    const series = get(store.speedHistory('d1'));

    expect(series.samples).toHaveLength(120);
    expect(series.moving).toBe(true);
    // Each second's last tick is speed + 3; the current second has seen no tick yet and holds the last.
    expect(last(series.samples, 4)).toEqual([1003, 2003, 3003, 3003]);
  });

  it('forgets samples older than 120 seconds', async () => {
    const { store } = await load();
    run('d1', 200, (s) => s * 10);

    const series = get(store.speedHistory('d1'));

    expect(series.samples).toHaveLength(120);
    expect(series.samples[0]).toBe(81 * 10 + 3);
    expect(series.samples.every((sample) => sample !== null)).toBe(true);
  });

  it('holds the speed while Go sends no tick because nothing changed', async () => {
    const { store } = await load();
    run('d1', 2, () => 5000);
    vi.advanceTimersByTime(10_000);

    const series = get(store.speedHistory('d1'));

    expect(series.moving).toBe(true);
    expect(last(series.samples, 11).every((sample) => sample === 5003)).toBe(true);
  });

  it('draws a pause as a drop to zero and then a gap, not as the last speed held', async () => {
    const { store } = await load();
    run('d1', 3, () => 8000);
    handlers['download:updated']({ data: makeDownload({ status: 'paused', downloadSpeed: 0 }) });
    vi.advanceTimersByTime(5000);

    const series = get(store.speedHistory('d1'));

    expect(series.moving).toBe(false);
    expect(last(series.samples, 8)).toEqual([8003, 8003, 0, null, null, null, null, null]);
  });

  it('starts drawing again when the download resumes', async () => {
    const { store } = await load();
    run('d1', 2, () => 8000);
    handlers['download:updated']({ data: makeDownload({ status: 'paused' }) });
    vi.advanceTimersByTime(3000);
    handlers['download:updated']({ data: makeDownload({ status: 'downloading', downloadSpeed: 100 }) });
    run('d1', 1, () => 200);

    const series = get(store.speedHistory('d1'));

    expect(series.moving).toBe(true);
    expect(last(series.samples, 3)).toEqual([null, 203, 203]);
  });

  it('keeps nothing for a download that never moved', async () => {
    seeded = [makeDownload({ id: 'q', status: 'queued' })];
    const { store } = await load();
    vi.advanceTimersByTime(3000);

    const series = get(store.speedHistory('q'));

    expect(series).toEqual({ samples: new Array(120).fill(null), moving: false });
  });

  it('drops the history of a removed download', async () => {
    const { store } = await load();
    run('d1', 3, () => 8000);
    handlers['download:removed']({ data: { id: 'd1' } });
    handlers['download:added']({ data: makeDownload({ status: 'queued' }) });

    expect(get(store.speedHistory('d1'))).toEqual({ samples: new Array(120).fill(null), moving: false });
  });

  it('keeps a history per download', async () => {
    seeded = [makeDownload({ id: 'a' }), makeDownload({ id: 'b' })];
    const { store } = await load();
    tick('a', 100);
    tick('b', 900);
    vi.advanceTimersByTime(1000);

    expect(last(get(store.speedHistory('a')).samples, 2)).toEqual([100, 100]);
    expect(last(get(store.speedHistory('b')).samples, 2)).toEqual([900, 900]);
  });

  it('runs a timer only while somebody reads, one sample a second, and leaves none behind', async () => {
    const { store } = await load();
    const timersBefore = vi.getTimerCount();
    const history = store.speedHistory('d1');
    expect(vi.getTimerCount()).toBe(timersBefore);

    const seen: Array<number | null>[] = [];
    const stop = history.subscribe((series) => seen.push(series.samples));
    expect(vi.getTimerCount()).toBe(timersBefore + 1);
    tick('d1', 300);
    vi.advanceTimersByTime(2000);
    expect(seen).toHaveLength(3);
    expect(last(seen[2], 3)).toEqual([300, 300, 300]);

    stop();
    expect(vi.getTimerCount()).toBe(timersBefore);
  });
});
