import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/downloads', async (actual) => ({
  ...(await actual<typeof import('../services/downloads')>()),
  getDownload: vi.fn(),
}));

import { locale, msg } from '../i18n';
import { getDownload, type Download, type DownloadStatus, type FileState } from '../services/downloads';
import { downloads } from '../stores/downloads';
import { toasts } from '../stores/toasts';
import { renderDom } from '../testing/dom';
import { bytesSize } from '../utils/format';
import DownloadDetailsModal from './DownloadDetailsModal.svelte';

const GB = 1024 ** 3;
const MB = 1024 ** 2;

const file = (path: string, bytesDone: number, size = 10 * GB, selected = true): FileState => ({ path, size, selected, bytesDone });

function record(id: string, status: DownloadStatus, files: FileState[], patch: Partial<Download> = {}): Download {
  return {
    id,
    name: `Game ${id}`,
    type: 'game',
    source: 'magnet:?xt=urn:btih:abc',
    infoHash: id,
    destination: '/downloads',
    status,
    progress: 0.3,
    downloaded: 3 * GB,
    total: 10 * GB,
    downloadSpeed: 40 * MB,
    uploadSpeed: 2 * MB,
    etaSeconds: -1,
    seeders: 4,
    peers: 9,
    files,
    seeding: false,
    addedAt: '2026-11-01T10:00:00Z',
    completedAt: null,
    error: '',
    origin: {},
    ...patch,
  };
}

const bytesText = (done: number, total = 10 * GB) => msg('modals.downloadDetailsFileBytes', { done: bytesSize(done), total: bytesSize(total) });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// Lets mocked answers and the effects they cause land.
async function settle() {
  for (let i = 0; i < 5; i += 1) await Promise.resolve();
  await tick();
}

const fileRow = (path: string) => (screen.getByText(path).closest('.file') as HTMLElement);

beforeEach(() => {
  locale.set('en');
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
  vi.mocked(getDownload).mockReset();
});

afterEach(() => {
  downloads.set([]);
  toasts.set([]);
  document.body.replaceChildren();
});

