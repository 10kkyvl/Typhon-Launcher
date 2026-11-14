import { render, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { cloudSyncNow, type CloudStatus } from '../services/cloudsaves';
import { locateSaves, type LibraryGame, type SavesResult } from '../services/library';
import { createBackup, listBackups, restoreBackup, type Snapshot } from '../services/savebackup';
import type { Settings } from '../services/settings';
import { cloudStatus } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { closeSaveBackups, openSaveBackups, saveBackupsEnabled } from '../stores/savebackup';
import { settings } from '../stores/settings';
import { queuedToasts, toasts } from '../stores/toasts';
import { authState } from '../stores/user';
import { user } from '../testing/dom';
import GameSaves from './GameSaves.svelte';
import SaveBackupsModal from './SaveBackupsModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  locateSaves: vi.fn(),
}));
vi.mock('../services/savebackup', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/savebackup')>()),
  listBackups: vi.fn(),
  createBackup: vi.fn(),
  restoreBackup: vi.fn(),
}));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudSyncNow: vi.fn(),
}));

const found = { path: 'C:/Saves/Hades', candidates: null, unreadable: 0, hint: null } as unknown as SavesResult;
const snapshot = (id: string): Snapshot =>
  ({ id, gameId: 'g1', kind: 'manual', createdAt: '2026-11-01T10:00:00Z', sourcePath: '', sizeBytes: 10, files: 1, digest: '' }) as Snapshot;

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'igdb:1' } as LibraryGame]);
  settings.set({ cloudSaves: true } as Settings);
  authState.set('authenticated');
  cloudStatus.set({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games: [{ gameId: 'g1', state: 'synced', code: '', message: '', at: '', syncedAt: '' }] } as CloudStatus);
  saveBackupsEnabled.set(true);
  vi.mocked(locateSaves).mockResolvedValue(found);
  vi.mocked(listBackups).mockResolvedValue([]);
});

afterEach(() => {
  closeSaveBackups();
  libraryGames.set([]);
  settings.set(null);
  authState.set('bootstrapping');
  cloudStatus.set(null);
  saveBackupsEnabled.set(false);
  toasts.set([]);
  vi.mocked(locateSaves).mockReset();
  vi.mocked(listBackups).mockReset();
  vi.mocked(createBackup).mockReset();
  vi.mocked(restoreBackup).mockReset();
  vi.mocked(cloudSyncNow).mockReset();
  document.body.replaceChildren();
});

const createButton = () => screen.getByRole('button', { name: new RegExp(`${msg('saves.createBackup')}|${msg('saves.creating')}`) });

