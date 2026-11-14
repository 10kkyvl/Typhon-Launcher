import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { cloudSyncNow, type CloudGameStatus, type CloudStatus } from '../services/cloudsaves';
import { locateSaves, type LibraryGame, type SavesResult } from '../services/library';
import { createBackup, deleteBackup, listBackups, type Snapshot } from '../services/savebackup';
import { openFolder, selectFolder, type Settings } from '../services/settings';
import { closeCloudConflict, cloudConflictTarget, cloudStatus } from '../stores/cloudsaves';
import { libraryGames, runningGames } from '../stores/library';
import { saveBackupsEnabled } from '../stores/savebackup';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { authState } from '../stores/user';
import { renderDom } from '../testing/dom';
import GameSaves from './GameSaves.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  locateSaves: vi.fn(),
  setSavesDir: vi.fn(async () => {}),
}));
vi.mock('../services/savebackup', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/savebackup')>()),
  listBackups: vi.fn(),
  createBackup: vi.fn(),
  deleteBackup: vi.fn(),
}));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  openFolder: vi.fn(),
  selectFolder: vi.fn(),
}));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudSyncNow: vi.fn(),
}));

const found = (path = 'C:/Users/me/Saved Games/Hades'): SavesResult => ({
  path,
  candidates: null,
  unreadable: 0,
  hint: null,
}) as unknown as SavesResult;
const notFound = { path: '', candidates: null, unreadable: 0, hint: null } as unknown as SavesResult;

const snapshot = (patch: Partial<Snapshot> = {}): Snapshot =>
  ({ id: 's1', gameId: 'g1', kind: 'manual', createdAt: '2026-11-01T10:00:00Z', sourcePath: '', sizeBytes: 10, files: 1, digest: '', ...patch }) as Snapshot;

const cloudGame = (patch: Partial<CloudGameStatus>): CloudGameStatus => ({ gameId: 'g1', state: 'synced', code: '', message: '', at: '', syncedAt: '', ...patch });
const statusWith = (...games: CloudGameStatus[]): CloudStatus => ({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function cloudOn() {
  settings.set({ cloudSaves: true } as Settings);
  authState.set('authenticated');
}

beforeEach(() => {
  libraryGames.set([
    { id: 'g1', title: 'Hades', canonicalGameId: 'igdb:1' } as LibraryGame,
    { id: 'g2', title: 'Celeste', canonicalGameId: 'igdb:2' } as LibraryGame,
  ]);
  settings.set({ cloudSaves: false } as Settings);
  saveBackupsEnabled.set(true);
  vi.mocked(locateSaves).mockResolvedValue(found());
  vi.mocked(listBackups).mockResolvedValue([]);
});

afterEach(() => {
  closeCloudConflict();
  libraryGames.set([]);
  runningGames.set(new Set());
  settings.set(null);
  cloudStatus.set(null);
  authState.set('bootstrapping');
  saveBackupsEnabled.set(false);
  toasts.set([]);
  vi.mocked(locateSaves).mockReset();
  vi.mocked(listBackups).mockReset();
  vi.mocked(createBackup).mockReset();
  vi.mocked(deleteBackup).mockReset();
  vi.mocked(openFolder).mockReset();
  vi.mocked(selectFolder).mockReset();
  vi.mocked(cloudSyncNow).mockReset();
  document.body.replaceChildren();
});

// Lets every pending answer land: a few macrotasks, each after the microtasks before it.
async function settle() {
  for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0));
}

const cloudSection = () => screen.getByText(msg('cloudsaves.sectionTitle')).parentElement as HTMLElement;

