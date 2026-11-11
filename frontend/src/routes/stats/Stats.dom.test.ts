import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/playlog', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/playlog')>()),
  getStats: vi.fn(async () => ({
    period: 'week',
    from: '2026-11-05',
    to: '2026-11-11',
    totalSeconds: 4000,
    sessions: 3,
    days: [],
    top: [
      { gameId: 'g1', title: 'Hades', seconds: 3000, sessions: 2 },
      { gameId: 'g2', title: 'Celeste', seconds: 1000, sessions: 1 },
    ],
    longest: null,
  })),
}));

import type { LibraryGame } from '../../lib/services/library';
import { libraryGames } from '../../lib/stores/library';
import { renderDom } from '../../lib/testing/dom';
import Stats from './Stats.svelte';

afterEach(() => libraryGames.set([]));

describe('Stats: most played', () => {
  // The bar under each game draws its share of the period's playtime; the row's text says it.
  it('says each game’s share of the playtime in the row, the bar only draws it', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole, container } = renderDom(Stats);

    const named = async (title: string) => {
      let found = '';
      await findByRole('button', {
        name: (name) => {
          if (name.includes(title)) found = name;
          return name.includes(title);
        },
      });
      return found;
    };
    expect(await named('Hades')).toMatch(/ · 75%$/);
    expect(await named('Celeste')).toMatch(/ · 25%$/);
    for (const share of container.querySelectorAll('.share')) expect(share.getAttribute('aria-hidden')).toBe('true');
  });
});