describe('DownloadDetailsModal: per-file progress follows Go while the download moves data', () => {
  it('reads the files once on open and then once a second while the download runs', async () => {
    downloads.set([record('a', 'downloading', [file('Game/data.bin', 0)])]);
    vi.mocked(getDownload).mockResolvedValueOnce(record('a', 'downloading', [file('Game/data.bin', 2 * GB)]));
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();

    expect(within(fileRow('Game/data.bin')).getByText(bytesText(2 * GB))).toBeTruthy();
    expect(getDownload).toHaveBeenCalledTimes(1);

    vi.mocked(getDownload).mockResolvedValueOnce(record('a', 'downloading', [file('Game/data.bin', 5 * GB)]));
    await vi.advanceTimersByTimeAsync(1000);
    await settle();

    expect(getDownload).toHaveBeenCalledTimes(2);
    expect(getDownload).toHaveBeenLastCalledWith('a');
    expect(within(fileRow('Game/data.bin')).getByText(bytesText(5 * GB))).toBeTruthy();
  });

  // Go moves a file's bytes only while it downloads (download.Manager.sample); a verify sets them once
  // as it ends, with a status change. A read during verifying or metadata copies the list for nothing.
  it('reads once on open while the download verifies or fetches its metadata, and polls no more', async () => {
    for (const status of ['verifying', 'metadata'] as const) {
      vi.mocked(getDownload).mockReset();
      vi.mocked(getDownload).mockResolvedValue(record('a', status, [file('Game/data.bin', GB)]));
      downloads.set([record('a', status, [file('Game/data.bin', 0)])]);
      const view = renderDom(DownloadDetailsModal, { open: true, id: 'a' });
      await settle();
      await vi.advanceTimersByTimeAsync(3000);
      await settle();
      expect(getDownload, status).toHaveBeenCalledTimes(1);
      expect(vi.getTimerCount(), status).toBe(0);
      view.unmount();
    }
  });

  it('reads once more when the status changes, so the numbers it stopped at are Go’s last', async () => {
    vi.mocked(getDownload).mockResolvedValueOnce(record('a', 'verifying', [file('Game/data.bin', 2 * GB)]));
    downloads.set([record('a', 'verifying', [file('Game/data.bin', 0)])]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();
    expect(getDownload).toHaveBeenCalledTimes(1);

    vi.mocked(getDownload).mockResolvedValueOnce(record('a', 'paused', [file('Game/data.bin', 7 * GB)]));
    downloads.set([record('a', 'paused', [file('Game/data.bin', 2 * GB)])]);
    await settle();

    expect(getDownload).toHaveBeenCalledTimes(2);
    expect(within(fileRow('Game/data.bin')).getByText(bytesText(7 * GB))).toBeTruthy();
    await vi.advanceTimersByTimeAsync(5000);
    expect(getDownload).toHaveBeenCalledTimes(2);
  });

  it('a status change while a read is out reads again once that read is back', async () => {
    const out = deferred<Download>();
    vi.mocked(getDownload).mockReturnValueOnce(out.promise);
    downloads.set([record('a', 'downloading', [file('Game/data.bin', 0)])]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();

    downloads.set([record('a', 'paused', [file('Game/data.bin', 3 * GB)])]);
    await settle();
    expect(getDownload).toHaveBeenCalledTimes(1);

    vi.mocked(getDownload).mockResolvedValueOnce(record('a', 'paused', [file('Game/data.bin', 4 * GB)]));
    out.resolve(record('a', 'downloading', [file('Game/data.bin', GB)]));
    await settle();
    await settle();

    expect(getDownload).toHaveBeenCalledTimes(2);
    expect(within(fileRow('Game/data.bin')).getByText(bytesText(4 * GB))).toBeTruthy();
  });


  it('stops asking when the dialog closes, and leaves no timer behind', async () => {
    vi.mocked(getDownload).mockResolvedValue(record('a', 'downloading', [file('Game/data.bin', GB)]));
    downloads.set([record('a', 'downloading', [file('Game/data.bin', 0)])]);
    const view = renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();
    expect(vi.getTimerCount()).toBe(1);

    await view.rerender({ open: false, id: 'a' });
    await settle();
    const calls = vi.mocked(getDownload).mock.calls.length;
    await vi.advanceTimersByTimeAsync(5000);

    expect(vi.getTimerCount()).toBe(0);
    expect(getDownload).toHaveBeenCalledTimes(calls);
  });

  it('stops asking when the download stops moving data, and starts again when it moves', async () => {
    vi.mocked(getDownload).mockResolvedValue(record('a', 'paused', [file('Game/data.bin', GB)]));
    downloads.set([record('a', 'downloading', [file('Game/data.bin', 0)])]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();

    downloads.set([record('a', 'paused', [file('Game/data.bin', GB)])]);
    await settle();
    const calls = vi.mocked(getDownload).mock.calls.length;
    await vi.advanceTimersByTimeAsync(5000);
    expect(vi.getTimerCount()).toBe(0);
    expect(getDownload).toHaveBeenCalledTimes(calls);

    // The change itself reads once, then the ticks take over.
    downloads.set([record('a', 'downloading', [file('Game/data.bin', GB)])]);
    await settle();
    expect(getDownload).toHaveBeenCalledTimes(calls + 1);
    await vi.advanceTimersByTimeAsync(1000);
    expect(getDownload).toHaveBeenCalledTimes(calls + 2);
    expect(vi.getTimerCount()).toBe(1);
  });

  it('stops asking when the download leaves the list', async () => {
    vi.mocked(getDownload).mockResolvedValue(record('a', 'downloading', [file('Game/data.bin', GB)]));
    downloads.set([record('a', 'downloading', [file('Game/data.bin', 0)])]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();
    expect(vi.getTimerCount()).toBe(1);

    downloads.set([]);
    await settle();
    const calls = vi.mocked(getDownload).mock.calls.length;
    await vi.advanceTimersByTimeAsync(5000);

    expect(vi.getTimerCount()).toBe(0);
    expect(getDownload).toHaveBeenCalledTimes(calls);
  });

  it('asks once on open for a paused, failed or completed download, and never again', async () => {
    for (const status of ['paused', 'failed', 'completed'] as const) {
      vi.mocked(getDownload).mockReset();
      vi.mocked(getDownload).mockResolvedValue(record('a', status, [file('Game/data.bin', 4 * GB)]));
      downloads.set([record('a', status, [file('Game/data.bin', 0)])]);
      const view = renderDom(DownloadDetailsModal, { open: true, id: 'a' });
      await settle();
      await vi.advanceTimersByTimeAsync(5000);

      expect(getDownload, status).toHaveBeenCalledTimes(1);
      expect(vi.getTimerCount(), status).toBe(0);
      expect(within(fileRow('Game/data.bin')).getByText(bytesText(4 * GB))).toBeTruthy();
      view.unmount();
    }
  });

  it('drops an answer for a download the dialog no longer shows', async () => {
    const late = deferred<Download>();
    vi.mocked(getDownload).mockReturnValueOnce(late.promise);
    downloads.set([
      record('a', 'paused', [file('Shared/data.bin', 0)]),
      record('b', 'paused', [file('Shared/data.bin', 0)]),
    ]);
    const view = renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();
    vi.mocked(getDownload).mockResolvedValue(record('b', 'paused', [file('Shared/data.bin', GB)]));
    await view.rerender({ open: true, id: 'b' });
    await settle();

    late.resolve(record('a', 'paused', [file('Shared/data.bin', 9 * GB)]));
    await settle();

    expect(within(fileRow('Shared/data.bin')).getByText(bytesText(GB))).toBeTruthy();
    expect(screen.queryByText(bytesText(9 * GB))).toBeNull();
  });

  it('drops an answer that a newer status event has overtaken', async () => {
    const late = deferred<Download>();
    vi.mocked(getDownload).mockReturnValueOnce(late.promise);
    downloads.set([record('a', 'downloading', [file('Game/data.bin', 0)])]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();

    downloads.set([record('a', 'completed', [file('Game/data.bin', 10 * GB)])]);
    await settle();
    late.resolve(record('a', 'downloading', [file('Game/data.bin', 3 * GB)]));
    await settle();

    expect(within(fileRow('Game/data.bin')).getByText(bytesText(10 * GB))).toBeTruthy();
  });

  it('keeps the last numbers through a failed read, says nothing, and tries again on the next tick', async () => {
    vi.mocked(getDownload).mockResolvedValueOnce(record('a', 'downloading', [file('Game/data.bin', 2 * GB)]));
    vi.mocked(getDownload).mockRejectedValueOnce(new Error('typhon:download.not_found: gone'));
    vi.mocked(getDownload).mockResolvedValueOnce(record('a', 'downloading', [file('Game/data.bin', 6 * GB)]));
    downloads.set([record('a', 'downloading', [file('Game/data.bin', 0)])]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();

    await vi.advanceTimersByTimeAsync(1000);
    await settle();
    expect(getDownload).toHaveBeenCalledTimes(2);
    expect(within(fileRow('Game/data.bin')).getByText(bytesText(2 * GB))).toBeTruthy();
    expect(get(toasts)).toHaveLength(0);

    await vi.advanceTimersByTimeAsync(1000);
    await settle();
    expect(within(fileRow('Game/data.bin')).getByText(bytesText(6 * GB))).toBeTruthy();
  });
});

describe('DownloadDetailsModal: what it says', () => {
  beforeEach(() => {
    vi.mocked(getDownload).mockRejectedValue(new Error('unavailable in browser'));
  });

  it('says a file\'s bytes in one message, once: as the value of its bar, named by the path', async () => {
    downloads.set([record('a', 'paused', [file('Game/data.bin', 3 * GB), file('Game/extra.bin', 0, GB, false)])]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();

    const data = screen.getByRole('progressbar', { name: 'Game/data.bin' });
    expect(data.getAttribute('aria-valuetext')).toBe(bytesText(3 * GB));
    const shown = within(fileRow('Game/data.bin')).getByText(bytesText(3 * GB));
    expect(shown.closest('[aria-hidden="true"]')).not.toBeNull();

    const extra = screen.getByRole('progressbar', { name: 'Game/extra.bin' });
    expect(extra.getAttribute('aria-valuetext')).toBe(msg('modals.downloadDetailsSkipped'));
    expect(within(fileRow('Game/extra.bin')).getByText(msg('modals.downloadDetailsSkipped')).closest('[aria-hidden="true"]')).not.toBeNull();
  });

  it('shows speeds only while the download moves data, whatever the record still carries', async () => {
    const down = () => screen.queryByText(msg('modals.downloadDetailsDownloadSpeed'));
    const up = () => screen.queryByText(msg('modals.downloadDetailsUploadSpeed'));

    downloads.set([record('a', 'verifying', [file('Game/data.bin', 3 * GB)])]);
    const view = renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();
    expect(down()).toBeNull();
    expect(up()).toBeNull();
    view.unmount();

    downloads.set([record('a', 'downloading', [file('Game/data.bin', 3 * GB)])]);
    const running = renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();
    expect(down()).not.toBeNull();
    expect(up()).not.toBeNull();
    running.unmount();

    downloads.set([record('a', 'completed', [file('Game/data.bin', 10 * GB)], { seeding: true })]);
    renderDom(DownloadDetailsModal, { open: true, id: 'a' });
    await settle();
    expect(down()).toBeNull();
    expect(up()).not.toBeNull();
  });
});
