import { cleanup, screen, within } from '@testing-library/svelte';
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
import { applyStyles } from '../../lib/testing/styles';
import statTileSource from '../../lib/components/StatTile.svelte?raw';
import { playtime, relativeDate, shortDate } from '../../lib/utils/format';
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
  // Unmounted first: a reset under the open screen reads the feed again.
  cleanup();
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
    const day = (label: string) => screen.getByRole('heading', { name: (name) => name.startsWith(label) });
    const first = day(labels[0]);
    const second = day(labels[1]);
    const order = (a: Node, b: Node) => a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING;
    expect(order(first, hades)).toBeTruthy();
    expect(order(hades, second)).toBeTruthy();
    expect(order(second, celeste)).toBeTruthy();
  });

  it('heads each day with a section heading under the page title, its count of events beside it', async () => {
    const events = [event(1, now(), 'Hades'), event(2, now(), 'Celeste'), event(3, daysAgo(1), 'Tunic')];
    vi.mocked(feed).mockResolvedValueOnce(page(events));
    authState.set('authenticated');
    renderDom(Activity);
    await feedColumn().findByText('Tunic');

    const today = relativeDate(events[0].createdAt);
    const yesterday = relativeDate(events[2].createdAt);
    const days = feedColumn().getAllByRole('heading', { level: 2 });
    expect(days.map((heading) => heading.textContent?.replace(/\s+/g, ' ').trim())).toEqual([`${today} 2`, `${yesterday} 1`]);
    const todaySection = feedColumn().getByRole('region', { name: `${today} 2` });
    expect(within(todaySection).getByText('Hades')).toBeTruthy();
    expect(within(todaySection).getByText('Celeste')).toBeTruthy();
    expect(within(todaySection).queryByText('Tunic')).toBeNull();
    expect(feedColumn().queryAllByRole('heading', { level: 4 })).toHaveLength(0);
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

  it('a failed "Show more" keeps the list and the button, and says so beside it once, not in a toast', async () => {
    vi.mocked(feed).mockResolvedValueOnce(page([event(1, now(), 'Hades')], 5)).mockRejectedValueOnce(new Error('offline'));
    authState.set('authenticated');
    const { user } = renderDom(Activity);

    const more = await screen.findByRole('button', { name: msg('transfers.activityShowMore') });
    await user.click(more);

    await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(more.getAttribute('aria-busy')).toBeNull());
    expect(feedColumn().getByText('Hades')).toBeTruthy();
    const said = await feedColumn().findByRole('alert');
    expect(said.textContent).toContain(msg('transfers.activityMoreFailed'));
    expect(said.closest('.more')).toBe(more.closest('.more'));
    expect(get(toasts)).toEqual([]);
    expect(feedColumn().getAllByText(msg('transfers.activityMoreFailed'))).toHaveLength(1);
  });

  it('a "Show more" pressed again takes the failure away while it loads', async () => {
    vi.mocked(feed)
      .mockResolvedValueOnce(page([event(1, now(), 'Hades')], 5))
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise(() => {}));
    authState.set('authenticated');
    const { user } = renderDom(Activity);
    const more = await screen.findByRole('button', { name: msg('transfers.activityShowMore') });
    await user.click(more);
    await feedColumn().findByText(msg('transfers.activityMoreFailed'));

    await user.click(more);

    await vi.waitFor(() => expect(feedColumn().queryByText(msg('transfers.activityMoreFailed'))).toBeNull());
    expect(document.activeElement).toBe(more);
  });
});

describe('Activity: coming back to the feed', () => {
  it('a failed "Show more" is not said again once a later visit has read the first page anew', async () => {
    vi.mocked(feed)
      .mockResolvedValueOnce(page([event(1, now(), 'Hades')], 5))
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(page([event(1, now(), 'Hades')], 5));
    authState.set('authenticated');
    const first = renderDom(Activity);
    await first.user.click(await screen.findByRole('button', { name: msg('transfers.activityShowMore') }));
    await feedColumn().findByText(msg('transfers.activityMoreFailed'));
    first.unmount();

    renderDom(Activity);

    await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(3));
    await vi.waitFor(() => expect(screen.getByRole('button', { name: msg('transfers.activityShowMore') }).getAttribute('aria-busy')).toBeNull());
    expect(feedColumn().getByText('Hades')).toBeTruthy();
    expect(feedColumn().queryByText(msg('transfers.activityMoreFailed'))).toBeNull();
  });
});

