import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  feed: vi.fn(),
}));
vi.mock('../../lib/stores/router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/router')>()),
  navigate: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { weekSummary } from '../../lib/profile/week';
import type { Settings } from '../../lib/services/settings';
import { feed, type FeedEvent, type FeedPage } from '../../lib/services/social';
import { resetFeed } from '../../lib/stores/feed';
import { navigate } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { authState, authView } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { relativeDate } from '../../lib/utils/format';
import activitySource from './Activity.svelte?raw';
import Activity from './Activity.svelte';
import WeekActivity from './WeekActivity.svelte';

function event(id: number, createdAt: Date, title = `Game ${id}`): FeedEvent {
  return {
    id,
    user: { id: 'u2', username: 'neo', displayName: 'Neo', avatarUrl: '' },
    kind: 'completed',
    game: { igdbId: id, title, coverUrl: '' },
    createdAt: createdAt.toISOString(),
    seconds: 0,
    achievements: [],
    review: null,
    reactions: [],
    mine: [],
    note: '',
  } as unknown as FeedEvent;
}

const page = (events: FeedEvent[], next = 0) => ({ events, next }) as FeedPage;

/** The middle column, the feed's: popular games on the right name the same games. */
const feedColumn = () => within(document.querySelector('.center') as HTMLElement);

const now = () => new Date();
const daysAgo = (days: number) => {
  const at = now();
  at.setDate(at.getDate() - days);
  return at;
};

beforeEach(() => {
  vi.mocked(feed).mockReset();
  vi.mocked(navigate).mockReset();
});

afterEach(() => {
  resetFeed();
  authState.set('bootstrapping');
  settings.set(null);
  toasts.set([]);
});

describe('Activity: before the feed can be shown', () => {
  it('a guest is asked to sign in or create an account, and no feed is asked for', async () => {
    authState.set('guest');
    const { user } = renderDom(Activity);

    expect(screen.getByRole('heading', { name: msg('transfers.activityGuestTitle') })).toBeTruthy();
    expect(screen.getByText(msg('transfers.activityGuestDescription'))).toBeTruthy();
    const create = screen.getByRole('button', { name: msg('transfers.activityCreateAccount') });
    expect(screen.getByRole('button', { name: msg('transfers.activitySignIn') })).toBeTruthy();
    expect(feed).not.toHaveBeenCalled();

    await user.click(create);

    await vi.waitFor(() => expect(get(authState)).toBe('unauthenticated'));
    expect(get(authView)).toBe('register');
  });

  it('a player without account sync is asked to turn it on, and no feed is asked for', async () => {
    settings.set({ accountSync: false } as Settings);
    authState.set('authenticated');
    const { user } = renderDom(Activity);

    expect(screen.getByRole('heading', { name: msg('transfers.activityConsentTitle') })).toBeTruthy();
    expect(feed).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: msg('transfers.activityEnableSync') }));

    expect(await screen.findByRole('dialog', { name: msg('modals.socialConsentTitle') })).toBeTruthy();
  });
});

describe('Activity: the feed', () => {
  it('groups the events by the day they happened, newest day first', async () => {
    const today = event(1, now(), 'Hades');
    const yesterday = event(2, daysAgo(1), 'Celeste');
    vi.mocked(feed).mockResolvedValueOnce(page([today, yesterday]));
    authState.set('authenticated');
    renderDom(Activity);

    const labels = [relativeDate(today.createdAt), relativeDate(yesterday.createdAt)];
    const hades = await feedColumn().findByText('Hades');
    const celeste = feedColumn().getByText('Celeste');
    const headings = screen.getAllByRole('heading').map((heading) => heading.textContent?.trim());
    expect(headings.filter((text) => labels.includes(text ?? ''))).toEqual(labels);
    const first = screen.getByRole('heading', { name: labels[0] });
    const second = screen.getByRole('heading', { name: labels[1] });
    const order = (a: Node, b: Node) => a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING;
    expect(order(first, hades)).toBeTruthy();
    expect(order(hades, second)).toBeTruthy();
    expect(order(second, celeste)).toBeTruthy();
  });

  it('"Show more" adds the next page under the list and goes when nothing is left', async () => {
    vi.mocked(feed).mockResolvedValueOnce(page([event(1, now(), 'Hades')], 5)).mockResolvedValueOnce(page([event(2, now(), 'Celeste')], 0));
    authState.set('authenticated');
    const { user } = renderDom(Activity);

    await user.click(await screen.findByRole('button', { name: msg('transfers.activityShowMore') }));

    expect(await feedColumn().findByText('Celeste')).toBeTruthy();
    expect(feedColumn().getByText('Hades')).toBeTruthy();
    expect(vi.mocked(feed).mock.calls.map(([cursor]) => cursor)).toEqual(['', '5']);
    expect(screen.queryByRole('button', { name: msg('transfers.activityShowMore') })).toBeNull();
  });

  it('a failed "Show more" keeps the list and the button to try again', async () => {
    vi.mocked(feed).mockResolvedValueOnce(page([event(1, now(), 'Hades')], 5)).mockRejectedValueOnce(new Error('offline'));
    authState.set('authenticated');
    const { user } = renderDom(Activity);

    await user.click(await screen.findByRole('button', { name: msg('transfers.activityShowMore') }));

    await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(screen.getByRole('button', { name: msg('transfers.activityShowMore') }).getAttribute('aria-busy')).toBeNull());
    expect(feedColumn().getByText('Hades')).toBeTruthy();
    expect(get(toasts).map((item) => item.message)).toEqual([msg('state.feedLoadFailed')]);
  });
});

