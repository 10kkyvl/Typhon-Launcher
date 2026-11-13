import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { resetHistory, route } from '../stores/router';
import type { FeedEvent } from '../services/social';
import { renderDom } from '../testing/dom';
import FeedRow from './FeedRow.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

const event = {
  id: 7,
  user: { id: 'u2', username: 'neo', displayName: 'Neo', avatarUrl: '' },
  kind: 'completed',
  game: { igdbId: 1, title: 'Hades', coverUrl: 'cover.png', artworkUrl: 'art.png' },
  createdAt: new Date().toISOString(),
  seconds: 0,
  achievements: [],
  review: null,
  reactions: [],
  mine: [],
  note: '',
} as unknown as FeedEvent;

// The accessible name of the one button whose name holds the game's title.
function gameButtonName(): string {
  let found = '';
  screen.getByRole('button', {
    name: (name) => {
      if (!name.includes('Hades')) return false;
      found = name;
      return true;
    },
  });
  return found;
}

describe('FeedRow: the game it opens', () => {
  it.each([false, true])('is named by its title once (compact: %s): the picture says nothing of its own', (compact) => {
    renderDom(FeedRow, { event, compact });
    expect(gameButtonName().split('Hades').length - 1).toBe(1);
  });
});

describe('FeedRow: the name of who it is about', () => {
  afterEach(() => {
    resetHistory();
  });

  it('opens the profile through a text link the keyboard presses', async () => {
    const { user } = renderDom(FeedRow, { event });
    const name = screen.getAllByRole('button', { name: 'Neo' }).find((button) => button.textContent?.trim() === 'Neo') as HTMLElement;
    expect(name.classList.contains('text-link')).toBe(true);
    // A long name stays on the entry's first line, cut with an ellipsis, as it was before the link.
    expect(name.classList.contains('truncate')).toBe(true);
    name.focus();

    await user.keyboard('{Enter}');

    expect(get(route)).toMatchObject({ name: 'user', params: { username: 'neo' } });
  });
});

