import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { setCompletedAt, setStatus, type LibraryGame } from '../services/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import GameStatusModal from './GameStatusModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  setStatus: vi.fn(),
  setCompletedAt: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  vi.mocked(setStatus).mockReset();
  vi.mocked(setCompletedAt).mockReset();
  toasts.set([]);
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const game = { id: 'g1', title: 'Hades', status: '' } as LibraryGame;

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

function openDialog(of: LibraryGame = game) {
  render(GameStatusModal, { target: app, props: { open: true, game: of } });
  return screen.getByRole('dialog', { name: msg('modals.gameStatusTitle') });
}

async function finishWithDate() {
  const dialog = openDialog();
  const keys = user();
  await keys.click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));
  const save = within(dialog).getByRole('button', { name: msg('common.save') });
  save.focus();
  await keys.keyboard('{Enter}');
  return { keys, dialog, save };
}

describe('GameStatusModal: choosing a status', () => {
  it('opens with the focus on the status the game has', () => {
    const dialog = openDialog({ ...game, status: 'playing' } as LibraryGame);

    expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: msg('games.statusPlaying') }));
    expect(document.activeElement?.getAttribute('aria-pressed')).toBe('true');
  });

  it('saves a status without a date at once and closes', async () => {
    vi.mocked(setStatus).mockResolvedValueOnce({ ...game, status: 'playing' } as LibraryGame);
    const dialog = openDialog();

    await user().click(within(dialog).getByRole('button', { name: msg('games.statusPlaying') }));

    expect(setStatus).toHaveBeenCalledExactlyOnceWith('g1', 'playing');
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it('keeps the focus on the pressed status, busy, and holds the others with the reason while it is saved', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setStatus).mockReturnValueOnce(saved.promise);
    const dialog = openDialog();
    const keys = user();
    const playing = within(dialog).getByRole('button', { name: msg('games.statusPlaying') });
    playing.focus();

    await keys.keyboard('{Enter}');

    expect(playing.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(playing);
    const other = within(dialog).getByRole('button', { name: msg('games.statusBacklog') });
    expect(other).toHaveProperty('disabled', false);
    expect(other.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(other)).toContain(msg('modals.actionRunning'));
    await keys.click(other);
    await keys.keyboard('{Escape}');
    expect(setStatus).toHaveBeenCalledTimes(1);
    expect(dialog.isConnected).toBe(true);
    saved.resolve({} as LibraryGame);
  });

  it('says a failed status in the dialog, in the launcher\'s words, and moves the focus there', async () => {
    vi.mocked(setStatus).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась'));
    const dialog = openDialog();
    const keys = user();
    const playing = within(dialog).getByRole('button', { name: msg('games.statusPlaying') });
    playing.focus();

    await keys.keyboard('{Enter}');

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.gameStatusUpdateFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(dialog.textContent).not.toContain('запись не удалась');
    expect(get(toasts)).toEqual([]);
    await vi.waitFor(() => expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true));
  });
});

