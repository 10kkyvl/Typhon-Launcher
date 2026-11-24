import { cleanup, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
// The library rows act only in the desktop app.
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  libraryFolders: vi.fn(),
  addLibraryFolder: vi.fn(),
  removeLibraryFolder: vi.fn(),
  selectFolder: vi.fn(),
}));
vi.mock('../../lib/services/relocate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/relocate')>()),
  selectMoveTargetFolder: vi.fn(),
  moveLibrary: vi.fn(),
  cancelMove: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { moveSummary, stageLabel } from '../../lib/relocate/moveText';
import { cancelMove, moveLibrary, selectMoveTargetFolder, type MoveJob } from '../../lib/services/relocate';
import {
  addLibraryFolder,
  libraryFolders,
  removeLibraryFolder,
  selectFolder,
  type LibraryFolder,
  type Settings as SettingsData,
} from '../../lib/services/settings';
import { moves } from '../../lib/stores/relocate';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import LibraryCard from './LibraryCard.svelte';
import LibraryFoldersRow from './LibraryFoldersRow.svelte';
import LibraryLocationRow from './LibraryLocationRow.svelte';
import type { SettingsPage } from './settingsPage.svelte';

const GB = 1024 ** 3;
const PRIMARY: LibraryFolder = { path: '/games', primary: true, freeBytes: 10 * GB, totalBytes: 100 * GB, available: true };
const EXTRA: LibraryFolder = { path: '/extra', primary: false, freeBytes: 5 * GB, totalBytes: 50 * GB, available: true };
const THIRD: LibraryFolder = { path: '/third', primary: false, freeBytes: 1 * GB, totalBytes: 10 * GB, available: false };

const SETTINGS = { libraryPath: '/lib', gamesPath: '/games', extraLibraryPaths: ['/extra', '/third'] } as unknown as SettingsData;

const JOB: MoveJob = {
  id: 'm1',
  scope: 'library',
  stage: 'copy',
  title: 'Some Game',
  source: '/lib',
  target: '/new/lib',
  staging: '',
  renamed: false,
  totalBytes: 4 * GB,
  copiedBytes: 1 * GB,
  phase: '',
  currentFile: '',
  queue: ['a', 'b'],
  startedAt: '',
  updatedAt: '',
};

const toastTexts = () => get(toasts).map((item) => item.message);

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  const promise = new Promise<T>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  settings.set(SETTINGS);
  vi.mocked(libraryFolders).mockResolvedValue([PRIMARY, EXTRA, THIRD]);
});

afterEach(() => {
  // Unmounted first: a mounted folder list would ask for the folders again when the settings reset.
  cleanup();
  settings.set(null);
  moves.set([]);
  toasts.set([]);
  for (const fn of [libraryFolders, addLibraryFolder, removeLibraryFolder, selectFolder, selectMoveTargetFolder, moveLibrary, cancelMove]) {
    vi.mocked(fn).mockReset();
  }
  vi.restoreAllMocks();
});

describe('the library folders row', () => {
  it('lists each folder with its space, and a Remove on every folder but the primary', async () => {
    renderDom(LibraryFoldersRow, {});
    await screen.findByText('/extra');
    const list = screen.getByRole('list', { name: msg('settings.libraryFoldersLabel') });
    expect(within(list).getAllByRole('listitem')).toHaveLength(3);
    // The whole path is on screen: no native title repeats it.
    expect(screen.getByText('/extra').hasAttribute('title')).toBe(false);
    expect(description(screen.getByRole('button', { name: msg('settings.libraryFolderAdd') }))).toBe(msg('settings.libraryFoldersSub'));
    expect(screen.getByText('/games')).toBeTruthy();
    expect(screen.getByText(msg('settings.libraryFolderPrimary'))).toBeTruthy();
    expect(screen.getByText(msg('settings.libraryFolderUnavailable'))).toBeTruthy();
    expect(screen.getAllByRole('button', { name: msg('settings.libraryFolderRemove') })).toHaveLength(2);
  });

  it('adds a folder picked in the system dialog', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('/new');
    vi.mocked(addLibraryFolder).mockResolvedValueOnce({ ...SETTINGS, extraLibraryPaths: ['/extra', '/third', '/new'] } as SettingsData);
    const { user } = renderDom(LibraryFoldersRow, {});
    await user.click(screen.getByRole('button', { name: msg('settings.libraryFolderAdd') }));
    await waitFor(() => expect(addLibraryFolder).toHaveBeenCalledExactlyOnceWith('/new'));
    expect(get(settings)?.extraLibraryPaths).toEqual(['/extra', '/third', '/new']);
  });

  it('says a folder that could not be added on a toast', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('/new');
    vi.mocked(addLibraryFolder).mockRejectedValueOnce(new Error('nope'));
    const { user } = renderDom(LibraryFoldersRow, {});
    await user.click(screen.getByRole('button', { name: msg('settings.libraryFolderAdd') }));
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.libraryFolderAddFailed')]));
  });

  it('asks before it removes a folder, then removes it', async () => {
    vi.mocked(removeLibraryFolder).mockResolvedValueOnce({ ...SETTINGS, extraLibraryPaths: ['/third'] } as SettingsData);
    const { user } = renderDom(LibraryFoldersRow, {});
    await screen.findByText('/extra');
    await user.click(screen.getAllByRole('button', { name: msg('settings.libraryFolderRemove') })[0]);
    const dialog = await screen.findByRole('dialog', { name: msg('settings.libraryFolderRemoveTitle') });
    expect(removeLibraryFolder).not.toHaveBeenCalled();
    vi.mocked(libraryFolders).mockResolvedValue([PRIMARY, THIRD]);
    await user.click(within(dialog).getByRole('button', { name: msg('settings.libraryFolderRemove') }));
    await waitFor(() => expect(removeLibraryFolder).toHaveBeenCalledExactlyOnceWith('/extra'));
    await waitFor(() => expect(screen.queryByText('/extra')).toBeNull());
  });
});

