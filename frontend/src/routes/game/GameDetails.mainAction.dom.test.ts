// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, runningGames } from '../../lib/stores/library';
import { fireEvent, screen, within } from '@testing-library/svelte';
import { get, type Writable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import {
  addCatalogGame,
  launchesInFlight,
  playGame,
  playGameProfile,
  stopGame,
  type LibraryGame,
} from '../../lib/services/library';
import { resumeDownload, type Download } from '../../lib/services/downloads';
import { getMetadataView, type MetadataView } from '../../lib/services/metadata';
import type { MoveJob } from '../../lib/services/relocate';
import {
  getCatalogGame,
  getRelatedReleases,
  getReleasesForGame,
  type CatalogGame,
  type RelatedReleases,
  type ReleaseGroup,
} from '../../lib/services/sources';
import { prepareUpdatePlan, type Update, type VerifyState } from '../../lib/services/updates';
import { downloads } from '../../lib/stores/downloads';
import { gamePropertiesTarget } from '../../lib/stores/gameProperties';
import { moves } from '../../lib/stores/relocate';
import { toasts } from '../../lib/stores/toasts';
import { updates, verifications } from '../../lib/stores/updates';
import { renderDom } from '../../lib/testing/dom';
import GameDetails from './GameDetails.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => {
  const { writable: store } = await import('svelte/store');
  return {
    ...(await importOriginal<typeof import('../../lib/services/library')>()),
    playGame: vi.fn(async () => {}),
    playGameProfile: vi.fn(async () => true),
    stopGame: vi.fn(async () => {}),
    addCatalogGame: vi.fn(),
    // A launch another screen started: the service's own set, written by the test.
    launchesInFlight: store<ReadonlySet<string>>(new Set()),
  };
});
vi.mock('../../lib/services/downloads', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/downloads')>()),
  resumeDownload: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/updates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/updates')>()),
  prepareUpdatePlan: vi.fn(async () => {}),
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

const inFlight = launchesInFlight as unknown as Writable<ReadonlySet<string>>;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function view(patch: Partial<MetadataView> = {}): MetadataView {
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

function download(patch: Partial<Download>): Download {
  return {
    id: 'd1',
    name: 'Hades repack',
    status: 'downloading',
    progress: 0.37,
    addedAt: '2026-02-01T00:00:00Z',
    completedAt: null,
    error: '',
    origin: { gameId: 'c1' },
    ...patch,
  } as Download;
}

function update(patch: Partial<Update> = {}): Update {
  return {
    gameId: 'g1',
    title: 'Hades',
    state: 'update_available',
    availability: {
      available: true,
      kind: 'update',
      gameId: 'g1',
      installedReleaseId: 'r1',
      targetReleaseId: 'r2',
      installedVersion: '1.0',
      targetVersion: '1.1',
      confidence: 1,
      strategy: 'patch',
      estimatedDownloadBytes: 0,
      requiresFullInstall: false,
      patchCount: 1,
    },
    plan: null,
    planning: false,
    progress: 0,
    canRollback: false,
    checkedAt: '',
    ...patch,
  } as Update;
}

const plan = {
  id: 'p1',
  gameId: 'g1',
  installedReleaseId: 'r1',
  targetReleaseId: 'r2',
  sourceId: 's1',
  installedVersion: '1.0',
  targetVersion: '1.1',
  strategy: 'patch',
  steps: [],
  downloadBytes: 1024,
  reusedBytes: 0,
  requiredDiskBytes: 1024,
  backupAvailable: false,
} as unknown as NonNullable<Update['plan']>;

/** The hero's action row. */
function actions(): HTMLElement {
  const row = document.querySelector<HTMLElement>('[data-part="game-actions"]');
  if (!row) throw new Error('the page has no action row');
  return row;
}

async function mainAction(name: string | RegExp): Promise<HTMLElement> {
  await vi.waitFor(() => expect(within(actions()).getByRole('button', { name })).toBeTruthy());
  return within(actions()).getByRole('button', { name });
}

const playName = msg('games.play');

beforeEach(() => {
  // jsdom has no ResizeObserver; the page's tabs and segmented controls measure with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getMetadataView).mockImplementation(async (id) => view({ game: { id, title: 'Hades' } as CatalogGame }));
  vi.mocked(getCatalogGame).mockResolvedValue(null);
  vi.mocked(getReleasesForGame).mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockResolvedValue([]);
  vi.mocked(playGame).mockReset().mockResolvedValue(undefined);
  vi.mocked(playGameProfile).mockReset().mockResolvedValue(true);
  vi.mocked(stopGame).mockReset().mockResolvedValue(undefined);
  vi.mocked(addCatalogGame).mockReset();
  vi.mocked(resumeDownload).mockReset().mockResolvedValue(undefined);
  vi.mocked(prepareUpdatePlan).mockReset().mockResolvedValue(undefined);
  inFlight.set(new Set());
  libraryGames.set([hades]);
});

afterEach(() => {
  vi.mocked(getMetadataView).mockReset();
  vi.mocked(getCatalogGame).mockReset();
  // A page left from a test can still ask after it: answer with nothing, not undefined.
  vi.mocked(getReleasesForGame).mockReset().mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockReset().mockResolvedValue([]);
  libraryGames.set([]);
  runningGames.set(new Set());
  downloads.set([]);
  updates.set([]);
  verifications.set({});
  moves.set([]);
  gamePropertiesTarget.set(null);
  toasts.set([]);
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('GameDetails main action: a finished download of a game not installed', () => {
  it('installs the newest one, not the first in the list', async () => {
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
    downloads.set([
      download({ id: 'old', status: 'failed', addedAt: '2026-01-01T00:00:00Z', error: '' }),
      download({ id: 'new', status: 'completed', addedAt: '2026-03-01T00:00:00Z' }),
    ]);
    renderDom(GameDetails, { id: 'c1' });

    expect(await mainAction(msg('games.primaryInstall'))).toBeTruthy();
    expect(within(actions()).queryByRole('button', { name: msg('games.hubRetryDownload') })).toBeNull();
  });
});
