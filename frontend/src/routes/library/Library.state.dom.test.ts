// First: services/library reaches this store back through stores/cloudsaves, and a store loaded
// inside that cycle would keep the real service instead of the mock below.
import { libraryGames, libraryLoaded, libraryLoadError, runningGames } from '../../lib/stores/library';
import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { sortEntries } from '../../lib/library/query';
import type { Download } from '../../lib/services/downloads';
import { getCollections, getGames, playGame, type LibraryCollection, type LibraryGame } from '../../lib/services/library';
import { getCatalogGames } from '../../lib/services/sources';
import { collections, collectionsLoadError, collectionsState } from '../../lib/stores/collections';
import { downloads } from '../../lib/stores/downloads';
import { DEFAULT_CHOICES, libraryChoices, librarySort } from '../../lib/stores/libraryPrefs';
import { gameArt } from '../../lib/stores/metadata';
import { toast } from '../../lib/stores/toasts';
import { watchAlertsLoaded, watchlist } from '../../lib/stores/watchlist';
import type { Entry as WatchEntry } from '../../lib/services/watchlist';
import { libraryView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
  playGame: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/sources')>()),
  getCatalogGames: vi.fn(async () => []),
}));
vi.mock('../../lib/stores/toasts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(() => 0),
}));
vi.mock('../../lib/library/query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/library/query')>();
  return { ...actual, sortEntries: vi.fn(actual.sortEntries) };
});

function game(id: string, title: string, patch: Partial<LibraryGame> = {}): LibraryGame {
  return {
    id,
    title,
    executable: 'game.exe',
    installDir: `C:\\Games\\${id}`,
    cover: '',
    version: '',
    sizeBytes: 0,
    lastPlayed: null,
    playtimeSeconds: 0,
    installedAt: '',
    uninstalled: false,
    tags: [],
    ...patch,
  };
}

const hades = game('g-hades', 'Hades', { lastPlayed: '2026-11-01T10:00:00Z', canonicalGameId: 'c-hades' });
const celeste = game('g-celeste', 'Celeste', { canonicalGameId: 'c-celeste' });

function download(gameId: string, downloaded: number): Download {
  return { id: `d-${gameId}`, name: `${gameId}.torrent`, status: 'downloading', total: 100, downloaded, addedAt: '2026-11-01T00:00:00Z', origin: { gameId } } as Download;
}

const listTitles = () => [...document.querySelectorAll('.list-row .list-title')].map((el) => el.textContent?.trim());
const heading = (name: string) => screen.queryByRole('heading', { name });
const recentRow = () => screen.getAllByRole('heading', { level: 2, name: msg('games.recentLabel') })[0].closest('section') as HTMLElement;

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

beforeEach(() => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  librarySort.set('title');
  libraryView.set('list');
  libraryGames.set([hades, celeste]);
  libraryLoaded.set(true);
  libraryLoadError.set(null);
  collections.set([]);
  collectionsState.set('ready');
  collectionsLoadError.set(null);
  downloads.set([]);
  gameArt.set({});
  vi.mocked(toast).mockClear();
});

afterEach(() => {
  libraryGames.set([]);
  runningGames.set(new Set());
  document.body.replaceChildren();
});

describe('Library: why the list is empty', () => {
  it('shows loading until the first list arrives, not "empty"', () => {
    libraryLoaded.set(false);
    libraryGames.set([]);
    renderDom(Library);

    expect(heading(msg('library.loadingTitle'))).not.toBeNull();
    expect(heading(msg('games.libraryEmptyTitle'))).toBeNull();
  });

  it('says the library is empty once an empty list arrived', () => {
    libraryGames.set([]);
    renderDom(Library);

    expect(heading(msg('games.libraryEmptyTitle'))).not.toBeNull();
  });

  it('says every game is hidden when "All" has none to show', () => {
    libraryGames.set([{ ...hades, hidden: true }]);
    renderDom(Library);

    expect(heading(msg('library.allHiddenTitle'))).not.toBeNull();
  });

  it('names what narrows an empty list and clears all of it with one action', async () => {
    const { user } = renderDom(Library);
    await user.type(screen.getByLabelText(msg('games.librarySearchPlaceholder')), 'zzz');
    await user.click(screen.getByRole('button', { name: msg('games.filterHidden') }));

    expect(heading(msg('games.nothingFoundTitle'))).not.toBeNull();
    const what = [msg('library.narrowSearch'), msg('library.narrowFilter')].join(', ');
    expect(screen.getByText(msg('library.noMatchDescription', { what }))).toBeTruthy();

    const clear = screen.getByRole('button', { name: msg('library.clearFilters') });
    clear.focus();
    await user.keyboard('{Enter}');

    expect(listTitles()).toEqual(['Celeste', 'Hades']);
    expect((screen.getByLabelText(msg('games.librarySearchPlaceholder')) as HTMLInputElement).value).toBe('');
    expect(screen.getByRole('button', { name: msg('games.filterHidden') }).getAttribute('aria-pressed')).toBe('false');
    expect(document.activeElement?.closest('.list')).not.toBeNull();
  });

  it('hands focus to the search field when the list stays empty after clearing', async () => {
    libraryGames.set([{ ...hades, hidden: true }]);
    const { user } = renderDom(Library);
    const search = screen.getByLabelText(msg('games.librarySearchPlaceholder'));
    await user.type(search, 'zzz');
    const clear = screen.getByRole('button', { name: msg('library.clearFilters') });
    clear.focus();

    await user.keyboard('{Enter}');

    expect(heading(msg('library.allHiddenTitle'))).not.toBeNull();
    expect(clear.isConnected).toBe(false);
    expect(document.activeElement).toBe(search);
  });
});

