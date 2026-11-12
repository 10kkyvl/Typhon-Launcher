import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { Events } from '@wailsio/runtime';
import { tick } from 'svelte';
import { dismissMetadataMatch, ensureMetadataFresh, getMetadataView, refreshMetadata, type MetadataView } from '../../lib/services/metadata';
import { getCatalogGame, getReleasesForGame, type CatalogGame, type ReleaseGroup } from '../../lib/services/sources';
import { libraryGames } from '../../lib/stores/library';
import { metadataAvailable } from '../../lib/stores/metadata';
import { resetHistory, route } from '../../lib/stores/router';
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
  // jsdom has no ResizeObserver; the page's tabs and segmented controls measure with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getMetadataView).mockImplementation(async (id) => view({ game: { id, title: '' } as CatalogGame, resolved: false, match: 'idle' }));
  vi.mocked(getCatalogGame).mockResolvedValue(null);
  vi.mocked(getReleasesForGame).mockResolvedValue([]);
  libraryGames.set([hades]);
});


// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
// Read before the press; the getter answers the words of a message that came in since, '' until then.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

afterEach(() => {
  vi.mocked(getMetadataView).mockReset();
  vi.mocked(refreshMetadata).mockReset();
  vi.mocked(dismissMetadataMatch).mockReset();
  vi.mocked(ensureMetadataFresh).mockReset();
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getReleasesForGame).mockReset();
  metadataAvailable.set(false);
  libraryGames.set([]);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('GameDetails: the breadcrumb', () => {
  it('leads back through a muted text link that the keyboard presses', async () => {
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const crumbs = screen.getByRole('navigation');
    const back = within(crumbs).getByRole('button', { name: msg('games.installedStatusWord') });
    expect(back.classList.contains('text-link')).toBe(true);
    expect(back.classList.contains('muted')).toBe(true);
    back.focus();

    await user.keyboard('{Enter}');

    expect(get(route).name).toBe('installed');
  });
});

describe('GameDetails: a long summary', () => {
  it('opens the rest of its text with a disclosure button tied to the text', async () => {
    const summary = 'A rogue-like dungeon crawler. '.repeat(30).trim();
    vi.mocked(getMetadataView).mockResolvedValue(view({ game: { id: 'c1', title: 'Hades', summary } as MetadataView['game'] }));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const more = await screen.findByRole('button', { name: msg('games.detailExpandSummary') });
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(more.hasAttribute('aria-pressed')).toBe(false);
    const text = document.getElementById(more.getAttribute('aria-controls') ?? '');
    expect(text?.textContent?.endsWith('…')).toBe(true);
    more.focus();

    await user.keyboard('{Enter}');

    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(text?.textContent).toBe(summary);
  });
});

describe('GameDetails: retrying what failed to load', () => {
  it('keeps the downloads notice and the focus on its busy Retry, then moves the focus on, not to the page', async () => {
    const again = deferred<ReleaseGroup[]>();
    vi.mocked(getReleasesForGame).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = (await screen.findByText(msg('games.detailReleasesFailed'))).closest('.notice') as HTMLElement;
    expect(notice).toBeInstanceOf(HTMLElement);
    const retry = within(notice).getByRole('button', { name: msg('games.catalogRetry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve([]);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    // No download to focus in an empty list: the notice's fallback, the game's title.
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Hades' })));
  });

  it('says the downloads failure again when the retry fails again', async () => {
    vi.mocked(getReleasesForGame).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = (await screen.findByText(msg('games.detailReleasesFailed'))).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('games.catalogRetry') });
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('games.detailReleasesFailed')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });

  it('keeps the metadata notice and the focus on its busy Retry, then focuses the game title', async () => {
    metadataAvailable.set(true);
    vi.mocked(getMetadataView).mockResolvedValue(view({ resolved: false, match: 'failed' }));
    const again = deferred<MetadataView>();
    vi.mocked(refreshMetadata).mockReturnValueOnce(again.promise);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = (await screen.findByText(msg('games.detailMetaFailedTitle'))).closest('.notice') as HTMLElement;
    expect(notice).toBeInstanceOf(HTMLElement);
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve(view({}));
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Hades' })));
  });

  it('says the metadata failure again, once, when the retry fails again', async () => {
    metadataAvailable.set(true);
    vi.mocked(getMetadataView).mockResolvedValue(view({ resolved: false, match: 'failed' }));
    vi.mocked(refreshMetadata).mockRejectedValueOnce(new Error('offline'));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = (await screen.findByText(msg('games.detailMetaFailedTitle'))).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toContain(msg('games.detailMetaFailedTitle')));
    expect(said()).toContain(msg('games.detailMetaFailedHint'));
    expect(get(toasts).map((item) => item.message)).not.toContain(msg('games.detailMetaRefreshError'));
    expect(document.activeElement).toBe(retry);
  });

  it('keeps the page error and the focus on its busy Retry, then focuses the game title', async () => {
    libraryGames.set([]);
    const again = deferred<CatalogGame | null>();
    vi.mocked(getCatalogGame).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(GameDetails, { id: 'c9' });
    const failed = await screen.findByRole('heading', { level: 1, name: msg('games.detailCatalogLoadError') });
    const retry = screen.getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(failed.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve({ id: 'c9', title: 'Celeste' } as CatalogGame);
    await vi.waitFor(() => expect(failed.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Celeste' })));
  });
});

describe('GameDetails: genres and themes', () => {
  it('are neutral status badges', async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({ game: { id: 'c1', title: 'Hades', genres: ['Roguelike'] } as MetadataView['game'] }));
    renderDom(GameDetails, { id: 'g1' });

    const tag = await screen.findByText('Roguelike', { selector: '[data-part="game-section"] *' });

    expect(tag.classList.contains('badge')).toBe(true);
    expect(tag.classList.contains('neutral')).toBe(true);
  });
});

describe('GameDetails: an uninstalled game', () => {
  it('states that it was uninstalled in an info notice read with the page', async () => {
    libraryGames.set([{ ...hades, uninstalled: true }]);
    renderDom(GameDetails, { id: 'g1' });

    const note = (await screen.findByText(msg('games.detailUninstalledNote'))).closest('.notice');

    expect(note?.getAttribute('data-tone')).toBe('info');
    expect(note?.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });
});

// The page's own listener for metadata pushed by the backend while it is open.
function pushMetadata(next: MetadataView) {
  const call = vi.mocked(Events.On).mock.calls.findLast(([name]) => name === 'metadata:updated');
  if (!call) throw new Error('the page does not listen for metadata');
  (call[1] as (event: { data: MetadataView }) => void)({ data: next });
}

describe('GameDetails: the outcome of a metadata search', () => {
  beforeEach(() => metadataAvailable.set(true));

  it('is read with the page when the page opens on it', async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({ resolved: false, match: 'unmatched' }));
    renderDom(GameDetails, { id: 'g1' });

    const notice = (await screen.findByText(msg('games.detailMetaUnmatchedTitle'))).closest('.notice') as HTMLElement;

    expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('is announced when the search ran while the page was open', async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({ resolved: false, match: 'searching' }));
    renderDom(GameDetails, { id: 'g1' });
    await screen.findByText(msg('games.detailMetaSearchingTitle'));

    pushMetadata(view({ resolved: false, match: 'unmatched' }));
    await tick();

    const notice = (await screen.findByText(msg('games.detailMetaUnmatchedTitle'))).closest('.notice') as HTMLElement;
    expect(notice.querySelector('[role="status"]')?.textContent).toContain(msg('games.detailMetaUnmatchedTitle'));
  });
});

describe('GameDetails: skipping the metadata search', () => {
  it('shows no search while it skips, keeps the focus on the busy Skip, then focuses the title', async () => {
    metadataAvailable.set(true);
    vi.mocked(getMetadataView).mockResolvedValue(view({ resolved: false, match: 'unmatched' }));
    const answer = deferred<MetadataView>();
    vi.mocked(dismissMetadataMatch).mockReturnValueOnce(answer.promise);
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const notice = (await screen.findByText(msg('games.detailMetaUnmatchedTitle'))).closest('.notice') as HTMLElement;
    const skip = within(notice).getByRole('button', { name: msg('games.detailSkipSearch') });
    skip.focus();

    await user.keyboard('{Enter}');

    expect(skip.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(skip);
    expect(screen.queryByText(msg('games.detailMetaSearchingTitle'))).toBeNull();

    answer.resolve(view({ resolved: false, match: 'skipped' }));
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Hades' })));
  });
});
