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

function release(id: string, gameId: string, title: string): ReleaseGroup {
  return {
    sourceName: 'Fixture source',
    release: {
      id, sourceId: 'fixture', rawTitle: title, title, normalizedTitle: title.toLowerCase(), canonicalGameId: gameId,
      version: '', size: 1024, uploadedAt: null, uris: [], matchStatus: 'matched', matchConfidence: 1, matchMethod: 'override',
      availability: 'available', firstSeenAt: '', lastSeenAt: '', createdAt: '',
    },
  } as ReleaseGroup;
}

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

describe('GameDetails main action: a launch in flight', () => {
  it('sends one launch for a double press and says it is launching, busy, while the call runs', async () => {
    const launch = deferred<void>();
    vi.mocked(playGame).mockReturnValueOnce(launch.promise);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const play = await mainAction(playName);

    await user.dblClick(play);

    expect(playGame).toHaveBeenCalledTimes(1);
    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(play.textContent).toContain(msg('games.mainActionLaunching'));
    expect(play.hasAttribute('disabled')).toBe(false);
    launch.resolve();
  });

  it('is busy while a launch another screen started is in flight, and a press sends nothing', async () => {
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const play = await mainAction(playName);

    inFlight.set(new Set(['g1']));
    await vi.waitFor(() => expect(play.getAttribute('aria-busy')).toBe('true'));
    await user.click(play);

    expect(playGame).not.toHaveBeenCalled();
    expect(play.textContent).toContain(msg('games.mainActionLaunching'));
  });

  it('never reads the second click of the double click that started the game as Stop', async () => {
    // The game starts at once: Go reports it running before the second click lands.
    vi.mocked(playGame).mockImplementation(async (id) => {
      runningGames.set(new Set([id]));
    });
    renderDom(GameDetails, { id: 'g1' });
    const play = await mainAction(playName);

    await fireEvent.click(play, { detail: 1 });
    await vi.waitFor(() => expect(play.textContent).toContain(msg('games.stop')));
    await fireEvent.click(play, { detail: 2 });

    expect(playGame).toHaveBeenCalledTimes(1);
    expect(stopGame).not.toHaveBeenCalled();
  });

  it('never reads a held Enter that started the game as Stop', async () => {
    vi.mocked(playGame).mockImplementation(async (id) => {
      runningGames.set(new Set([id]));
    });
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const play = await mainAction(playName);
    play.focus();

    await user.keyboard('{Enter>4/}');

    await vi.waitFor(() => expect(play.textContent).toContain(msg('games.stop')));
    expect(playGame).toHaveBeenCalledTimes(1);
    expect(stopGame).not.toHaveBeenCalled();
  });

  it('keeps Stop busy while the stop runs: a second press sends nothing', async () => {
    runningGames.set(new Set(['g1']));
    const stopping = deferred<void>();
    vi.mocked(stopGame).mockReturnValueOnce(stopping.promise);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const stop = await mainAction(msg('games.stop'));

    await user.click(stop);
    await user.click(stop);

    expect(stopGame).toHaveBeenCalledTimes(1);
    expect(stop.getAttribute('aria-busy')).toBe('true');
    expect(stop.textContent).toContain(msg('games.mainActionStopping'));
    stopping.resolve();
  });

  it('says the launching state once in a live region', async () => {
    const launch = deferred<void>();
    vi.mocked(playGame).mockReturnValueOnce(launch.promise);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const play = await mainAction(playName);

    await user.click(play);

    const region = within(actions()).getByRole('status');
    expect(region.textContent?.trim()).toBe(msg('games.mainActionLaunching'));
    launch.resolve();
  });
});

describe('GameDetails main action: a failed stop', () => {
  it("says the code's own text", async () => {
    runningGames.set(new Set(['g1']));
    vi.mocked(stopGame).mockRejectedValueOnce(new Error('typhon:library.process_gone: процесс не найден'));
    const { user } = renderDom(GameDetails, { id: 'g1' });

    await user.click(await mainAction(msg('games.stop')));

    await vi.waitFor(() => expect(get(toasts).map((item) => item.message)).toContain(msg('library.process_gone')));
    expect(get(toasts).map((item) => item.message)).not.toContain(msg('games.errorStopFailed'));
  });
});

