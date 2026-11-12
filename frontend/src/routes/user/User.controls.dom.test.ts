import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { GameCard, PlayedGame } from '../../lib/services/social';
import { renderDom } from '../../lib/testing/dom';
import GameRow from './GameRow.svelte';
import UserRecent from './UserRecent.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

// The accessible name of the one button whose name holds `title`.
function nameOf(title: string): string {
  let found = '';
  screen.getByRole('button', {
    name: (name) => {
      if (!name.includes(title)) return false;
      found = name;
      return true;
    },
  });
  return found;
}

const times = (text: string, word: string) => text.split(word).length - 1;

afterEach(() => {
  document.body.replaceChildren();
});

describe("a user's page: rows that open a game", () => {
  it('a game row is named by its title once: the cover says nothing of its own', () => {
    renderDom(GameRow, { game: { igdbId: 1, title: 'Hades', coverUrl: '' } as GameCard, meta: 'Both own it' });

    const name = nameOf('Hades');
    expect(name.startsWith('Hades')).toBe(true);
    expect(times(name, 'Hades')).toBe(1);
  });

  it('a recently played game is named by its title once', () => {
    const games = [{ igdbId: 1, title: 'Hades', coverUrl: '', heroUrl: '', lastPlayedAt: '2026-11-01T10:00:00Z', playtimeSeconds: 0 }] as unknown as PlayedGame[];
    renderDom(UserRecent, { games });

    const name = nameOf('Hades');
    expect(name.startsWith('Hades')).toBe(true);
    expect(times(name, 'Hades')).toBe(1);
  });
});
