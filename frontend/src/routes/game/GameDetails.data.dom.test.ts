import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { libraryGames } from '../../lib/stores/library';
import type { LibraryGame } from '../../lib/services/library';
import { fetchMetadata, startDownloadFrom, type DownloadOrigin } from '../../lib/services/downloads';
import { getMetadataView, refreshMetadata, type MetadataView } from '../../lib/services/metadata';
import { metadataAvailable } from '../../lib/stores/metadata';
import { get } from 'svelte/store';
import {
  getCatalogGame,
  getRelatedReleases,
  getReleasesForGame,
  prepareReleaseDownload,
  type CatalogGame,
  type RelatedReleases,
  type Release,
  type ReleaseGroup,
  type Source,
} from '../../lib/services/sources';
import type { Settings } from '../../lib/services/settings';
import { resetHistory } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { sources } from '../../lib/stores/sources';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import GameDetails from './GameDetails.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/metadata')>()),
  getMetadataView: vi.fn(),
  refreshMetadata: vi.fn(),
  dismissMetadataMatch: vi.fn(),
  ensureMetadataFresh: vi.fn(),
}));
vi.mock('../../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/sources')>()),
  getCatalogGame: vi.fn(),
  getReleasesForGame: vi.fn(),
  getRelatedReleases: vi.fn(),
  prepareReleaseDownload: vi.fn(),
}));
vi.mock('../../lib/services/downloads', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/downloads')>()),
  fetchMetadata: vi.fn(),
  startDownloadFrom: vi.fn(),
  discardMetadata: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  appInfo: vi.fn(async () => ({ platform: 'darwin', devMock: false })),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function view(patch: Partial<MetadataView>): MetadataView {
  return {
    game: { id: 'c1', title: 'Hades' },
    cover: '',
    hero: '',
    screenshots: [],
    resolved: true,
    stale: false,
    provider: 'igdb',
    match: 'idle',
    ...patch,
  } as MetadataView;
}

function release(patch: Partial<Release>): Release {
  return {
    id: 'r1', sourceId: 'fixture', rawTitle: 'Raw', title: 'Raw', normalizedTitle: 'raw', canonicalGameId: 'c1',
    version: '', size: 1024, uploadedAt: null, uris: [], matchStatus: 'matched', matchConfidence: 1, matchMethod: 'override',
    availability: 'available', firstSeenAt: '', lastSeenAt: '', createdAt: '',
    ...patch,
  } as Release;
}

const group = (patch: Partial<Release>): ReleaseGroup => ({ sourceName: 'Fixture source', release: release(patch) }) as ReleaseGroup;

const hades = {
  id: 'g1',
  title: 'Hades',
  canonicalGameId: 'c1',
  installDir: '/games/hades',
  executable: 'Hades.exe',
  installedAt: '2026-01-01T00:00:00Z',
  uninstalled: false,
  playtimeSeconds: 0,
  sizeBytes: 0,
  version: '',
} as unknown as LibraryGame;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getMetadataView).mockImplementation(async (id) => view({ game: { id, title: '' } as CatalogGame, resolved: false }));
  vi.mocked(getCatalogGame).mockResolvedValue(null);
  vi.mocked(getReleasesForGame).mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockResolvedValue([]);
  libraryGames.set([hades]);
});

