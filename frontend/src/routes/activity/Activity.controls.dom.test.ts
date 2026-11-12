import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { weekSummary } from '../../lib/profile/week';
import type { PopularGame } from '../../lib/social/popular';
import { renderDom } from '../../lib/testing/dom';
import PopularGames from './PopularGames.svelte';
import WeekActivity from './WeekActivity.svelte';

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

describe('Activity: rows that open a game', () => {
  it('a popular game is named by its title once: the cover says nothing of its own', () => {
    const games = [{ game: { igdbId: 1, title: 'Hades', coverUrl: '' }, count: 2, playing: 0, names: ['Ann', 'Bob'] }] as unknown as PopularGame[];
    renderDom(PopularGames, { games });

    const name = nameOf('Hades');
    expect(name.startsWith('Hades')).toBe(true);
    expect(times(name, 'Hades')).toBe(1);
  });

  it('a game of the week is named by its title once', () => {
    const week = weekSummary(
      [{ date: '2026-11-10', entries: [{ game: { id: 'g', title: 'Hades', cover: '', archived: false, playtimeSeconds: 3600, status: '' }, seconds: 3600 }] }],
      new Date(2026, 10, 11),
    );
    renderDom(WeekActivity, { week });

    const name = nameOf('Hades');
    expect(name.startsWith('Hades')).toBe(true);
    expect(times(name, 'Hades')).toBe(1);
  });
});