describe('Activity: a feed that could not be read', () => {
  it('a failed first page is a danger notice with Retry, not the "no friends yet" state, and no toast', async () => {
    vi.mocked(feed).mockRejectedValueOnce(new Error('offline'));
    authState.set('authenticated');
    renderDom(Activity);

    const notice = (await feedColumn().findByRole('alert')).closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');
    expect(notice.textContent).toContain(msg('state.feedLoadFailed'));
    expect(within(notice).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(feedColumn().queryByText(msg('transfers.activityFeedEmptyTitle'))).toBeNull();
    expect(feedColumn().queryByRole('button', { name: msg('social.friendsAddFriend') })).toBeNull();
    expect(get(toasts)).toEqual([]);
  });

  it('a retry that brings the feed shows it and hands the focus into it', async () => {
    vi.mocked(feed).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(page([event(1, now(), 'Hades')]));
    authState.set('authenticated');
    const { user } = renderDom(Activity);
    const notice = (await feedColumn().findByRole('alert')).closest('.notice') as HTMLElement;
    within(notice).getByRole('button', { name: msg('common.retry') }).focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(feedColumn().getByText('Hades')).toBeTruthy();
    await vi.waitFor(() => expect(feedColumn().getAllByRole('button')).toContain(document.activeElement));
  });

  it('a retry that fails again keeps the notice and the focus on its Retry', async () => {
    vi.mocked(feed).mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new Error('still offline'));
    authState.set('authenticated');
    const { user } = renderDom(Activity);
    const region = await feedColumn().findByRole('alert');
    const notice = region.closest('.notice') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    // Said again: the failure's words come into the live region as a new node.
    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent).toContain(msg('state.feedLoadFailed'));
    expect(get(toasts)).toEqual([]);
  });

  it('says "no friends yet" only after a read that came back with nothing', async () => {
    vi.mocked(feed).mockResolvedValueOnce(page([]));
    authState.set('authenticated');
    renderDom(Activity);

    expect(await feedColumn().findByText(msg('transfers.activityFeedEmptyTitle'))).toBeTruthy();
    expect(feedColumn().getByRole('button', { name: msg('social.friendsAddFriend') })).toBeTruthy();
    expect(feedColumn().queryByRole('alert')).toBeNull();
  });
});

