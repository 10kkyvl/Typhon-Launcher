// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import GameQuickMenu from '../../lib/components/GameQuickMenu.svelte';
import { closeGameMenu, gameMenu } from '../../lib/stores/gameMenu';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { playGame, stopGame } from '../../lib/services/library';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { gameArt } from '../../lib/stores/metadata';
import { libraryView } from '../../lib/stores/ui';
import { playtime, relativeDate } from '../../lib/utils/format';
import { renderDom } from '../../lib/testing/dom';
import { libraryGenres } from '../../lib/stores/genres';
import { genreLabel } from '../../lib/metadata/labels';
import Library from './Library.svelte';
import { backdropSources, rememberedGame } from '../../lib/backdrop/source';
import { ambientState, HOVER_LEAVE_MS, HOVER_REST_MS, pickAmbient } from '../../lib/stores/ambient';
import { stageGame, stagePictureFailed } from '../../lib/stores/stage';
import { carousel, played, recentTitles, results, shownTitles, slideTitles } from './libraryTesting';

/** The carousel's slide blocks, the ones off screen included. */
const blocks = () => [...(carousel()?.querySelectorAll<HTMLElement>('[data-picture]') ?? [])];
const blockOf = (title: string) => blocks().find((block) => block.querySelector('.title')?.textContent?.trim() === title) as HTMLElement;
// A release of the stage is published after the task that made it.
const settleTask = async () => {
  await Promise.resolve();
  await tick();
};

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/stores/genres', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/genres')>()),
  requestLibraryGenres: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
  allTags: vi.fn(async () => []),
  steamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
  playGame: vi.fn(async () => {}),
  stopGame: vi.fn(async () => {}),
}));

const wide = played('g-wide', 'Wide One', 9, { canonicalGameId: 'c-wide' });
const poster = played('g-poster', 'Poster One', 8, { canonicalGameId: 'c-poster' });
const bare = played('g-bare', 'Bare One', 7);
const fourth = played('g-fourth', 'Fourth One', 6);
const fifth = played('g-fifth', 'Fifth One', 5);

beforeEach(() => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  libraryView.set('grid');
  libraryLoaded.set(true);
  libraryGames.set([fifth, bare, wide, fourth, poster]);
  gameArt.set({
    'c-wide': { cover: 'wide-cover.png', hero: 'wide-hero.png', heroIsPoster: false },
    'c-poster': { cover: 'poster-cover.png', hero: 'poster-cover.png', heroIsPoster: true },
  });
  vi.mocked(playGame).mockClear();
  vi.mocked(stopGame).mockClear();
});

afterEach(() => {
  libraryGames.set([]);
  runningGames.set(new Set());
  gameArt.set({});
  document.body.replaceChildren();
});

