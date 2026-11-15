import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { installErrorText } from '../../lib/install/installErrors';
import { cancelDownload, removeDownload, resumeDownload, type Download } from '../../lib/services/downloads';
import { downloads } from '../../lib/stores/downloads';
import { resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import Downloads from './Downloads.svelte';
import FailedList from './FailedList.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/downloads', async (actual) => ({
  ...(await actual<typeof import('../../lib/services/downloads')>()),
  resumeDownload: vi.fn(),
  cancelDownload: vi.fn(),
  removeDownload: vi.fn(),
}));

function failed(id: string, name: string, patch: Partial<Download> = {}): Download {
  return {
    id,
    name,
    type: 'torrent',
    source: '',
    infoHash: id,
    destination: `D:/Games/${name}`,
    status: 'failed',
    progress: 0,
    downloaded: 0,
    total: 2 * 1024 ** 3,
    downloadSpeed: 0,
    uploadSpeed: 0,
    etaSeconds: -1,
    seeders: 0,
    peers: 0,
    files: [],
    seeding: false,
    addedAt: '2026-11-01T00:00:00Z',
    completedAt: null,
    error: 'typhon:download.no_free_space: на диске D: нет места',
    origin: {},
    ...patch,
  };
}

function deferred() {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// What Go does on Resume: the download goes back to the queue.
const goRequeues = (id: string) => downloads.update((list) => list.map((d) => (d.id === id ? { ...d, status: 'queued', error: '' } : d)));
const goRemoves = (id: string) => downloads.update((list) => list.filter((d) => d.id !== id));

const retry = (title: string) => screen.getByRole('button', { name: msg('downloads.failed.retryFor', { title }) });
const cancelX = (title: string) => screen.getByRole('button', { name: msg('downloads.queue.cancelFor', { title }) });

let onannounce: ReturnType<typeof vi.fn<(text: string) => void>>;
let onopen: ReturnType<typeof vi.fn<(id: string) => void>>;
let heading: HTMLElement;

function mount() {
  return renderDom(FailedList, { onopen, onannounce });
}

beforeEach(() => {
  vi.mocked(resumeDownload).mockReset();
  vi.mocked(cancelDownload).mockReset();
  vi.mocked(removeDownload).mockReset();
  onannounce = vi.fn<(text: string) => void>();
  onopen = vi.fn<(id: string) => void>();
  heading = document.createElement('h1');
  heading.tabIndex = -1;
  document.body.append(heading);
});

afterEach(() => {
  downloads.set([]);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
});

describe('FailedList: a row', () => {
  it('says what failed by its code, never Go’s prose, and how much was downloaded', () => {
    downloads.set([failed('a', 'Alpha', { downloaded: 512 * 1024 ** 2, progress: 0.25 }), failed('b', 'Beta')]);
    mount();

    const [first, second] = screen.getAllByRole('listitem');
    expect(first.textContent).toContain(installErrorText('typhon:download.no_free_space'));
    expect(first.textContent).not.toContain('нет места');
    expect(first.textContent).not.toContain('typhon:');
    expect(first.textContent).toContain(msg('downloads.failed.downloaded', { done: bytesSize(512 * 1024 ** 2), total: bytesSize(2 * 1024 ** 3) }));
    expect(second.textContent).toContain(msg('downloads.failed.nothing'));
  });

  it('leads to the game’s page by its title; Details, named with the download, opens the details', async () => {
    downloads.set([failed('a', 'Alpha', { origin: { gameId: 'g-alpha' } })]);
    const { user } = mount();

    expect(screen.queryByRole('button', { name: msg('transfers.gamePage') })).toBeNull();
    await user.click(screen.getByRole('button', { name: msg('downloads.failed.detailsFor', { title: 'Alpha' }) }));
    expect(onopen).toHaveBeenCalledWith('a');

    await user.click(screen.getByRole('button', { name: 'Alpha' }));
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-alpha' } });
  });
});

describe('FailedList: retry', () => {
  it('a double press sends one Resume; the button is busy and keeps focus while Go answers', async () => {
    const call = deferred();
    vi.mocked(resumeDownload).mockReturnValue(call.promise);
    downloads.set([failed('a', 'Alpha')]);
    const { user } = mount();
    const button = retry('Alpha');
    button.focus();

    await user.keyboard('{Enter}{Enter}');

    expect(vi.mocked(resumeDownload).mock.calls).toEqual([['a']]);
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(button);
    call.resolve();
    await vi.waitFor(() => expect(button.getAttribute('aria-busy')).not.toBe('true'));
  });

  it('a refused retry says why by its code and leaves the row as Go has it, focus on Retry', async () => {
    const err = new Error('typhon:download.no_client');
    vi.mocked(resumeDownload).mockRejectedValue(err);
    downloads.set([failed('a', 'Alpha')]);
    const { user } = mount();
    retry('Alpha').focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(get(toasts).map((t) => t.message)).toEqual([installErrorText(err)]));
    expect(document.activeElement).toBe(retry('Alpha'));
    expect(onannounce).not.toHaveBeenCalled();
  });

  it('when the download leaves for the queue, focus goes to the next failed row’s Retry, else the one above', async () => {
    vi.mocked(resumeDownload).mockResolvedValue();
    downloads.set([failed('a', 'Alpha'), failed('b', 'Beta'), failed('c', 'Gamma')]);
    const { user } = mount();

    retry('Beta').focus();
    await user.keyboard('{Enter}');
    goRequeues('b');
    await vi.waitFor(() => expect(document.activeElement).toBe(retry('Gamma')));
    expect(onannounce).toHaveBeenCalledWith(msg('downloads.failed.retried', { title: 'Beta' }));

    await user.keyboard('{Enter}');
    goRequeues('c');
    await vi.waitFor(() => expect(document.activeElement).toBe(retry('Alpha')));
  });

  it('with no failed row left, focus follows the download to its grip in the queue', async () => {
    vi.mocked(resumeDownload).mockResolvedValue();
    downloads.set([failed('a', 'Alpha')]);
    const { user } = renderDom(Downloads);

    retry('Alpha').focus();
    await user.keyboard('{Enter}');
    goRequeues('a');

    await vi.waitFor(() => expect(document.activeElement?.getAttribute('aria-label')).toBe(msg('downloads.queue.grip', { title: 'Alpha', position: 1, total: 1 })));
  });
});