describe('Activity: the week card with no play', () => {
  it('says so and still offers all the activity', async () => {
    const { user } = renderDom(WeekActivity, { week: weekSummary([], new Date(2026, 10, 11)) });

    expect(screen.getByText(msg('transfers.activityWeekEmpty'))).toBeTruthy();
    await user.click(screen.getByRole('button', { name: msg('social.viewAllActivity') }));

    expect(navigate).toHaveBeenCalledWith('profile');
  });
});

describe('Activity: the week chart', () => {
  const played = (date: string, seconds: number) => ({
    date,
    entries: [{ game: { id: 'g', title: 'Hades', cover: '', archived: false, playtimeSeconds: seconds, status: '' }, seconds }],
  });

  it('is the shared bar chart: seven days, today set apart, no axis, the table says each day', () => {
    const week = weekSummary([played('2026-11-10', 3600), played('2026-11-11', 1800)], new Date(2026, 10, 11));
    const { container } = renderDom(WeekActivity, { week });

    const table = screen.getByRole('table', { name: msg('transfers.activityWeekTitle') });
    expect(within(table).getAllByRole('row').slice(1)).toHaveLength(7);
    expect([...container.querySelectorAll('.col')].map((col) => col.classList.contains('current'))).toEqual([false, false, false, false, false, false, true]);
    expect(container.querySelector('.axis')).toBeNull();
    expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  });
});

describe('Activity: the columns by the width of the window', () => {
  // jsdom lays nothing out: the rules are read from the source. Below 1400px the right column goes,
  // below 1100px the left one; the middle (the feed) always stays.
  const rules = [...activitySource.matchAll(/@media \(max-width: (\d+)px\) \{([\s\S]*?)\n {2}\}/g)].map(([, width, body]) => ({
    width: Number(width),
    hidden: [...body.matchAll(/\.(\w+) \{\s*display: none;\s*\}/g)].map(([, name]) => name),
  }));

  it('hide the right column below 1400px and the left one below 1100px, never the feed', () => {
    expect(rules).toEqual([
      { width: 1400, hidden: ['right'] },
      { width: 1100, hidden: ['left'] },
    ]);
  });

  it('the hidden columns are the ones that hold the week and the friends playing now', () => {
    const markup = activitySource.replace(/<(script|style)\b[\s\S]*?<\/\1>/g, '');
    expect(markup).toMatch(/<div class="left">[\s\S]*<WeekActivity\b[\s\S]*<div class="center">/);
    expect(markup).toMatch(/<div class="right">[\s\S]*activityPlayingTitle[\s\S]*<PopularGames\b/);
  });
});

describe('Activity: what the feed column holds while the first page loads', () => {
  it('shows the spinner named by what is loading', async () => {
    vi.mocked(feed).mockReturnValueOnce(new Promise(() => {}));
    authState.set('authenticated');
    renderDom(Activity);

    const spinner = (await screen.findByText(msg('transfers.activityLoading'))).closest('.spinner[role="status"]');
    expect(spinner).not.toBeNull();
    expect(within(document.body).queryByText(msg('transfers.activityFeedEmptyTitle'))).toBeNull();
  });
});
