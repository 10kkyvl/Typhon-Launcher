import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { notesLimit, setNotes } from '../../lib/services/library';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
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

describe('GameNotes: a note over the limit', () => {
  it('is a shared-look field that says it is invalid and is described by its counter', async () => {
    const { user } = renderDom(GameNotes, { gameId: 'g1', stored: '' });
    const field = screen.getByRole('textbox', { name: msg('games.notesHeading') });
    expect(field.classList.contains('input')).toBe(true);
    await vi.waitFor(() => expect(description(field)).toBe(msg('games.notesCounter', { count: 0, limit: 5 })));
    expect(field.getAttribute('aria-invalid')).not.toBe('true');

    await user.type(field, 'abcdef');

    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(description(field)).toBe(msg('games.notesCounter', { count: 6, limit: 5 }));
  });
});
