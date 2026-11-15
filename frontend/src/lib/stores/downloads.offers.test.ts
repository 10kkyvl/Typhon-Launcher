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
