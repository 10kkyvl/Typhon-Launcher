import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { weekSummary } from '../../lib/profile/week';
import { feed, type FeedEvent, type FeedPage } from '../../lib/services/social';
import type { PopularGame } from '../../lib/social/popular';
import { resetFeed } from '../../lib/stores/feed';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import Activity from './Activity.svelte';
import PopularGames from './PopularGames.svelte';
import WeekActivity from './WeekActivity.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  feed: vi.fn(),
}));

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

describe('Activity: a card with nothing in it', () => {
  it('popular games and the week say so with the base empty state, as a line in the card', () => {
    renderDom(PopularGames, { games: [] });
    renderDom(WeekActivity, { week: weekSummary([], new Date(2026, 10, 11)) });

    for (const key of ['transfers.activityPopularEmpty', 'transfers.activityWeekEmpty'] as const) {
      expect(screen.getByText(msg(key)).closest('.empty.inline'), key).not.toBeNull();
    }
  });
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

describe('Activity: more of the feed', () => {
  afterEach(() => {
    resetFeed();
    authState.set('bootstrapping');
    vi.mocked(feed).mockReset();
  });

  it('"Show more" is busy while the next page loads, keeps its words and the focus', async () => {
    const event = {
      id: 7,
      user: { id: 'u2', username: 'neo', displayName: 'Neo', avatarUrl: '' },
      kind: 'completed',
      game: { igdbId: 1, title: 'Hades', coverUrl: '' },
      createdAt: new Date().toISOString(),
      seconds: 0,
      achievements: [],
      review: null,
      reactions: [],
      mine: [],
      note: '',
    } as unknown as FeedEvent;
    vi.mocked(feed).mockResolvedValueOnce({ events: [event], next: 5 } as FeedPage).mockReturnValueOnce(new Promise(() => {}));
    authState.set('authenticated');
    const { user } = renderDom(Activity);
    const more = await screen.findByRole('button', { name: msg('transfers.activityShowMore') });
    more.focus();

    await user.keyboard('{Enter}');

    expect(more.getAttribute('aria-busy')).toBe('true');
    expect(more.hasAttribute('disabled')).toBe(false);
    expect(more.textContent?.trim()).toBe(msg('transfers.activityShowMore'));
    expect(document.activeElement).toBe(more);
  });
});