describe('Library: a failed load', () => {
  function failed() {
    libraryLoaded.set(false);
    libraryGames.set([]);
    libraryLoadError.set(new Error('typhon:library.game_not_found: raw text'));
    const { user } = renderDom(Library);
    const notice = screen.getByText(msg('library.loadFailedTitle')).closest('.notice') as HTMLElement;
    return { user, notice, retry: within(notice).getByRole('button', { name: msg('common.retry') }) };
  }

  it('is a notice with Retry, and Retry loads the list', async () => {
    vi.mocked(getGames).mockResolvedValueOnce([hades]);
    const { user, notice, retry } = failed();
    expect(heading(msg('games.libraryEmptyTitle'))).toBeNull();
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(listTitles()).toEqual(['Hades']);
  });

  it('keeps the notice when the retry fails again, and says it again', async () => {
    vi.mocked(getGames).mockRejectedValueOnce(new Error('typhon:library.game_not_found: again'));
    const { user, notice, retry } = failed();
    const words = (notice.querySelector('.body')?.firstElementChild?.textContent ?? '').replace(/\s+/g, ' ').trim();
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(words));
    expect(words).not.toBe('');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });
});

describe('Library: collections that failed to load', () => {
  it('say so in the row and load on Retry', async () => {
    collectionsState.set('failed');
    collectionsLoadError.set(new Error('boom'));
    vi.mocked(getCollections).mockResolvedValueOnce([{ id: 'k1', name: 'Cozy', createdAt: '', updatedAt: '' } as LibraryCollection]);
    const { user } = renderDom(Library);
    const notice = screen.getByText(msg('games.collectionsLoadFailed')).closest('.notice') as HTMLElement;

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(screen.getByRole('button', { name: 'Cozy' })).toBeTruthy();
  });

  it('keep the notice when the retry fails again, and say it again', async () => {
    collectionsState.set('failed');
    collectionsLoadError.set(new Error('boom'));
    vi.mocked(getCollections).mockRejectedValueOnce(new Error('boom again'));
    const { user } = renderDom(Library);
    const notice = screen.getByText(msg('games.collectionsLoadFailed')).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    const words = (notice.querySelector('.body')?.firstElementChild?.textContent ?? '').replace(/\s+/g, ' ').trim();
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(words));
    expect(words).not.toBe('');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });
});

describe('Library: the Watched filter', () => {
  const entry = { gameId: 'c-watch', title: 'Watched Game' } as WatchEntry;

  afterEach(() => {
    watchlist.set([]);
    watchAlertsLoaded.set(false);
  });

  const chip = (name: string) => screen.queryByRole('button', { name });

  it('returns to All when the watchlist empties while the page is open', async () => {
    watchlist.set([entry]);
    watchAlertsLoaded.set(true);
    const { user } = renderDom(Library);
    await user.click(screen.getByRole('button', { name: msg('games.filterWatched') }));
    expect(listTitles()).toEqual(['Watched Game']);

    watchlist.set([]);
    await tick();

    expect(chip(msg('games.filterAll'))?.getAttribute('aria-pressed')).toBe('true');
    expect(chip(msg('games.filterWatched'))).toBeNull();
    expect(listTitles()).toEqual(['Celeste', 'Hades']);
  });

  it('shows All when Watched was stored and the watchlist is known to be empty', () => {
    libraryChoices.set({ ...DEFAULT_CHOICES, filter: 'watched' });
    watchAlertsLoaded.set(true);
    renderDom(Library);

    expect(chip(msg('games.filterAll'))?.getAttribute('aria-pressed')).toBe('true');
    expect(chip(msg('games.filterWatched'))).toBeNull();
    expect(listTitles()).toEqual(['Celeste', 'Hades']);
  });

  it('keeps a stored Watched while the watchlist has not loaded yet, with its chip on screen', () => {
    libraryChoices.set({ ...DEFAULT_CHOICES, filter: 'watched' });
    renderDom(Library);

    expect(chip(msg('games.filterWatched'))?.getAttribute('aria-pressed')).toBe('true');
  });
});