afterEach(() => {
  // The page goes first: one left mounted would ask again when the stores below are emptied.
  cleanup();
  vi.mocked(getMetadataView).mockReset();
  vi.mocked(refreshMetadata).mockReset();
  metadataAvailable.set(false);
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getReleasesForGame).mockReset().mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockReset().mockResolvedValue([]);
  vi.mocked(prepareReleaseDownload).mockReset();
  vi.mocked(fetchMetadata).mockReset();
  vi.mocked(startDownloadFrom).mockReset();
  libraryGames.set([]);
  sources.set([]);
  settings.set(null);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// Presses Download on a release row, then Start in the add-download dialog it opens, and answers the
// origin the download was started with: what Go records as the game it belongs to.
async function downloadFrom(row: HTMLElement, user: ReturnType<typeof renderDom>['user']): Promise<DownloadOrigin> {
  settings.set({ language: 'ru', libraryPath: '/library', downloadsPath: '/downloads' } as Settings);
  vi.mocked(fetchMetadata).mockResolvedValue({ infoHash: 'hash', name: 'image', totalBytes: 1, files: [{ path: 'game.iso', size: 1 }] } as never);
  vi.mocked(startDownloadFrom).mockResolvedValue(undefined as never);
  await user.click(within(row).getByRole('button', { name: msg('ui.download') }));
  const dialog = await screen.findByRole('dialog');
  await user.click(await within(dialog).findByRole('button', { name: msg('modals.addDownloadStart') }));
  await vi.waitFor(() => expect(startDownloadFrom).toHaveBeenCalledTimes(1));
  return vi.mocked(startDownloadFrom).mock.calls[0][3] as DownloadOrigin;
}

describe('GameDetails: whose game a console download from the page is', () => {
  beforeEach(() => {
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades', coverUrl: 'https://img/hades.jpg' } as CatalogGame);
  });

  it("records the page's own console release under the page's game, its title and cover", async () => {
    vi.mocked(getReleasesForGame).mockResolvedValue([group({ id: 'own', platform: 'ps2', canonicalGameId: 'c1-alias' })]);
    vi.mocked(prepareReleaseDownload).mockResolvedValue({ uri: 'magnet:?xt=urn:btih:own', name: 'Raw', releaseId: 'own', sourceId: 'fixture', gameId: 'c1-alias' });
    const { user } = renderDom(GameDetails, { id: 'c1' });
    const list = (await screen.findAllByRole('button', { name: msg('ui.download') }))[0].closest('section') as HTMLElement;

    const origin = await downloadFrom(list, user);

    expect(origin).toMatchObject({ gameId: 'c1', title: 'Hades', cover: 'https://img/hades.jpg', platform: 'ps2', releaseId: 'own' });
  });

  it('records a console release of another version under that version, its title and cover', async () => {
    const original: RelatedReleases = {
      game: { id: 'server-2', title: 'Hades PS2', relation: 'original', coverUrl: 'https://img/ps2.jpg' },
      gameId: 'c2',
      onPc: false,
      groups: [group({ id: 'other', platform: 'ps2', canonicalGameId: 'c2' })],
    };
    vi.mocked(getRelatedReleases).mockResolvedValue([original]);
    vi.mocked(prepareReleaseDownload).mockResolvedValue({ uri: 'magnet:?xt=urn:btih:other', name: 'Raw', releaseId: 'other', sourceId: 'fixture', gameId: 'c2' });
    const { user } = renderDom(GameDetails, { id: 'c1' });
    const section = (await screen.findByRole('heading', { level: 2, name: msg('games.otherVersionsHeading') })).closest('section') as HTMLElement;

    const origin = await downloadFrom(section, user);

    expect(origin).toMatchObject({ gameId: 'c2', title: 'Hades PS2', cover: 'https://img/ps2.jpg', platform: 'ps2', releaseId: 'other' });
  });
});

describe('GameDetails: the sources changing while the page is open', () => {
  const feed = {
    id: 'feed', name: 'Feed', type: 'url', url: 'https://feed', enabled: true, status: 'idle', health: 'healthy',
    lastUpdatedAt: null, lastError: '', fingerprint: 'v1', feedVersion: 1, entries: 1, invalid: 0, matched: 1,
    review: 0, unmatched: 0, createdAt: '',
  } as unknown as Source;
  const lead = () => within(document.querySelector('[data-part="game-actions"]') as HTMLElement).getByRole('button', { name: msg('games.primaryDownload') });

  // A card that is not installed: its title is known at once, so the releases are asked once.
  beforeEach(() => {
    libraryGames.set([{ ...hades, uninstalled: true }]);
    sources.set([feed]);
    vi.mocked(getReleasesForGame).mockResolvedValue([group({ id: 'r1', rawTitle: 'Hades v1' })]);
  });

  it('reloads nothing when a refresh sends the same set of sources again', async () => {
    renderDom(GameDetails, { id: 'g1' });
    await vi.waitFor(() => expect(lead()).toBeTruthy());
    expect(getReleasesForGame).toHaveBeenCalledTimes(1);
    expect(getRelatedReleases).toHaveBeenCalledTimes(1);

    sources.set([{ ...feed, status: 'updating' } as Source]);
    await tick();
    sources.set([{ ...feed, lastUpdatedAt: '2026-11-14T10:00:00Z' } as Source]);
    await tick();

    expect(getReleasesForGame).toHaveBeenCalledTimes(1);
    expect(getRelatedReleases).toHaveBeenCalledTimes(1);
  });

  it('reloads on a real change and keeps the list, Download and its focus until the answer', async () => {
    renderDom(GameDetails, { id: 'g1' });
    await vi.waitFor(() => expect(lead()).toBeTruthy());
    const download = lead();
    download.focus();
    const answer = deferred<ReleaseGroup[]>();
    vi.mocked(getReleasesForGame).mockReturnValueOnce(answer.promise);

    sources.set([{ ...feed, fingerprint: 'v2', entries: 2 } as Source]);
    await tick();

    expect(getReleasesForGame).toHaveBeenCalledTimes(2);
    expect(screen.getByText('Hades v1')).toBeTruthy();
    expect(download.isConnected).toBe(true);
    expect(download.hasAttribute('disabled')).toBe(false);
    expect(screen.queryByText(msg('games.primaryResolving'))).toBeNull();
    expect(document.activeElement).toBe(download);

    answer.resolve([group({ id: 'r1', rawTitle: 'Hades v1' }), group({ id: 'r2', rawTitle: 'Hades v2' })]);
    expect(await screen.findByText('Hades v2')).toBeTruthy();
    expect(document.activeElement).toBe(download);
  });
});

const toastsOf = (kind: string) => get(toasts).filter((item) => item.kind === kind).map((item) => item.message);
const metaNotice = async () => (await screen.findByText(msg('games.detailMetaFailedTitle'))).closest('.notice') as HTMLElement;

describe('GameDetails: a metadata refresh that answers "failed"', () => {
  beforeEach(() => metadataAvailable.set(true));

  it('from the menu says the failure, not "updated"', async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({ resolved: true, match: 'idle' }));
    vi.mocked(refreshMetadata).mockResolvedValueOnce(view({ resolved: true, match: 'failed' }));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    await user.click(await screen.findByRole('button', { name: msg('games.moreLabel') }));

    await user.click(await screen.findByRole('menuitem', { name: msg('games.detailMetaRefreshLabel') }));

    await vi.waitFor(() => expect(toastsOf('danger')).toContain(msg('games.detailMetaRefreshError')));
    expect(toastsOf('success')).not.toContain(msg('games.detailMetaRefreshedToast'));
  });

  it("from the failed notice's Retry keeps the notice and its Retry, and says no success", async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({ resolved: false, match: 'failed' }));
    const answer = deferred<MetadataView>();
    vi.mocked(refreshMetadata).mockReturnValueOnce(answer.promise);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = await metaNotice();
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');
    answer.resolve(view({ resolved: false, match: 'failed' }));

    await vi.waitFor(() => expect(retry.getAttribute('aria-busy')).not.toBe('true'));
    expect(toastsOf('success')).toEqual([]);
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });
});

