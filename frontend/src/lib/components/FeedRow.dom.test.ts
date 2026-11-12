import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
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