describe('Library: the recent games', () => {
  const alpha = game('g-alpha', 'Alpha', { lastPlayed: '2026-10-01T10:00:00Z' });
  const beta = game('g-beta', 'Beta', { lastPlayed: '2026-11-01T10:00:00Z' });
  const gamma = game('g-gamma', 'Gamma', { lastPlayed: '2026-10-15T10:00:00Z' });
  const delta = game('g-delta', 'Delta');
  const echo = game('g-echo', 'Echo', { uninstalled: true, lastPlayed: '2026-11-05T10:00:00Z' });
  // Played before the carousel's three (Beta, Gamma, Alpha): the row holds these.
  const foxtrot = game('g-foxtrot', 'Foxtrot', { lastPlayed: '2026-09-15T10:00:00Z' });
  const golf = game('g-golf', 'Golf', { lastPlayed: '2026-09-01T10:00:00Z' });

  beforeEach(() => libraryGames.set([alpha, beta, gamma, delta, echo, foxtrot, golf]));

  it('"Show all" lists the games of the row in play order, whatever the sort', async () => {
    const { user } = renderDom(Library);

    await user.click(screen.getByRole('button', { name: msg('games.libraryShowAllButton') }));

    expect(listTitles()).toEqual(['Beta', 'Gamma', 'Alpha', 'Foxtrot', 'Golf']);
  });

  it('keeps the carousel\'s games in the row once the carousel is put away', async () => {
    const { user } = renderDom(Library);
    expect(within(recentRow()).queryAllByText('Beta')).toHaveLength(0);

    await user.click(screen.getByRole('button', { name: msg('library.carousel.hide') }));

    expect(within(recentRow()).queryAllByText('Beta').length).toBeGreaterThan(0);
  });

  it('caps the row at twelve games', () => {
    libraryGames.set(Array.from({ length: 15 }, (_, i) => game(`g-${i}`, `Game ${i}`, { lastPlayed: `2026-10-${String(i + 1).padStart(2, '0')}T10:00:00Z` })));
    renderDom(Library);

    expect(within(recentRow()).getAllByRole('button', { name: msg('ui.play') })).toHaveLength(12);
  });

  it('moves focus on when a game started from its capsule leaves the row for the carousel', async () => {
    const { user } = renderDom(Library);
    const play = within(recentRow()).getAllByRole('button', { name: msg('ui.play') })[0];
    play.focus();

    await user.keyboard('{Enter}');
    runningGames.set(new Set(['g-foxtrot']));
    await tick();
    await tick();

    expect(screen.getByRole('region', { name: msg('library.carousel.label') }).textContent).toContain('Foxtrot');
    expect(play.isConnected).toBe(false);
    expect(document.activeElement).not.toBe(document.body);
    expect(recentRow().contains(document.activeElement)).toBe(true);
  });
});

describe('Library: the player\'s choices', () => {
  it('survive leaving the page and coming back', async () => {
    const first = renderDom(Library);
    await first.user.click(screen.getByRole('button', { name: msg('games.filterInstalled') }));
    await first.user.type(screen.getByLabelText(msg('games.librarySearchPlaceholder')), 'ha');
    await first.user.click(screen.getByRole('button', { name: msg('library.carousel.hide') }));
    first.unmount();

    renderDom(Library);

    expect(screen.getByRole('button', { name: msg('games.filterInstalled') }).getAttribute('aria-pressed')).toBe('true');
    expect((screen.getByLabelText(msg('games.librarySearchPlaceholder')) as HTMLInputElement).value).toBe('ha');
    expect(screen.queryByRole('region', { name: msg('library.carousel.label') })).toBeNull();
    expect(listTitles()).toEqual(['Hades']);
  });
});