describe('GameStatusModal: the completion date', () => {
  it('is a date field named by its label', async () => {
    const dialog = openDialog();
    await user().click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));

    const field = within(dialog).getByLabelText(msg('modals.gameCompletedAt'));
    expect(field.getAttribute('type')).toBe('date');
    expect(field.getAttribute('aria-invalid')).not.toBe('true');
  });

  it('says a date out of range at the field, and holds Save with that reason', async () => {
    const dialog = openDialog();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));
    const field = within(dialog).getByLabelText(msg('modals.gameCompletedAt'));

    await fireEvent.input(field, { target: { value: '1970-05-01' } });

    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(describedBy(field)).toEqual([msg('modals.gameCompletedAtInvalid')]);
    const save = within(dialog).getByRole('button', { name: msg('common.save') });
    expect(save).toHaveProperty('disabled', false);
    expect(save.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(save)).toContain(msg('modals.gameCompletedAtInvalid'));
    await keys.click(save);
    expect(setStatus).not.toHaveBeenCalled();
  });

  it('says a saved status with a refused date at the date field, which takes the focus, and keeps the saved status', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValue(new Error('typhon:library.something_new: запись не удалась'));

    const { dialog } = await finishWithDate();

    const field = within(dialog).getByLabelText(msg('modals.gameCompletedAt'));
    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('games.blocks.statusDateFailed'));
    expect(describedBy(field)).toEqual([msg('games.blocks.statusDateFailed')]);
    expect(field.getAttribute('aria-invalid')).toBe('true');
    await vi.waitFor(() => expect(document.activeElement).toBe(field));
    expect(get(toasts)).toEqual([]);
    expect(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }).getAttribute('aria-pressed')).toBe('true');
  });

  it('names the date failure\'s reason when its code has words', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValue(new Error('typhon:library.completed_at_in_future: дата в будущем'));

    const { dialog } = await finishWithDate();

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('games.blocks.statusDateFailedReason', { reason: msg('library.completed_at_in_future') }));
  });

  it('saving again sends only the date: the status is already saved', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась')).mockResolvedValueOnce(undefined);
    const { keys, dialog } = await finishWithDate();
    await within(dialog).findByRole('alert');

    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog', { name: msg('modals.gameStatusTitle') })).toBeNull());
    expect(setStatus).toHaveBeenCalledTimes(1);
    expect(setCompletedAt).toHaveBeenCalledTimes(2);
  });

  it('a failed status says the status failed and sends no date', async () => {
    vi.mocked(setStatus).mockRejectedValue(new Error('typhon:library.something_new: запись не удалась'));

    const { dialog } = await finishWithDate();

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.gameStatusUpdateFailed'));
    expect(setCompletedAt).not.toHaveBeenCalled();
  });
});

