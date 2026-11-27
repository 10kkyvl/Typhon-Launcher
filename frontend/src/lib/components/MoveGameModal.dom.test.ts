import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { cancelMove, moveGame, moveGameTarget, selectMoveTargetFolder, type MoveJob } from '../services/relocate';
import { getStorageInfoFor, type StorageInfo } from '../services/system';
import { libraryGames } from '../stores/library';
import { closeMoveGame, moves, openMoveGame } from '../stores/relocate';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { bytesSize } from '../utils/format';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import MoveGameModal from './MoveGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/relocate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/relocate')>()),
  selectMoveTargetFolder: vi.fn(),
  moveGameTarget: vi.fn(),
  moveGame: vi.fn(),
  cancelMove: vi.fn(),
}));
vi.mock('../services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/system')>()),
  getStorageInfoFor: vi.fn(),
}));

const coded = (code: string) => new Error(`typhon:${code}`);
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

function job(patch: Partial<MoveJob> = {}): MoveJob {
  return {
    id: 'm1',
    scope: 'game',
    stage: 'copy',
    gameId: 'g1',
    title: 'Hades',
    source: 'C:/Games/Hades',
    target: 'D:/Games/Hades',
    staging: '',
    renamed: false,
    totalBytes: 100,
    copiedBytes: 10,
    phase: 'copying',
    currentFile: '',
    startedAt: '',
    updatedAt: '',
    ...patch,
  };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([
    { id: 'g1', title: 'Hades', installDir: 'C:/Games/Hades', sizeBytes: 0 } as LibraryGame,
    { id: 'g2', title: 'Celeste', installDir: 'C:/Games/Celeste', sizeBytes: 0 } as LibraryGame,
  ]);
  vi.mocked(selectMoveTargetFolder).mockResolvedValue('D:/Games');
  vi.mocked(moveGameTarget).mockImplementation(async (id: string) => (id === 'g1' ? 'D:/Games/Hades' : 'D:/Games/Celeste'));
  vi.mocked(getStorageInfoFor).mockResolvedValue({ volume: 'D', filesystem: 'NTFS', freeBytes: 10, totalBytes: 20 } as StorageInfo);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  closeMoveGame();
  moves.set([]);
  libraryGames.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

async function openDialog(gameId = 'g1') {
  render(MoveGameModal, { target: app });
  openMoveGame(gameId);
  return screen.findByRole('dialog', { name: msg('modals.moveGameTitle') });
}

const browseButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.moveGameBrowse') });
const startButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.moveGameStart') });

async function pickTarget(dialog: HTMLElement) {
  await user().click(browseButton(dialog));
  await vi.waitFor(() => expect(startButton(dialog).getAttribute('aria-disabled')).toBeNull());
  expect(startButton(dialog)).toHaveProperty('disabled', false);
}

describe('MoveGameModal: before the move', () => {
  it('states that the move is safe in an info notice read with the dialog', async () => {
    const dialog = await openDialog();
    const notice = within(dialog).getByText(msg('modals.moveGameSafetyText')).closest('[data-tone="info"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('keeps the focus on "browse", busy, while a folder is picked', async () => {
    const chosen = held<string>();
    vi.mocked(selectMoveTargetFolder).mockReturnValueOnce(chosen.promise);
    const dialog = await openDialog();
    const browse = browseButton(dialog);
    browse.focus();

    await user().keyboard('{Enter}');

    expect(browse.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(browse);
    chosen.resolve('');
    await vi.waitFor(() => expect(browse.getAttribute('aria-busy')).toBeNull());
  });

  it('shows the chosen folder as text, not a box, with the drive’s free space read on Browse', async () => {
    const dialog = await openDialog();
    expect(within(dialog).queryByRole('textbox')).toBeNull();
    const group = within(dialog).getByRole('group', { name: msg('modals.moveGameNewFolder') });
    expect(within(group).getByText(msg('modals.moveGameFolderPlaceholder'))).toBeTruthy();

    await pickTarget(dialog);

    const words = describedBy(browseButton(dialog)).join(' ');
    expect(words).toContain('D:/Games');
    expect(words).toContain(msg('modals.moveGameDiskFree', { free: bytesSize(10), total: bytesSize(20) }));
    expect(within(dialog).getByText(msg('modals.moveGameWillMoveTo', { path: 'D:/Games/Hades' }))).toBeTruthy();
  });

  it('puts no native title on the path it will move to', async () => {
    const dialog = await openDialog();
    await pickTarget(dialog);
    expect(dialog.querySelector('[title]')).toBeNull();
  });

  it('says while it checks the folder, and holds Move with that reason', async () => {
    const target = held<string>();
    vi.mocked(moveGameTarget).mockReturnValueOnce(target.promise);
    const dialog = await openDialog();
    await user().click(browseButton(dialog));

    await vi.waitFor(() => expect(describedBy(browseButton(dialog)).join(' ')).toContain(msg('modals.moveGameChecking')));
    const start = startButton(dialog);
    expect(start.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(start)).toContain(msg('modals.moveGameChecking'));
    await user().click(start);
    expect(vi.mocked(moveGame)).not.toHaveBeenCalled();

    target.resolve('D:/Games/Hades');
    await vi.waitFor(() => expect(start.getAttribute('aria-disabled')).toBeNull());
  });

  it('holds Move with a reason until a folder is chosen', async () => {
    const dialog = await openDialog();
    const start = startButton(dialog);
    expect(start.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(start)).toContain(msg('transfers.moveErrNoTarget'));
  });

  it.each([
    ['relocate.same_location', 'transfers.moveErrSameLocation'],
    ['relocate.target_not_empty', 'transfers.moveErrTargetNotEmpty'],
    ['relocate.target_inside_source', 'transfers.moveErrTargetInsideSource'],
    ['relocate.target_is_drive_root', 'transfers.moveErrTargetIsDriveRoot'],
  ] as const)('says a refused folder (%s) at the field and holds Move with it', async (code, key) => {
    vi.mocked(moveGameTarget).mockRejectedValueOnce(coded(code));
    const dialog = await openDialog();
    await user().click(browseButton(dialog));

    const group = within(dialog).getByRole('group', { name: msg('modals.moveGameNewFolder') });
    const alert = await within(group).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg(key));
    expect(describedBy(browseButton(dialog))).toContain(msg(key));
    const start = startButton(dialog);
    expect(start.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(start)).toContain(msg(key));
  });

  it('says an unknown free space as a fact of the folder, not as a failure', async () => {
    vi.mocked(getStorageInfoFor).mockRejectedValueOnce(new Error('stat D: failed'));
    const dialog = await openDialog();
    await pickTarget(dialog);
    expect(describedBy(browseButton(dialog)).join(' ')).toContain(msg('modals.moveGameFreeSpaceUnknown'));
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });
});

describe('MoveGameModal: starting the move', () => {
  it('sends one MoveGame with the game and the chosen folder for any number of presses', async () => {
    const started = held<MoveJob>();
    vi.mocked(moveGame).mockReturnValueOnce(started.promise);
    const dialog = await openDialog();
    await pickTarget(dialog);
    const start = startButton(dialog);
    start.focus();
    const keys = user();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    await keys.click(start);

    expect(vi.mocked(moveGame).mock.calls).toEqual([['g1', 'D:/Games']]);
    started.resolve(job());
    await within(dialog).findByRole('progressbar');
  });

  it('keeps the focus on "start", busy, and holds Browse and Cancel with a reason while the move starts', async () => {
    const started = held<MoveJob>();
    vi.mocked(moveGame).mockReturnValueOnce(started.promise);
    const dialog = await openDialog();
    await pickTarget(dialog);
    const start = startButton(dialog);
    start.focus();
    const keys = user();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('modals.moveGameStarting') });
    expect(busy).toBe(start);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(cancel)).toContain(msg('modals.actionRunning'));
    expect(browseButton(dialog).getAttribute('aria-disabled')).toBe('true');
    await keys.keyboard('{Escape}');
    await keys.click(cancel);
    expect(screen.getByRole('dialog')).toBe(dialog);

    started.reject(new Error('locked'));
    await vi.waitFor(() => expect(cancel.getAttribute('aria-disabled')).toBeNull());
  });

  it('says a refused start by its code at the field, and focus goes there', async () => {
    const started = held<MoveJob>();
    vi.mocked(moveGame).mockReturnValueOnce(started.promise);
    const dialog = await openDialog();
    await pickTarget(dialog);
    startButton(dialog).focus();
    await user().keyboard('{Enter}');

    started.reject(coded('relocate.not_enough_space'));

    const group = within(dialog).getByRole('group', { name: msg('modals.moveGameNewFolder') });
    const alert = await within(group).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('transfers.moveErrNotEnoughSpace'));
    await vi.waitFor(() => expect(document.activeElement).toBe(browseButton(dialog)));
    expect(get(toasts)).toEqual([]);
  });

  it('says an uncoded refusal in its own words, never the error text', async () => {
    vi.mocked(moveGame).mockRejectedValueOnce(new Error('rename C:\\Games: access is denied'));
    const dialog = await openDialog();
    await pickTarget(dialog);
    await user().click(startButton(dialog));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.moveGameStartFailed'));
  });

  it('hands the focus to "continue in background" once the move runs, never to its cancel', async () => {
    vi.mocked(moveGame).mockResolvedValueOnce(job());
    const dialog = await openDialog();
    await pickTarget(dialog);
    startButton(dialog).focus();

    await user().keyboard('{Enter}');

    const keep = await within(dialog).findByRole('button', { name: msg('modals.moveGameInBackground') });
    await vi.waitFor(() => expect(document.activeElement).toBe(keep));
    expect(within(dialog).getByRole('button', { name: msg('modals.moveGameCancelMove') })).toBeTruthy();
  });
});

