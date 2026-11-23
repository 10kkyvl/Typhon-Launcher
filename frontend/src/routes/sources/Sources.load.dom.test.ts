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
  app = mountPoint();
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

describe('Sources: the first read of the list', () => {
  // R1: the store's first read rejects, and the page draws the failure as a list with nothing in it.
  it('R1: a failed first read shows the empty list', async () => {
    vi.mocked(listSources).mockRejectedValue(new Error('service not ready'));
    await expect(initSources()).rejects.toThrow('service not ready');
    show();

    expect(screen.getByRole('heading', { level: 2, name: msg('transfers.sourcesEmptyTitle') })).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