describe('Library: the source menu', () => {
  it('leaves out the store of a game that is only in "Hidden"', async () => {
    libraryGames.set([
      hades,
      game('g-witcher', 'Witcher', { external: { provider: 'steam', appId: '1', launchUri: 'steam://run/1' } }),
      game('g-secret', 'Secret', { source: 'discovered', hidden: true }),
    ]);
    const { user } = renderDom(Library);

    await user.click(screen.getByRole('button', { name: msg('games.filterSourceAll') }));

    const items = screen.getAllByRole('menuitemradio').map((item) => item.textContent?.trim());
    expect(items).toEqual([msg('games.filterSourceAll'), msg('games.sourceTyphon'), msg('games.sourceSteam')]);
  });

  const witcher = game('g-witcher', 'Witcher', { external: { provider: 'steam', appId: '1', launchUri: 'steam://run/1' } });
  const menuItems = () => screen.getAllByRole('menuitemradio').map((item) => item.textContent?.trim());

  it('stays on screen while a stored source narrows the list, even one no game has now', async () => {
    libraryChoices.set({ ...DEFAULT_CHOICES, source: 'disk' });
    const { user } = renderDom(Library);

    expect(listTitles()).toEqual([]);
    const trigger = screen.getByRole('button', { name: msg('games.sourceDisk') });
    await user.click(trigger);

    expect(menuItems()).toEqual([msg('games.filterSourceAll'), msg('games.sourceTyphon'), msg('games.sourceDisk')]);
  });

  it('stays on screen when hiding the last game of the other source leaves one source', async () => {
    libraryGames.set([hades, witcher]);
    const { user } = renderDom(Library);
    await user.click(screen.getByRole('button', { name: msg('games.filterSourceAll') }));
    await user.click(screen.getByRole('menuitemradio', { name: msg('games.sourceSteam') }));
    expect(listTitles()).toEqual(['Witcher']);

    libraryGames.set([{ ...hades, hidden: true }, witcher]);
    await tick();

    expect(screen.getByRole('button', { name: msg('games.sourceSteam') })).toBeTruthy();
    expect(listTitles()).toEqual(['Witcher']);
  });

  it('offers inside "Hidden" the sources of the hidden games', async () => {
    libraryGames.set([
      hades,
      { ...witcher, hidden: true },
      game('g-secret', 'Secret', { source: 'discovered', hidden: true }),
    ]);
    const { user } = renderDom(Library);
    await user.click(screen.getByRole('button', { name: msg('games.filterHidden') }));

    await user.click(screen.getByRole('button', { name: msg('games.filterSourceAll') }));

    expect(menuItems()).toEqual([msg('games.filterSourceAll'), msg('games.sourceSteam'), msg('games.sourceDisk')]);
  });
});

describe('Library: cheap updates', () => {
  it('asks the catalog once for a download it does not know, over many progress ticks', async () => {
    renderDom(Library);
    for (let i = 0; i < 10; i += 1) {
      downloads.set([download('c-ticking', i)]);
      await tick();
    }

    expect(vi.mocked(getCatalogGames).mock.calls.filter(([ids]) => ids.includes('c-ticking'))).toHaveLength(1);
  });

  it('re-sorts nothing when one game\'s art arrives or a download ticks', async () => {
    downloads.set([download('c-quiet', 1)]);
    renderDom(Library);
    await tick();
    const sorted = vi.mocked(sortEntries).mock.calls.length;
    expect(sorted).toBeGreaterThan(0);

    gameArt.set({ 'c-hades': { cover: 'hades.png', hero: '', heroIsPoster: false } });
    downloads.set([download('c-quiet', 50)]);
    await tick();

    expect(vi.mocked(sortEntries).mock.calls.length).toBe(sorted);
  });
});

describe('Library: errors of Play', () => {
  it.each([
    ['a coded error shows the text of its code', new Error('typhon:library.game_not_found: сырой текст'), msg('library.game_not_found')],
    ['an unknown code shows the general text', new Error('typhon:library.no_such_code: сырой текст'), msg('games.errorPlayFailed')],
    ['an uncoded error shows the general text', new Error('Не удалось: сырой текст'), msg('games.errorPlayFailed')],
  ])('%s, never the raw message', async (_name, err, expected) => {
    vi.mocked(playGame).mockRejectedValueOnce(err);
    const { user } = renderDom(Library);

    await user.click(within(screen.getByRole('region', { name: msg('library.carousel.label') })).getByRole('button', { name: msg('ui.play') }));

    await vi.waitFor(() => expect(toast).toHaveBeenCalled());
    const [text, kind] = vi.mocked(toast).mock.calls[0];
    expect(kind).toBe('danger');
    expect(text).toBe(expected);
    expect(text).not.toContain('сырой');
  });
});

describe('Library: the order of titles', () => {
  it('follows the interface language, also when it changes while the page is open', async () => {
    libraryGames.set([game('g-witcher', 'Witcher'), game('g-vedmak', 'Ведьмак')]);
    locale.set('ru');
    renderDom(Library);
    expect(listTitles()).toEqual(['Ведьмак', 'Witcher']);

    locale.set('en');
    await tick();

    expect(listTitles()).toEqual(['Witcher', 'Ведьмак']);
  });
});