describe('MoveGameModal: a running move', () => {
  it('opens on "continue in background", says what a cancel leaves, and leaves the move running when closed', async () => {
    moves.set([job()]);
    const dialog = await openDialog();
    const keep = within(dialog).getByRole('button', { name: msg('modals.moveGameInBackground') });
    await vi.waitFor(() => expect(document.activeElement).toBe(keep));
    expect(within(dialog).getByText(msg('modals.moveGameCancelNote'))).toBeTruthy();

    await user().click(keep);

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(cancelMove)).not.toHaveBeenCalled();
  });

  it('sends one Cancel for the running move, and says a refused cancel in the dialog by code', async () => {
    moves.set([job()]);
    const cancelled = held<void>();
    vi.mocked(cancelMove).mockReturnValueOnce(cancelled.promise);
    const dialog = await openDialog();
    const cancel = within(dialog).getByRole('button', { name: msg('modals.moveGameCancelMove') });
    cancel.focus();
    const keys = user();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(cancelMove).mock.calls).toEqual([['m1']]);
    const busy = within(dialog).getByRole('button', { name: msg('modals.moveGameCancelling') });
    expect(busy).toBe(cancel);
    expect(busy.getAttribute('aria-busy')).toBe('true');

    cancelled.reject(coded('relocate.job_not_found'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('transfers.moveErrJobNotFound'));
    expect(get(toasts)).toEqual([]);
  });
});

