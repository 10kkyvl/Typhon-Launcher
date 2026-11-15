import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { discardMetadata, fetchMetadata, startDownloadFrom, type Download, type TorrentInfo } from '../services/downloads';
import { getSettings } from '../services/settings';
import { appInfo, type AppInfo } from '../services/system';
import { settings } from '../stores/settings';
import { user } from '../testing/dom';
import DownloadsPage from './components/DownloadsPage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/downloads', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/downloads')>()),
  fetchMetadata: vi.fn(),
  startDownloadFrom: vi.fn(),
  discardMetadata: vi.fn(async () => {}),
  cancelFetchMetadata: vi.fn(async () => {}),
}));
vi.mock('../services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/system')>()),
  appInfo: vi.fn(),
}));

const SOURCE = 'magnet:?xt=urn:btih:feed123';
const first: TorrentInfo = {
  infoHash: 'feed123',
  name: 'Hades build',
  totalBytes: 150,
  files: [
    { path: 'game.bin', size: 100, selected: true, bytesDone: 0 },
    { path: 'extras.bin', size: 50, selected: true, bytesDone: 0 },
  ],
};
// The same source fetched again: the files in another order, with one more.
const again: TorrentInfo = {
  infoHash: 'feed123',
  name: 'Hades build',
  totalBytes: 160,
  files: [
    { path: 'extras.bin', size: 50, selected: true, bytesDone: 0 },
    { path: 'readme.txt', size: 10, selected: true, bytesDone: 0 },
    { path: 'game.bin', size: 100, selected: true, bytesDone: 0 },
  ],
};
const nothingWaiting = () => new Error('typhon:download.metadata_required: fetch the torrent metadata first');

beforeEach(async () => {
  // jsdom has no layout, and the navigator takes an element without client rects for hidden.
  vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(function (this: Element) {
    return [{ left: 0, top: 0, right: 10, bottom: 10, width: 10, height: 10, x: 0, y: 0, toJSON: () => ({}) }] as unknown as DOMRectList;
  });
  settings.set({ ...(await getSettings()), libraryPath: '/library', downloadsPath: '/downloads' });
  vi.mocked(appInfo).mockResolvedValue({ version: '2.0.0', platform: 'darwin', arch: 'arm64', devMock: true, portable: false, dataDir: '/data' } satisfies AppInfo);
});

afterEach(() => {
  settings.set(null);
  vi.mocked(fetchMetadata).mockReset();
  vi.mocked(startDownloadFrom).mockReset();
  vi.mocked(discardMetadata).mockClear();
  vi.restoreAllMocks();
});

function control(key: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(`[data-bp-focus="${key}"]`);
  expect(found, key).not.toBeNull();
  return found!;
}

function choices(): Record<string, string | null> {
  return Object.fromEntries(
    [...document.querySelectorAll('[data-bp-focus^="file:"]')].map((row) => [row.querySelector('.file-path')?.textContent ?? '', row.getAttribute('aria-checked')]),
  );
}

async function openFiles() {
  render(DownloadsPage, { props: { onback: vi.fn(), ongame: vi.fn(), requestText: vi.fn(async () => SOURCE) } });
  const keys = user();
  await keys.click(control('transfers:add'));
  await vi.waitFor(() => control('files:start'));
  // The player leaves the extras out.
  await keys.click(control('file:1'));
  expect(choices()).toEqual({ 'game.bin': 'true', 'extras.bin': 'false' });
  return keys;
}

describe('Big Picture downloads: a start Go answers with nothing waiting', () => {
  it('fetches the source once more, keeps the files chosen, asks for Start again and starts with it', async () => {
    vi.mocked(fetchMetadata).mockResolvedValueOnce(first).mockResolvedValueOnce(again);
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(nothingWaiting()).mockResolvedValueOnce({ id: 'd1' } as Download);
    const keys = await openFiles();

    await keys.click(control('files:start'));

    await vi.waitFor(() => expect(choices()).toEqual({ 'extras.bin': 'false', 'readme.txt': 'true', 'game.bin': 'true' }));
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
    expect(vi.mocked(fetchMetadata).mock.calls[1]).toEqual([SOURCE]);
    expect(screen.getByText(msg('modals.addDownloadRestartNeeded'))).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(document.activeElement).toBe(control('files:start'));
    expect(discardMetadata).not.toHaveBeenCalled();

    await keys.click(control('files:start'));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(startDownloadFrom).toHaveBeenCalledTimes(2);
    expect(vi.mocked(startDownloadFrom).mock.calls[1].slice(0, 3)).toEqual(['feed123', '/downloads', [1, 2]]);
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
  });

  it('says a failed fetch after it as a failed fetch, fetches no more by itself, and its Retry keeps the choices', async () => {
    vi.mocked(fetchMetadata)
      .mockResolvedValueOnce(first)
      .mockRejectedValueOnce(new Error('no peers'))
      .mockResolvedValueOnce(again);
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(nothingWaiting());
    const keys = await openFiles();

    await keys.click(control('files:start'));

    expect((await screen.findByRole('alert')).textContent?.trim()).toBe(msg('bp.transfers.actionError'));
    await vi.waitFor(() => expect(document.activeElement).toBe(control('metadata:retry')));
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
    expect(startDownloadFrom).toHaveBeenCalledTimes(1);

    await keys.click(control('metadata:retry'));
    await vi.waitFor(() => expect(choices()).toEqual({ 'extras.bin': 'false', 'readme.txt': 'true', 'game.bin': 'true' }));
    expect(fetchMetadata).toHaveBeenCalledTimes(3);
    expect(screen.getByText(msg('modals.addDownloadRestartNeeded'))).toBeTruthy();
  });

  it('says any other failure on the files step and fetches nothing', async () => {
    vi.mocked(fetchMetadata).mockResolvedValueOnce(first);
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error('typhon:download.destination_unavailable: no disk'));
    const keys = await openFiles();

    await keys.click(control('files:start'));

    await vi.waitFor(() => control('files:start-retry'));
    expect(fetchMetadata).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(msg('modals.addDownloadRestartNeeded'))).toBeNull();
    expect(choices()).toEqual({ 'game.bin': 'true', 'extras.bin': 'false' });
  });
});
