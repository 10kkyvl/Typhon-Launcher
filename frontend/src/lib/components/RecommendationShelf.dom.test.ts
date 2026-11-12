import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
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
});
