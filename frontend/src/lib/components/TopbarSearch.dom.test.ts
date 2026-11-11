import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/search', () => ({ searchAll: vi.fn() }));
vi.mock('../services/watchlist', async (actual) => ({
  ...(await actual<typeof import('../services/watchlist')>()),
  watchGame: vi.fn(),
  unwatchGame: vi.fn(),
}));

import { locale } from '../i18n';
import { debounceMs } from '../search/overlay';
import { searchAll, type GameHit, type ReleaseHit, type SearchResult } from '../services/search';
import { watchGame } from '../services/watchlist';
import { navigate, resetHistory, route } from '../stores/router';
import { toasts } from '../stores/toasts';
import { watchlist } from '../stores/watchlist';
import { user } from '../testing/dom';
import SearchHarness from '../testing/SearchHarness.svelte';

const game = (id: string, title: string, more: Partial<GameHit> = {}): GameHit => ({ id, title, installed: false, releases: 2, sources: 1, score: 1, ...more });
const release = (id: string, title: string): ReleaseHit => ({ id, sourceId: 'feed', sourceName: 'Feed A', title, size: 0 });

const DOOM = game('g1', 'Doom', { year: 1993 });
const DOOM2 = game('g2', 'Doom II', { installed: true });
const RELEASE = release('r1', 'DOOM-GOG');
const FOUND: SearchResult = { query: 'doom', games: [DOOM, DOOM2], releases: [RELEASE], moreGames: 4, moreReleases: 2 };
const NOTHING: SearchResult = { query: 'zzz', games: [], releases: [], moreGames: 0, moreReleases: 0 };

let app: HTMLDivElement;

beforeEach(() => {
  vi.useFakeTimers();
  locale.set('en');
  resetHistory();
  watchlist.set([]);
  toasts.set([]);
  vi.mocked(searchAll).mockReset();
  vi.mocked(searchAll).mockResolvedValue(FOUND);
  vi.mocked(watchGame).mockReset();
  vi.mocked(watchGame).mockImplementation(async (gameId: string) => ({ gameId }) as never);
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

function setup(props: { dialog?: boolean } = {}) {
  const view = render(SearchHarness, { target: app, props });
  const keys = user();
  const field = () => screen.getByRole('combobox', { name: 'Search games and releases' }) as HTMLInputElement;
  const grid = () => screen.queryByRole('grid');
  const popup = () => document.querySelector<HTMLElement>('[data-part="popover"][data-id="search"]');
  const active = () => {
    const id = field().getAttribute('aria-activedescendant');
    return id ? document.getElementById(id) : null;
  };
  const said = () => screen.getAllByRole('status').map((region) => region.textContent?.trim() ?? '').filter(Boolean);
  // The search answers after its debounce; the list is on screen a tick later.
  const settle = async () => {
    await vi.advanceTimersByTimeAsync(debounceMs);
    await tick();
    await tick();
  };
  const find = async (text = 'doom') => {
    await keys.click(field());
    await keys.keyboard(text);
    await settle();
  };
  return { keys, field, grid, popup, active, said, settle, find, rerender: view.rerender };
}

const cellsOf = (row: HTMLElement) => within(row).getAllByRole('gridcell');
const rowOf = (name: RegExp) => screen.getAllByRole('row').find((row) => name.test(row.textContent ?? '')) as HTMLElement;

describe('TopbarSearch: the field and its results', () => {
  it('is a named combobox that says it has no results open', () => {
    const { field } = setup();
    expect(field().getAttribute('aria-expanded')).toBe('false');
    expect(field().getAttribute('aria-autocomplete')).toBe('list');
    expect(field().getAttribute('aria-haspopup')).toBe('grid');
    expect(field().hasAttribute('aria-controls')).toBe(false);
    expect(field().hasAttribute('aria-activedescendant')).toBe(false);
  });

  it('typing opens the results after the debounce, out of the page, as a named grid of grouped rows; focus stays in the field', async () => {
    const { keys, field, grid, popup } = setup();
    await keys.click(field());
    await keys.keyboard('doom');
    await vi.advanceTimersByTimeAsync(debounceMs - 1);
    await tick();
    expect(grid()).toBeNull();

    await vi.advanceTimersByTimeAsync(1);
    await tick();
    await tick();

    const results = screen.getByRole('grid', { name: 'Search results' });
    expect(popup()?.contains(results)).toBe(true);
    expect(app.contains(results)).toBe(false);
    expect(field().getAttribute('aria-expanded')).toBe('true');
    expect(field().getAttribute('aria-controls')).toBe(results.id);
    expect(document.activeElement).toBe(field());
    expect(searchAll).toHaveBeenCalledExactlyOnceWith('doom');

    const games = screen.getByRole('rowgroup', { name: 'Games' });
    const releases = screen.getByRole('rowgroup', { name: 'Releases without a match' });
    expect(within(games).getAllByRole('row').map((row) => cellsOf(row).length)).toEqual([2, 1, 1]);
    expect(within(releases).getAllByRole('row').map((row) => cellsOf(row).length)).toEqual([1]);
    expect(within(games).getAllByRole('row')[2].textContent).toContain('4 more in the catalog');
    expect(popup()?.textContent).toContain('and 2 more');
  });

  it('no part of the results is a Tab stop: the arrow keys are the one way through them', async () => {
    const { find, popup } = setup();
    await find();
    const stops = [...(popup() as HTMLElement).querySelectorAll<HTMLElement>('button, [href], input, [tabindex]')].filter((el) => el.tabIndex >= 0);
    expect(stops).toEqual([]);
  });

  it('is placed under the search island and exactly as wide', async () => {
    const island = DOMRect.fromRect({ x: 300, y: 50, width: 520, height: 36 });
    const real = HTMLElement.prototype.getBoundingClientRect;
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.dataset.part === 'topbar-search') return island;
      if (this.dataset.part === 'popover') return DOMRect.fromRect({ x: 0, y: 0, width: 520, height: 300 });
      return real.call(this);
    });
    const { find, popup } = setup();
    await find();
    const shown = popup() as HTMLElement;
    expect(shown.style.position).toBe('fixed');
    expect([shown.style.left, shown.style.top, shown.style.minWidth]).toEqual(['300px', '92px', '520px']);
    expect(shown.dataset.placement).toBe('bottom-start');
  });
});