describe('the library card: the import row', () => {
  // The button says the row's own label: beside the words it squeezed the hint into a narrow column.
  it('puts the import button under its words, the disk usage button kept beside them', async () => {
    const page = {
      appInfo: { version: '2.0.0', platform: 'windows', arch: 'amd64', devMock: false, portable: false, dataDir: '' },
      importDetails: [],
      importing: false,
      runImportExternal: vi.fn(),
      openLibrarySetup: vi.fn(),
    } as unknown as SettingsPage;
    renderDom(LibraryCard, { page });
    await waitFor(() => expect(libraryFolders).toHaveBeenCalled());
    const rowOf = (name: string) => screen.getByRole('button', { name }).closest('.settings-row');
    expect(rowOf(msg('settings.extlibLabel'))?.classList.contains('stack')).toBe(true);
    expect(rowOf(msg('diskusage.settingsOpen'))?.classList.contains('stack')).toBe(false);
  });
});

describe('the library move row', () => {
  it('is off without a library', () => {
    settings.set({ libraryPath: '' } as unknown as SettingsData);
    renderDom(LibraryLocationRow, {});
    const move = screen.getByRole('button', { name: msg('settings.generalLibraryMoveButtonLabel') }) as HTMLButtonElement;
    expect(move.disabled).toBe(true);
    expect(description(move)).toBe(msg('settings.generalLibraryMoveEmptySub'));
  });

  it('marks Move with an icon a screen reader does not read', () => {
    settings.set({ libraryPath: '/lib' } as unknown as SettingsData);
    renderDom(LibraryLocationRow, {});
    const move = screen.getByRole('button', { name: msg('settings.generalLibraryMoveButtonLabel') });
    expect([...move.querySelectorAll('svg')].map((icon) => icon.getAttribute('aria-hidden'))).toEqual(['true']);
  });

  it('moves the library to the folder picked', async () => {
    vi.mocked(selectMoveTargetFolder).mockResolvedValueOnce('/new');
    const started = deferred<MoveJob>();
    vi.mocked(moveLibrary).mockReturnValueOnce(started.promise);
    const { user } = renderDom(LibraryLocationRow, {});
    expect(screen.getByText(msg('settings.generalLibraryMoveCurrentSub', { path: '/lib' }))).toBeTruthy();
    await user.click(screen.getByRole('button', { name: msg('settings.generalLibraryMoveButtonLabel') }));
    await waitFor(() => expect(moveLibrary).toHaveBeenCalledExactlyOnceWith('/new'));
    started.resolve(JOB);
  });

  it('shows a running move with its progress, and cancels it', async () => {
    moves.set([JOB]);
    vi.mocked(cancelMove).mockResolvedValueOnce();
    const { user } = renderDom(LibraryLocationRow, {});
    screen.getByRole('progressbar', { name: msg('settings.generalLibraryMoveLabel') });
    // The line is one catalogued message, not pieces joined in the markup.
    const line = msg('settings.generalLibraryMoveProgressQueue', { stage: stageLabel(JOB.stage), title: JOB.title, count: 2, summary: moveSummary(JOB) });
    expect(screen.getByText(line)).toBeTruthy();
    expect(screen.getByText(msg('settings.generalLibraryMoveCancelHint'))).toBeTruthy();
    await user.click(screen.getByRole('button', { name: msg('settings.generalLibraryMoveCancelButton') }));
    await waitFor(() => expect(cancelMove).toHaveBeenCalledExactlyOnceWith('m1'));
  });

  it('words the move line without a title or a queue', () => {
    moves.set([{ ...JOB, title: '', queue: null }]);
    renderDom(LibraryLocationRow, {});
    expect(screen.getByText(msg('settings.generalLibraryMoveProgressNoTitle', { stage: stageLabel(JOB.stage), summary: moveSummary(JOB) }))).toBeTruthy();
  });

  it('says a cancel that failed on a toast', async () => {
    moves.set([JOB]);
    vi.mocked(cancelMove).mockRejectedValueOnce(new Error('late'));
    const { user } = renderDom(LibraryLocationRow, {});
    await user.click(screen.getByRole('button', { name: msg('settings.generalLibraryMoveCancelButton') }));
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.generalLibraryMoveCancelFailed')]));
  });
});

