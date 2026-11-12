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
  it('says so in a danger notice that announces itself', async () => {
    vi.mocked(list).mockRejectedValue(new Error('offline'));
    renderDom(ReviewSummary, { canonicalGameId: 'c1' });

    const alert = await screen.findByRole('alert');

    expect(alert.textContent?.trim()).toBe(msg('reviews.summaryError'));
    expect(alert.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
  });
});
