import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { setCompletedAt, setStatus, type LibraryGame } from '../services/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import GameStatusModal from './GameStatusModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  setStatus: vi.fn(),
  setCompletedAt: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.mocked(setStatus).mockReset();
  vi.mocked(setCompletedAt).mockReset();
  toasts.set([]);
  app.remove();
  vi.restoreAllMocks();
});

const game = { id: 'g1', title: 'Hades', status: '' } as LibraryGame;

async function finishWithDate() {
  render(GameStatusModal, { target: app, props: { open: true, game } });
  const keys = user();
  const dialog = screen.getByRole('dialog', { name: msg('modals.gameStatusTitle') });
  await keys.click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));
  await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));
  return { keys, dialog };
}

describe('GameStatusModal: the status saves and its date does not', () => {
  it('says the status is saved and the date is not, and shows the saved status', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValue(new Error('typhon:library.something_new: запись не удалась'));

    const { dialog } = await finishWithDate();

    await vi.waitFor(() => expect(get(toasts).map((item) => item.message)).toEqual([msg('games.blocks.statusDateFailed')]));
    expect(dialog.isConnected).toBe(true);
    expect(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }).getAttribute('aria-pressed')).toBe('true');
  });

  it('names the date failure\'s reason when its code has a text', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValue(new Error('typhon:library.completed_at_in_future: дата в будущем'));

    await finishWithDate();

    await vi.waitFor(() =>
      expect(get(toasts).map((item) => item.message)).toEqual([
        msg('games.blocks.statusDateFailedReason', { reason: msg('library.completed_at_in_future') }),
      ]),
    );
  });

  it('saving again sends only the date: the status is already saved', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась')).mockResolvedValueOnce(undefined);
    const { keys, dialog } = await finishWithDate();
    await vi.waitFor(() => expect(get(toasts)).toHaveLength(1));

    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog', { name: msg('modals.gameStatusTitle') })).toBeNull());
    expect(setStatus).toHaveBeenCalledTimes(1);
    expect(setCompletedAt).toHaveBeenCalledTimes(2);
  });

  it('a failed status says the status failed', async () => {
    vi.mocked(setStatus).mockRejectedValue(new Error('typhon:library.something_new: запись не удалась'));

    await finishWithDate();

    await vi.waitFor(() => expect(get(toasts).map((item) => item.message)).toEqual([msg('modals.gameStatusUpdateFailed')]));
    expect(setCompletedAt).not.toHaveBeenCalled();
  });
});
