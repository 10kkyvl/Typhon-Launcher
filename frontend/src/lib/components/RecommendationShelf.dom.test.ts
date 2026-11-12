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
    const { user, rerender } = renderDom(RecommendationShelf, { title: 'Picks', items: [], emptyText: 'None', ondismiss: () => {}, onrefresh });
    const refresh = screen.getByRole('button', { name: msg('games.recommendationRefresh') });
    await user.click(refresh);
    expect(onrefresh).toHaveBeenCalledOnce();

    await rerender({ title: 'Picks', items: [], emptyText: 'None', ondismiss: () => {}, onrefresh, loading: true });

    const status = screen.getByRole('status');
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('games.recommendationLoading'));
    expect(refresh.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(refresh);
  });
});
