import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  cancelDownload: vi.fn(),
  pauseDownload: vi.fn(),
  resumeDownload: vi.fn(),
  removeDownload: vi.fn(),
  getDownload: vi.fn(() => new Promise(() => {})),
}));
vi.mock('../../lib/services/install', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/install')>()),
  retryInstall: vi.fn(),
}));

import { msg } from '../../lib/i18n';
import { cancelDownload, pauseDownload, removeDownload, resumeDownload, type Download } from '../../lib/services/downloads';
import { retryInstall, type Installation } from '../../lib/services/install';
import { downloads } from '../../lib/stores/downloads';
import { installations } from '../../lib/stores/install';
import { resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import Downloads from './Downloads.svelte';

function download(patch: Partial<Download> = {}): Download {
  return {
    id: 'd1',
    name: 'Game One',
    type: 'torrent',
    source: 'magnet:?xt=urn:btih:hash1',
    infoHash: 'hash1',
    destination: 'C:\\Games\\d1',
    status: 'downloading',
    progress: 0.4,
    downloaded: 0,
    total: 1000,
    downloadSpeed: 100,
    uploadSpeed: 0,
    etaSeconds: 10,
    seeders: 1,
    peers: 1,
    files: [],
    seeding: false,
    stalled: false,
    stalledSince: null,
    addedAt: '2026-09-08T00:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...patch,
  };
}

const goSets = (id: string, patch: Partial<Download>) => downloads.update((list) => list.map((d) => (d.id === id ? { ...d, ...patch } : d)));
const panel = (name: string) => screen.getByRole('region', { name });
const grip = (title: string) => screen.getByRole('button', { name: new RegExp(`^«${title}», `) });
const cancelX = (title: string) => screen.getByRole('button', { name: msg('downloads.queue.cancelFor', { title }) });

let scrolled: HTMLElement[];

beforeEach(() => {
  scrolled = [];
  Element.prototype.scrollIntoView = vi.fn(function (this: HTMLElement) {
    scrolled.push(this);
  });
  for (const fn of [cancelDownload, pauseDownload, removeDownload, resumeDownload, retryInstall]) {
    vi.mocked(fn).mockReset();
    vi.mocked(fn).mockResolvedValue(undefined);
  }
});

afterEach(() => {
  downloads.set([]);
  installations.set([]);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
});

describe('Downloads: focus goes with a download its own press moves to another section', () => {
  it('Resume that queues the download takes focus to its grip in the queue', async () => {
    vi.mocked(resumeDownload).mockImplementation(async (id) => goSets(id, { status: 'queued', downloadSpeed: 0 }));
    downloads.set([download({ status: 'paused', downloadSpeed: 0 }), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);

    within(panel('Game One')).getByRole('button', { name: msg('downloads.panel.resumeNamed', { name: 'Game One' }) }).focus();
    await user.keyboard('{Enter}');

    await waitFor(() => expect(document.activeElement).toBe(grip('Game One')));
  });
});

describe('Downloads: a row Go takes away from under the focus', () => {
  const pauseOf = (name: string) => within(panel(name)).getByRole('button', { name: msg('downloads.panel.pauseNamed', { name }) });
  const moreOf = (name: string) => within(panel(name)).getByRole('button', { name: msg('downloads.panel.more', { name }) });

  it('the focused panel completes: focus goes to the same control of the panel now in its place', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    renderDom(Downloads);
    pauseOf('Game One').focus();

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });

    await waitFor(() => expect(document.activeElement).toBe(pauseOf('Game Two')));
    expect(scrolled).toContain(panel('Game Two'));
  });

  it('the only panel completes: focus goes to the page heading, not to the download’s new row', async () => {
    downloads.set([download(), download({ id: 'q', name: 'Queued One', status: 'queued' })]);
    renderDom(Downloads);
    moreOf('Game One').focus();

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });

    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });

  it('a pause the panel showed is not followed: a later completion goes to the panel now in its place', async () => {
    vi.mocked(pauseDownload).mockImplementation(async (id) => goSets(id, { status: 'paused', downloadSpeed: 0 }));
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);
    const toggle = pauseOf('Game One');
    toggle.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(toggle.getAttribute('aria-label')).toBe(msg('downloads.panel.resumeNamed', { name: 'Game One' })));

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });

    await waitFor(() => expect(document.activeElement).toBe(pauseOf('Game Two')));
  });

  it('a queued download that starts: focus goes to the grip of the row now in its place', async () => {
    downloads.set([download({ id: 'a', name: 'Alpha', status: 'queued' }), download({ id: 'b', name: 'Beta', status: 'queued' })]);
    renderDom(Downloads);
    grip('Alpha').focus();

    goSets('a', { status: 'downloading' });

    await waitFor(() => expect(document.activeElement).toBe(grip('Beta')));
  });

  it('the focused Pause goes as its download turns to verifying: focus stays in the panel, on its menu', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    renderDom(Downloads);
    pauseOf('Game One').focus();

    goSets('d1', { status: 'verifying', downloadSpeed: 0 });

    await waitFor(() => expect(document.activeElement).toBe(moreOf('Game One')));
    expect(within(panel('Game One')).queryByRole('button', { name: msg('downloads.panel.pauseNamed', { name: 'Game One' }) })).toBeNull();
  });

  it('an item of a panel’s open menu has focus and Go completes that download: focus goes on from the panel', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);
    moreOf('Game One').focus();
    await user.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: msg('downloads.panel.details') }).focus();

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });

    await waitFor(() => expect(document.activeElement).toBe(moreOf('Game Two')));
    expect(screen.queryByRole('menu')).toBeNull();
  });

  it('Clear the list has focus and its section empties by Go’s doing: focus goes to the page heading', async () => {
    downloads.set([download({ status: 'completed', completedAt: '2026-09-08T01:00:00Z' }), download({ id: 'd2', name: 'Game Two' })]);
    renderDom(Downloads);
    const clear = screen.getByRole('button', { name: msg('downloads.done.clear') });
    clear.focus();

    downloads.update((list) => list.filter((d) => d.id !== 'd1'));

    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });

  it('a section’s heading has focus and the section empties by Go’s doing: focus goes to the page heading', async () => {
    downloads.set([download({ status: 'failed', error: 'typhon:download.unavailable' }), download({ id: 'd2', name: 'Game Two' })]);
    renderDom(Downloads);
    const failedHeading = screen.getByRole('heading', { level: 2, name: new RegExp(`^${msg('transfers.downloadsFailedHeading')}`) });
    failedHeading.focus();

    goSets('d1', { status: 'queued' });

    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));
  });

  it('focus elsewhere on the page is left alone', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    renderDom(Downloads);
    const add = screen.getAllByRole('button', { name: msg('transfers.downloadsAddAction') })[0];
    add.focus();

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });
    await new Promise((done) => setTimeout(done, 20));

    expect(document.activeElement).toBe(add);
  });

  it('an open menu keeps focus while another panel completes', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);
    moreOf('Game Two').focus();
    await user.keyboard('{Enter}');
    const item = screen.getByRole('menuitem', { name: msg('downloads.panel.details') });
    item.focus();

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });
    await new Promise((done) => setTimeout(done, 20));

    expect(document.activeElement).toBe(item);
  });

  it('an open dialog keeps focus while the panel it came from completes', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);
    moreOf('Game One').focus();
    await user.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: msg('downloads.panel.details') }).focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog', { name: 'Game One' });
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    const held = document.activeElement;

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });
    await new Promise((done) => setTimeout(done, 20));

    expect(document.activeElement).toBe(held);
  });
});

