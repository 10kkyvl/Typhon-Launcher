// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames } from '../../lib/stores/library';
import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { CloudGameStatus, CloudStatus } from '../../lib/services/cloudsaves';
import { locateSaves, type LibraryGame } from '../../lib/services/library';
import { getMetadataView, type MetadataView } from '../../lib/services/metadata';
import { listBackups } from '../../lib/services/savebackup';
import { getCatalogGame, getRelatedReleases, getReleasesForGame, type CatalogGame } from '../../lib/services/sources';
import { cloudConflictTarget, cloudStatus } from '../../lib/stores/cloudsaves';
import { saveBackupsEnabled, saveBackupTarget } from '../../lib/stores/savebackup';
import { renderDom, user } from '../../lib/testing/dom';
import GameDetails from './GameDetails.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  locateSaves: vi.fn(),
}));
vi.mock('../../lib/services/savebackup', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/savebackup')>()),
  listBackups: vi.fn(),
}));
vi.mock('../../lib/services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/metadata')>()),
  getMetadataView: vi.fn(),
  ensureMetadataFresh: vi.fn(async () => false),
}));
vi.mock('../../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/sources')>()),
  getCatalogGame: vi.fn(),
  getReleasesForGame: vi.fn(),
  getRelatedReleases: vi.fn(),
}));

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
  profiles: [],
  lastProfile: '',
} as unknown as LibraryGame;

const view = (id: string): MetadataView => ({
  game: { id, title: 'Hades' } as CatalogGame,
  cover: '',
  hero: '',
  heroIsPoster: false,
  screenshots: [],
  resolved: true,
  stale: false,
  provider: 'igdb',
  match: 'idle',
});

const conflictOf = (gameId: string): CloudGameStatus => ({
  gameId,
  state: 'conflict',
  code: '',
  message: '',
  at: '',
  syncedAt: '',
  conflict: { localDigest: 'a', localUpdatedAt: '2026-11-01T10:00:00Z', localSize: 10, remoteDigest: 'b', remoteUpdatedAt: '2026-11-02T12:00:00Z', remoteSize: 20, remoteDevice: 'Deck' },
});
const statusWith = (...games: CloudGameStatus[]): CloudStatus => ({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games });

const savesTabName = msg('games.detailTabSaves');
const tab = (name: string) => screen.queryByRole('tab', { name });

beforeEach(() => {
  // jsdom has no ResizeObserver; the page's tabs measure with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getMetadataView).mockImplementation(async (id) => view(id));
  vi.mocked(getCatalogGame).mockResolvedValue(null);
  vi.mocked(getReleasesForGame).mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockResolvedValue([]);
  vi.mocked(locateSaves).mockResolvedValue({ path: '/saves/hades', candidates: [] } as unknown as Awaited<ReturnType<typeof locateSaves>>);
  vi.mocked(listBackups).mockResolvedValue([]);
  libraryGames.set([hades]);
  saveBackupsEnabled.set(true);
  cloudStatus.set(null);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  libraryGames.set([]);
  saveBackupsEnabled.set(false);
  cloudStatus.set(null);
  saveBackupTarget.set(null);
  cloudConflictTarget.set(null);
});

describe('GameDetails: the Saves tab', () => {
  it('is offered for a library game with backups on and holds the saves view', async () => {
    renderDom(GameDetails, { id: 'g1' });
    await vi.waitFor(() => expect(tab(savesTabName)).not.toBeNull());

    await user().click(tab(savesTabName) as HTMLElement);

    expect(tab(savesTabName)?.getAttribute('aria-selected')).toBe('true');
    const panel = document.querySelector('[data-part="game-main"]') as HTMLElement;
    await vi.waitFor(() => expect(within(panel).getByRole('button', { name: msg('saves.createBackup') })).toBeTruthy());
    expect(listBackups).toHaveBeenCalledWith('g1');
    expect(panel.querySelector('[data-part="game-section"]'), 'the overview blocks give the column to the saves').toBeNull();
  });

  it('is not offered when there is nothing to show, and for a page without a library card', async () => {
    saveBackupsEnabled.set(false);
    const first = renderDom(GameDetails, { id: 'g1' });
    await screen.findByRole('heading', { level: 1 });
    expect(tab(savesTabName)).toBeNull();
    first.unmount();

    saveBackupsEnabled.set(true);
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
    renderDom(GameDetails, { id: 'c1' });
    await screen.findByRole('heading', { level: 1 });
    expect(tab(savesTabName)).toBeNull();
  });

  it('gives the page back to the overview when the tab goes while it is open', async () => {
    renderDom(GameDetails, { id: 'g1' });
    await vi.waitFor(() => expect(tab(savesTabName)).not.toBeNull());
    await user().click(tab(savesTabName) as HTMLElement);

    saveBackupsEnabled.set(false);

    await vi.waitFor(() => expect(tab(savesTabName)).toBeNull());
    await vi.waitFor(() => expect(document.querySelector('[data-part="game-main"] [data-part="game-section"]')).not.toBeNull());
  });

  it('opens from the menu\'s saves item, in place of the dialog', async () => {
    renderDom(GameDetails, { id: 'g1' });
    await vi.waitFor(() => expect(tab(savesTabName)).not.toBeNull());
    const actions = document.querySelector('[data-part="game-actions"]') as HTMLElement;

    await user().click(within(actions).getByRole('button', { name: msg('games.moreLabel') }));
    await user().click(await screen.findByRole('menuitem', { name: msg('games.detailSavesRollback') }));

    await vi.waitFor(() => expect(tab(savesTabName)?.getAttribute('aria-selected')).toBe('true'));
    expect(get(saveBackupTarget)).toBeNull();
  });
});

describe('GameDetails: a save conflict of the game', () => {
  it('is said under the hero on every tab, with the way to settle it', async () => {
    cloudStatus.set(statusWith(conflictOf('g1')));
    renderDom(GameDetails, { id: 'g1' });

    const resolve = await screen.findByRole('button', { name: msg('cloudsaves.resolve') });
    const hero = document.querySelector('[data-part="game-hero"]') as HTMLElement;
    const main = document.querySelector('[data-part="game-main"]') as HTMLElement;
    expect(hero.compareDocumentPosition(resolve) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(resolve.compareDocumentPosition(main) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    await user().click(tab(savesTabName) as HTMLElement);
    expect(screen.getAllByRole('button', { name: msg('cloudsaves.resolve') }).length).toBeGreaterThan(0);

    await user().click(resolve);
    expect(get(cloudConflictTarget)?.gameId).toBe('g1');
  });

  it('is not said for another game\'s conflict', async () => {
    cloudStatus.set(statusWith(conflictOf('g2')));
    renderDom(GameDetails, { id: 'g1' });
    await screen.findByRole('heading', { level: 1 });

    expect(screen.queryByRole('button', { name: msg('cloudsaves.resolve') })).toBeNull();
  });
});
