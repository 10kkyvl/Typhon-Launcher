import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { libraryGames } from '../../lib/stores/library';
import type { LibraryGame } from '../../lib/services/library';
import { fetchMetadata, startDownloadFrom, type DownloadOrigin } from '../../lib/services/downloads';
import { getMetadataView, type MetadataView } from '../../lib/services/metadata';
import {
  getCatalogGame,
  getRelatedReleases,
  getReleasesForGame,
  prepareReleaseDownload,
  type CatalogGame,
  type Release,
  type ReleaseGroup,
} from '../../lib/services/sources';
import type { Settings } from '../../lib/services/settings';
import { resetHistory } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
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
  vi.mocked(getMetadataView).mockReset();
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getReleasesForGame).mockReset().mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockReset().mockResolvedValue([]);
  vi.mocked(prepareReleaseDownload).mockReset();
  vi.mocked(fetchMetadata).mockReset();
  vi.mocked(startDownloadFrom).mockReset();
  libraryGames.set([]);
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
});
