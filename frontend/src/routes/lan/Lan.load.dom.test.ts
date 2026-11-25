import { screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
// In the app: the store subscribes to Go's events and reads through the service.
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/lan', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/lan')>()),
  getPeers: vi.fn(),
  getOffers: vi.fn(),
  getShares: vi.fn(),
  getStats: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { getOffers, getPeers, getShares, getStats } from '../../lib/services/lan';
import { initLan } from '../../lib/stores/lan';
import { renderDom, user } from '../../lib/testing/dom';
import Lan from './Lan.svelte';
import { offer, resetScreen, sectionOf, sharingOn, stats } from './lanTesting';

beforeEach(() => {
  sharingOn();
  vi.mocked(getPeers).mockResolvedValue([]);
  vi.mocked(getOffers).mockResolvedValue([]);
  vi.mocked(getShares).mockResolvedValue([]);
  vi.mocked(getStats).mockResolvedValue(stats());
});

afterEach(() => {
  resetScreen();
  vi.useRealTimers();
  vi.mocked(getPeers).mockReset();
  vi.mocked(getOffers).mockReset();
  vi.mocked(getShares).mockReset();
  vi.mocked(getStats).mockReset();
});

const loadingTitle = () => screen.queryByRole('heading', { level: 2, name: msg('transfers.lanLoading') });
const nothingNearby = () => screen.queryByRole('heading', { level: 3, name: msg('transfers.lanEmptyTitle') });
const retry = () => screen.getByRole('button', { name: msg('common.retry') });

async function failFirstRead() {
  vi.mocked(getShares).mockRejectedValueOnce(new Error('service not ready'));
  await initLan();
}

describe('Lan: the first read', () => {
  it('says it is loading, not that nothing is nearby, until the first read answers', () => {
    vi.mocked(getShares).mockReturnValueOnce(new Promise(() => {}));
    void initLan();
    const { container } = renderDom(Lan);

    expect(loadingTitle()).toBeTruthy();
    expect(nothingNearby()).toBeNull();
    expect(screen.queryByText(msg('transfers.lanNoShares'))).toBeNull();
    expect(container.querySelector('.tile')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('says nothing is nearby only after a read that found nothing', async () => {
    await initLan();
    renderDom(Lan);

    expect(loadingTitle()).toBeNull();
    expect(nothingNearby()).toBeTruthy();
  });

  it('says a failed first read as a danger notice with a Retry, and no empty sections', async () => {
    await failFirstRead();
    renderDom(Lan);

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain(msg('transfers.lanLoadFailed'));
    expect(alert.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(retry()).toBeTruthy();
    expect(nothingNearby()).toBeNull();
    expect(loadingTitle()).toBeNull();
    expect(screen.queryByText(msg('transfers.lanNoShares'))).toBeNull();
  });

  it('says a refusal Go words by its code in the words of that code', async () => {
    vi.mocked(getShares).mockRejectedValueOnce(new Error('typhon:lan.library_unavailable: lan: library service unavailable'));
    await initLan();
    renderDom(Lan);

    expect(screen.getByRole('alert').textContent).toContain(msg('errSources.lanLibraryUnavailable'));
    expect(screen.getByRole('alert').textContent).not.toContain('library service unavailable');
  });

  it('Retry reads again and shows the sections, focus going to the first control they hold', async () => {
    await failFirstRead();
    vi.mocked(getOffers).mockResolvedValue([offer()]);
    renderDom(Lan);
    const keys = user();
    retry().focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(sectionOf('transfers.lanNearbyHeading')).toBeTruthy());
    expect(screen.queryByRole('alert')).toBeNull();
    await waitFor(() => expect(document.activeElement?.tagName).toBe('BUTTON'));
    expect(sectionOf('transfers.lanNearbyHeading').contains(document.activeElement)).toBe(true);
  });

  it('Retry that finds nothing shows the empty sections, focus going to the page heading', async () => {
    await failFirstRead();
    renderDom(Lan);
    const keys = user();
    retry().focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(nothingNearby()).toBeTruthy());
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });

  it('Retry that fails again keeps the notice and says it again, focus staying on Retry', async () => {
    await failFirstRead();
    vi.mocked(getShares).mockRejectedValueOnce(new Error('still not ready'));
    renderDom(Lan);
    const keys = user();
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    const region = notice.querySelector('.body') as HTMLElement;
    const before = region.firstElementChild;
    const button = retry();
    button.focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(getShares).toHaveBeenCalledTimes(2);
    expect(notice.isConnected).toBe(true);
    expect(region.textContent).toContain(msg('transfers.lanLoadFailed'));
    expect(document.activeElement).toBe(button);
    expect(nothingNearby()).toBeNull();
  });
});

describe('Lan: the offers poll', () => {
  it('reads the offers again while the screen is shown, and stops when it leaves', async () => {
    vi.useFakeTimers();
    await initLan();
    vi.mocked(getOffers).mockClear();
    const { unmount } = renderDom(Lan);

    await vi.advanceTimersByTimeAsync(10_000);
    const whileShown = vi.mocked(getOffers).mock.calls.length;
    unmount();
    await vi.advanceTimersByTimeAsync(30_000);

    expect(whileShown).toBeGreaterThanOrEqual(2);
    expect(vi.mocked(getOffers)).toHaveBeenCalledTimes(whileShown);
  });
});
