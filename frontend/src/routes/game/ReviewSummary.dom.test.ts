import { render, screen, within } from '@testing-library/svelte';
import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { notifyReviewsChanged } from '../../lib/game/reviewsRefresh';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { list, type Page } from '../../lib/services/reviews';
import { renderDom, user } from '../../lib/testing/dom';
import GameReviews from './GameReviews.svelte';
import ReviewSummary from './ReviewSummary.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/reviews', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/reviews')>()),
  list: vi.fn(),
}));

beforeEach(() => {
  // jsdom has no ResizeObserver; the reviews block's sort and filter strips measure their plate with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  vi.mocked(list).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

const review = {
  id: 1,
  author: { username: 'mira', displayName: 'Mira', avatarUrl: '' },
  recommended: true,
  body: 'Fine.',
  playtimeSeconds: 0,
  helpful: 0,
  unhelpful: 0,
  myVote: '',
  mine: false,
  createdAt: '2026-11-01T10:00:00Z',
  updatedAt: '2026-11-01T10:00:00Z',
} as unknown as Page['reviews'][number];

const answer = (total: number, positive: number): Page => ({ summary: { total, positive } as Page['summary'], reviews: [review], next: '' });
const scoreLine = (total: number, percent: number) =>
  msg('reviews.summaryLine', { percent, reviews: msg('reviews.reviewsCount', { count: total }) });

describe('ReviewSummary: the summary failed to load', () => {
  // The reviews list below fails on the same request and announces it with its Retry: the summary
  // states it in the flow of the page instead of saying it twice.
  it('says so in a danger notice read with the page, not announced', async () => {
    vi.mocked(list).mockRejectedValue(new Error('offline'));
    renderDom(ReviewSummary, { canonicalGameId: 'c1' });

    const notice = (await screen.findByText(msg('reviews.summaryError'))).closest('.notice');

    expect(notice?.getAttribute('data-tone')).toBe('danger');
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
  });
});

describe('ReviewSummary and the reviews list on one page', () => {
  it('make one request to the reviews service per page open and per change of reviews', async () => {
    const first = deferred<Page>();
    const again = deferred<Page>();
    vi.mocked(list).mockReturnValueOnce(first.promise).mockReturnValueOnce(again.promise);
    renderDom(ReviewSummary, { canonicalGameId: 'c1' });
    renderDom(GameReviews, { canonicalGameId: 'c1' });

    expect(vi.mocked(list).mock.calls).toEqual([['c1', 'helpful', 'all', '']]);
    first.resolve(answer(10, 9));
    await screen.findByText(scoreLine(10, 90));
    await screen.findByText('Fine.');

    notifyReviewsChanged();
    flushSync();

    expect(vi.mocked(list).mock.calls).toEqual([['c1', 'helpful', 'all', ''], ['c1', 'helpful', 'all', '']]);
    again.resolve(answer(11, 10));
    await screen.findByText(scoreLine(11, 91));
  });
});

describe('ReviewSummary: retrying a failed summary', () => {
  it('offers a Retry that loads it again and shows the score', async () => {
    vi.mocked(list).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(answer(4, 1));
    const { user } = renderDom(ReviewSummary, { canonicalGameId: 'c1' });
    const notice = (await screen.findByText(msg('reviews.summaryError'))).closest('.notice') as HTMLElement;

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));

    await screen.findByText(scoreLine(4, 25));
    expect(notice.isConnected).toBe(false);
  });

  // On a page with a heading, the notice would hand focus to that heading: the score that came in its
  // place is closer to where the player was.
  it('lands focus on the score that came in its place when the score can be pressed', async () => {
    const page = document.createElement('main');
    const title = document.createElement('h1');
    title.tabIndex = -1;
    title.textContent = 'Game';
    page.append(title);
    document.body.append(page);
    try {
      vi.mocked(list).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(answer(4, 1));
      render(ReviewSummary, { target: page, props: { canonicalGameId: 'c1', onopen: () => {} } });
      const keys = user();
      const notice = (await screen.findByText(msg('reviews.summaryError'))).closest('.notice') as HTMLElement;
      within(notice).getByRole('button', { name: msg('common.retry') }).focus();

      await keys.keyboard('{Enter}');

      const score = (await screen.findByText(scoreLine(4, 25))).closest('button');
      await vi.waitFor(() => expect(document.activeElement).toBe(score));
    } finally {
      page.remove();
    }
  });
});

describe('ReviewSummary: the game changes under the card', () => {
  it("does not stay hidden for the next game because the previous one was unknown", async () => {
    vi.mocked(list).mockRejectedValueOnce(new AccountError('unknown_game')).mockRejectedValueOnce(new Error('offline'));
    const { rerender } = renderDom(ReviewSummary, { canonicalGameId: 'c1' });
    await vi.waitFor(() => expect(vi.mocked(list)).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(screen.queryByText(msg('reviews.summaryTitle'))).toBeNull());

    await rerender({ canonicalGameId: 'c2' });

    expect(await screen.findByText(msg('reviews.summaryError'))).toBeTruthy();
  });

  it("does not show the previous game's score while the next one's loads", async () => {
    const next = deferred<Page>();
    vi.mocked(list).mockResolvedValueOnce(answer(10, 9)).mockReturnValueOnce(next.promise);
    const { rerender } = renderDom(ReviewSummary, { canonicalGameId: 'c1' });
    await screen.findByText(scoreLine(10, 90));

    await rerender({ canonicalGameId: 'c2' });

    expect(screen.queryByText(scoreLine(10, 90))).toBeNull();
    next.resolve(answer(4, 1));
    await screen.findByText(scoreLine(4, 25));
  });
});