describe('TopbarSearch: keys', () => {
  it('ArrowDown and ArrowUp walk every row across the groups and wrap; aria-activedescendant follows', async () => {
    const { keys, find, active, field } = setup();
    await find();
    expect(active()).toBeNull();

    const seen: string[] = [];
    for (let i = 0; i < 5; i++) {
      await keys.keyboard('{ArrowDown}');
      seen.push(active()?.textContent?.trim().replace(/\s+/g, ' ') ?? '');
    }
    expect(seen[0]).toContain('Doom');
    expect(seen[1]).toContain('Doom II');
    expect(seen[2]).toContain('4 more in the catalog');
    expect(seen[3]).toContain('DOOM-GOG');
    expect(seen[4]).toBe(seen[0]);
    expect(active()?.getAttribute('role')).toBe('gridcell');

    await keys.keyboard('{ArrowUp}');
    expect(active()?.textContent).toContain('DOOM-GOG');
    expect(document.activeElement).toBe(field());
  });

  it('scrolls the results, and only them, to the active row', async () => {
    const { keys, find, popup } = setup();
    await find();
    const box = popup() as HTMLElement;
    Object.defineProperty(box, 'clientHeight', { configurable: true, value: 100 });
    screen.getAllByRole('row').forEach((row, index) => {
      Object.defineProperty(row, 'offsetTop', { configurable: true, value: index * 60 });
      Object.defineProperty(row, 'offsetHeight', { configurable: true, value: 60 });
    });

    await keys.keyboard('{ArrowUp}');
    expect(box.scrollTop).toBe(3 * 60 + 60 - 100);
    await keys.keyboard('{ArrowDown}');
    expect(box.scrollTop).toBe(0);
  });

  it('keeps the active row in view when a newer answer moves it further down', async () => {
    const moved: SearchResult = { query: 'doom', games: [game('g3', 'Doom 3'), game('g4', 'Doom Eternal'), DOOM], releases: [], moreGames: 0, moreReleases: 0 };
    vi.mocked(searchAll).mockResolvedValueOnce(FOUND).mockResolvedValueOnce(moved);
    // Each row 60px high, stacked in document order.
    vi.spyOn(HTMLElement.prototype, 'offsetTop', 'get').mockImplementation(function (this: HTMLElement) {
      return this.getAttribute('role') === 'row' ? [...document.querySelectorAll('[role="row"]')].indexOf(this) * 60 : 0;
    });
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockImplementation(function (this: HTMLElement) {
      return this.getAttribute('role') === 'row' ? 60 : 0;
    });
    const { keys, find, popup, active, settle } = setup();
    await find('doo');
    const box = popup() as HTMLElement;
    Object.defineProperty(box, 'clientHeight', { configurable: true, value: 130 });

    // The keys go down while the search for the next letter is running.
    await keys.keyboard('m');
    await keys.keyboard('{ArrowDown}');
    expect(active()?.textContent).toContain('Doom');
    expect(box.scrollTop).toBe(0);

    await settle();
    expect(popup()).toBe(box);
    const kept = active() as HTMLElement;
    expect(kept.closest('[role="row"]')).toBe(screen.getAllByRole('row')[2]);
    expect(box.scrollTop).toBe(2 * 60 + 60 - 130);
  });

  it('ArrowRight and ArrowLeft move between the cells of the active row, and are the caret\'s keys with none active', async () => {
    const { keys, find, active, field } = setup();
    await find();
    await keys.keyboard('{ArrowLeft}');
    expect(field().selectionStart).toBe(3);
    expect(active()).toBeNull();

    await keys.keyboard('{ArrowDown}');
    const row = screen.getAllByRole('row')[0];
    await keys.keyboard('{ArrowRight}');
    expect(active()).toBe(cellsOf(row)[1]);
    expect(within(active() as HTMLElement).getByRole('button', { name: 'Watch for releases' })).toBeTruthy();
    expect(field().selectionStart).toBe(3);
    await keys.keyboard('{ArrowRight}');
    expect(active()).toBe(cellsOf(row)[1]);
    await keys.keyboard('{ArrowLeft}');
    expect(active()).toBe(cellsOf(row)[0]);
  });

  it('Home and End stay the text\'s keys: they leave the rows for the caret', async () => {
    const { keys, find, active, field, grid } = setup();
    await find();
    await keys.keyboard('{ArrowDown}');
    await keys.keyboard('{Home}');
    expect(active()).toBeNull();
    expect(field().selectionStart).toBe(0);
    expect(grid()).not.toBeNull();
  });

  it('typing takes the keys back to the text: the row is no longer active', async () => {
    const { keys, find, active } = setup();
    await find();
    await keys.keyboard('{ArrowDown}');
    await keys.keyboard('s');
    expect(active()).toBeNull();
  });

  it('Enter on a result opens it, closes the results and leaves the field: the new page has the keys', async () => {
    const { keys, find, grid, field } = setup();
    await find();
    await keys.keyboard('{ArrowDown}{Enter}');
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
    expect(grid()).toBeNull();
    expect(document.activeElement).not.toBe(field());

    await keys.keyboard('{ArrowDown}');
    expect(grid()).toBeNull();
  });

  it('Enter on a release opens it in its source and leaves the field', async () => {
    const { keys, find, field } = setup();
    await find();
    await keys.keyboard('{ArrowUp}{Enter}');
    expect(get(route)).toMatchObject({ name: 'sources', params: { sourceId: 'feed', releaseId: 'r1' } });
    expect(document.activeElement).not.toBe(field());
  });

  it('Enter on "more in the catalog", and Enter with no row active, search the catalog and leave the field', async () => {
    const first = setup();
    await first.find();
    await first.keys.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}{Enter}');
    expect(get(route)).toMatchObject({ name: 'catalog', params: { q: 'doom' } });
    expect(document.activeElement).not.toBe(first.field());

    resetHistory();
    await first.keys.click(first.field());
    await first.settle();
    await first.keys.keyboard('{Enter}');
    expect(get(route)).toMatchObject({ name: 'catalog', params: { q: 'doom' } });
    expect(document.activeElement).not.toBe(first.field());
  });

  it('Enter on a watch cell toggles the watch and keeps the results open and the focus in the field', async () => {
    const { keys, find, grid, field, active } = setup();
    await find();
    await keys.keyboard('{ArrowDown}{ArrowRight}{Enter}');
    await tick();
    expect(watchGame).toHaveBeenCalledExactlyOnceWith('g1');
    expect(get(route).name).toBe('library');
    expect(grid()).not.toBeNull();
    expect(document.activeElement).toBe(field());
    expect(within(active() as HTMLElement).getByRole('button', { name: 'Stop watching' })).toBeTruthy();
  });

  it('the watch toggle says what pressing it does, by its name alone: no "pressed" on top of "Stop watching"', async () => {
    const { keys, find } = setup();
    await find();
    const toggle = screen.getByRole('button', { name: 'Watch for releases' });
    expect(toggle.hasAttribute('aria-pressed')).toBe(false);

    await keys.click(toggle);
    await tick();
    const watching = screen.getByRole('button', { name: 'Stop watching' });
    expect(watching.hasAttribute('aria-pressed')).toBe(false);
    expect(screen.queryByRole('button', { name: 'Watch for releases' })).toBeNull();
  });

  it('a watch that fails says so and leaves the results as they were', async () => {
    vi.mocked(watchGame).mockRejectedValue(new Error('boom'));
    const { keys, find, grid } = setup();
    await find();
    await keys.keyboard('{ArrowDown}{ArrowRight}{Enter}');
    await tick();
    expect(get(toasts).map((toast) => toast.kind)).toEqual(['danger']);
    expect(grid()).not.toBeNull();
  });

  it('a held Enter opens one result and nothing after it', async () => {
    const { keys, find } = setup();
    await find();
    await keys.keyboard('{ArrowDown}{Enter>4/}');
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g1' } });
  });

  it('Escape closes the results and keeps the text; a second Escape clears the field', async () => {
    const { keys, find, grid, field } = setup();
    await find();
    await keys.keyboard('{Escape}');
    expect(grid()).toBeNull();
    expect(field().value).toBe('doom');
    expect(document.activeElement).toBe(field());

    await keys.keyboard('{Escape}');
    expect(field().value).toBe('');
    await vi.advanceTimersByTimeAsync(debounceMs);
    expect(grid()).toBeNull();
  });

  it('Tab closes the results and moves on to the next control', async () => {
    const { keys, find, grid } = setup();
    await find();
    await keys.tab();
    expect(grid()).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'After' }));
  });

  it('the field losing focus closes the results and does not take focus back (a browser has <body> focused mid-way)', async () => {
    const { find, field, grid } = setup();
    await find();

    field().blur();
    await tick();

    expect(grid()).toBeNull();
    expect(document.activeElement).toBe(document.body);
  });

  it('results that arrive after focus has left the field do not open', async () => {
    const { keys, field, grid, settle } = setup();
    await keys.click(field());
    await keys.keyboard('doom');
    await keys.tab();
    await settle();
    expect(grid()).toBeNull();
    expect(screen.queryByText('Doom II')).toBeNull();
  });
});

