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

describe('GameReviews: retrying a failed load', () => {
  it('keeps the notice and the focus on its busy Retry until the answer, then focuses the section', async () => {
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

    again.resolve(empty);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { name: msg('reviews.sectionTitle') })));
  });

  it('keeps the notice when the retry fails again', async () => {
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
  });
});
