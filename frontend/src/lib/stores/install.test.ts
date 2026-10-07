import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { Installation, InstallStatus, InstallType } from '../services/install';

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

let seeded: Installation[] = [];

vi.mock('../services/install', () => ({
  listInstallations: vi.fn(async () => seeded),
}));

function installation(patch: Partial<Installation> = {}): Installation {
  return {
    id: 'i1',
    downloadId: 'd1',
    gameId: 'g1',
    name: 'Game One',
    type: 'archive_zip',
    status: 'installing',
    mode: 'move',
    sourcePath: '',
    contentRoot: '',
    destination: '',
    installerPath: '',
    workingDir: '',
    archivePath: '',
    progress: 0,
    currentFile: '',
    bytesDone: 0,
    bytesTotal: 0,
    executable: '',
    candidates: null,
    detectedVersion: '',
    versionSource: '',
    startedAt: '2026-09-01T10:00:00Z',
    completedAt: null,
    error: '',
    engine: '',
    silent: false,
    ...patch,
  };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./install');
  const toasts = await import('./toasts');
  await store.initInstalls();
  return { store, toast: vi.mocked(toasts.toast) };
}

const STATUSES: InstallStatus[] = [
  'pending',
  'preparing',
  'installing',
  'extracting',
  'verifying',
  'waiting_for_user',
  'completed',
  'failed',
  'cancelled',
  'interrupted',
];

const TYPES: InstallType[] = [
  'portable',
  'archive_zip',
  'archive_7z',
  'archive_rar',
  'exe_installer',
  'msi_installer',
  'unknown',
];

beforeEach(() => {
  vi.clearAllMocks();
  seeded = [];
});

describe('install status', () => {
  it('has a label of its own for every status', async () => {
    const { store } = await load();
    const labels = STATUSES.map((status) => store.installStatusLabels(status));
    expect(labels.every((label) => typeof label === 'string' && label.trim() !== '')).toBe(true);
    expect(new Set(labels).size).toBe(STATUSES.length);
  });

  it('has a label of its own for every install type', async () => {
    const { store } = await load();
    const labels = TYPES.map((type) => store.installTypeLabels(type));
    expect(labels.every((label) => typeof label === 'string' && label.trim() !== '')).toBe(true);
    expect(new Set(labels).size).toBe(TYPES.length);
  });

  it('counts only the statuses that are still working as active', async () => {
    const { store } = await load();
    const active = STATUSES.filter((status) => store.installActive(status));
    expect(active).toEqual(['pending', 'preparing', 'installing', 'extracting', 'verifying']);
  });

  it('does not treat a wizard waiting for the player as active work', async () => {
    const { store } = await load();
    expect(store.installActive('waiting_for_user')).toBe(false);
  });
});

describe('installations', () => {
  it('loads what the backend already has', async () => {
    seeded = [installation({ id: 'a' }), installation({ id: 'b', downloadId: 'd2' })];
    const { store } = await load();

    expect(get(store.installations).map((item) => item.id)).toEqual(['a', 'b']);
  });

  it('replaces an installation by id and appends a new one', async () => {
    seeded = [installation({ id: 'a', progress: 0.1 })];
    const { store } = await load();

    store.upsertInstallation(installation({ id: 'a', progress: 0.6 }));
    store.upsertInstallation(installation({ id: 'b', downloadId: 'd2' }));

    const list = get(store.installations);
    expect(list.map((item) => item.id)).toEqual(['a', 'b']);
    expect(list[0].progress).toBe(0.6);
  });

  it('keeps the most recently started installation for a download', async () => {
    seeded = [
      installation({ id: 'new', downloadId: 'd1', startedAt: '2026-09-02T10:00:00Z' }),
      installation({ id: 'old', downloadId: 'd1', startedAt: '2026-09-01T10:00:00Z' }),
      installation({ id: 'other', downloadId: 'd2', startedAt: '2026-08-01T10:00:00Z' }),
    ];
    const { store } = await load();

    const byDownload = get(store.installationsByDownload);
    expect(byDownload.get('d1')?.id).toBe('new');
    expect(byDownload.get('d2')?.id).toBe('other');
    expect(byDownload.size).toBe(2);
  });

  it('follows a retry: the new attempt replaces the failed one for the same download', async () => {
    seeded = [installation({ id: 'first', status: 'failed', startedAt: '2026-09-01T10:00:00Z' })];
    const { store } = await load();

    handlers['install:started']({ data: installation({ id: 'second', startedAt: '2026-09-01T11:00:00Z' }) });

    expect(get(store.installationsByDownload).get('d1')?.id).toBe('second');
  });

  it('updates a running installation from install:updated', async () => {
    seeded = [installation({ progress: 0.2 })];
    const { store } = await load();

    handlers['install:updated']({ data: installation({ progress: 0.7 }) });

    expect(get(store.installations)[0].progress).toBe(0.7);
  });

  it('keeps a cancelled installation in the list without a toast', async () => {
    seeded = [installation()];
    const { store, toast } = await load();

    handlers['install:cancelled']({ data: installation({ status: 'cancelled' }) });

    expect(get(store.installations)[0].status).toBe('cancelled');
    expect(toast).not.toHaveBeenCalled();
  });

  it('drops an installation that was removed', async () => {
    seeded = [installation({ id: 'a' }), installation({ id: 'b', downloadId: 'd2' })];
    const { store } = await load();

    handlers['install:removed']({ data: { id: 'a' } });

    expect(get(store.installations).map((item) => item.id)).toEqual(['b']);
  });
});

describe('install notifications', () => {
  it('tells the player when an install is done', async () => {
    const { store, toast } = await load();

    handlers['install:completed']({ data: installation({ status: 'completed', name: 'Hades' }) });

    expect(get(store.installations)[0].status).toBe('completed');
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][0]).toContain('Hades');
    expect(toast.mock.calls[0][1]).toBe('success');
  });

  it('shows a known failure code as its translated text, not the raw code', async () => {
    const { toast } = await load();

    handlers['install:failed']({
      data: installation({ status: 'failed', name: 'Hades', error: 'typhon:install.not_enough_space: нужно 5 ГБ' }),
    });

    const [text, kind] = toast.mock.calls[0];
    expect(kind).toBe('danger');
    expect(text).toContain('Hades');
    expect(text).not.toContain('typhon:');
    expect(text).not.toContain('install.not_enough_space');
  });

  it('keeps the original text when the failure has no code', async () => {
    const { toast } = await load();

    handlers['install:failed']({ data: installation({ status: 'failed', name: 'Hades', error: 'disk went away' }) });

    expect(toast.mock.calls[0][0]).toContain('disk went away');
  });
});
