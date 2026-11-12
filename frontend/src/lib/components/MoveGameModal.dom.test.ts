import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { moveGame, moveGameTarget, selectMoveTargetFolder, type MoveJob } from '../services/relocate';
import { getStorageInfoFor, type StorageInfo } from '../services/system';
import { libraryGames } from '../stores/library';
import { closeMoveGame, moves, openMoveGame } from '../stores/relocate';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import MoveGameModal from './MoveGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
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
  libraryGames.set([{ id: 'g1', title: 'Hades', installDir: 'C:/Games/Hades', sizeBytes: 0 } as LibraryGame]);
  vi.mocked(selectMoveTargetFolder).mockResolvedValue('D:/Games');
  vi.mocked(moveGameTarget).mockResolvedValue('D:/Games/Hades');
  vi.mocked(getStorageInfoFor).mockResolvedValue({ volume: 'D', filesystem: 'NTFS', freeBytes: 10, totalBytes: 20 } as StorageInfo);
});

afterEach(() => {
  closeMoveGame();
  moves.set([]);
  libraryGames.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

async function openDialog() {
  render(MoveGameModal, { target: app });
  openMoveGame('g1');
  return screen.findByRole('dialog', { name: msg('modals.moveGameTitle') });
}

describe('MoveGameModal', () => {
  it('states that the move is safe in an info notice read with the dialog', async () => {
    const dialog = await openDialog();
    const notice = within(dialog).getByText(msg('modals.moveGameSafetyText')).closest('[data-tone="info"]') as HTMLElement;
    expect(notice).not.toBeNull();
    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('names the new folder field by its label, and keeps the focus on "browse", busy, while a folder is picked', async () => {
    const chosen = deferred<string>();
    vi.mocked(selectMoveTargetFolder).mockReturnValueOnce(chosen.promise);
    const dialog = await openDialog();
    const keys = user();
    expect(within(dialog).getByRole('textbox', { name: msg('modals.moveGameNewFolder') })).toBeTruthy();
    const browse = within(dialog).getByRole('button', { name: msg('modals.moveGameBrowse') });
    browse.focus();

    await keys.keyboard('{Enter}');

    expect(browse.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(browse);
    chosen.resolve('');
  });

  it('keeps the focus on "start", busy, while the move starts, and says a refusal in a danger notice', async () => {
    const started = deferred<MoveJob>();
    vi.mocked(moveGame).mockReturnValueOnce(started.promise);
    const dialog = await openDialog();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('modals.moveGameBrowse') }));
    const start = within(dialog).getByRole('button', { name: msg('modals.moveGameStart') });
    await vi.waitFor(() => expect(start).toHaveProperty('disabled', false));
    start.focus();

    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('modals.moveGameStarting') });
    expect(busy).toBe(start);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);

    started.reject(new Error('locked'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.moveGameStartFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });
});