describe('TopbarSearch: pointer', () => {
  it('a press on a result opens it; the results close and the field lets go, so the arrows do not bring them back', async () => {
    const { keys, find, grid, field } = setup();
    await find();
    await keys.click(cellsOf(rowOf(/Doom II/))[0]);
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g2' } });
    expect(grid()).toBeNull();
    expect(document.activeElement).not.toBe(field());

    await keys.keyboard('{ArrowDown}');
    expect(grid()).toBeNull();
  });

  it('a press on a release or on "more in the catalog" leaves the field too', async () => {
    const { keys, find, field } = setup();
    await find();
    await keys.click(cellsOf(rowOf(/DOOM-GOG/))[0]);
    expect(get(route).name).toBe('sources');
    expect(document.activeElement).not.toBe(field());

    await keys.click(field());
    await keys.click(cellsOf(rowOf(/more in the catalog/))[0]);
    expect(get(route)).toMatchObject({ name: 'catalog', params: { q: 'doom' } });
    expect(document.activeElement).not.toBe(field());
  });

  it('a press on a watch toggle toggles it, keeps the results open and the focus in the field', async () => {
    const { keys, find, grid, field } = setup();
    await find();
    await keys.click(screen.getByRole('button', { name: 'Watch for releases' }));
    await tick();
    expect(watchGame).toHaveBeenCalledExactlyOnceWith('g1');
    expect(get(route).name).toBe('library');
    expect(grid()).not.toBeNull();
    expect(document.activeElement).toBe(field());
  });

  it('the pointer moving over a row makes it the active one, and the arrow keys go on from there', async () => {
    const { keys, find, active } = setup();
    await find();
    await keys.keyboard('{ArrowDown}');
    const second = cellsOf(rowOf(/Doom II/))[0];
    await fireEvent.pointerMove(second);
    expect(active()).toBe(second);

    await keys.keyboard('{ArrowDown}');
    expect(active()?.textContent).toContain('4 more in the catalog');
    await keys.keyboard('{ArrowDown}');
    expect(active()?.textContent).toContain('DOOM-GOG');
  });

  it('rows passing under a resting pointer do not take the active row from the keyboard; a moving pointer does', async () => {
    const { keys, find, active } = setup();
    await find();
    await keys.keyboard('{ArrowDown}');
    const first = active();
    const second = cellsOf(rowOf(/Doom II/))[0];
    const watch = cellsOf(screen.getAllByRole('row')[0])[1];

    // What a browser sends when the list scrolls under a pointer that has not moved.
    for (const cell of [second, watch]) {
      await fireEvent.pointerOver(cell);
      await fireEvent.pointerEnter(cell);
      await fireEvent.mouseOver(cell);
      await fireEvent.mouseEnter(cell);
    }
    expect(active()).toBe(first);

    await fireEvent.pointerMove(watch);
    expect(active()).toBe(watch);
    await fireEvent.pointerMove(second);
    expect(active()).toBe(second);
  });

  it('a press outside closes the results; a press in the island beside the text keeps them and the focus', async () => {
    const { keys, find, grid, field } = setup();
    await find();
    const beside = field().parentElement as HTMLElement;
    const pressed = await fireEvent.mouseDown(beside);
    await keys.pointer({ keys: '[MouseLeft]', target: beside });
    expect(pressed).toBe(false);
    expect(document.activeElement).toBe(field());
    expect(grid()).not.toBeNull();
    await keys.click(document.body);
    expect(grid()).toBeNull();
  });

  it('a press in the island beside the text puts focus in the field', async () => {
    const { keys, field } = setup();
    await keys.pointer({ keys: '[MouseLeft]', target: field().parentElement as HTMLElement });
    expect(document.activeElement).toBe(field());
  });
});

