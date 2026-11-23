import { Events } from '@wailsio/runtime';
import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { listSources, type Source } from '../../lib/services/sources';
import { initSources } from '../../lib/stores/sources';
import Sources from './Sources.svelte';
import { MAIN, MIRROR, mountPoint, resetScreen, rowNames } from './sourcesTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
// In the app: the store subscribes to Go's events and reads the list through the service.
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/sources', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/sources')>()),
  listSources: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = mountPoint(false);
  vi.mocked(Events.On).mockClear();
});

afterEach(() => {
  resetScreen();
  vi.mocked(listSources).mockReset();
});

/** Sends Go's `name` event with `data` to every handler the store set up for it. */
function emit(name: string, data: unknown): void {
  for (const [event, handler] of vi.mocked(Events.On).mock.calls) {
    if (event === name) (handler as (e: { data: unknown }) => void)({ data });
  }
}

const show = () => render(Sources, { target: app, props: {} });

describe('Sources: what Go says after the first read', () => {
  it('a source Go reports without a name has been removed: its row leaves', async () => {
    vi.mocked(listSources).mockResolvedValue([MAIN, MIRROR]);
    await initSources();
    show();
    expect(rowNames()).toEqual(['Main feed', 'Mirror']);

    emit('source:updated', { id: 's1', name: '' });

    await waitFor(() => expect(rowNames()).toEqual(['Mirror']));
  });

  it('a source Go reports changed takes its new state; one it reports new joins the list', async () => {
    vi.mocked(listSources).mockResolvedValue([MAIN]);
    await initSources();
    show();

    emit('source:updated', { ...MAIN, status: 'disabled', enabled: false } as Source);
    emit('source:updated', MIRROR);

    await waitFor(() => expect(rowNames()).toEqual(['Main feed', 'Mirror']));
    expect(screen.getByText(msg('transfers.sourcesStatusDisabled'))).toBeTruthy();
  });
});

// R1: until a read of the list succeeded, an empty list is not "no sources yet".
describe('Sources: the first read of the list', () => {
  const emptyTitle = () => screen.queryByRole('heading', { level: 2, name: msg('transfers.sourcesEmptyTitle') });
  const loadingTitle = () => screen.queryByRole('heading', { level: 2, name: msg('transfers.sourcesLoading') });
  const retry = () => screen.getByRole('button', { name: msg('common.retry') });

  async function failFirstRead() {
    vi.mocked(listSources).mockRejectedValueOnce(new Error('service not ready'));
    await initSources();
  }

  it('R1: says it is loading, not that there are no sources, until the first read answers', () => {
    vi.mocked(listSources).mockReturnValueOnce(new Promise(() => {}));
    void initSources();
    show();

    expect(loadingTitle()).toBeTruthy();
    expect(emptyTitle()).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('radiogroup')).toBeNull();
  });

  it('R1: says a failed first read as a danger notice with a Retry, and no empty list', async () => {
    await failFirstRead();
    show();

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain(msg('transfers.sourcesLoadFailed'));
    expect(alert.closest('.notice')?.getAttribute('data-tone')).toBe('danger');
    expect(retry()).toBeTruthy();
    expect(emptyTitle()).toBeNull();
    expect(loadingTitle()).toBeNull();
    expect(screen.getByRole('button', { name: msg('transfers.sourcesAddAction') })).toBeTruthy();
  });

  it('R1: Retry reads the list again and shows it, focus going to the first control of the list', async () => {
    await failFirstRead();
    vi.mocked(listSources).mockResolvedValueOnce([MAIN, MIRROR]);
    const { container } = show();
    const keys = (await import('../../lib/testing/dom')).user();
    retry().focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(rowNames()).toEqual(['Main feed', 'Mirror']));
    expect(screen.queryByRole('alert')).toBeNull();
    const list = container.querySelector('[data-source-list]') as HTMLElement;
    await waitFor(() => expect(list.contains(document.activeElement)).toBe(true));
  });

  it('R1: Retry that finds no source shows the empty list, focus going to the page heading', async () => {
    await failFirstRead();
    vi.mocked(listSources).mockResolvedValueOnce([]);
    show();
    const keys = (await import('../../lib/testing/dom')).user();
    retry().focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(emptyTitle()).toBeTruthy());
    expect(screen.queryByRole('alert')).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  it('R1: a Retry that fails again keeps the notice and says it again, focus staying on Retry', async () => {
    await failFirstRead();
    vi.mocked(listSources).mockRejectedValueOnce(new Error('still not ready'));
    show();
    const keys = (await import('../../lib/testing/dom')).user();
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    const region = notice.querySelector('.body') as HTMLElement;
    const before = region.firstElementChild;
    const button = retry();
    button.focus();

    await keys.keyboard('{Enter}');

    await waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(listSources).toHaveBeenCalledTimes(2);
    expect(notice.isConnected).toBe(true);
    expect(region.textContent).toContain(msg('transfers.sourcesLoadFailed'));
    expect(document.activeElement).toBe(button);
    expect(emptyTitle()).toBeNull();
  });

  it('R1: a source Go reports after a failed read is not drawn as the whole list', async () => {
    await failFirstRead();
    show();

    emit('source:updated', MAIN);

    await waitFor(() => expect(screen.getByRole('alert')).toBeTruthy());
    expect(rowNames()).toEqual([]);
    expect(screen.queryByRole('radiogroup')).toBeNull();
  });
});
