// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { playGame, stopGame } from '../../lib/services/library';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { gameArt } from '../../lib/stores/metadata';
import { libraryView } from '../../lib/stores/ui';
import { playtime, relativeDate } from '../../lib/utils/format';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';
import { carousel, played, recentTitles, slideTitles } from './libraryTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
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
  it('shows the three last played games, each with the picture kind its art gives', () => {
    renderDom(Library);

    expect(slideTitles()).toEqual(['Wide One', 'Poster One', 'Bare One']);
    const kinds = [...(carousel()?.querySelectorAll('[data-picture]') ?? [])].map((slide) => slide.getAttribute('data-picture'));
    expect(kinds).toEqual(['wide', 'poster', 'generated']);
    expect(carousel()?.querySelector('img.wide')?.getAttribute('src')).toBe('wide-hero.png');
    expect(carousel()?.querySelector('img.poster')?.getAttribute('src')).toBe('poster-cover.png');
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

  it('goes away for the session and comes back, its games in the "Recent" row meanwhile', async () => {
    const { user } = renderDom(Library);
    const hide = screen.getByRole('button', { name: msg('library.carousel.hide') });
    hide.focus();

    await user.keyboard('{Enter}');

    expect(carousel()).toBeNull();
    expect(recentTitles()).toEqual(['Wide One', 'Poster One', 'Bare One', 'Fourth One', 'Fifth One']);
    const show = screen.getByRole('button', { name: msg('library.carousel.show') });
    expect(document.activeElement).toBe(show);

    await user.keyboard('{Enter}');

    expect(slideTitles()).toEqual(['Wide One', 'Poster One', 'Bare One']);
    expect(recentTitles()).toEqual(['Fourth One', 'Fifth One']);
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('library.carousel.hide') }));
  });

  it('stays put away after leaving the page and coming back', async () => {
    const first = renderDom(Library);
    await first.user.click(screen.getByRole('button', { name: msg('library.carousel.hide') }));
    first.unmount();

    renderDom(Library);

    expect(carousel()).toBeNull();
    expect(screen.getByRole('button', { name: msg('library.carousel.show') })).toBeTruthy();
  });
});
