import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { list, type Page } from '../../lib/services/reviews';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import GameReviews from './GameReviews.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/reviews', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/reviews')>()),
  list: vi.fn(),
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

// The words a notice says again after its retry fails (Notice's assistive-only copy).
const saidAgain = (notice: Element) => notice.querySelector('[data-part="notice-again"]')?.textContent ?? '';

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
    retry.focus();
    await user.keyboard('{Enter}');

    again.reject(new Error('still offline'));

    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    await vi.waitFor(() => expect(saidAgain(notice)).toBe(msg('reviews.loadError')));
  });
});
