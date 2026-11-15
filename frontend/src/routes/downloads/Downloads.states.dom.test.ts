import { screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { listDownloads, type Download } from '../../lib/services/downloads';
import { downloads, downloadsLoaded, downloadsLoadError, initDownloads } from '../../lib/stores/downloads';
import { resetHistory, route } from '../../lib/stores/router';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// The list's first read is Go's answer to List: pending, read or refused.
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  listDownloads: vi.fn(),
}));

/** The first read refused with `error`; the page mounts after it. */
async function firstReadFails(error: Error) {
  vi.mocked(listDownloads).mockRejectedValueOnce(error);
  await initDownloads();
}

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: 'C:\\Games\\d1',
    status: 'completed',
    progress: 1,
    downloaded: 1000,
    total: 1000,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-09-08T00:00:00Z',
    completedAt: '2026-09-08T01:00:00Z',
    error: '',
    origin: {},
    ...patch,
  };
}

afterEach(() => {
  downloads.set([]);
  downloadsLoaded.set(false);
  downloadsLoadError.set(null);
  vi.mocked(listDownloads).mockReset();
  resetHistory();
  document.body.replaceChildren();
});

const emptyHeading = () => screen.queryByRole('heading', { name: msg('downloads.page.emptyTitle') });
const calmLine = () => screen.queryByText(msg('downloads.page.calm'));

describe('Downloads: before the list has answered', () => {
  it('shows a loading state, not "nothing here"', () => {
    vi.mocked(listDownloads).mockReturnValueOnce(new Promise(() => {}));
    void initDownloads();
    renderDom(Downloads);

    expect(screen.getByText(msg('downloads.page.loading'))).toBeTruthy();
    expect(emptyHeading()).toBeNull();
    expect(calmLine()).toBeNull();
  });

  it('shows what events brought meanwhile, not the loading state', () => {
    downloads.set([download({ id: 'q', name: 'Queued One', status: 'queued', progress: 0 })]);
    renderDom(Downloads);

    expect(screen.getByText('Queued One')).toBeTruthy();
    expect(screen.queryByText(msg('downloads.page.loading'))).toBeNull();
  });

  it('says a failed first read with a Retry, and no empty state', async () => {
    await firstReadFails(new Error('typhon:download.state_unread: bad json'));
    vi.mocked(listDownloads).mockResolvedValueOnce([]);
    const { user } = renderDom(Downloads);

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain(msg('downloads.page.loadFailed'));
    expect(alert.textContent).toContain(msg('errInstall.downloadStateUnread'));
    expect(emptyHeading()).toBeNull();
    expect(screen.queryByText(msg('downloads.page.loading'))).toBeNull();

    await user.click(screen.getByRole('button', { name: msg('common.retry') }));

    expect(listDownloads).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(emptyHeading()).not.toBeNull());
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('keeps the failure and its Retry when the read fails again, says it again, focus staying on Retry', async () => {
    await firstReadFails(new Error('boom'));
    vi.mocked(listDownloads).mockRejectedValueOnce(new Error('boom again'));
    const { user } = renderDom(Downloads);
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    // Said again: the message comes into the notice's live region again, as a new node.
    const region = notice.querySelector('.body') as HTMLElement;
    const before = region.firstElementChild;
    retry.focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.firstElementChild?.textContent).toContain(msg('downloads.page.loadFailed'));
    expect(listDownloads).toHaveBeenCalledTimes(2);
    expect(notice.isConnected).toBe(true);
    expect(notice.textContent).toContain(msg('downloads.page.loadFailedText'));
    expect(document.activeElement).toBe(retry);
    expect(emptyHeading()).toBeNull();
  });
});


describe('Downloads: nothing at all', () => {
  it('says so, leads to the stores and offers to add a download', async () => {
    downloadsLoaded.set(true);
    const { user } = renderDom(Downloads);
    const heading = emptyHeading() as HTMLElement;
    expect(heading.tagName).toBe('H2');
    const empty = heading.closest('.empty') as HTMLElement;
    expect(empty.textContent).toContain(msg('downloads.page.emptyText'));
    expect(calmLine()).toBeNull();

    await user.click(within(empty).getByRole('button', { name: msg('transfers.downloadsAddAction') }));
    expect(await screen.findByRole('dialog')).toBeTruthy();
    await user.keyboard('{Escape}');

    await user.click(within(empty).getByRole('button', { name: msg('downloads.page.stores') }));
    expect(get(route).name).toBe('catalog');
  });
});


describe('Downloads: something listed, nothing under way', () => {
  it('says the calm in one line where the active panels stand, with the way to the stores', async () => {
    downloads.set([download({ id: 'c', name: 'Done One' })]);
    const { user } = renderDom(Downloads);

    const calm = calmLine()?.closest('.calm') as HTMLElement;
    expect(calm).not.toBeNull();
    expect(emptyHeading()).toBeNull();
    expect(screen.queryByRole('heading', { name: new RegExp(`^${msg('transfers.downloadsActiveHeading')}`) })).toBeNull();

    await user.click(within(calm).getByRole('button', { name: msg('downloads.page.stores') }));
    expect(get(route).name).toBe('catalog');
  });

  it('has no calm line while something is under way', () => {
    downloads.set([download({ id: 'a', status: 'downloading', progress: 0.5, completedAt: null })]);
    renderDom(Downloads);
    expect(calmLine()).toBeNull();
  });
});