describe('GameSaves: the cloud copy', () => {
  it('says cloud saves are off and offers their settings', async () => {
    renderDom(GameSaves, { gameId: 'g1' });

    expect(within(cloudSection()).getByText(msg('cloudsaves.disabledHint'))).toBeTruthy();
    expect(within(cloudSection()).getByRole('button', { name: msg('cloudsaves.openSettings') })).toBeTruthy();
  });

  it('asks a signed-out player to sign in', async () => {
    settings.set({ cloudSaves: true } as Settings);
    authState.set('guest');
    renderDom(GameSaves, { gameId: 'g1' });

    expect(within(cloudSection()).getByText(msg('cloudsaves.needsAccount'))).toBeTruthy();
    expect(within(cloudSection()).queryByRole('button')).toBeNull();
  });

  it('says a game the catalog does not know has no cloud copy', async () => {
    cloudOn();
    libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: '' } as LibraryGame]);
    renderDom(GameSaves, { gameId: 'g1' });

    expect(within(cloudSection()).getByText(msg('cloud.no_canonical_id'))).toBeTruthy();
    expect(within(cloudSection()).queryByRole('button')).toBeNull();
  });

  it('keeps Sync now focused and busy while it syncs, and a second press sends nothing', async () => {
    cloudOn();
    cloudStatus.set(statusWith(cloudGame({ state: 'synced' })));
    const syncing = deferred<void>();
    vi.mocked(cloudSyncNow).mockReturnValue(syncing.promise);
    const { user } = renderDom(GameSaves, { gameId: 'g1' });
    const sync = within(cloudSection()).getByRole('button', { name: msg('cloudsaves.syncNow') });
    sync.focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(sync.getAttribute('aria-busy')).toBe('true');
    expect(sync.textContent).toContain(msg('cloudsaves.syncing'));
    expect(document.activeElement).toBe(sync);
    expect(vi.mocked(cloudSyncNow)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(cloudSyncNow)).toHaveBeenCalledWith('g1');
    syncing.resolve();
    await waitFor(() => expect(sync.hasAttribute('aria-busy')).toBe(false));
  });

  it('says a failed sync by its code, and in its own words when the code has none', async () => {
    cloudOn();
    cloudStatus.set(statusWith(cloudGame({ state: 'synced' })));
    vi.mocked(cloudSyncNow).mockRejectedValueOnce(new Error('typhon:cloud.quota: over the limit'));
    vi.mocked(cloudSyncNow).mockRejectedValueOnce(new Error('typhon:cloud.something_new: какой-то текст из Go'));
    const { user } = renderDom(GameSaves, { gameId: 'g1' });
    const sync = within(cloudSection()).getByRole('button', { name: msg('cloudsaves.syncNow') });

    await user.click(sync);
    const alert = await within(cloudSection()).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('cloud.quota'));

    await user.click(sync);
    await waitFor(() => expect(within(cloudSection()).getByRole('alert').textContent?.trim()).toBe(msg('cloudsaves.fallback')));
  });

  it('shows a failed pass with its code text in a danger notice', async () => {
    cloudOn();
    cloudStatus.set(statusWith(cloudGame({ state: 'failed', code: 'cloud.network' })));
    renderDom(GameSaves, { gameId: 'g1' });

    expect(within(cloudSection()).getByText(msg('cloudsaves.stateFailed'))).toBeTruthy();
    const reason = within(cloudSection()).getByText(msg('cloud.network'));
    expect(reason.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('opens the conflict dialog for this game from a conflict', async () => {
    cloudOn();
    cloudStatus.set(statusWith(cloudGame({ state: 'conflict' })));
    const { user } = renderDom(GameSaves, { gameId: 'g1' });

    expect(within(cloudSection()).getByText(msg('cloudsaves.stateConflict'))).toBeTruthy();
    expect(within(cloudSection()).queryByRole('button', { name: msg('cloudsaves.pull') })).toBeNull();
    await user.click(within(cloudSection()).getByRole('button', { name: msg('cloudsaves.resolve') }));

    expect(get(cloudConflictTarget)).toEqual({ gameId: 'g1', title: 'Hades' });
  });
});

describe('GameSaves: the backups', () => {
  it('says there are no backups yet', async () => {
    renderDom(GameSaves, { gameId: 'g1' });

    expect(await screen.findByText(msg('saves.empty'))).toBeTruthy();
  });

  it('says a folder it could not find in its own words, not the error text', async () => {
    vi.mocked(locateSaves).mockRejectedValue(new Error('typhon:something.unknown: путь не найден'));
    renderDom(GameSaves, { gameId: 'g1' });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('saves.folderFailed'));
  });

  it('says a folder it could not open in its own words, not the error text', async () => {
    vi.mocked(openFolder).mockRejectedValue(new Error('exec: explorer.exe not found'));
    const { user } = renderDom(GameSaves, { gameId: 'g1' });

    await user.click(await screen.findByRole('button', { name: msg('saves.open') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('saves.openFailed'));
  });

  it('says a folder it could not change in its own words, not the error text', async () => {
    vi.mocked(selectFolder).mockRejectedValue(new Error('dialog: cancelled by the system'));
    const { user } = renderDom(GameSaves, { gameId: 'g1' });

    await user.click(await screen.findByRole('button', { name: msg('saves.change') }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('saves.folderNotSet'));
  });

  it('moves focus from "Choose folder" to "Change" once the chosen folder replaces it', async () => {
    vi.mocked(locateSaves).mockResolvedValueOnce(notFound).mockResolvedValue(found('D:/Saves/Hades'));
    vi.mocked(selectFolder).mockResolvedValue('D:/Saves/Hades');
    const { user } = renderDom(GameSaves, { gameId: 'g1' });
    const choose = await screen.findByRole('button', { name: msg('saves.pickFolder') });
    choose.focus();

    await user.keyboard('{Enter}');

    const change = await screen.findByRole('button', { name: msg('saves.change') });
    await waitFor(() => expect(document.activeElement).toBe(change));
  });

  it('moves focus to the next backup once a deleted one leaves the list', async () => {
    const list = [snapshot({ id: 's1' }), snapshot({ id: 's2', createdAt: '2026-10-01T10:00:00Z' })];
    vi.mocked(listBackups).mockResolvedValueOnce(list).mockResolvedValue([list[1]]);
    vi.mocked(deleteBackup).mockResolvedValue();
    const { user } = renderDom(GameSaves, { gameId: 'g1' });
    await screen.findAllByRole('button', { name: msg('saves.delete') });
    const first = screen.getAllByRole('button', { name: msg('saves.delete') })[0];
    first.focus();

    await user.keyboard('{Enter}');
    const confirm = await screen.findByRole('dialog');
    await user.click(within(confirm).getByRole('button', { name: msg('saves.delete') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    const left = screen.getAllByRole('button', { name: msg('saves.delete') });
    expect(left).toHaveLength(1);
    await waitFor(() => expect(document.activeElement).toBe(left[0]));
  });

  it('moves focus to "Back up now" once the last backup is deleted', async () => {
    vi.mocked(listBackups).mockResolvedValueOnce([snapshot()]).mockResolvedValue([]);
    vi.mocked(deleteBackup).mockResolvedValue();
    const { user } = renderDom(GameSaves, { gameId: 'g1' });
    const del = await screen.findByRole('button', { name: msg('saves.delete') });
    del.focus();

    await user.keyboard('{Enter}');
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('saves.delete') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(await screen.findByText(msg('saves.empty'))).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('saves.createBackup') })));
  });

  it('gives focus back to Delete when the deletion fails and the backup stays', async () => {
    vi.mocked(listBackups).mockResolvedValue([snapshot()]);
    vi.mocked(deleteBackup).mockRejectedValue(new Error('typhon:savebackup.busy'));
    const { user } = renderDom(GameSaves, { gameId: 'g1' });
    const del = await screen.findByRole('button', { name: msg('saves.delete') });
    del.focus();

    await user.keyboard('{Enter}');
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('saves.delete') }));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(del));
  });
});

