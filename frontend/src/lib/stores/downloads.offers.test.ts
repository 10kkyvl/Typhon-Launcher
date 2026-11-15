import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Download } from '../services/downloads';
import type { PlayThrottle } from '../services/playThrottle';

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

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./downloads');
  const offers = await import('./downloadOffers');
  const throttle = await import('./playThrottle');
  await store.initDownloads();
  return { offers, throttle };
}

const blocked: PlayThrottle = { active: false, mode: 'off', downloadLimit: 0, uploadLimit: 0, schedule: { blocked: true, nextOpenAt: '' } };

beforeEach(() => {
  vi.clearAllMocks();
  seeded = [];
});

describe('waiting for the schedule', () => {
  it('names the downloads the schedule holds back, derived from the play-throttle state', async () => {
    seeded = [
      makeDownload({ id: 'queued', status: 'queued' }),
      makeDownload({ id: 'running', status: 'downloading' }),
      makeDownload({ id: 'now', status: 'queued', ignoreSchedule: true }),
      makeDownload({ id: 'paused', status: 'paused' }),
      makeDownload({ id: 'done', status: 'completed' }),
    ];
    const { offers, throttle } = await load();
    expect([...get(offers.waitingForSchedule)]).toEqual([]);

    throttle.playThrottle.set(blocked);

    expect([...get(offers.waitingForSchedule)].sort()).toEqual(['queued', 'running']);
  });

  it('offers "Download now" through the shared offers only where the schedule holds the download', async () => {
    seeded = [makeDownload({ id: 'q', status: 'queued' }), makeDownload({ id: 'p', status: 'paused' })];
    const { offers, throttle } = await load();
    throttle.playThrottle.set(blocked);

    const byId = get(offers.downloadOffers);
    expect(byId.get('q')).toContain('downloadNow');
    expect(byId.get('p')).not.toContain('downloadNow');
    expect(byId.get('p')).toContain('resume');
  });
});

describe('the offers keep still while only progress moves', () => {
  function progress(id: string, step: number) {
    handlers['download:progress']({
      data: { id, status: 'downloading', progress: step / 20, downloaded: step * 10, downloadSpeed: 1000 + step, uploadSpeed: 0, etaSeconds: 5, seeders: 1, peers: 1, stalled: false },
    });
  }

  async function watched() {
    seeded = [makeDownload({ id: 'a', status: 'downloading' }), makeDownload({ id: 'b', status: 'queued' })];
    const { offers, throttle } = await load();
    throttle.playThrottle.set(blocked);
    const byId: Array<ReadonlyMap<string, readonly string[]>> = [];
    const waiting: Array<ReadonlySet<string>> = [];
    const stops = [offers.downloadOffers.subscribe((value) => byId.push(value)), offers.waitingForSchedule.subscribe((value) => waiting.push(value))];
    return { byId, waiting, throttle, stop: () => stops.forEach((stop) => stop()) };
  }

  it('a burst of progress ticks notifies nobody and keeps the same array for the download', async () => {
    const { byId, waiting, stop } = await watched();
    const first = byId[0].get('a');

    for (let step = 0; step < 12; step++) progress('a', step);

    expect(byId).toHaveLength(1);
    expect(waiting).toHaveLength(1);
    expect(byId[0].get('a')).toBe(first);
    stop();
  });

  it('a status change notifies once, with a new array for that download only', async () => {
    const { byId, waiting, stop } = await watched();
    progress('a', 1);

    handlers['download:updated']({ data: makeDownload({ id: 'a', status: 'paused' }) });

    expect(byId).toHaveLength(2);
    expect(byId[1].get('a')).not.toBe(byId[0].get('a'));
    expect(byId[1].get('a')).toContain('resume');
    expect(byId[1].get('b')).toBe(byId[0].get('b'));
    expect(waiting).toHaveLength(2);
    expect([...waiting[1]]).toEqual(['b']);
    stop();
  });

  it('a new throttle that keeps the schedule blocked notifies nobody; lifting the block notifies', async () => {
    const { byId, waiting, throttle, stop } = await watched();

    throttle.playThrottle.set({ ...blocked, active: true, mode: 'limit', downloadLimit: 1024 });
    expect(byId).toHaveLength(1);
    expect(waiting).toHaveLength(1);

    throttle.playThrottle.set({ ...blocked, schedule: { blocked: false, nextOpenAt: '' } });
    expect(byId).toHaveLength(2);
    expect(byId[1].get('b')).not.toContain('downloadNow');
    expect(waiting).toHaveLength(2);
    expect(waiting[1].size).toBe(0);
    stop();
  });

  it('"Download now" on one download changes that download only', async () => {
    const { byId, stop } = await watched();

    handlers['download:updated']({ data: makeDownload({ id: 'b', status: 'queued', ignoreSchedule: true }) });

    expect(byId).toHaveLength(2);
    expect(byId[1].get('b')).not.toContain('downloadNow');
    expect(byId[1].get('a')).toBe(byId[0].get('a'));
    stop();
  });

  it('a download that arrives or leaves is news', async () => {
    const { byId, stop } = await watched();

    handlers['download:added']({ data: makeDownload({ id: 'c', status: 'completed' }) });
    handlers['download:removed']({ data: { id: 'a' } });

    expect(byId).toHaveLength(3);
    expect([...byId[2].keys()]).toEqual(['b', 'c']);
    stop();
  });
});
