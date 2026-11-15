import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Download } from '../services/downloads';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
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
  const store = await import('./downloads');
  await store.initDownloads();
  return { store };
}

beforeEach(() => {
  vi.clearAllMocks();
  seeded = [];
});

describe('totals count what is moving', () => {
  it('sums download speed over downloading downloads only: a verifying one with a stale speed adds nothing', async () => {
    seeded = [
      makeDownload({ id: 'a', downloadSpeed: 100, uploadSpeed: 10 }),
      makeDownload({ id: 'b', status: 'verifying', downloadSpeed: 40_000, uploadSpeed: 4_000 }),
      makeDownload({ id: 'c', status: 'paused', downloadSpeed: 7, uploadSpeed: 7 }),
    ];
    const { store } = await load();

    expect(get(store.stats)).toMatchObject({ downSpeed: 100, upSpeed: 10 });
  });

  it('sums upload over downloading and seeding downloads', async () => {
    seeded = [
      makeDownload({ id: 'a', downloadSpeed: 100, uploadSpeed: 10 }),
      makeDownload({ id: 'b', status: 'completed', seeding: true, downloadSpeed: 0, uploadSpeed: 500 }),
      makeDownload({ id: 'c', status: 'completed', seeding: false, uploadSpeed: 900 }),
    ];
    const { store } = await load();

    expect(get(store.stats)).toMatchObject({ downSpeed: 100, upSpeed: 510 });
  });

  it('keeps counting active and queued downloads as before', async () => {
    seeded = [
      makeDownload({ id: 'a' }),
      makeDownload({ id: 'b', status: 'paused' }),
      makeDownload({ id: 'c', status: 'queued' }),
      makeDownload({ id: 'd', status: 'completed' }),
    ];
    const { store } = await load();

    expect(get(store.stats)).toMatchObject({ activeCount: 2, queuedCount: 1 });
  });
});