describe('GameSaves: another game in the same view', () => {
  it('shows the new game once its id changes, without a remount', async () => {
    vi.mocked(listBackups).mockImplementation(async (id) => [snapshot({ id: `${id}-s`, gameId: id, createdAt: id === 'g1' ? '2026-11-01T10:00:00Z' : '2026-09-01T10:00:00Z' })]);
    vi.mocked(locateSaves).mockImplementation(async (id) => found(`C:/Saves/${id}`));
    const { rerender } = renderDom(GameSaves, { gameId: 'g1' });
    expect(await screen.findByText('C:/Saves/g1')).toBeTruthy();

    await rerender({ gameId: 'g2' });

    expect(await screen.findByText('C:/Saves/g2')).toBeTruthy();
    expect(screen.queryByText('C:/Saves/g1')).toBeNull();
    expect(vi.mocked(listBackups)).toHaveBeenLastCalledWith('g2');
    expect(screen.getAllByRole('listitem')).toHaveLength(1);
  });

  it('drops what an action of the previous game answers after the switch', async () => {
    const made = deferred<Snapshot>();
    vi.mocked(createBackup).mockReturnValue(made.promise);
    vi.mocked(listBackups).mockImplementation(async (id) => (id === 'g1' ? [snapshot({ id: 'old', gameId: 'g1' })] : []));
    const { user, rerender } = renderDom(GameSaves, { gameId: 'g1' });
    const create = await screen.findByRole('button', { name: msg('saves.createBackup') });
    await waitFor(() => expect(create).toHaveProperty('disabled', false));
    await user.click(create);
    expect(screen.getByRole('button', { name: msg('saves.creating') })).toBe(create);

    await rerender({ gameId: 'g2' });
    expect(await screen.findByText(msg('saves.empty'))).toBeTruthy();
    expect(create.textContent).toContain(msg('saves.createBackup'));
    expect(create.hasAttribute('aria-busy')).toBe(false);

    made.reject(new Error('typhon:savebackup.no_saves'));
    await settle();

    expect(screen.getByText(msg('saves.empty'))).toBeTruthy();
    expect(screen.queryAllByRole('listitem')).toHaveLength(0);
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
