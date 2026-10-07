import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { BackupEvent } from '../services/savebackup';
import type { LibraryGame } from '../services/library';

vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => vi.fn()) } }));

let enabled: () => Promise<boolean> = async () => true;
let emit: (event: BackupEvent) => void = () => {};

vi.mock('../services/savebackup', () => ({
  backupsEnabled: vi.fn(() => enabled()),
  onBackupEvent: vi.fn((handler: (event: BackupEvent) => void) => {
    emit = handler;
    return vi.fn();
  }),
}));

vi.mock('../services/library', () => ({
  getGames: vi.fn(async () => []),
  getRunningGames: vi.fn(async () => []),
}));

function event(patch: Partial<BackupEvent> = {}): BackupEvent {
  return { gameId: 'g1', kind: 'session', snapshot: null, status: 'created', code: '', error: '', ...patch };
}

async function load() {
  vi.resetModules();
  const store = await import('./savebackup');
  const library = await import('./library');
  const toasts = await import('./toasts');
  library.libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false } as LibraryGame]);
  store.initSaveBackups();
  await Promise.resolve();
  await Promise.resolve();
  return { store, library, toast: vi.mocked(toasts.toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  enabled = async () => true;
});

describe('save backups switch', () => {
  it('follows the backend when backups are shipped', async () => {
    enabled = async () => true;
    const { store } = await load();

    expect(get(store.saveBackupsEnabled)).toBe(true);
  });

  it('stays off when the backend says they are not shipped', async () => {
    enabled = async () => false;
    const { store } = await load();

    expect(get(store.saveBackupsEnabled)).toBe(false);
  });

  it('stays off when the question itself fails', async () => {
    enabled = async () => {
      throw new Error('unavailable');
    };
    const { store } = await load();

    expect(get(store.saveBackupsEnabled)).toBe(false);
  });
});

describe('automatic backup notices', () => {
  it('stays quiet about a backup the player started by hand', async () => {
    const { toast } = await load();

    emit(event({ kind: 'manual', status: 'failed', code: 'savebackup.disabled' }));

    expect(toast).not.toHaveBeenCalled();
  });

  it('stays quiet about a backup that simply worked', async () => {
    const { toast } = await load();

    emit(event({ status: 'created' }));
    emit(event({ status: 'skipped' }));

    expect(toast).not.toHaveBeenCalled();
  });

  it('names the game and the reason when an automatic backup fails', async () => {
    const { toast } = await load();

    emit(event({ status: 'failed', code: 'savebackup.saves_not_found' }));

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
    expect(String(toast.mock.calls[0][0])).toContain('Hades');
  });

  it('uses a general reason when the failure code is not one the interface knows', async () => {
    const { toast } = await load();

    emit(event({ status: 'failed', code: 'something.unexpected' }));

    expect(toast).toHaveBeenCalledTimes(1);
    expect(String(toast.mock.calls[0][0])).not.toContain('something.unexpected');
  });

  it('warns when a backup was made but the old ones could not be rotated', async () => {
    const { toast } = await load();

    emit(event({ status: 'created', error: 'could not delete the oldest snapshot', code: 'savebackup.rotation_failed' }));

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBeUndefined();
  });
});

describe('save backups dialog target', () => {
  it('opens for a known game with its title, and closes', async () => {
    const { store } = await load();

    store.openSaveBackups('g1');
    expect(get(store.saveBackupTarget)).toEqual({ gameId: 'g1', title: 'Hades' });

    store.closeSaveBackups();
    expect(get(store.saveBackupTarget)).toBeNull();
  });

  it('stays closed for a game the library does not know', async () => {
    const { store } = await load();

    store.openSaveBackups('missing');

    expect(get(store.saveBackupTarget)).toBeNull();
  });
});
