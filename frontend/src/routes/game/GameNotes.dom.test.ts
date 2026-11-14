import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { notesLimit, setNotes } from '../../lib/services/library';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import GameNotesHarness from '../../lib/testing/GameNotesHarness.svelte';
import { description } from '../../lib/testing/hints';
import GameNotes from './GameNotes.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  notesLimit: vi.fn(),
  setNotes: vi.fn(),
}));

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(notesLimit).mockResolvedValue(5);
  vi.mocked(setNotes).mockResolvedValue(undefined);
});

afterEach(() => {
  vi.mocked(notesLimit).mockReset();
  vi.mocked(setNotes).mockReset();
  toasts.set([]);
  vi.restoreAllMocks();
});

describe('GameNotes: a save that fails', () => {
  it('says why as an alert by the field, and the field is described by it', async () => {
    vi.useFakeTimers();
    vi.mocked(setNotes).mockRejectedValue(new Error('disk full'));
    const { user } = renderDom(GameNotes, { gameId: 'g1', stored: '' });
    await vi.advanceTimersByTimeAsync(0);
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });

    await user.type(field, 'abc');
    await vi.advanceTimersByTimeAsync(2000);

    const alert = screen.getByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('games.notesSaveError'));
    expect(description(field)).toContain(msg('games.notesSaveError'));
    expect(field.getAttribute('aria-invalid')).not.toBe('true');
  });
});

describe('GameNotes: a note over the limit', () => {
  it('is a shared-look field that says it is invalid and is described by its counter', async () => {
    const { user } = renderDom(GameNotes, { gameId: 'g1', stored: '' });
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });
    expect(field.classList.contains('input')).toBe(true);
    await vi.waitFor(() => expect(description(field)).toBe(msg('games.notesCounter', { count: 0, limit: 5 })));
    expect(field.getAttribute('aria-invalid')).not.toBe('true');

    await user.type(field, 'abcdef');

    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(description(field)).toContain(msg('games.notesCounter', { count: 6, limit: 5 }));
  });

  it('never says "Saved" while over the limit, and says once that it is too long and not saved', async () => {
    vi.useFakeTimers();
    const { user } = renderDom(GameNotes, { gameId: 'g1', stored: '' });
    await vi.advanceTimersByTimeAsync(0);
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });

    // Typed faster than the autosave delay: "abcde" is queued before the sixth letter crosses the limit.
    await user.type(field, 'abcdef');
    await vi.advanceTimersByTimeAsync(2000);

    expect(document.body.textContent).not.toContain(msg('games.notesSaved'));
    const alerts = screen.getAllByRole('alert');
    expect(alerts.map((alert) => alert.textContent?.trim())).toEqual([msg('games.notesTooLong')]);
    expect(description(field)).toContain(msg('games.notesTooLong'));

    await user.type(field, 'g');
    await vi.advanceTimersByTimeAsync(2000);
    expect(screen.getAllByRole('alert')).toEqual(alerts);
    expect(document.body.textContent).not.toContain(msg('games.notesSaved'));
  });

  it('sends nothing past the limit, and saves on the next input once back under it', async () => {
    vi.useFakeTimers();
    const { user } = renderDom(GameNotes, { gameId: 'g1', stored: '' });
    await vi.advanceTimersByTimeAsync(0);
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });
    await user.type(field, 'abc');
    await vi.advanceTimersByTimeAsync(2000);
    expect(vi.mocked(setNotes).mock.calls).toEqual([['g1', 'abc']]);

    await user.type(field, 'def');
    await vi.advanceTimersByTimeAsync(2000);
    field.blur();
    await vi.advanceTimersByTimeAsync(2000);
    expect(vi.mocked(setNotes).mock.calls.every(([, text]) => [...text].length <= 5)).toBe(true);

    field.focus();
    await user.keyboard('{Backspace}{Backspace}');
    await vi.advanceTimersByTimeAsync(2000);
    expect(vi.mocked(setNotes).mock.lastCall).toEqual(['g1', 'abcd']);
  });
});

// The page creates the block again for each game (`{#key}`) and keeps itself mounted: a save that
// runs after the block went (a flush on destroy, a retry of a failed save) still belongs to the game
// the text was typed for, never to the game the page shows now.
describe('GameNotes: a save that outlives its block', () => {
  type Game = { id: string; notes: string } | null;
  const gameA = { id: 'game-a', notes: '' } as Game;
  const gameB = { id: 'game-b', notes: 'B keeps this' } as Game;

  it('a failed save retried when the page moves to another game is written to the first game', async () => {
    vi.mocked(setNotes).mockRejectedValueOnce(new Error('typhon:library.save_failed: disk full'));
    const { user, rerender } = renderDom(GameNotesHarness, { game: gameA });
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });

    await user.type(field, 'A-txt');
    field.blur();
    await vi.waitFor(() => expect(screen.getByRole('alert')).toBeTruthy());

    await rerender({ game: gameB });
    await vi.waitFor(() => expect(setNotes).toHaveBeenCalledTimes(2));

    expect(vi.mocked(setNotes).mock.calls).toEqual([
      ['game-a', 'A-txt'],
      ['game-a', 'A-txt'],
    ]);
  });

  it('text typed and not yet saved when the page moves on is saved to the game it was typed for', async () => {
    const { user, rerender } = renderDom(GameNotesHarness, { game: gameA });
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });

    await user.type(field, 'A-txt');
    await rerender({ game: gameB });

    await vi.waitFor(() => expect(setNotes).toHaveBeenCalled());
    expect(vi.mocked(setNotes).mock.calls).toEqual([['game-a', 'A-txt']]);
  });

  it('text not yet saved when the page leaves the game is saved to that game without an error', async () => {
    const { user, rerender } = renderDom(GameNotesHarness, { game: gameA });
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });

    await user.type(field, 'A-txt');
    await rerender({ game: null });

    await vi.waitFor(() => expect(setNotes).toHaveBeenCalled());
    expect(vi.mocked(setNotes).mock.calls).toEqual([['game-a', 'A-txt']]);
    await Promise.resolve();
    expect(get(toasts)).toEqual([]);
  });
});
