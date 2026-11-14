import { afterEach, describe, expect, it, vi } from 'vitest';
import { list, type Page } from '../services/reviews';
import { firstReviewsPage } from './reviewsFeed';
import { notifyReviewsChanged } from './reviewsRefresh';

vi.mock('../services/reviews', () => ({ list: vi.fn() }));

const page = (total: number): Page => ({ summary: { total, positive: total } as Page['summary'], reviews: [], next: '' });

afterEach(() => {
  vi.mocked(list).mockReset();
});

describe('firstReviewsPage', () => {
  it('asks once for two asks of the same key in flight, and both get its answer', async () => {
    vi.mocked(list).mockResolvedValueOnce(page(3));

    const [a, b] = await Promise.all([firstReviewsPage('g', 'helpful', 'all'), firstReviewsPage('g', 'helpful', 'all')]);

    expect(vi.mocked(list).mock.calls).toEqual([['g', 'helpful', 'all', '']]);
    expect([a.summary.total, b.summary.total]).toEqual([3, 3]);
  });

  it('asks again once the answer has come, so a retry is a new request', async () => {
    vi.mocked(list).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(page(1));
    await expect(firstReviewsPage('g', 'helpful', 'all')).rejects.toThrow('offline');

    const again = await firstReviewsPage('g', 'helpful', 'all');

    expect(again.summary.total).toBe(1);
    expect(vi.mocked(list)).toHaveBeenCalledTimes(2);
  });

  it('does not answer an ask after a change of reviews with a request from before it', async () => {
    let before!: (value: Page) => void;
    vi.mocked(list).mockReturnValueOnce(new Promise((resolve) => (before = resolve))).mockResolvedValueOnce(page(2));
    const old = firstReviewsPage('g', 'helpful', 'all');

    notifyReviewsChanged();
    const fresh = await firstReviewsPage('g', 'helpful', 'all');
    before(page(1));

    expect(fresh.summary.total).toBe(2);
    expect((await old).summary.total).toBe(1);
  });

  it('keeps games, orders and filters apart', async () => {
    vi.mocked(list).mockResolvedValue(page(0));

    await Promise.all([
      firstReviewsPage('g', 'helpful', 'all'),
      firstReviewsPage('h', 'helpful', 'all'),
      firstReviewsPage('g', 'recent', 'all'),
      firstReviewsPage('g', 'helpful', 'negative'),
    ]);

    expect(vi.mocked(list)).toHaveBeenCalledTimes(4);
  });
});
