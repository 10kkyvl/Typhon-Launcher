import { screen, waitFor, within } from '@testing-library/svelte';
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
import LibraryFoldersRow from './LibraryFoldersRow.svelte';
import LibraryLocationRow from './LibraryLocationRow.svelte';

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
  document.body.replaceChildren();
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

describe('the library move row', () => {
  it('is off without a library', () => {
    settings.set({ libraryPath: '' } as unknown as SettingsData);
    renderDom(LibraryLocationRow, {});
    expect((screen.getByRole('button', { name: msg('settings.generalLibraryMoveButtonLabel') }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(msg('settings.generalLibraryMoveEmptySub'))).toBeTruthy();
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
    const bar = screen.getByRole('progressbar', { name: msg('settings.generalLibraryMoveLabel') });
    const text = bar.parentElement?.textContent ?? '';
    for (const part of [stageLabel(JOB.stage), JOB.title, moveSummary(JOB), msg('settings.generalLibraryMoveCancelHint')]) expect(text).toContain(part);
    expect((screen.getByRole('button', { name: msg('settings.generalLibraryMoveButtonLabel') }) as HTMLButtonElement).disabled).toBe(true);
    await user.click(screen.getByRole('button', { name: msg('settings.generalLibraryMoveCancelButton') }));
    await waitFor(() => expect(cancelMove).toHaveBeenCalledExactlyOnceWith('m1'));
  });

  it('says a cancel that failed on a toast', async () => {
    moves.set([JOB]);
    vi.mocked(cancelMove).mockRejectedValueOnce(new Error('late'));
    const { user } = renderDom(LibraryLocationRow, {});
    await user.click(screen.getByRole('button', { name: msg('settings.generalLibraryMoveCancelButton') }));
    await waitFor(() => expect(toastTexts()).toEqual([msg('settings.generalLibraryMoveCancelFailed')]));
  });
});