describe('Downloads: the hand-off brings its target into view', () => {
  const rowOf = (el: HTMLElement) => el.closest<HTMLElement>('[data-download-row]');
  const goRemoves = (id: string) => downloads.update((list) => list.filter((d) => d.id !== id));

  it('a cancelled queue row hands focus to the row now in its place and brings that row into view', async () => {
    downloads.set([download({ status: 'queued' }), download({ id: 'd2', name: 'Game Two', status: 'queued' })]);
    const { user } = renderDom(Downloads);

    cancelX('Game One').focus();
    await user.keyboard('{Enter}');
    goRemoves('d1');

    await waitFor(() => expect(document.activeElement).toBe(cancelX('Game Two')));
    expect(scrolled).toContain(rowOf(cancelX('Game Two')));
    expect(scrolled).not.toContain(cancelX('Game Two'));
  });

  async function confirmCancel(user: ReturnType<typeof renderDom>['user'], title: string) {
    cancelX(title).focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('ui.cancelDownload') }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  }

  it('a confirmed Cancel of the only queued row: the dialog gives focus to the page heading and brings it into view', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    downloads.set([download({ status: 'queued', downloaded: 100 })]);
    const { user } = renderDom(Downloads);

    await confirmCancel(user, 'Game One');

    const heading = screen.getByRole('heading', { level: 1 });
    await waitFor(() => expect(document.activeElement).toBe(heading));
    await waitFor(() => expect(scrolled).toContain(heading));
  });

  it('a confirmed Cancel in the queue: the dialog gives focus to the row now in its place and brings that row into view', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    downloads.set([download({ status: 'queued', downloaded: 100 }), download({ id: 'd2', name: 'Game Two', status: 'queued' })]);
    const { user } = renderDom(Downloads);

    await confirmCancel(user, 'Game One');

    await waitFor(() => expect(document.activeElement).toBe(cancelX('Game Two')));
    await waitFor(() => expect(scrolled).toContain(rowOf(cancelX('Game Two'))));
  });

  it('a confirmed Cancel in the failed list: the dialog gives focus to the row now in its place and brings that row into view', async () => {
    vi.mocked(cancelDownload).mockImplementation(async (id) => goRemoves(id));
    downloads.set([
      download({ status: 'failed', downloaded: 100, error: 'typhon:download.unavailable' }),
      download({ id: 'd2', name: 'Game Two', status: 'failed', error: 'typhon:download.unavailable' }),
    ]);
    const { user } = renderDom(Downloads);

    await confirmCancel(user, 'Game One');

    await waitFor(() => expect(document.activeElement).toBe(cancelX('Game Two')));
    await waitFor(() => expect(scrolled).toContain(rowOf(cancelX('Game Two'))));
  });

  it('a panel that goes with its cancel dialog open: the dialog gives focus to the panel now in its place and brings it into view', async () => {
    downloads.set([download({ downloaded: 100 }), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);
    within(panel('Game One')).getByRole('button', { name: msg('downloads.panel.more', { name: 'Game One' }) }).focus();
    await user.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: msg('downloads.panel.cancel') }).focus();
    await user.keyboard('{Enter}');
    await screen.findByRole('dialog');

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });

    const more = () => within(panel('Game Two')).getByRole('button', { name: msg('downloads.panel.more', { name: 'Game Two' }) });
    await waitFor(() => expect(document.activeElement).toBe(more()));
    await waitFor(() => expect(scrolled).toContain(panel('Game Two')));
  });

  it('a confirmed Clear that empties the section: the dialog gives focus to the page heading and brings it into view', async () => {
    vi.mocked(removeDownload).mockImplementation(async (id) => goRemoves(id));
    downloads.set(
      Array.from({ length: 6 }, (_, i) =>
        download({ id: `u${i}`, name: `Update ${i}`, status: 'completed', completedAt: '2026-09-08T01:00:00Z', origin: { purpose: 'update' } }),
      ),
    );
    const { user } = renderDom(Downloads);
    screen.getByRole('button', { name: msg('downloads.done.clear') }).focus();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('downloads.done.clearConfirm') }));

    const heading = screen.getByRole('heading', { level: 1 });
    await waitFor(() => expect(document.activeElement).toBe(heading));
    await waitFor(() => expect(scrolled).toContain(heading));
  });
});