describe('GameDetails main action: focus stays on it when it changes', () => {
  it('stays on the same element from Play to Stop while an update is offered', async () => {
    updates.set([update()]);
    vi.mocked(playGame).mockImplementation(async (id) => {
      runningGames.set(new Set([id]));
    });
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const play = await mainAction(playName);
    play.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(play.textContent).toContain(msg('games.stop')));
    expect(play.isConnected).toBe(true);
    expect(document.activeElement).toBe(play);
  });

  it('moves from the profile chooser to the main action when the chooser goes with the launch', async () => {
    libraryGames.set([{ ...hades, profiles: [{ id: 'p1', name: 'DirectX 11', executable: 'dx11.exe' }] } as unknown as LibraryGame]);
    vi.mocked(playGameProfile).mockImplementation(async (id) => {
      runningGames.set(new Set([id]));
      return true;
    });
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const play = await mainAction(playName);
    const chooser = within(actions()).getByRole('button', { name: msg('games.playProfileChoose') });
    chooser.focus();

    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('menuitemradio', { name: 'DirectX 11' }));

    await vi.waitFor(() => expect(chooser.isConnected).toBe(false));
    expect(playGameProfile).toHaveBeenCalledWith('g1', 'p1');
    await vi.waitFor(() => expect(document.activeElement).toBe(play));
  });

  it('stays on the same element when Retry download turns into the download progress', async () => {
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
    downloads.set([download({ status: 'failed', error: '' })]);
    vi.mocked(resumeDownload).mockImplementation(async () => {
      downloads.set([download({ status: 'downloading', progress: 0.1 })]);
    });
    const { user } = renderDom(GameDetails, { id: 'c1' });
    const retry = await mainAction(msg('games.hubRetryDownload'));
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(retry.textContent).toContain(msg('state.downloadsStatusDownloading')));
    expect(retry.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });

  it('moves from Update to the main action when the update starts and Update goes', async () => {
    updates.set([update({ plan })]);
    renderDom(GameDetails, { id: 'g1' });
    const lead = await mainAction(playName);
    const button = await mainAction(msg('games.mainActionUpdate'));
    button.focus();

    updates.set([update({ plan, state: 'updating', step: 'download', progress: 0.2 })]);

    await vi.waitFor(() => expect(button.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(lead));
    expect(lead.getAttribute('aria-busy')).toBe('true');
  });

  it('moves from Add to library to the main action when the game joins the library', async () => {
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
    const added = { ...hades, id: 'g9', installedAt: '', uninstalled: true, executable: '' } as unknown as LibraryGame;
    vi.mocked(addCatalogGame).mockImplementation(async () => {
      libraryGames.set([added]);
      return added;
    });
    vi.mocked(getReleasesForGame).mockResolvedValue([release('r1', 'c1', 'Hades')]);
    const { user } = renderDom(GameDetails, { id: 'c1' });
    const lead = await mainAction(msg('games.primaryDownload'));
    const add = await mainAction(msg('games.detailAddToLibraryButton'));
    add.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(add.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(lead));
  });
});

describe('GameDetails main action: an installed game with another download of it', () => {
  it.each(['paused', 'queued', 'downloading'] as const)(
    'keeps Play while that download is %s',
    async (status) => {
      downloads.set([download({ status })]);
      renderDom(GameDetails, { id: 'g1' });

      expect(await mainAction(playName)).toBeTruthy();
    },
  );
});

describe('GameDetails main action: an installed game without an executable', () => {
  it('offers no Play without an executable, and points to choosing one in the properties', async () => {
    libraryGames.set([{ ...hades, executable: '' }]);
    const { user } = renderDom(GameDetails, { id: 'g1' });

    const choose = await mainAction(msg('games.mainActionChooseExecutable'));
    expect(within(actions()).queryByRole('button', { name: playName })).toBeNull();
    await user.click(choose);

    expect(playGame).not.toHaveBeenCalled();
    expect(get(gamePropertiesTarget)).toEqual({ gameId: 'g1' });
  });
});

