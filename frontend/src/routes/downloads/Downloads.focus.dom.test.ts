import { screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  cancelDownload: vi.fn(),
  pauseDownload: vi.fn(),
  resumeDownload: vi.fn(),
  getDownload: vi.fn(() => new Promise(() => {})),
}));

import { msg } from '../../lib/i18n';
import { cancelDownload, pauseDownload, resumeDownload, type Download } from '../../lib/services/downloads';
import { downloads } from '../../lib/stores/downloads';
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
  for (const fn of [cancelDownload, pauseDownload, resumeDownload]) {
    vi.mocked(fn).mockReset();
    vi.mocked(fn).mockResolvedValue(undefined);
  }
});

afterEach(() => {
  downloads.set([]);
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
    expect(scrolled).toContain(pauseOf('Game Two'));
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
  it('a cancelled queue row hands focus to the row now in its place and scrolls it into view', async () => {
    downloads.set([download({ status: 'queued' }), download({ id: 'd2', name: 'Game Two', status: 'queued' })]);
    const { user } = renderDom(Downloads);

    cancelX('Game One').focus();
    await user.keyboard('{Enter}');
    downloads.update((list) => list.filter((d) => d.id !== 'd1'));

    await waitFor(() => expect(document.activeElement).toBe(cancelX('Game Two')));
    expect(scrolled).toContain(cancelX('Game Two'));
  });
});