describe('Downloads: what a press leaves behind, and Go moving a focused row', () => {
  const pauseOf = (name: string) => within(panel(name)).getByRole('button', { name: msg('downloads.panel.pauseNamed', { name }) });
  const resumeOf = (name: string) => within(panel(name)).getByRole('button', { name: msg('downloads.panel.resumeNamed', { name }) });
  const moreIn = (row: HTMLElement, name: string) => within(row).getByRole('button', { name: msg('downloads.panel.more', { name }) });
  const rowOf = (id: string) => document.querySelector<HTMLElement>(`[data-download-row="${id}"]`) as HTMLElement;
  const extracting = (downloadId: string) =>
    ({ id: `i-${downloadId}`, downloadId, gameId: '', status: 'extracting', progress: 0.1, bytesDone: 1, bytesTotal: 10 }) as Installation;

  it('a Pause Go answers before its event: once the panel shows Resume, a later completion is not followed', async () => {
    downloads.set([download(), download({ id: 'd2', name: 'Game Two' })]);
    const { user } = renderDom(Downloads);
    pauseOf('Game One').focus();

    await user.keyboard('{Enter}');
    await waitFor(() => expect(pauseDownload).toHaveBeenCalledWith('d1'));
    await new Promise((done) => setTimeout(done, 0));
    goSets('d1', { status: 'paused', downloadSpeed: 0 });
    await waitFor(() => expect(document.activeElement).toBe(resumeOf('Game One')));

    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });

    await waitFor(() => expect(document.activeElement).toBe(pauseOf('Game Two')));
  });

  it('a focused completed row Go moves to "Installing": focus goes to the row now in its place, not along', async () => {
    downloads.set([
      download({ status: 'completed', completedAt: '2026-09-08T02:00:00Z' }),
      download({ id: 'd2', name: 'Game Two', status: 'completed', completedAt: '2026-09-08T01:00:00Z' }),
    ]);
    renderDom(Downloads);
    moreIn(rowOf('d1'), 'Game One').focus();

    installations.set([extracting('d1')]);

    await waitFor(() => expect(document.activeElement).toBe(moreIn(rowOf('d2'), 'Game Two')));
  });

  it('the catch-all starts from the focused row: the last queued row that starts hands focus to the row above', async () => {
    downloads.set([
      download({ id: 'a', name: 'Alpha', status: 'queued' }),
      download({ id: 'b', name: 'Beta', status: 'queued' }),
      download({ id: 'c', name: 'Gamma', status: 'queued' }),
    ]);
    renderDom(Downloads);
    grip('Gamma').focus();

    goSets('c', { status: 'downloading' });

    await waitFor(() => expect(document.activeElement).toBe(grip('Beta')));
  });

  it('a panel gone with its cancel dialog open leaves no press behind for its download’s later row', async () => {
    downloads.set([
      download({ downloaded: 100 }),
      download({ id: 'd2', name: 'Game Two' }),
      download({ id: 'd3', name: 'Game Three', status: 'completed', completedAt: '2026-09-08T00:00:00Z' }),
    ]);
    const { user } = renderDom(Downloads);
    moreIn(panel('Game One'), 'Game One').focus();
    await user.keyboard('{Enter}');
    screen.getByRole('menuitem', { name: msg('downloads.panel.cancel') }).focus();
    await user.keyboard('{Enter}');
    await screen.findByRole('dialog');
    goSets('d1', { status: 'completed', completedAt: '2026-09-08T01:00:00Z' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

    // Later, focus on the download's completed row, which Go then takes away.
    moreIn(rowOf('d1'), 'Game One').focus();
    downloads.update((list) => list.filter((d) => d.id !== 'd1'));

    await waitFor(() => expect(document.activeElement).toBe(moreIn(rowOf('d3'), 'Game Three')));
  });

  it('a Retry Go took whose row stayed (the install failed again) is not followed when Go moves the row later', async () => {
    const failedInstall = (error: string) =>
      ({ id: 'i-d1', downloadId: 'd1', gameId: '', status: 'failed', error, progress: 0, bytesDone: 0, bytesTotal: 0 }) as Installation;
    downloads.set([
      download({ status: 'completed', completedAt: '2026-09-08T02:00:00Z' }),
      download({ id: 'd2', name: 'Game Two', status: 'completed', completedAt: '2026-09-08T01:00:00Z' }),
    ]);
    installations.set([failedInstall('typhon:install.no_free_space')]);
    const { user } = renderDom(Downloads);
    const retry = within(rowOf('d1')).getByRole('button', { name: msg('downloads.done.retryNamed', { name: 'Game One' }) });
    retry.focus();

    await user.keyboard('{Enter}');
    await waitFor(() => expect(retryInstall).toHaveBeenCalledWith('i-d1'));
    installations.set([failedInstall('typhon:install.disk_full')]);
    await new Promise((done) => setTimeout(done, 0));
    expect(document.activeElement).toBe(retry);

    installations.set([extracting('d1')]);

    await waitFor(() =>
      expect(document.activeElement).toBe(within(rowOf('d2')).getByRole('button', { name: msg('downloads.done.installNamed', { name: 'Game Two' }) })),
    );
  });
});
