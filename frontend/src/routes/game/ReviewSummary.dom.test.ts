import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { list } from '../../lib/services/reviews';
import { renderDom } from '../../lib/testing/dom';
import ReviewSummary from './ReviewSummary.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/reviews', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/reviews')>()),
  list: vi.fn(),
}));

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  vi.mocked(list).mockReset();
  vi.restoreAllMocks();
});

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