describe('TopbarSearch: what closes it and what it says', () => {
  it('a route change closes the results', async () => {
    const { find, grid } = setup();
    await find();
    expect(grid()).not.toBeNull();
    navigate('settings');
    await tick();
    expect(grid()).toBeNull();
  });

  it('a dialog opening closes the results', async () => {
    const { find, grid, rerender } = setup();
    await find();
    expect(grid()).not.toBeNull();
    await rerender({ dialog: true });
    await tick();
    expect(screen.getByRole('dialog', { name: 'Other' })).toBeTruthy();
    expect(grid()).toBeNull();
  });

  it('Ctrl+K and Cmd+K focus the field from anywhere, and do nothing while a dialog is open', async () => {
    const { keys, field, rerender } = setup();
    screen.getByRole('button', { name: 'After' }).focus();
    await keys.keyboard('{Control>}k{/Control}');
    expect(document.activeElement).toBe(field());
    screen.getByRole('button', { name: 'After' }).focus();
    await keys.keyboard('{Meta>}k{/Meta}');
    expect(document.activeElement).toBe(field());

    await rerender({ dialog: true });
    await tick();
    const inDialog = document.activeElement;
    await keys.keyboard('{Control>}k{/Control}');
    expect(document.activeElement).toBe(inDialog);
  });

  it('Ctrl+K and Cmd+K go by where the key sits: a Russian layout focuses the field too, AltGr does not', async () => {
    const { field } = setup();
    const after = screen.getByRole('button', { name: 'After' });
    const press = async (init: KeyboardEventInit) => {
      after.focus();
      await fireEvent.keyDown(after, init);
      return document.activeElement === field();
    };

    expect(await press({ key: 'л', code: 'KeyK', ctrlKey: true })).toBe(true);
    expect(await press({ key: 'л', code: 'KeyK', metaKey: true })).toBe(true);
    expect(await press({ key: 'k', code: 'KeyK', ctrlKey: true })).toBe(true);
    expect(await press({ key: 'k', code: 'KeyK', ctrlKey: true, altKey: true })).toBe(false);
    expect(await press({ key: 'л', code: 'KeyK' })).toBe(false);
    expect(await press({ key: 'k', code: 'KeyL', ctrlKey: true })).toBe(false);
  });

  it('says that it is searching, then how many results came back', async () => {
    const { keys, field, said, settle } = setup();
    await keys.click(field());
    await keys.keyboard('doom');
    expect(said()).toEqual(['Searching…']);
    await settle();
    expect(said()).toEqual(['3 results']);
  });

  it('marks results on screen as stale while a newer search runs', async () => {
    const { keys, find, settle } = setup();
    await find();
    await keys.keyboard('s');
    expect(screen.getByRole('grid').getAttribute('aria-busy')).toBe('true');
    await settle();
    expect(screen.getByRole('grid').getAttribute('aria-busy')).toBe('false');
  });

  it('says that nothing was found, on screen and to a screen reader, with no grid', async () => {
    vi.mocked(searchAll).mockResolvedValue(NOTHING);
    const { find, grid, popup, said, field } = setup();
    await find('zzz');
    expect(grid()).toBeNull();
    expect(popup()?.textContent).toContain('Nothing found');
    expect(said()).toEqual(['Nothing found']);
    expect(field().getAttribute('aria-expanded')).toBe('false');
  });

  it('says why a search failed, on screen and to a screen reader', async () => {
    vi.mocked(searchAll).mockRejectedValue(new Error('down'));
    const { find, popup, said } = setup();
    await find();
    expect(popup()?.textContent).toContain('Search unavailable');
    expect(said()).toEqual(['Search unavailable']);
  });

  it('keeps "nothing found" on screen, marked busy, while the next search runs', async () => {
    vi.mocked(searchAll).mockResolvedValue(NOTHING);
    const { keys, find, popup, settle } = setup();
    await find('zzz');
    await keys.keyboard('z');
    await tick();
    expect(popup()?.textContent).toContain('Nothing found');
    expect(screen.getByText('Nothing found', { selector: '.results-note' }).getAttribute('aria-busy')).toBe('true');
    await keys.keyboard('z');
    await tick();
    expect(popup()).not.toBeNull();

    await settle();
    expect(searchAll).toHaveBeenLastCalledWith('zzzzz');
    expect(screen.getByText('Nothing found', { selector: '.results-note' }).getAttribute('aria-busy')).toBe('false');
  });

  it('keeps the error on screen, marked busy, while the next search runs', async () => {
    vi.mocked(searchAll).mockRejectedValueOnce(new Error('down'));
    const { keys, find, popup, settle, grid } = setup();
    await find();
    await keys.keyboard('s');
    await tick();
    expect(popup()?.textContent).toContain('Search unavailable');
    expect(screen.getByText('Search unavailable', { selector: '.results-note' }).getAttribute('aria-busy')).toBe('true');

    await settle();
    expect(popup()?.textContent).not.toContain('Search unavailable');
    expect(grid()).not.toBeNull();
  });

  it('shows nothing while the first search is still running: no empty box', async () => {
    const { keys, field, popup } = setup();
    await keys.click(field());
    await keys.keyboard('doom');
    await tick();
    expect(popup()).toBeNull();
    expect(document.querySelector('.results')).toBeNull();
  });
});