describe('Activity: a feed cleared under the open screen', () => {
  // What the social store does on every change of the auth state (a reconnect, another account).
  const reconnect = () => {
    authState.set('authenticated');
    resetFeed();
  };

  it('asks for the first page again, once, and shows what it brought', async () => {
    vi.mocked(feed).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(page([event(1, now(), 'Hades')]));
    authState.set('offline');
    renderDom(Activity);
    await feedColumn().findByRole('alert');

    reconnect();

    expect(await feedColumn().findByText('Hades')).toBeTruthy();
    expect(feed).toHaveBeenCalledTimes(2);
    expect(feedColumn().queryByRole('alert')).toBeNull();
    expect(feedColumn().queryByRole('status')).toBeNull();
  });

  it('a Retry under way when the feed is cleared answers with the read that follows, and focus goes into the feed', async () => {
    let fail = (_reason: Error) => {};
    vi.mocked(feed)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise<FeedPage>((_resolve, reject) => (fail = reject)))
      .mockResolvedValueOnce(page([event(1, now(), 'Hades')]));
    authState.set('offline');
    const { user } = renderDom(Activity);
    const notice = (await feedColumn().findByRole('alert')).closest('.notice') as HTMLElement;
    within(notice).getByRole('button', { name: msg('common.retry') }).focus();
    await user.keyboard('{Enter}');
    await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(2));

    reconnect();
    await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(3));
    fail(new Error('the old read, cut short'));

    expect(await feedColumn().findByText('Hades')).toBeTruthy();
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(feedColumn().getAllByRole('button')).toContain(document.activeElement));
    expect(feed).toHaveBeenCalledTimes(3);
  });

  it('focus on the Retry of a notice the clearing takes away goes to the page heading, and stays there when the feed comes', async () => {
    let answer = (_page: FeedPage) => {};
    vi.mocked(feed)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise<FeedPage>((resolve) => (answer = resolve)));
    authState.set('offline');
    renderDom(Activity);
    const notice = (await feedColumn().findByRole('alert')).closest('.notice') as HTMLElement;
    within(notice).getByRole('button', { name: msg('common.retry') }).focus();

    reconnect();

    await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(2));
    expect(await feedColumn().findByText(msg('transfers.activityLoading'))).toBeTruthy();
    expect(notice.isConnected).toBe(false);
    const heading = screen.getByRole('heading', { level: 1, name: msg('transfers.activityTitle') });
    await vi.waitFor(() => expect(document.activeElement).toBe(heading));

    answer(page([event(1, now(), 'Hades')]));

    expect(await feedColumn().findByText('Hades')).toBeTruthy();
    expect(document.activeElement).toBe(heading);
  });

  it('a clearing leaves alone focus that was not in the feed column', async () => {
    vi.mocked(feed).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(new Promise(() => {}));
    authState.set('offline');
    renderDom(Activity);
    await feedColumn().findByRole('alert');
    const heading = screen.getByRole('heading', { level: 1, name: msg('transfers.activityTitle') });
    const outside = document.createElement('button');
    document.body.append(outside);
    outside.focus();

    reconnect();

    expect(await feedColumn().findByText(msg('transfers.activityLoading'))).toBeTruthy();
    expect(document.activeElement).toBe(outside);
    expect(heading).not.toBe(document.activeElement);
    outside.remove();
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

  const valueLabels = (container: HTMLElement) =>
    [...container.querySelectorAll('.chart-box .col')].map((col) => col.querySelector('.value')?.textContent?.trim() ?? null);

  it('is the shared bar chart: seven days, today set apart, no axis, each day a bar of one choice, nothing chosen', () => {
    const week = weekSummary([played('2026-11-10', 3600), played('2026-11-11', 1800)], new Date(2026, 10, 11));
    const { container } = renderDom(WeekActivity, { week });

    const group = screen.getByRole('radiogroup', { name: msg('transfers.activityWeekTitle') });
    const bars = within(group).getAllByRole('radio');
    expect(bars).toHaveLength(7);
    expect(bars.map((bar) => bar.getAttribute('aria-checked'))).toEqual(Array(7).fill('false'));
    expect(bars.filter((bar) => bar.tabIndex === 0)).toEqual([bars[6]]);
    expect(bars.map((bar) => bar.classList.contains('current'))).toEqual([false, false, false, false, false, false, true]);
    expect(container.querySelector('.axis')).toBeNull();
    expect(valueLabels(container)).toEqual(Array(7).fill(null));
    expect(screen.queryByRole('table')).toBeNull();
    expect(container.querySelector('[title]')).toBeNull();
  });

  it('the arrows move the choice along the days, and the chosen day shows its time over its bar', async () => {
    const week = weekSummary([played('2026-11-10', 3600), played('2026-11-11', 1800)], new Date(2026, 10, 11));
    const { container, user } = renderDom(WeekActivity, { week });
    const bars = within(screen.getByRole('radiogroup', { name: msg('transfers.activityWeekTitle') })).getAllByRole('radio');

    await user.tab();
    expect(document.activeElement).toBe(bars[6]);
    await user.keyboard('{ArrowLeft}');

    expect(document.activeElement).toBe(bars[5]);
    expect(bars[5].getAttribute('aria-checked')).toBe('true');
    expect(valueLabels(container)).toEqual([null, null, null, null, null, playtime(3600), null]);

    await user.keyboard('{End}');
    expect(valueLabels(container)).toEqual([null, null, null, null, null, null, playtime(1800)]);
  });

  it('a day says its date and time in a hint when the pointer rests on its bar', async () => {
    const week = weekSummary([played('2026-11-10', 3600)], new Date(2026, 10, 11));
    const { user } = renderDom(WeekActivity, { week });
    const bars = within(screen.getByRole('radiogroup', { name: msg('transfers.activityWeekTitle') })).getAllByRole('radio');
    const monday = week.days[5];

    await user.hover(bars[5]);

    await vi.waitFor(() =>
      expect(document.querySelector('[data-part="tooltip"]')?.textContent?.trim()).toBe(
        msg('common.chartBucket', { label: shortDate(monday.at), value: playtime(3600) }),
      ),
    );
  });
});