describe('a saves action outlives the view that started it', () => {
  it('through the dialog: "Back up now" stays busy when the dialog is closed and opened again, a second press sends nothing, and its answer refreshes the list', async () => {
    const made = deferred<Snapshot>();
    vi.mocked(createBackup).mockReturnValue(made.promise);
    render(SaveBackupsModal, { target: app });
    const keys = user();
    openSaveBackups('g1');
    const dialog = await screen.findByRole('dialog');
    const create = await within(dialog).findByRole('button', { name: msg('saves.createBackup') });
    await waitFor(() => expect(create).toHaveProperty('disabled', false));
    await keys.click(create);

    // The footer's Close: the dialog cannot be dismissed while busy, but it can be closed.
    await keys.click(within(dialog).getAllByRole('button', { name: msg('common.close') }).at(-1) as HTMLElement);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    openSaveBackups('g1');
    await screen.findByRole('dialog');
    await screen.findByText(msg('saves.empty'));

    const again = createButton();
    expect(again.getAttribute('aria-busy')).toBe('true');
    expect(again.textContent).toContain(msg('saves.creating'));
    await keys.click(again);
    expect(vi.mocked(createBackup)).toHaveBeenCalledTimes(1);

    vi.mocked(listBackups).mockResolvedValue([snapshot('new')]);
    made.resolve(snapshot('new'));

    expect(await screen.findAllByRole('button', { name: msg('saves.delete') })).toHaveLength(1);
    await waitFor(() => expect(createButton().hasAttribute('aria-busy')).toBe(false));
  });

  it('through a remount of the view: a restore and a cloud sync stay busy, a second press sends nothing', async () => {
    vi.mocked(listBackups).mockResolvedValue([snapshot('s1'), snapshot('s2')]);
    const restoring = deferred<void>();
    vi.mocked(restoreBackup).mockReturnValue(restoring.promise);
    const syncing = deferred<void>();
    vi.mocked(cloudSyncNow).mockReturnValue(syncing.promise);
    const keys = user();
    const first = render(GameSaves, { target: app, props: { gameId: 'g1' } });
    await keys.click(screen.getByRole('button', { name: msg('cloudsaves.syncNow') }));
    const restore = (await screen.findAllByRole('button', { name: msg('saves.restore') }))[0];
    await keys.click(restore);
    await keys.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('saves.restore') }));

    first.unmount();
    render(GameSaves, { target: app, props: { gameId: 'g1' } });
    const restores = await screen.findAllByRole('button', { name: msg('saves.restore') });

    expect(restores[0].getAttribute('aria-busy')).toBe('true');
    expect(restores[1]).toHaveProperty('disabled', true);
    expect(createButton()).toHaveProperty('disabled', true);
    const sync = screen.getByRole('button', { name: msg('cloudsaves.syncing') });
    expect(sync.getAttribute('aria-busy')).toBe('true');
    await keys.click(sync);
    expect(vi.mocked(cloudSyncNow)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(restoreBackup)).toHaveBeenCalledTimes(1);

    restoring.resolve();
    syncing.resolve();
    await waitFor(() => expect(screen.getAllByRole('button', { name: msg('saves.restore') })[0].hasAttribute('aria-busy')).toBe(false));
    expect(screen.getByRole('button', { name: msg('cloudsaves.syncNow') }).hasAttribute('aria-busy')).toBe(false);
  });

  it('says in a toast, with the game, a backup action that fails after the view that started it went', async () => {
    let fail!: (err: unknown) => void;
    vi.mocked(createBackup).mockReturnValue(new Promise((_, reject) => (fail = reject)));
    const keys = user();
    const first = render(GameSaves, { target: app, props: { gameId: 'g1' } });
    const create = await screen.findByRole('button', { name: msg('saves.createBackup') });
    await waitFor(() => expect(create).toHaveProperty('disabled', false));
    await keys.click(create);

    first.unmount();
    render(GameSaves, { target: app, props: { gameId: 'g1' } });
    fail(new Error('typhon:savebackup.disabled'));

    const said = () => [...get(toasts), ...queuedToasts()].map((t) => [t.kind, t.message]);
    await waitFor(() => expect(said()).toContainEqual(['danger', msg('saves.failedFor', { title: 'Hades', reason: msg('savebackup.disabled') })]));
    await waitFor(() => expect(createButton().hasAttribute('aria-busy')).toBe(false));
  });

  it('says in a toast, with the game, a cloud action that fails after the view moved to another game', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'igdb:1' } as LibraryGame, { id: 'g2', title: 'Celeste', canonicalGameId: 'igdb:2' } as LibraryGame]);
    let fail!: (err: unknown) => void;
    vi.mocked(cloudSyncNow).mockReturnValue(new Promise((_, reject) => (fail = reject)));
    const keys = user();
    const view = render(GameSaves, { target: app, props: { gameId: 'g1' } });
    await keys.click(screen.getByRole('button', { name: msg('cloudsaves.syncNow') }));

    await view.rerender({ gameId: 'g2' });
    fail(new Error('typhon:cloud.quota'));

    const said = () => [...get(toasts), ...queuedToasts()].map((t) => [t.kind, t.message]);
    await waitFor(() => expect(said()).toContainEqual(['danger', msg('cloudsaves.failedFor', { title: 'Hades', reason: msg('cloud.quota') })]));
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
