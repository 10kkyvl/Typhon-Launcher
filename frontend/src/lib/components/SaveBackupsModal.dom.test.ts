import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { locateSaves, type LibraryGame, type SavesResult } from '../services/library';
import { openReferenceLink } from '../services/metadata';
import { createBackup, listBackups, type Snapshot } from '../services/savebackup';
import { openFolder, type Settings } from '../services/settings';
import { libraryGames, runningGames } from '../stores/library';
import { closeSaveBackups, openSaveBackups } from '../stores/savebackup';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import SaveBackupsModal from './SaveBackupsModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  locateSaves: vi.fn(),
}));
vi.mock('../services/savebackup', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/savebackup')>()),
  listBackups: vi.fn(),
  createBackup: vi.fn(),
}));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  openFolder: vi.fn(),
}));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  loadCloudStatus: vi.fn(async () => ({ enabled: false, signedIn: false, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games: [] })),
  cloudSyncNow: vi.fn(),
  cloudPullRemote: vi.fn(),
  cloudDeleteRemote: vi.fn(),
}));
vi.mock('../services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/metadata')>()),
  openReferenceLink: vi.fn(async () => {}),
}));

const found: SavesResult = {
  path: 'C:/Users/me/Saved Games/Hades',
  candidates: null,
  unreadable: 0,
  hint: { source: 'PCGamingWiki', license: 'CC BY-NC-SA 3.0', via: '', pageUrl: '' },
};

const snapshot = (patch: Partial<Snapshot> = {}): Snapshot =>
  ({ id: 's1', gameId: 'g1', kind: 'manual', createdAt: '2026-11-01T10:00:00Z', sourcePath: '', sizeBytes: 10, files: 1, digest: '', ...patch }) as Snapshot;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'igdb:1' } as LibraryGame]);
  settings.set({ cloudSaves: false } as Settings);
  vi.mocked(locateSaves).mockResolvedValue(found);
  vi.mocked(listBackups).mockResolvedValue([]);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  closeSaveBackups();
  libraryGames.set([]);
  runningGames.set(new Set());
  settings.set(null);
  toasts.set([]);
  vi.mocked(locateSaves).mockReset();
  vi.mocked(listBackups).mockReset();
  vi.mocked(createBackup).mockReset();
  vi.mocked(openFolder).mockReset();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

async function openDialog() {
  render(SaveBackupsModal, { target: app });
  openSaveBackups('g1');
  return screen.findByRole('dialog', { name: msg('saves.title', { title: 'Hades' }) });
}