describe('Activity: the games of the week', () => {
  const entry = (id: string, title: string, seconds: number, archived = false) => ({
    game: { id, title, cover: '', archived, playtimeSeconds: seconds, status: '' },
    seconds,
  });

  it('a title leads to its game; an archived one is plain text; the time stays with each', async () => {
    const week = weekSummary([{ date: '2026-11-10', entries: [entry('g1', 'Hades', 3600), entry('old', 'Archived game', 1800, true)] }], new Date(2026, 10, 11));
    const { user } = renderDom(WeekActivity, { week });

    await user.click(screen.getByRole('button', { name: 'Hades' }));

    expect(navigate).toHaveBeenCalledWith('game', { id: 'g1' });
    expect(screen.queryByRole('button', { name: /Archived game/ })).toBeNull();
    const rows = screen.getAllByRole('listitem').map((row) => row.textContent?.replace(/\s+/g, ' ').trim());
    expect(rows).toHaveLength(2);
    expect(rows[1]).toContain('Archived game');
  });
});

describe('Activity: the profile card', () => {
  it('centres its three figures under the centred name, through the tile itself', () => {
    vi.mocked(feed).mockReturnValueOnce(new Promise(() => {}));
    authState.set('authenticated');
    const { container } = renderDom(Activity);

    const tiles = [...container.querySelectorAll<HTMLElement>('.left .tile')];
    expect(tiles.map((tile) => tile.querySelector('.label')?.textContent)).toEqual([
      msg('transfers.activityStatGames'),
      msg('transfers.activityStatHours'),
      msg('transfers.activityStatCompleted'),
    ]);
    applyStyles(tiles[0], statTileSource);
    expect(tiles.map((tile) => [getComputedStyle(tile).alignItems, getComputedStyle(tile).textAlign])).toEqual(Array(3).fill(['center', 'center']));
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
    expect(markup).toMatch(/<div class="left">[\s\S]*<WeekActivity\b[\s\S]*<div class="center"[ >]/);
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

  it('never says "no friends yet", not even for a moment, before a read has come back, on entry or after a clearing', async () => {
    let answer = (_page: FeedPage) => {};
    vi.mocked(feed)
      .mockReturnValueOnce(new Promise(() => {}))
      .mockReturnValueOnce(new Promise<FeedPage>((resolve) => (answer = resolve)));
    // Nodes keep their words after they leave the page: one that came and went is still seen here.
    const seen: Node[] = [];
    const observer = new MutationObserver((records) => {
      for (const record of records) seen.push(...record.addedNodes, ...record.removedNodes);
    });
    observer.observe(document.body, { childList: true, subtree: true });
    const said = () => seen.some((node) => node.textContent?.includes(msg('transfers.activityFeedEmptyTitle')));
    try {
      authState.set('authenticated');
      renderDom(Activity);
      await feedColumn().findByText(msg('transfers.activityLoading'));

      resetFeed();
      await vi.waitFor(() => expect(feed).toHaveBeenCalledTimes(2));
      await feedColumn().findByText(msg('transfers.activityLoading'));
      await Promise.resolve();
      expect(said()).toBe(false);

      answer(page([]));
      expect(await feedColumn().findByText(msg('transfers.activityFeedEmptyTitle'))).toBeTruthy();
      expect(said()).toBe(true);
    } finally {
      observer.disconnect();
    }
  });
});