describe('Library: the carousel on the page', () => {
  it('shows the three last played games: one with wide art on the stage, the others with their cover in the block', () => {
    renderDom(Library);

    expect(slideTitles()).toEqual(['Wide One', 'Poster One', 'Bare One']);
    expect(blocks().map((block) => block.dataset.picture)).toEqual(['stage', 'cover', 'cover']);
    expect(blockOf('Wide One').querySelector('img, svg.generated-art')).toBeNull();
    expect(blockOf('Poster One').querySelector('img')?.getAttribute('src')).toBe('poster-cover.png');
    expect(blockOf('Bare One').querySelector('svg.generated-art')).not.toBeNull();
  });

  it('puts the game of the slide on screen on the stage, and nothing once the page is left', async () => {
    const { user, unmount } = renderDom(Library);
    expect(get(stageGame)).toBe('g-wide');

    await user.click(within(carousel() as HTMLElement).getByRole('button', { name: msg('library.carousel.pick', { index: '2', total: '3', title: 'Poster One' }) }));
    await settleTask();
    expect(get(stageGame)).toBe('g-poster');

    unmount();
    await settleTask();
    expect(get(stageGame)).toBeNull();
  });

  it('shows the cover in the block once the stage cannot load the wide art', async () => {
    gameArt.set({ 'c-wide': { cover: 'wide-cover.png', hero: 'broken-hero.png', heroIsPoster: false } });
    renderDom(Library);
    expect(blockOf('Wide One').dataset.picture).toBe('stage');

    stagePictureFailed('broken-hero.png');
    await tick();

    expect(blockOf('Wide One').dataset.picture).toBe('cover');
    expect(blockOf('Wide One').querySelector('img')?.getAttribute('src')).toBe('wide-cover.png');
  });

  // The art store has no entry for a game it has not answered for, which for a game with no art at all
  // lasts: the block does not wait on it with no picture at all.
  it('shows the cover of a game the art store has not answered for, and stands it on the stage once its wide art comes', async () => {
    gameArt.set({ 'c-poster': { cover: 'poster-cover.png', hero: 'poster-cover.png', heroIsPoster: true } });
    libraryGames.set([fifth, bare, { ...wide, cover: 'own-wide.png' }, fourth, poster]);
    renderDom(Library);
    expect(blockOf('Wide One').dataset.picture).toBe('cover');
    expect(blockOf('Wide One').querySelector('img')?.getAttribute('src')).toBe('own-wide.png');

    gameArt.set({ 'c-wide': { cover: 'wide-cover.png', hero: 'wide-hero.png', heroIsPoster: false } });
    await tick();

    expect(blockOf('Wide One').dataset.picture).toBe('stage');
  });

  it('says under the title when the game was last played and for how long', () => {
    renderDom(Library);

    const line = carousel()?.querySelector('[data-picture] .line')?.textContent ?? '';
    expect(line).toContain(msg('games.libraryLastSessionLabel', { date: relativeDate(wide.lastPlayed) }));
    expect(line).toContain(msg('games.libraryPlaytimeLabel', { time: playtime(3600) }));
  });

  it('plays the game on screen, and stops it once it runs', async () => {
    const { user } = renderDom(Library);
    const shown = carousel() as HTMLElement;

    await user.click(within(shown).getByRole('button', { name: msg('ui.play') }));
    expect(playGame).toHaveBeenCalledWith('g-wide');

    runningGames.set(new Set(['g-wide']));
    await tick();
    await user.click(within(shown).getByRole('button', { name: msg('ui.stop') }));
    expect(stopGame).toHaveBeenCalledWith('g-wide');
  });

  it('keeps its games out of the "Recent" row', () => {
    renderDom(Library);

    expect(recentTitles()).toEqual(['Fourth One', 'Fifth One']);
  });

  it('leaves out a hidden game, and shows the next one instead', () => {
    libraryGames.set([fifth, bare, { ...wide, hidden: true }, fourth, poster]);
    renderDom(Library);

    expect(slideTitles()).toEqual(['Poster One', 'Bare One', 'Fourth One']);
    expect(recentTitles()).toEqual(['Fifth One']);
  });

  // Decision В11: the carousel is the page's stage and stays while there are slides.
  it('cannot be put away: everything above the page\'s heading is the carousel itself', () => {
    renderDom(Library);

    const region = carousel() as HTMLElement;
    const bleed = region.closest('.bleed') as HTMLElement;
    expect(bleed).toBeTruthy();
    const outside = [...bleed.querySelectorAll('button')].filter((button) => !region.contains(button));
    expect(outside.map((button) => button.getAttribute('aria-label') ?? button.textContent?.trim())).toEqual([]);
    expect(screen.getByRole('heading', { level: 1 }).closest('header')?.querySelectorAll('button')).toHaveLength(0);
  });

  it('stays with a choice to put it away stored by an earlier build, its games kept out of the "Recent" row', () => {
    libraryChoices.set(Object.assign({}, DEFAULT_CHOICES, { heroHidden: true }));
    renderDom(Library);

    expect(slideTitles()).toEqual(['Wide One', 'Poster One', 'Bare One']);
    expect(recentTitles()).toEqual(['Fourth One', 'Fifth One']);
  });
});

