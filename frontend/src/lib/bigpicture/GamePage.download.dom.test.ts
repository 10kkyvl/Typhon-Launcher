// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames } from '../stores/library';
import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { cancelFetchMetadata, fetchMetadata, startDownloadFrom, type Download, type TorrentInfo } from '../services/downloads';
import { getMetadataView, type MetadataView } from '../services/metadata';
import { getSettings } from '../services/settings';
import { getCatalogGame, getReleasesForGame, prepareReleaseDownload, type CatalogGame, type ReleaseGroup } from '../services/sources';
import { appInfo, type AppInfo } from '../services/system';
import { settings } from '../stores/settings';
import { user } from '../testing/dom';
import GamePage from './components/GamePage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/metadata')>()),
  getMetadataView: vi.fn(),
  ensureMetadataFresh: vi.fn(async () => false),
}));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getCatalogGame: vi.fn(),
  getReleasesForGame: vi.fn(),
  prepareReleaseDownload: vi.fn(),
}));
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

const group: ReleaseGroup = {
  sourceName: 'Source',
  release: {
    id: 'r1',
    sourceId: 's1',
    rawTitle: 'Hades',
    title: 'Hades',
    normalizedTitle: 'hades',
    canonicalGameId: 'c1',
    version: '1.0',
    size: 150,
    uploadedAt: null,
    uris: ['magnet:?xt=urn:btih:feed123'],
  } as ReleaseGroup['release'],
};

beforeEach(async () => {
  // jsdom has no layout, and the navigator takes an element without client rects for hidden.
  vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(function (this: Element) {
    return [{ left: 0, top: 0, right: 10, bottom: 10, width: 10, height: 10, x: 0, y: 0, toJSON: () => ({}) }] as unknown as DOMRectList;
  });
  libraryGames.set([]);
  settings.set({ ...(await getSettings()), libraryPath: '/library', downloadsPath: '/downloads' });
  vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
  vi.mocked(getMetadataView).mockResolvedValue({
    game: { id: 'c1', title: 'Hades' } as CatalogGame,
    cover: '',
    hero: '',
    heroIsPoster: false,
    screenshots: [],
    resolved: true,
    stale: false,
    provider: 'igdb',
    match: 'idle',
    storeLinks: {},
  } as MetadataView);
  vi.mocked(getReleasesForGame).mockResolvedValue([group]);
  vi.mocked(prepareReleaseDownload).mockResolvedValue({
    uri: 'magnet:?xt=urn:btih:feed123',
    name: 'Hades',
    releaseId: 'r1',
    sourceId: 's1',
    gameId: 'c1',
  });
  vi.mocked(appInfo).mockResolvedValue({ version: '2.0.0', platform: 'darwin', arch: 'arm64', devMock: true, portable: false, dataDir: '/data' } satisfies AppInfo);
});

afterEach(() => {
  settings.set(null);
  vi.mocked(fetchMetadata).mockReset();
  vi.mocked(startDownloadFrom).mockReset();
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getMetadataView).mockReset();
  vi.mocked(getReleasesForGame).mockReset();
  vi.mocked(prepareReleaseDownload).mockReset();
  vi.restoreAllMocks();
});

function choices(): Record<string, string | null> {
  const list = screen.getByRole('group', { name: msg('bp.game.selectFiles') });
  return Object.fromEntries(
    [...list.querySelectorAll('[role="checkbox"]')].map((row) => [row.querySelector('.file-path')?.textContent ?? '', row.getAttribute('aria-checked')]),
  );
}

async function openFiles() {
  render(GamePage, { props: { id: 'c1', onback: vi.fn(), ondownloads: vi.fn(), onlaunch: vi.fn(async () => true), requestText: vi.fn(async () => null) } });
  const keys = user();
  await keys.click(await screen.findByRole('tab', { name: new RegExp(msg('bp.game.tabReleases')) }));
  const release = await vi.waitFor(() => {
    const found = document.querySelector<HTMLButtonElement>('[data-bp-focus="game:release:r1"]');
    expect(found).not.toBeNull();
    return found!;
  });
  await keys.click(release);
  const start = await screen.findByRole('button', { name: msg('bp.game.downloadStart') });
  // The player leaves the extras out.
  await keys.click(screen.getByRole('checkbox', { name: /extras\.bin/ }));
  expect(choices()).toEqual({ 'game.bin': 'true', 'extras.bin': 'false' });
  return { keys, start };
}

describe('Big Picture game page: a start Go answers with nothing waiting', () => {
  it('fetches the release once more, keeps the files chosen, asks for Start again and starts with it', async () => {
    vi.mocked(fetchMetadata).mockResolvedValueOnce(first).mockResolvedValueOnce(again);
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(nothingWaiting()).mockResolvedValueOnce({ id: 'd1' } as Download);
    const { keys, start } = await openFiles();

    start.focus();
    await keys.keyboard('{Enter}');

    const startAgain = await screen.findByRole('button', { name: msg('bp.game.downloadStart') });
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
    expect(choices()).toEqual({ 'extras.bin': 'false', 'readme.txt': 'true', 'game.bin': 'true' });
    expect(screen.getByText(msg('modals.addDownloadRestartNeeded'))).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(document.activeElement).toBe(startAgain);

    await keys.click(startAgain);
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
    const { keys, start } = await openFiles();

    await keys.click(start);

    expect((await screen.findByRole('alert')).textContent?.trim()).toBe(msg('bp.game.downloadFetchFailed'));
    const retry = screen.getByRole('button', { name: msg('bp.game.releaseRetry') });
    expect(document.activeElement).toBe(retry);
    expect(fetchMetadata).toHaveBeenCalledTimes(2);
    expect(startDownloadFrom).toHaveBeenCalledTimes(1);

    await keys.click(retry);
    await screen.findByRole('button', { name: msg('bp.game.downloadStart') });
    expect(fetchMetadata).toHaveBeenCalledTimes(3);
    expect(choices()).toEqual({ 'extras.bin': 'false', 'readme.txt': 'true', 'game.bin': 'true' });
    expect(screen.getByText(msg('modals.addDownloadRestartNeeded'))).toBeTruthy();
  });

  it('can be closed while it fetches again, which cancels the fetch', async () => {
    vi.mocked(fetchMetadata).mockResolvedValueOnce(first).mockReturnValueOnce(new Promise(() => {}));
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(nothingWaiting());
    const { keys, start } = await openFiles();

    await keys.click(start);
    await screen.findByText(msg('bp.game.downloadFetching'));
    await keys.click(screen.getByRole('button', { name: msg('bp.game.downloadClose') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(cancelFetchMetadata).toHaveBeenCalledWith('magnet:?xt=urn:btih:feed123');
  });

  it('says any other failure on the files step and fetches nothing', async () => {
    vi.mocked(fetchMetadata).mockResolvedValueOnce(first);
    vi.mocked(startDownloadFrom).mockRejectedValueOnce(new Error('typhon:download.destination_unavailable: no disk'));
    const { keys, start } = await openFiles();

    await keys.click(start);

    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('bp.game.downloadStart') })).toBe(start);
    expect(fetchMetadata).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(msg('modals.addDownloadRestartNeeded'))).toBeNull();
    expect(choices()).toEqual({ 'game.bin': 'true', 'extras.bin': 'false' });
  });
});