describe('MoveGameModal: an answer after the dialog moved on', () => {
  it('drops a folder picked for the game before, when the dialog is open for another', async () => {
    const chosen = held<string>();
    vi.mocked(selectMoveTargetFolder).mockReturnValueOnce(chosen.promise);
    const dialog = await openDialog('g1');
    await user().click(browseButton(dialog));
    closeMoveGame();
    openMoveGame('g2');
    const next = await screen.findByRole('dialog', { name: msg('modals.moveGameTitle') });
    await within(next).findByText('C:/Games/Celeste');

    chosen.resolve('D:/Games');
    await settle();

    expect(vi.mocked(moveGameTarget)).not.toHaveBeenCalled();
    expect(within(next).queryByText('D:/Games')).toBeNull();
    expect(within(next).getByText(msg('modals.moveGameFolderPlaceholder'))).toBeTruthy();
  });

  it('never shows a move started for one game as the other’s, and names the game it started for', async () => {
    const started = held<MoveJob>();
    vi.mocked(moveGame).mockReturnValueOnce(started.promise);
    const dialog = await openDialog('g1');
    await pickTarget(dialog);
    await user().click(startButton(dialog));
    closeMoveGame();
    openMoveGame('g2');
    const next = await screen.findByRole('dialog', { name: msg('modals.moveGameTitle') });
    await within(next).findByText('C:/Games/Celeste');

    started.resolve(job());
    await settle();

    expect(within(next).queryByRole('progressbar')).toBeNull();
    expect(within(next).queryByRole('button', { name: msg('modals.moveGameCancelMove') })).toBeNull();
    expect(get(toasts).map((t) => t.message)).toEqual([msg('modals.moveGameStartedFor', { title: 'Hades' })]);
  });

  it('says a start refused after the dialog moved on once, naming the game, with the reason by code', async () => {
    const started = held<MoveJob>();
    vi.mocked(moveGame).mockReturnValueOnce(started.promise);
    const dialog = await openDialog('g1');
    await pickTarget(dialog);
    await user().click(startButton(dialog));
    closeMoveGame();
    openMoveGame('g2');
    const next = await screen.findByRole('dialog', { name: msg('modals.moveGameTitle') });
    await within(next).findByText('C:/Games/Celeste');

    started.reject(coded('relocate.game_running'));
    await settle();

    expect(within(next).queryByRole('alert')).toBeNull();
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([
      [msg('modals.moveGameStartFailedForReason', { title: 'Hades', reason: msg('transfers.moveErrGameRunning') }), 'danger'],
    ]);
  });
});
