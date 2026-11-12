import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import FeedNote from './FeedNote.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());

const LONG = 'A long evening with the game, '.repeat(20).trim();

describe('FeedNote', () => {
  it('shows the rest of a long note through a muted text link that says it expands the note', async () => {
    const { user } = renderDom(FeedNote, { note: LONG });
    const more = screen.getByRole('button', { name: msg('social.noteMore') });
    expect(more.classList.contains('text-link')).toBe(true);
    expect(more.classList.contains('muted')).toBe(true);
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(more.hasAttribute('aria-pressed')).toBe(false);
    const text = document.getElementById(more.getAttribute('aria-controls') ?? '');
    expect(text?.textContent).not.toContain(LONG);
    more.focus();

    await user.keyboard('{Enter}');

    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(more.textContent?.trim()).toBe(msg('social.noteLess'));
    expect(text?.textContent).toContain(LONG);
  });

  it('adds and edits an own note through muted text links, in a field with the shared look', async () => {
    const { user } = renderDom(FeedNote, { note: '', own: true });
    const add = screen.getByRole('button', { name: msg('social.noteAdd') });
    expect(add.classList.contains('text-link')).toBe(true);
    add.focus();

    await user.keyboard('{Enter}');

    expect(screen.getByRole('textbox', { name: msg('social.notePlaceholder') }).classList.contains('input')).toBe(true);
  });

  it('offers the edit of an own note as a muted text link', () => {
    renderDom(FeedNote, { note: 'Short', own: true });
    expect(screen.getByRole('button', { name: msg('social.noteEdit') }).classList.contains('text-link')).toBe(true);
  });
});