describe('GameStatusModal: opened again, or turned to another game while it stays mounted (the game page)', () => {
  const celeste = { id: 'g2', title: 'Celeste', status: 'backlog' } as LibraryGame;

  function mounted(of: LibraryGame = game) {
    const result = render(GameStatusModal, { target: app, props: { open: true, game: of } });
    return { ...result, dialog: screen.getByRole('dialog', { name: msg('modals.gameStatusTitle') }) };
  }

  const pressed = (dialog: HTMLElement) =>
    within(dialog)
      .getAllByRole('button')
      .filter((button) => button.getAttribute('aria-pressed') === 'true')
      .map((button) => button.textContent?.trim());

  it('offers the new game\'s own status, not the one pressed for the game before', async () => {
    const { dialog, rerender } = mounted();
    await user().click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));

    await rerender({ open: true, game: celeste });

    expect(pressed(dialog)).toEqual([msg('games.statusBacklog')]);
    expect(within(dialog).queryByLabelText(msg('modals.gameCompletedAt'))).toBeNull();
  });

  async function pressThenTurn(status: string) {
    const saved = held<LibraryGame>();
    vi.mocked(setStatus).mockReturnValueOnce(saved.promise);
    const { dialog, rerender } = mounted();
    const keys = user();
    if (status === 'completed') {
      await keys.click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));
      within(dialog).getByRole('button', { name: msg('common.save') }).focus();
    } else {
      within(dialog).getByRole('button', { name: msg('games.statusPlaying') }).focus();
    }
    await keys.keyboard('{Enter}');
    expect(setStatus).toHaveBeenCalledExactlyOnceWith('g1', status);

    await rerender({ open: true, game: celeste });
    expect(pressed(dialog)).toEqual([msg('games.statusBacklog')]);
    return { saved, dialog };
  }

  const live = (dialog: HTMLElement) =>
    within(dialog).getAllByRole('button').filter((button) => button.getAttribute('aria-disabled') === 'true' || button.getAttribute('aria-busy') === 'true');
  const said = () => get(toasts).map((t) => [t.message, t.kind]);

  it('says a refusal for the game before once, in a toast naming that game, nothing in the new game\'s dialog, and leaves it live', async () => {
    const { saved, dialog } = await pressThenTurn('playing');

    saved.reject(new Error('typhon:library.not_found: no game'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(said()).toEqual([[msg('modals.gameStatusFailedFor', { title: 'Hades' }), 'danger']]);
    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(dialog.textContent).not.toContain('no game');
    expect(live(dialog)).toEqual([]);
  });

  it('names the reason of a refusal for the game before when its code has words', async () => {
    const { saved, dialog } = await pressThenTurn('playing');

    saved.reject(new Error('typhon:library.invalid_status: bad status'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(said()).toEqual([[msg('modals.gameStatusFailedForReason', { title: 'Hades', reason: msg('library.invalid_status') }), 'danger']]);
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('says a date refused for the game before, after its status was saved, in a toast naming that game', async () => {
    vi.mocked(setCompletedAt).mockRejectedValueOnce(new Error('typhon:library.completed_at_too_old: too old'));
    const { saved, dialog } = await pressThenTurn('completed');

    saved.resolve({ ...game, status: 'completed' } as LibraryGame);
    await vi.waitFor(() => expect(said()).toEqual([[msg('modals.gameStatusDateFailedForReason', { title: 'Hades', reason: msg('library.completed_at_too_old') }), 'danger']]));

    expect(vi.mocked(setCompletedAt).mock.calls[0][0]).toBe('g1');
    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(within(dialog).queryByLabelText(msg('modals.gameCompletedAt'))).toBeNull();
    expect(live(dialog)).toEqual([]);
  });

  it('says a date refused for the game before without words for its code as the bare sentence', async () => {
    vi.mocked(setCompletedAt).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась'));
    const { saved } = await pressThenTurn('completed');

    saved.resolve({ ...game, status: 'completed' } as LibraryGame);
    await vi.waitFor(() => expect(said()).toEqual([[msg('modals.gameStatusDateFailedFor', { title: 'Hades' }), 'danger']]));
  });

  it('a save for the game before, answered after the turn, leaves the new game\'s dialog open and sends its date to the game it was pressed for', async () => {
    const saved = held<LibraryGame>();
    vi.mocked(setStatus).mockReturnValueOnce(saved.promise);
    vi.mocked(setCompletedAt).mockResolvedValue(undefined);
    const { dialog, rerender } = mounted();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));
    const save = within(dialog).getByRole('button', { name: msg('common.save') });
    save.focus();
    await keys.keyboard('{Enter}');
    expect(setStatus).toHaveBeenCalledExactlyOnceWith('g1', 'completed');

    await rerender({ open: true, game: celeste });
    expect(pressed(dialog)).toEqual([msg('games.statusBacklog')]);
    saved.resolve({ ...game, status: 'completed' } as LibraryGame);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(dialog.isConnected).toBe(true);
    expect(setCompletedAt).toHaveBeenCalledTimes(1);
    expect(vi.mocked(setCompletedAt).mock.calls[0][0]).toBe('g1');
    expect(pressed(dialog)).toEqual([msg('games.statusBacklog')]);
    expect(get(toasts)).toEqual([]);
  });

  it('opened again after a refused status, says nothing of the refusal', async () => {
    vi.mocked(setStatus).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась'));
    const { dialog, rerender } = mounted();
    await user().click(within(dialog).getByRole('button', { name: msg('games.statusPlaying') }));
    await within(dialog).findByRole('alert');

    await rerender({ open: false, game });
    await rerender({ open: true, game });

    const again = screen.getByRole('dialog', { name: msg('modals.gameStatusTitle') });
    expect(within(again).queryByRole('alert')).toBeNull();
  });

  it('opened again after a refused date, shows the date field without the refusal', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась'));
    const { dialog, rerender } = mounted();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));
    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));
    await within(dialog).findByRole('alert');

    await rerender({ open: false, game });
    await rerender({ open: true, game });
    const again = screen.getByRole('dialog', { name: msg('modals.gameStatusTitle') });
    await keys.click(within(again).getByRole('button', { name: msg('games.statusCompleted') }));

    const field = within(again).getByLabelText(msg('modals.gameCompletedAt'));
    expect(within(again).queryByRole('alert')).toBeNull();
    expect(field.getAttribute('aria-invalid')).not.toBe('true');
  });

  it('keeps the pressed status and a refused date when the library sends the same game again', async () => {
    vi.mocked(setStatus).mockResolvedValue({ ...game, status: 'completed' } as LibraryGame);
    vi.mocked(setCompletedAt).mockRejectedValueOnce(new Error('typhon:library.something_new: запись не удалась'));
    const { dialog, rerender } = mounted();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.statusCompleted') }));
    await keys.click(within(dialog).getByRole('button', { name: msg('common.save') }));
    await within(dialog).findByRole('alert');

    await rerender({ open: true, game: { ...game, status: 'completed' } as LibraryGame });

    expect(within(dialog).getByRole('alert').textContent?.trim()).toBe(msg('games.blocks.statusDateFailed'));
    expect(pressed(dialog)).toEqual([msg('games.statusCompleted')]);
  });
});
