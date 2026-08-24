import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Download, DownloadStatus } from '../services/downloads';
import type { Installation, InstallStatus } from '../services/install';

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/downloads', () => ({
  listDownloads: vi.fn(async () => []),
  getDownload: vi.fn(),
  pauseDownload: vi.fn(),
  resumeDownload: vi.fn(),
  cancelDownload: vi.fn(),
  removeDownload: vi.fn(),
  forceStartDownload: vi.fn(),
  moveDownloadUp: vi.fn(),
  moveDownloadDown: vi.fn(),
}));
vi.mock('../services/install', () => ({ listInstallations: vi.fn(async () => []) }));

function makeDownload(overrides: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game',
    type: 'torrent',
    source: '',
    infoHash: '',
    destination: 'D:/games',
    status: 'downloading' as DownloadStatus,
    progress: 0.5,
    downloaded: 512,
    total: 1024,
    downloadSpeed: 1024 * 1024,
    uploadSpeed: 0,
    etaSeconds: 60,
    seeders: 1,
    peers: 1,
    files: [],
    seeding: false,
    addedAt: '2026-01-01T00:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...overrides,
  };
}

function makeInstall(overrides: Partial<Installation> = {}): Installation {
  return {
    id: 'i1',
    downloadId: 'd1',
    gameId: '',
    name: 'Game',
    type: 'archive_zip',
    status: 'extracting' as InstallStatus,
    mode: 'copy',
    sourcePath: '',
    contentRoot: '',
    destination: '',
    installerPath: '',
    workingDir: '',
    archivePath: '',
    progress: 0.25,
    currentFile: 'data/pak0.bin',
    bytesDone: 0,
    bytesTotal: 0,
    executable: '',
    candidates: null,
    detectedVersion: '',
    versionSource: '',
    startedAt: '2026-01-01T00:00:00Z',
    completedAt: null,
    error: '',
    engine: '',
    silent: false,
    ...overrides,
  };
}

async function load() {
  vi.resetModules();
  const downloadsStore = await import('./downloads');
  const installStore = await import('./install');
  const activityStore = await import('./activity');
  return { downloadsStore, installStore, activityStore };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('activity', () => {
  it('is empty when nothing is running', async () => {
    const { activityStore } = await load();
    expect(get(activityStore.activity)).toEqual([]);
    expect(get(activityStore.activitySummary).primary).toBeNull();
  });

  it('skips finished downloads and keeps running ones', async () => {
    const { downloadsStore, activityStore } = await load();
    downloadsStore.downloads.set([
      makeDownload({ id: 'd1' }),
      makeDownload({ id: 'd2', status: 'completed' }),
      makeDownload({ id: 'd3', status: 'failed' }),
      makeDownload({ id: 'd4', status: 'paused', progress: 0.1 }),
    ]);
    const items = get(activityStore.activity);
    expect(items.map((i) => i.downloadId)).toEqual(['d1', 'd4']);
    expect(items[0].detail).toBe('1,0 МБ/с · осталось 1 мин 0 сек');
    expect(items[1].tone).toBe('muted');
    expect(items[1].resumable).toBe(true);
  });

  it('replaces a download row with its installation', async () => {
    const { downloadsStore, installStore, activityStore } = await load();
    downloadsStore.downloads.set([makeDownload({ id: 'd1', status: 'verifying' })]);
    installStore.installations.set([makeInstall({ downloadId: 'd1' })]);
    const items = get(activityStore.activity);
    expect(items).toHaveLength(1);
    expect(items[0].kind).toBe('install');
    expect(items[0].status).toBe('Распаковка');
    expect(items[0].detail).toBe('data/pak0.bin');
  });

  it('puts an installation waiting for the user first', async () => {
    const { downloadsStore, installStore, activityStore } = await load();
    downloadsStore.downloads.set([makeDownload({ id: 'd2', name: 'Other' })]);
    installStore.installations.set([
      makeInstall({ id: 'i1', downloadId: 'd1', status: 'installing', progress: 0.5 }),
      makeInstall({ id: 'i2', downloadId: 'd3', status: 'waiting_for_user', progress: 0 }),
    ]);
    const items = get(activityStore.activity);
    expect(items.map((i) => i.key)).toEqual(['install:i2', 'install:i1', 'download:d2']);
    expect(get(activityStore.activitySummary).attention).toBe(true);
  });

  it('averages progress across every running item', async () => {
    const { downloadsStore, installStore, activityStore } = await load();
    downloadsStore.downloads.set([makeDownload({ id: 'd2', progress: 0.75 })]);
    installStore.installations.set([makeInstall({ downloadId: 'd1', progress: 0.25 })]);
    const summary = get(activityStore.activitySummary);
    expect(summary.count).toBe(2);
    expect(summary.progress).toBeCloseTo(0.5);
    expect(summary.primary?.kind).toBe('install');
  });

  it('ignores installations that already finished', async () => {
    const { installStore, activityStore } = await load();
    installStore.installations.set([
      makeInstall({ id: 'i1', status: 'completed' }),
      makeInstall({ id: 'i2', status: 'failed' }),
      makeInstall({ id: 'i3', status: 'cancelled' }),
    ]);
    expect(get(activityStore.activity)).toEqual([]);
  });
});