describe("GameDetails main action: work on an installed game's files", () => {
  it('shows a repair of its files as progress and offers no Play; a press does nothing', async () => {
    verifications.set({ g1: { gameId: 'g1', running: false, repairing: true, progress: 0.4 } as VerifyState });
    const { user } = renderDom(GameDetails, { id: 'g1' });

    const lead = await mainAction(new RegExp(msg('games.mainActionRepairing')));
    expect(within(actions()).queryByRole('button', { name: playName })).toBeNull();
    await user.click(lead);

    expect(playGame).not.toHaveBeenCalled();
  });

  it('keeps Stop for a game that already runs while its files are checked', async () => {
    runningGames.set(new Set(['g1']));
    verifications.set({ g1: { gameId: 'g1', running: true, repairing: false, progress: 0.4 } as VerifyState });
    renderDom(GameDetails, { id: 'g1' });

    expect(await mainAction(msg('games.stop'))).toBeTruthy();
  });

  it('shows a move of its files as progress and offers no Play; a press does nothing', async () => {
    moves.set([{ id: 'm1', scope: 'game', stage: 'copy', gameId: 'g1', title: 'Hades', copiedBytes: 50, totalBytes: 100 } as MoveJob]);
    const { user } = renderDom(GameDetails, { id: 'g1' });

    const lead = await mainAction(new RegExp(msg('games.mainActionMoving')));
    expect(within(actions()).queryByRole('button', { name: playName })).toBeNull();
    await user.click(lead);

    expect(playGame).not.toHaveBeenCalled();
  });
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

describe('GameDetails main action in English', () => {
  it('says Downloading, not Loading, for a running download', async () => {
    locale.set('en');
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
    downloads.set([download({ status: 'downloading' })]);
    renderDom(GameDetails, { id: 'c1' });

    await vi.waitFor(() => expect(actions().textContent).toContain('Downloading'));
    expect(actions().textContent).not.toContain('Loading');
  });
});

describe('GameDetails main action: Update', () => {
  it('without a plan is busy while the plan is prepared, then opens the confirm; a second press does nothing', async () => {
    updates.set([update()]);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const button = await mainAction(msg('games.mainActionUpdate'));

    await user.click(button);
    await user.click(button);

    expect(prepareUpdatePlan).toHaveBeenCalledTimes(1);
    expect(button.getAttribute('aria-busy')).toBe('true');
    updates.set([update({ planning: true })]);
    updates.set([update({ planning: false, plan })]);

    expect(await screen.findByRole('dialog', { name: msg('ui.updateGameTitle') })).toBeTruthy();
    expect(button.getAttribute('aria-busy')).toBeNull();
  });

  it('is pressable again when the plan request fails or the preparation ends without a plan', async () => {
    updates.set([update()]);
    vi.mocked(prepareUpdatePlan).mockRejectedValueOnce(new Error('offline'));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const button = await mainAction(msg('games.mainActionUpdate'));

    await user.click(button);

    await vi.waitFor(() => expect(get(toasts).map((item) => item.message)).toContain(msg('errUpdates.fallback')));
    expect(button.getAttribute('aria-busy')).toBeNull();

    await user.click(button);
    expect(button.getAttribute('aria-busy')).toBe('true');
    updates.set([update({ planning: true })]);
    updates.set([update({ planning: false, error: 'typhon:updates.no_target' })]);

    await vi.waitFor(() => expect(button.getAttribute('aria-busy')).toBeNull());
    expect(screen.queryByRole('dialog', { name: msg('ui.updateGameTitle') })).toBeNull();
    expect(prepareUpdatePlan).toHaveBeenCalledTimes(2);
  });

  it('pressed on the Screenshots tab goes to the overview and opens the confirm', async () => {
    updates.set([update({ plan })]);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const button = await mainAction(msg('games.mainActionUpdate'));
    await user.click(screen.getByRole('tab', { name: msg('games.detailTabScreenshots') }));

    await user.click(button);

    expect(await screen.findByRole('dialog', { name: msg('ui.updateGameTitle') })).toBeTruthy();
    expect(screen.getByRole('tab', { name: msg('games.detailTabOverview') }).getAttribute('aria-selected')).toBe('true');
  });
});

describe('GameDetails main action: Other versions', () => {
  const sequel: RelatedReleases = {
    game: { id: 'c2', title: 'Hades II', relation: 'sequel' as RelatedReleases['game']['relation'] },
    gameId: 'g2',
    onPc: true,
    groups: [release('r2', 'c2', 'Hades II')],
  };

  it('pressed on the Screenshots tab goes to the overview and brings the block into view', async () => {
    const scrolled: Element[] = [];
    const original = Element.prototype.scrollIntoView;
    // jsdom does not scroll: the element asked to come into view is what the test reads.
    Element.prototype.scrollIntoView = vi.fn(function (this: Element) {
      scrolled.push(this);
    });
    onTestFinished(() => {
      Element.prototype.scrollIntoView = original;
    });
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
    vi.mocked(getMetadataView).mockResolvedValue(view({ screenshots: [{ id: 's1', url: 'https://example.test/s1.jpg' }] as MetadataView['screenshots'] }));
    vi.mocked(getRelatedReleases).mockResolvedValue([sequel]);
    const { user } = renderDom(GameDetails, { id: 'c1' });
    const button = await mainAction(msg('games.primaryOtherVersions'));
    await user.click(await screen.findByRole('tab', { name: msg('games.detailTabScreenshots') }));

    await user.click(button);

    await vi.waitFor(() => expect(scrolled.some((node) => node.textContent?.includes('Hades II'))).toBe(true));
  });
});