describe('GameDetails: the metadata view that could not be read', () => {
  beforeEach(() => metadataAvailable.set(true));

  it('says so in a notice with Retry, not a toast over a page that looks as if the game had none', async () => {
    vi.mocked(getMetadataView).mockRejectedValueOnce(new Error('typhon:metadata.read_failed: disk'));
    renderDom(GameDetails, { id: 'g1' });

    const notice = await metaNotice();

    expect(notice.textContent).toContain(msg('games.detailMetaLoadError'));
    expect(within(notice).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(notice.textContent).not.toContain('typhon:');
    expect(toastsOf('danger')).toEqual([]);
  });

  it('reads it again on Retry and shows it, focus on the title', async () => {
    const summary = 'A rogue-like dungeon crawler.';
    vi.mocked(getMetadataView)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(view({ game: { id: 'c1', title: 'Hades', summary } as MetadataView['game'] }));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = await metaNotice();
    within(notice).getByRole('button', { name: msg('common.retry') }).focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(await screen.findByText(summary)).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Hades' })));
    expect(getMetadataView).toHaveBeenCalledTimes(2);
  });

  it('says the failure again when the read fails again, with the focus kept on the busy-then-free Retry', async () => {
    vi.mocked(getMetadataView).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = await metaNotice();
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(getMetadataView).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(notice.querySelector('.body')?.getAttribute('role')).toBe('status'));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(toastsOf('danger')).toEqual([]);
  });
});

describe('GameDetails: a catalog game that failed to load', () => {
  beforeEach(() => libraryGames.set([]));

  it('is said once, by the page, with the reason its code gives', async () => {
    vi.mocked(getCatalogGame).mockRejectedValue(new Error('typhon:catalog.save_failed: disk'));
    renderDom(GameDetails, { id: 'c9' });

    const failed = await screen.findByRole('heading', { level: 1, name: msg('games.detailCatalogLoadError') });

    expect(failed.closest('.empty')?.textContent).toContain(msg('errMetadata.catalogSaveFailed'));
    expect(toastsOf('danger')).toEqual([]);
  });

  it('does not stack a toast for each Retry that fails again', async () => {
    vi.mocked(getCatalogGame).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(GameDetails, { id: 'c9' });
    await screen.findByRole('heading', { level: 1, name: msg('games.detailCatalogLoadError') });
    const retry = screen.getByRole('button', { name: msg('common.retry') });

    await user.click(retry);
    await vi.waitFor(() => expect(getCatalogGame).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(retry.getAttribute('aria-busy')).not.toBe('true'));
    await user.click(retry);
    await vi.waitFor(() => expect(getCatalogGame).toHaveBeenCalledTimes(3));
    await vi.waitFor(() => expect(retry.getAttribute('aria-busy')).not.toBe('true'));

    expect(get(toasts)).toEqual([]);
    expect(screen.getByRole('heading', { level: 1, name: msg('games.detailCatalogLoadError') })).toBeTruthy();
  });
});
