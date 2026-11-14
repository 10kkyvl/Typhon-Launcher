import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { limits, list, mine, remove, type Page } from '../../lib/services/reviews';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import GameReviews from './GameReviews.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/reviews', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/reviews')>()),
  list: vi.fn(),
  mine: vi.fn(),
  limits: vi.fn(),
  remove: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const empty: Page = { summary: { total: 0, positive: 0 } as Page['summary'], reviews: [], next: '' };

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


// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
// Read before the press; the getter answers the words of a message that came in since, '' until then.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

beforeEach(() => {
  // jsdom has no ResizeObserver; the sort and filter strips measure their plate with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  vi.mocked(list).mockReset();
  vi.mocked(mine).mockReset();
  vi.mocked(limits).mockReset();
  vi.mocked(remove).mockReset();
  authState.set('bootstrapping');
  toasts.set([]);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('GameReviews: loading more', () => {
  it('keeps the focus on its busy button while the next page loads', async () => {
    const next = deferred<Page>();
    vi.mocked(list).mockResolvedValueOnce({ ...empty, reviews: [review], next: 'p2' }).mockReturnValueOnce(next.promise);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    const more = await screen.findByRole('button', { name: msg('reviews.loadMore') });
    more.focus();

    await user.keyboard('{Enter}');

    expect(more.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(more);
    next.resolve({ ...empty, reviews: [{ ...review, id: 2 }], next: 'p3' });
    await vi.waitFor(() => expect(more.hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(more);
  });
});

const reviewsShown = () => [...document.querySelectorAll('.review .body')].map((node) => node.textContent);

describe('GameReviews: the game changes under the block', () => {
  it("takes the previous game's reviews away while the next game's load, and shows that it loads", async () => {
    const next = deferred<Page>();
    vi.mocked(list).mockResolvedValueOnce({ ...empty, reviews: [review], next: 'p2' }).mockReturnValueOnce(next.promise);
    const { rerender } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    await screen.findByText('Fine.');
    const section = document.getElementById('reviews');

    await rerender({ canonicalGameId: 'c2' });

    expect(document.getElementById('reviews')).toBe(section);
    expect(reviewsShown()).toEqual([]);
    expect(screen.queryByRole('button', { name: msg('reviews.loadMore') })).toBeNull();
    expect(screen.getByRole('status').textContent).toContain(msg('reviews.loading'));
    next.resolve({ ...empty, reviews: [{ ...review, id: 7, body: 'Other game.' }] });
    await vi.waitFor(() => expect(reviewsShown()).toEqual(['Other game.']));
  });

  it('holds the list of the same game through a change of order until the new order answers', async () => {
    const recent = deferred<Page>();
    vi.mocked(list).mockResolvedValueOnce({ ...empty, reviews: [review], next: 'p2' }).mockReturnValueOnce(recent.promise);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    await screen.findByText('Fine.');

    await user.click(screen.getByRole('radio', { name: msg('reviews.sortRecent') }));

    expect(reviewsShown()).toEqual(['Fine.']);
    expect(vi.mocked(list)).toHaveBeenLastCalledWith('c1', 'recent', 'all', '');
    recent.resolve({ ...empty, reviews: [{ ...review, id: 3, body: 'Newest.' }] });
    await vi.waitFor(() => expect(reviewsShown()).toEqual(['Newest.']));
  });
});

describe('GameReviews: a game the reviews service does not know, then another', () => {
  it("does not hide the next game's reviews, nor its failure", async () => {
    vi.mocked(list).mockRejectedValueOnce(new AccountError('unknown_game')).mockRejectedValueOnce(new Error('offline'));
    const { rerender } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    await vi.waitFor(() => expect(vi.mocked(list)).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(document.getElementById('reviews')).toBeNull());

    await rerender({ canonicalGameId: 'c2' });

    expect(await screen.findByText(msg('reviews.loadError'))).toBeTruthy();
    expect(document.getElementById('reviews')).not.toBeNull();
  });
});

describe("GameReviews: Load more and its key's cursor", () => {
  it('does not send the cursor of the previous order with the new one', async () => {
    const recent = deferred<Page>();
    vi.mocked(list).mockResolvedValueOnce({ ...empty, reviews: [review], next: 'p2' }).mockReturnValueOnce(recent.promise);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    const more = await screen.findByRole('button', { name: msg('reviews.loadMore') });

    await user.click(screen.getByRole('radio', { name: msg('reviews.sortRecent') }));
    await user.click(more);

    const calls = vi.mocked(list).mock.calls;
    expect(calls.filter(([, sort, , cursor]) => cursor === 'p2' && sort !== 'helpful')).toEqual([]);
    recent.resolve({ ...empty, reviews: [{ ...review, id: 3, body: 'Newest.' }], next: 'r2' });
    await vi.waitFor(() => expect(reviewsShown()).toEqual(['Newest.']));
  });

  it('drops a page of the previous order that answers late, and is not left busy', async () => {
    const page2 = deferred<Page>();
    const recent = deferred<Page>();
    vi.mocked(list)
      .mockResolvedValueOnce({ ...empty, reviews: [review], next: 'p2' })
      .mockReturnValueOnce(page2.promise)
      .mockReturnValueOnce(recent.promise);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    await user.click(await screen.findByRole('button', { name: msg('reviews.loadMore') }));
    await user.click(screen.getByRole('radio', { name: msg('reviews.sortRecent') }));

    recent.resolve({ ...empty, reviews: [{ ...review, id: 3, body: 'Newest.' }], next: 'r2' });
    await vi.waitFor(() => expect(reviewsShown()).toEqual(['Newest.']));
    page2.resolve({ ...empty, reviews: [{ ...review, id: 2, body: 'Older page.' }], next: 'p3' });

    await vi.waitFor(() => expect(screen.getByRole('button', { name: msg('reviews.loadMore') }).hasAttribute('aria-busy')).toBe(false));
    expect(reviewsShown()).toEqual(['Newest.']);
  });

  it('shows a review the next page repeats once', async () => {
    vi.mocked(list)
      .mockResolvedValueOnce({ ...empty, reviews: [review], next: 'p2' })
      .mockResolvedValueOnce({ ...empty, reviews: [review, { ...review, id: 2, body: 'Second.' }], next: '' });
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });

    await user.click(await screen.findByRole('button', { name: msg('reviews.loadMore') }));

    await vi.waitFor(() => expect(reviewsShown()).toEqual(['Fine.', 'Second.']));
  });
});

describe('GameReviews: retrying a failed load', () => {
  it('keeps the notice and the focus on its busy Retry until the answer, then focuses the first review', async () => {
    const again = deferred<Page>();
    vi.mocked(list).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    const notice = (await screen.findByText(msg('reviews.loadError'))).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(screen.queryByRole('status', { name: msg('reviews.loading') })).toBeNull();

    again.resolve({ ...empty, reviews: [review] });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('.review .who')));
  });

  it('keeps the notice when the retry fails again, and says it again', async () => {
    const again = deferred<Page>();
    vi.mocked(list).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    const notice = (await screen.findByText(msg('reviews.loadError'))).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    const said = sayingAgain(notice);
    retry.focus();
    await user.keyboard('{Enter}');

    again.reject(new Error('still offline'));

    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    await vi.waitFor(() => expect(said()).toBe(msg('reviews.loadError')));
  });
});

describe('GameReviews: deleting my review', () => {
  type MineAnswer = Awaited<ReturnType<typeof mine>>;
  const mineOf = (value: unknown) => value as MineAnswer;
  const myReview = { recommended: true, body: 'Mine.', hidden: false };

  async function deleteMine(after: MineAnswer) {
    authState.set('authenticated');
    vi.mocked(list).mockResolvedValue(empty);
    vi.mocked(limits).mockResolvedValue({ minBodyRunes: 10, maxBodyRunes: 2000 } as Awaited<ReturnType<typeof limits>>);
    vi.mocked(mine).mockResolvedValueOnce(mineOf({ review: myReview, eligibility: { canPost: true } })).mockResolvedValueOnce(after);
    vi.mocked(remove).mockResolvedValueOnce(undefined);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    const del = await screen.findByRole('button', { name: msg('reviews.composerDelete') });
    del.focus();
    await user.keyboard('{Enter}');
    const dialog = screen.getByRole('dialog', { name: msg('reviews.composerDeleteConfirmTitle') });
    within(dialog).getByRole('button', { name: msg('reviews.composerDeleteConfirmButton') }).focus();
    await user.keyboard('{Enter}');
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  }

  it('lands focus on the reviews heading when nothing of the composer is left to press', async () => {
    await deleteMine(mineOf({ review: null, eligibility: { canPost: false, reason: 'review_repost_cooldown', retryAt: new Date(Date.now() + 3_600_000).toISOString() } }));

    const heading = screen.getByRole('heading', { name: msg('reviews.sectionTitle') });
    await vi.waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it('lands focus on the verdict of a new review when one can be written at once', async () => {
    await deleteMine(mineOf({ review: null, eligibility: { canPost: true } }));

    const first = screen.getByRole('radio', { name: msg('reviews.composerRecommend') });
    await vi.waitFor(() => expect(document.activeElement).toBe(first));
  });

  it('keeps the dialog open until the refresh after the delete answers, then lands focus on the heading', async () => {
    let refreshed!: (value: MineAnswer) => void;
    authState.set('authenticated');
    vi.mocked(list).mockResolvedValue(empty);
    vi.mocked(limits).mockResolvedValue({ minBodyRunes: 10, maxBodyRunes: 2000 } as Awaited<ReturnType<typeof limits>>);
    vi.mocked(mine)
      .mockResolvedValueOnce(mineOf({ review: myReview, eligibility: { canPost: true } }))
      .mockReturnValueOnce(new Promise((resolve) => (refreshed = resolve)));
    vi.mocked(remove).mockResolvedValueOnce(undefined);
    const { user } = renderDom(GameReviews, { canonicalGameId: 'c1' });
    const del = await screen.findByRole('button', { name: msg('reviews.composerDelete') });
    del.focus();
    await user.keyboard('{Enter}');
    const dialog = screen.getByRole('dialog', { name: msg('reviews.composerDeleteConfirmTitle') });
    within(dialog).getByRole('button', { name: msg('reviews.composerDeleteConfirmButton') }).focus();

    await user.keyboard('{Enter}');
    await vi.waitFor(() => expect(vi.mocked(mine)).toHaveBeenCalledTimes(2));

    expect(dialog.isConnected).toBe(true);
    refreshed(mineOf({ review: null, eligibility: { canPost: false, reason: 'review_repost_cooldown', retryAt: new Date(Date.now() + 3_600_000).toISOString() } }));
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    const heading = screen.getByRole('heading', { name: msg('reviews.sectionTitle') });
    await vi.waitFor(() => expect(document.activeElement).toBe(heading));
  });
});