// A button its own press puts to work keeps focus; a removed folder's dialog hands focus on.
describe('the library rows: focus', () => {
  it('keeps Add folder in focus and busy while it adds', async () => {
    vi.mocked(selectFolder).mockResolvedValueOnce('/new');
    const added = deferred<SettingsData>();
    vi.mocked(addLibraryFolder).mockReturnValueOnce(added.promise);
    const { user } = renderDom(LibraryFoldersRow, {});
    const add = screen.getByRole('button', { name: msg('settings.libraryFolderAdd') }) as HTMLButtonElement;
    add.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(addLibraryFolder).toHaveBeenCalledOnce());
    expect([add.disabled, add.getAttribute('aria-busy'), document.activeElement === add]).toEqual([false, 'true', true]);
    added.resolve(SETTINGS);
    await waitFor(() => expect(add.hasAttribute('aria-busy')).toBe(false));
  });

  it('keeps Move in focus and busy while the move starts and runs', async () => {
    vi.mocked(selectMoveTargetFolder).mockResolvedValueOnce('/new');
    const started = deferred<MoveJob>();
    vi.mocked(moveLibrary).mockReturnValueOnce(started.promise);
    const { user } = renderDom(LibraryLocationRow, {});
    const move = screen.getByRole('button', { name: msg('settings.generalLibraryMoveButtonLabel') }) as HTMLButtonElement;
    move.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(moveLibrary).toHaveBeenCalledOnce());
    expect([move.disabled, move.getAttribute('aria-busy'), document.activeElement === move]).toEqual([false, 'true', true]);
    moves.set([JOB]);
    started.resolve(JOB);
    await screen.findByRole('progressbar');
    expect(move.isConnected).toBe(true);
    expect([move.disabled, move.getAttribute('aria-busy'), document.activeElement === move]).toEqual([false, 'true', true]);
  });

  async function removeThrough(path: string, left: LibraryFolder[]) {
    vi.mocked(removeLibraryFolder).mockResolvedValueOnce({ ...SETTINGS, extraLibraryPaths: left.filter((f) => !f.primary).map((f) => f.path) } as SettingsData);
    const { user } = renderDom(LibraryFoldersRow, {});
    await screen.findByText(path);
    const item = screen.getByText(path).closest('li') as HTMLElement;
    const remove = within(item).getByRole('button', { name: msg('settings.libraryFolderRemove') });
    remove.focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog', { name: msg('settings.libraryFolderRemoveTitle') });
    vi.mocked(libraryFolders).mockResolvedValue(left);
    await user.click(within(dialog).getByRole('button', { name: msg('settings.libraryFolderRemove') }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(screen.queryByText(path)).toBeNull());
  }

  it("hands focus to the next folder's Remove when a folder is removed", async () => {
    await removeThrough('/extra', [PRIMARY, THIRD]);
    const next = within(screen.getByText('/third').closest('li') as HTMLElement).getByRole('button', { name: msg('settings.libraryFolderRemove') });
    await waitFor(() => expect(document.activeElement).toBe(next));
  });

  it('hands focus to Add folder when the last folder is removed', async () => {
    await removeThrough('/third', [PRIMARY, EXTRA]);
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('settings.libraryFolderAdd') })));
  });

  it('returns focus to Remove when the removal is cancelled', async () => {
    const { user } = renderDom(LibraryFoldersRow, {});
    await screen.findByText('/extra');
    const remove = within(screen.getByText('/extra').closest('li') as HTMLElement).getByRole('button', { name: msg('settings.libraryFolderRemove') });
    remove.focus();
    await user.keyboard('{Enter}');
    await screen.findByRole('dialog');
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(remove));
    expect(removeLibraryFolder).not.toHaveBeenCalled();
  });
});
