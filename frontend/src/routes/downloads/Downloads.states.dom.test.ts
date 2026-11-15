import { screen, waitFor, within } from '@testing-library/svelte';
import { get, type Writable } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Download } from '../../lib/services/downloads';
import { downloads } from '../../lib/stores/downloads';
import { resetHistory, route } from '../../lib/stores/router';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';
import { downloadsListLoad, reloadDownloadsList, type DownloadsListLoad } from './listLoad';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
// The list's first read is driven by hand: loading, read, failed.
vi.mock('./listLoad', async () => {
  const { writable } = await import('svelte/store');
  return { downloadsListLoad: writable({ loaded: true, error: null }), reloadDownloadsList: vi.fn() };
});

const listLoad = downloadsListLoad as Writable<DownloadsListLoad>;

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
  listLoad.set({ loaded: true, error: null });
  vi.mocked(reloadDownloadsList).mockReset();
  resetHistory();
  document.body.replaceChildren();
});

const emptyHeading = () => screen.queryByRole('heading', { name: msg('downloads.page.emptyTitle') });
const calmLine = () => screen.queryByText(msg('downloads.page.calm'));

describe('Downloads: before the list has answered', () => {
  it('shows a loading state, not "nothing here"', () => {
    listLoad.set({ loaded: false, error: null });
    renderDom(Downloads);

    expect(screen.getByText(msg('downloads.page.loading'))).toBeTruthy();
    expect(emptyHeading()).toBeNull();
    expect(calmLine()).toBeNull();
  });

  it('shows what events brought meanwhile, not the loading state', () => {
    listLoad.set({ loaded: false, error: null });
    downloads.set([download({ id: 'q', name: 'Queued One', status: 'queued', progress: 0 })]);
    renderDom(Downloads);

    expect(screen.getByText('Queued One')).toBeTruthy();
    expect(screen.queryByText(msg('downloads.page.loading'))).toBeNull();
  });

  it('says a failed first read with a Retry, and no empty state', async () => {
    listLoad.set({ loaded: false, error: new Error('typhon:download.state_unread: bad json') });
    vi.mocked(reloadDownloadsList).mockImplementation(async () => {
      listLoad.set({ loaded: true, error: null });
      return true;
    });
    const { user } = renderDom(Downloads);

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain(msg('downloads.page.loadFailed'));
    expect(alert.textContent).toContain(msg('errInstall.downloadStateUnread'));
    expect(emptyHeading()).toBeNull();
    expect(screen.queryByText(msg('downloads.page.loading'))).toBeNull();

    await user.click(screen.getByRole('button', { name: msg('common.retry') }));

    expect(reloadDownloadsList).toHaveBeenCalledOnce();
    await waitFor(() => expect(emptyHeading()).not.toBeNull());
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('keeps the failure and its Retry when the read fails again, says it again, focus staying on Retry', async () => {
    listLoad.set({ loaded: false, error: new Error('boom') });
    vi.mocked(reloadDownloadsList).mockImplementation(async () => {
      listLoad.set({ loaded: false, error: new Error('boom again') });
      return false;
    });
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
    expect(reloadDownloadsList).toHaveBeenCalledOnce();
    expect(notice.isConnected).toBe(true);
    expect(notice.textContent).toContain(msg('downloads.page.loadFailedText'));
    expect(document.activeElement).toBe(retry);
    expect(emptyHeading()).toBeNull();
  });
});


describe('Downloads: nothing at all', () => {
  it('says so, leads to the stores and offers to add a download', async () => {
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