describe('Library: the carousel and the window backdrop', () => {
  // What ShellBackdrop draws, from the real stores.
  const picked = () =>
    pickAmbient(get(ambientState), rememberedGame(get(libraryGames), get(gameArt)), (id) => backdropSources(id, get(libraryGames), get(gameArt)).length > 0);
  const card = (title: string) =>
    [...(results()?.querySelectorAll<HTMLElement>('.card') ?? [])].find((el) => el.textContent?.includes(title)) as HTMLElement;
  // A release is published after the task that made it.
  const settle = () => vi.advanceTimersByTimeAsync(0);

  it('tints the window with the slide on screen; a rested card overrides it and gives the window back after the leave', async () => {
    vi.useFakeTimers();
    const { user } = renderDom(Library);
    // The second slide: the first one is also the game the window falls back to.
    await user.click(within(carousel() as HTMLElement).getByRole('button', { name: msg('library.carousel.pick', { index: '2', total: '3', title: 'Poster One' }) }));
    await settle();
    expect(picked()).toBe('g-poster');

    await fireEvent.pointerEnter(card('Fourth One'), { pointerType: 'mouse' });
    await vi.advanceTimersByTimeAsync(HOVER_REST_MS);
    expect(picked()).toBe('g-fourth');
    // The card moves the blurred backdrop only: the stage keeps the slide on screen.
    expect(get(stageGame)).toBe('g-poster');

    await fireEvent.pointerLeave(card('Fourth One'));
    await vi.advanceTimersByTimeAsync(HOVER_LEAVE_MS);
    expect(picked()).toBe('g-poster');
  });

  it('lets the window go once the page is left', async () => {
    vi.useFakeTimers();
    const first = renderDom(Library);
    expect(get(ambientState).resting).toBe('g-wide');
    first.unmount();
    await settle();
    expect(get(ambientState).resting).toBeNull();
  });
});

describe('Library: a hidden game', () => {
  const secret = played('g-secret', 'Secret One', 10, { canonicalGameId: 'c-secret', hidden: true, source: 'discovered' });

  afterEach(() => libraryGenres.set({}));

  it('is in "Hidden" and nowhere else: not in the list, the carousel, the row, the source menu or the genres', async () => {
    libraryView.set('list');
    libraryGames.set([secret, fifth, bare, wide, fourth, poster]);
    libraryGenres.set({ 'c-secret': ['puzzle'], 'c-wide': ['shooter'] });
    const { user } = renderDom(Library);

    expect(shownTitles()).not.toContain('Secret One');
    expect(slideTitles()).not.toContain('Secret One');
    expect(recentTitles()).not.toContain('Secret One');
    expect(screen.queryByRole('button', { name: msg('games.filterSourceAll') })).toBeNull();

    await user.click(screen.getByRole('button', { name: msg('games.collectionAdd') }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByRole('button', { name: genreLabel('puzzle') })).toBeNull();
    expect(within(dialog).getByRole('button', { name: genreLabel('shooter') })).toBeTruthy();
    await user.keyboard('{Escape}');

    await user.click(screen.getByRole('radio', { name: msg('games.filterHidden') }));
    expect(shownTitles()).toEqual(['Secret One']);
  });
});

describe('Library: the carousel\'s game menu', () => {
  afterEach(() => closeGameMenu());

  it('names the game of the slide on screen by its id, also when two games share a title', async () => {
    const one = played('g-one', 'Twin', 9);
    const two = played('g-two', 'Twin', 8);
    const other = played('g-other', 'Other', 7);
    libraryGames.set([one, two, other]);
    const { user } = renderDom(Library);
    render(GameQuickMenu);
    const shown = carousel() as HTMLElement;
    await user.click(within(shown).getByRole('button', { name: msg('library.carousel.pick', { index: '2', total: '3', title: 'Twin' }) }));

    // Another game starts: the page's order changes while focus holds the carousel's order.
    runningGames.set(new Set(['g-other']));
    await tick();
    await user.keyboard('{Shift>}{F10}{/Shift}');

    expect(await screen.findByRole('menu')).toBeTruthy();
    expect(get(gameMenu)?.gameId).toBe('g-two');
  });
});