describe('FailedList: cancel', () => {
  it('with data on disk asks first and runs Go’s Cancel, not Remove; a refusal stays in the dialog by its code', async () => {
    const err = new Error('typhon:download.not_found');
    vi.mocked(cancelDownload).mockRejectedValueOnce(err).mockResolvedValueOnce();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    downloads.set([failed('a', 'Alpha', { downloaded: 1024 ** 2, progress: 0.1 })]);
    const { user } = mount();

    await user.click(cancelX('Alpha'));
    const dialog = await screen.findByRole('dialog', { name: msg('downloads.queue.cancelTitle', { title: 'Alpha' }) });
    expect(dialog.textContent).toContain(msg('downloads.queue.cancelDeletes', { size: bytesSize(1024 ** 2) }));
    await user.click(within(dialog).getByRole('button', { name: msg('ui.cancelDownload') }));

    await vi.waitFor(() => expect(dialog.textContent).toContain(installErrorText(err)));
    expect(vi.mocked(cancelDownload).mock.calls).toEqual([['a']]);
    expect(removeDownload).not.toHaveBeenCalled();
  });

  it('with nothing on disk cancels at once, says so, and hands focus to the row now in its place, then the page heading', async () => {
    vi.mocked(cancelDownload).mockResolvedValue();
    downloads.set([failed('a', 'Alpha'), failed('b', 'Beta')]);
    const { user } = mount();

    cancelX('Alpha').focus();
    await user.keyboard('{Enter}');
    expect(screen.queryByRole('dialog')).toBeNull();
    goRemoves('a');
    await vi.waitFor(() => expect(document.activeElement).toBe(cancelX('Beta')));
    expect(onannounce).toHaveBeenCalledWith(msg('downloads.queue.cancelled', { title: 'Alpha' }));

    await user.keyboard('{Enter}');
    goRemoves('b');
    await vi.waitFor(() => expect(document.activeElement).toBe(heading));
    expect(removeDownload).not.toHaveBeenCalled();
  });
});