describe('SaveBackupsModal: the saves folder', () => {
  it('names where the folder came from with a text link that opens the source page', async () => {
    const dialog = await openDialog();
    const keys = user();
    const link = await within(dialog).findByRole('button', { name: 'PCGamingWiki' });
    expect(link.classList.contains('text-link')).toBe(true);
    link.focus();

    await keys.keyboard('{Enter}');

    expect(vi.mocked(openReferenceLink)).toHaveBeenCalledWith('igdb:1', 'pcgamingwiki');
  });

  it('says it is looking for the folder with a labelled spinner', async () => {
    const located = deferred<SavesResult>();
    vi.mocked(locateSaves).mockReturnValueOnce(located.promise);
    const dialog = await openDialog();

    const statuses = within(dialog).getAllByRole('status').map((status) => status.textContent?.trim());
    expect(statuses).toContain(msg('saves.folderLoading'));
    located.resolve(found);
  });

  it('keeps the focus on "open", busy, while the folder opens, and says a failure in a danger notice', async () => {
    const opening = deferred<void>();
    vi.mocked(openFolder).mockReturnValueOnce(opening.promise);
    const dialog = await openDialog();
    const keys = user();
    const open = await within(dialog).findByRole('button', { name: msg('saves.open') });
    open.focus();

    await keys.keyboard('{Enter}');

    expect(open.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(open);
    expect(within(dialog).getByRole('button', { name: msg('saves.change') })).toHaveProperty('disabled', true);

    opening.reject(new Error('gone'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

describe('SaveBackupsModal: the backups', () => {
  it('keeps the focus on "create", busy, while the backup is made', async () => {
    const made = deferred<Snapshot>();
    vi.mocked(createBackup).mockReturnValueOnce(made.promise);
    const dialog = await openDialog();
    const keys = user();
    const create = await within(dialog).findByRole('button', { name: msg('saves.createBackup') });
    await vi.waitFor(() => expect(create).toHaveProperty('disabled', false));
    create.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('saves.creating') });
    expect(busy).toBe(create);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);
    made.resolve(snapshot());
  });

  it('states a running game and large backups in warning notices read with the dialog', async () => {
    runningGames.set(new Set(['g1']));
    vi.mocked(listBackups).mockResolvedValue([snapshot({ large: true })]);
    const dialog = await openDialog();

    for (const text of [msg('saves.gameRunning'), msg('saves.largeWarning')]) {
      const notice = (await within(dialog).findByText(text)).closest('[data-tone="warning"]') as HTMLElement;
      expect(notice).not.toBeNull();
      expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
    }
  });

  it('says a failed list in a danger notice', async () => {
    vi.mocked(listBackups).mockRejectedValue(new Error('disk'));
    const dialog = await openDialog();

    const text = await within(dialog).findByText(msg('saves.listFailed'));
    expect(text.closest('[data-tone="danger"]')).not.toBeNull();
  });
});

const footerClose = (dialog: HTMLElement) => within(dialog).getAllByRole('button', { name: msg('common.close') }).find((button) => !button.closest('.head')) as HTMLElement;

describe('SaveBackupsModal: the dialog around the saves', () => {
  it('opens with the focus inside the saves, not on the close button', async () => {
    const dialog = await openDialog();

    await within(dialog).findByRole('button', { name: msg('saves.open') });
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(document.activeElement?.closest('.head')).toBeNull();
  });

  it('says there is no backup yet in words of its own', async () => {
    const dialog = await openDialog();

    expect(await within(dialog).findByText(msg('saves.empty'))).toBeTruthy();
  });

  it('holds Close with the reason while the folder opens, and Escape waits too', async () => {
    const opening = held<void>();
    vi.mocked(openFolder).mockReturnValueOnce(opening.promise);
    const dialog = await openDialog();
    const keys = user();
    await keys.click(await within(dialog).findByRole('button', { name: msg('saves.open') }));
    const close = footerClose(dialog);

    expect(close).toHaveProperty('disabled', false);
    expect(close.getAttribute('aria-disabled')).toBe('true');
    expect(document.getElementById(close.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(msg('modals.actionRunning'));
    await keys.click(close);
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);

    opening.resolve();
    await vi.waitFor(() => expect(close.getAttribute('aria-disabled')).toBeNull());
    await keys.click(close);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('closes on Escape as on Close while a backup is made: the backup goes on without the dialog', async () => {
    const made = held<Snapshot>();
    vi.mocked(createBackup).mockReturnValueOnce(made.promise);
    const dialog = await openDialog();
    const keys = user();
    const create = await within(dialog).findByRole('button', { name: msg('saves.createBackup') });
    await vi.waitFor(() => expect(create).toHaveProperty('disabled', false));
    await keys.click(create);
    expect(footerClose(dialog).getAttribute('aria-disabled')).toBeNull();

    await keys.keyboard('{Escape}');

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    made.resolve(snapshot());
  });

  it('shows the backups of the game it is open for, not a late answer for the game before', async () => {
    const first = held<Snapshot[]>();
    libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'igdb:1' } as LibraryGame, { id: 'g2', title: 'Celeste', canonicalGameId: 'igdb:2' } as LibraryGame]);
    vi.mocked(listBackups).mockImplementation(async (id: string) => (id === 'g1' ? first.promise : []));
    const hades = await openDialog();
    closeSaveBackups();
    await vi.waitFor(() => expect(hades.isConnected).toBe(false));

    openSaveBackups('g2');
    const celeste = await screen.findByRole('dialog', { name: msg('saves.title', { title: 'Celeste' }) });
    await within(celeste).findByText(msg('saves.empty'));
    first.resolve([snapshot({ id: 's1', gameId: 'g1' })]);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(celeste).getByText(msg('saves.empty'))).toBeTruthy();
  });
});
