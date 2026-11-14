import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import type { Recommendation } from '../recommendations/display';
import RecommendationShelf from './RecommendationShelf.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

describe('RecommendationShelf while it looks for games', () => {
  it('shows the base spinner named by the search, and keeps its focused refresh busy', async () => {
    const onrefresh = vi.fn();
    const { user, rerender } = renderDom(RecommendationShelf, { title: 'Picks', items: [], emptyText: 'None', ondismiss: () => {}, onrefresh, loading: false });
    const refresh = screen.getByRole('button', { name: msg('games.recommendationRefresh') });
    await user.click(refresh);
    expect(onrefresh).toHaveBeenCalledOnce();

    await rerender({ title: 'Picks', items: [], emptyText: 'None', ondismiss: () => {}, onrefresh, loading: true });

    const status = screen.getByRole('status');
    expect(status.querySelector('.spinner')).toBeTruthy();
    expect(status.textContent?.trim()).toBe(msg('games.recommendationLoading'));
    expect(refresh.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(refresh);
  });

  // A live region inserted together with its words may not be read: the one that says the search
  // found nothing is the one that said it was looking.
  it('says it found nothing in the region that said it was looking', async () => {
    const props = { title: 'Picks', items: [], emptyText: 'Nothing to suggest', ondismiss: () => {}, loading: true };
    const { rerender } = renderDom(RecommendationShelf, props);
    const region = screen.getByRole('status');

    await rerender({ ...props, loading: false });

    expect(screen.getByRole('status')).toBe(region);
    expect(region.textContent?.trim()).toBe('Nothing to suggest');
  });

  // Its "Not interested" waits on the same search: a request runs, nothing is unavailable.
  it("keeps each pick's Not interested busy while it looks, not disabled", async () => {
    const item: Recommendation = { game: { id: 'c1', title: 'Hades', sortTitle: 'hades', createdAt: '2026-01-01T00:00:00Z' }, reason: 'popular' };
    const ondismiss = vi.fn();
    const props = { title: 'Picks', items: [item], emptyText: 'None', ondismiss, loading: true };
    const { user } = renderDom(RecommendationShelf, props);
    const dismiss = screen.getByRole('button', { name: msg('games.recommendationNotInterested') });

    expect(dismiss.getAttribute('aria-busy')).toBe('true');
    expect(dismiss.hasAttribute('aria-disabled')).toBe(false);
    await user.click(dismiss);
    expect(ondismiss).not.toHaveBeenCalled();
  });
});
