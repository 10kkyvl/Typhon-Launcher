// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames } from '../stores/library';
import { render, screen } from '@testing-library/svelte';
import type { Writable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { launchesInFlight, type LibraryGame } from '../services/library';
import { getMetadataView, type MetadataView } from '../services/metadata';
import { getCatalogGame, type CatalogGame } from '../services/sources';
import { user } from '../testing/dom';
import { moveFocus } from './navigation';
import GamePage from './components/GamePage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => {
  const { writable } = await import('svelte/store');
  return {
    ...(await importOriginal<typeof import('../services/library')>()),
    // A launch another screen started: the service's own set, written by the test.
    launchesInFlight: writable<ReadonlySet<string>>(new Set()),
  };
});
vi.mock('../services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/metadata')>()),
  getMetadataView: vi.fn(),
  ensureMetadataFresh: vi.fn(async () => false),
}));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getCatalogGame: vi.fn(),
  getReleasesForGame: vi.fn(async () => []),
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

const inFlight = launchesInFlight as unknown as Writable<ReadonlySet<string>>;

let app: HTMLDivElement;

beforeEach(() => {
  inFlight.set(new Set());
  app = document.createElement('div');
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'c1', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame]);
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
});

const scrollIntoView = Element.prototype.scrollIntoView;

afterEach(() => {
  app.remove();
  libraryGames.set([]);
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getMetadataView).mockReset();
  vi.restoreAllMocks();
  Element.prototype.scrollIntoView = scrollIntoView;
});

// jsdom has no layout: the hero's action row stands in one line, everything else far below it, so
// the navigator sees Launch and its neighbours where the real page puts them.
function layOutHeroActions() {
  const rect = (element: Element) => {
    const row = element.closest('.hero-actions');
    const index = row ? [...row.children].indexOf(element) : 0;
    const left = row ? index * 300 : 0;
    const top = row ? 500 : 3000 + [...document.querySelectorAll('[data-bp-focus]')].indexOf(element) * 100;
    return { left, top, right: left + 250, bottom: top + 60, width: 250, height: 60, x: left, y: top, toJSON: () => ({}) } as DOMRect;
  };
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    return rect(this);
  });
  vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(function (this: Element) {
    return [rect(this)] as unknown as DOMRectList;
  });
  // The navigator scrolls the control it focuses into view; jsdom has nothing to scroll.
  Element.prototype.scrollIntoView = () => {};
}

function renderPage(onlaunch: (id: string) => Promise<boolean>) {
  render(GamePage, { target: app, props: { id: 'g1', onback: vi.fn(), ondownloads: vi.fn(), onlaunch, requestText: vi.fn(async () => null) } });
}

// The launch Big Picture hands the page waits while a question (the save conflict) is open.
describe('Big Picture game page: a launch that waits on a question', () => {
  it('shows no error while the launch waits, and after it was given up Play works again', async () => {
    const asked = deferred<boolean>();
    const onlaunch = vi.fn().mockReturnValueOnce(asked.promise).mockReturnValueOnce(new Promise(() => {}));
    renderPage(onlaunch);
    const keys = user();
    const play = await screen.findByRole('button', { name: msg('bp.game.launch') });

    await keys.click(play);
    expect(onlaunch).toHaveBeenCalledWith('g1');
    expect(screen.queryByRole('alert')).toBeNull();

    asked.resolve(false);
    await vi.waitFor(() => expect(play).toHaveProperty('disabled', false));
    expect(screen.queryByRole('alert')).toBeNull();

    await keys.click(play);
    expect(onlaunch).toHaveBeenCalledTimes(2);
  });

  it('keeps Play focused and busy, not disabled, while the launch runs; a second press sends nothing', async () => {
    const launch = deferred<boolean>();
    const onlaunch = vi.fn().mockReturnValueOnce(launch.promise);
    renderPage(onlaunch);
    const keys = user();
    const play = await screen.findByRole('button', { name: msg('bp.game.launch') });
    play.focus();

    await keys.keyboard('{Enter}');
    await keys.click(play);

    expect(onlaunch).toHaveBeenCalledTimes(1);
    expect(play.hasAttribute('disabled')).toBe(false);
    expect(play.getAttribute('aria-busy')).toBe('true');
    // Busy is not unavailable: it says it starts the game, and the navigator still counts it.
    expect(play.hasAttribute('aria-disabled')).toBe(false);
    expect(play.textContent?.trim()).toBe(msg('bp.starting'));
    expect(play.querySelector('.spinner')).not.toBeNull();
    expect(document.activeElement).toBe(play);

    launch.resolve(false);
    await vi.waitFor(() => expect(play.getAttribute('aria-busy')).toBeNull());
    expect(play.textContent?.trim()).toBe(msg('bp.game.launch'));
    expect(document.activeElement).toBe(play);
  });

  it('is busy while a launch another screen started is in flight, and a press sends nothing', async () => {
    const onlaunch = vi.fn(async () => true);
    renderPage(onlaunch);
    const keys = user();
    const play = await screen.findByRole('button', { name: msg('bp.game.launch') });

    inFlight.set(new Set(['g1']));
    await vi.waitFor(() => expect(play.getAttribute('aria-busy')).toBe('true'));
    await keys.click(play);

    expect(onlaunch).not.toHaveBeenCalled();
  });

  it('keeps a busy Launch as the D-pad anchor: right reaches its neighbour, left comes back to Launch', async () => {
    layOutHeroActions();
    const launch = deferred<boolean>();
    renderPage(vi.fn().mockReturnValueOnce(launch.promise));
    const keys = user();
    const play = await screen.findByRole('button', { name: msg('bp.game.launch') });
    const favorite = screen.getByRole('button', { name: msg('bp.game.favoriteAdd') });
    play.focus();
    await keys.keyboard('{Enter}');
    expect(play.getAttribute('aria-busy')).toBe('true');

    expect(moveFocus(app, 'right')).toBe(true);
    expect(document.activeElement).toBe(favorite);
    expect(moveFocus(app, 'left')).toBe(true);
    expect(document.activeElement).toBe(play);
    launch.resolve(false);
  });

  it('offers no Launch for an installed game without an executable, and says where to choose one', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'c1', uninstalled: false, installDir: '/games/hades', executable: '' } as LibraryGame]);
    const onlaunch = vi.fn(async () => true);
    renderPage(onlaunch);

    expect(await screen.findByText(msg('bp.game.noExecutable'))).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('bp.game.launch') })).toBeNull();
    expect(onlaunch).not.toHaveBeenCalled();
  });

  it('shows a conflict that held the launch back again in its own words', async () => {
    renderPage(vi.fn(async () => {
      throw new Error('typhon:cloud.conflict_unresolved: diverged');
    }));
    const keys = user();

    await keys.click(await screen.findByRole('button', { name: msg('bp.game.launch') }));

    expect((await screen.findByRole('alert')).textContent?.trim()).toBe(msg('cloud.conflict_unresolved'));
  });
});
