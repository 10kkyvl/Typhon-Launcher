// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import GameQuickMenu from '../../lib/components/GameQuickMenu.svelte';
import { msg } from '../../lib/i18n';
import { playGame, stopGame } from '../../lib/services/library';
import type { Entry as WatchEntry } from '../../lib/services/watchlist';
import { closeGameMenu, gameMenu } from '../../lib/stores/gameMenu';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { toast } from '../../lib/stores/toasts';
import { libraryView } from '../../lib/stores/ui';
import { watchlist } from '../../lib/stores/watchlist';
import { bytesSize } from '../../lib/utils/format';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';
import { game, results, shownTitles } from './libraryTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
  playGame: vi.fn(async () => {}),
  stopGame: vi.fn(async () => {}),
  steamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
}));
vi.mock('../../lib/stores/toasts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(() => 0),
}));

const SIZE = 14_200_000_000;
const hades = game('g-hades', 'Hades', { sizeBytes: SIZE });
const celeste = game('g-celeste', 'Celeste');
const inside = game('g-inside', 'Inside');

function row(title: string): HTMLElement {
  const index = shownTitles().indexOf(title);
  expect(index, title).toBeGreaterThanOrEqual(0);
  return (results() as HTMLElement).children[index] as HTMLElement;
}

beforeEach(() => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  libraryView.set('list');
  libraryLoaded.set(true);
  libraryGames.set([hades, celeste, inside]);
  vi.mocked(playGame).mockReset();
  vi.mocked(stopGame).mockReset();
  vi.mocked(toast).mockClear();
});

afterEach(() => {
  closeGameMenu();
  libraryGames.set([]);
  runningGames.set(new Set());
  watchlist.set([]);
  document.body.replaceChildren();
});

describe('Library: the list view', () => {
  it('is a list named by the section heading, one row per game', () => {
    renderDom(Library);

    const list = screen.getByRole('list', { name: msg('games.allGamesTitle') });
    expect(list.tagName).toBe('UL');
    expect(within(list).getAllByRole('listitem')).toHaveLength(3);
    expect(shownTitles()).toEqual(['Celeste', 'Hades', 'Inside']);
  });

  it('plays a game from its row, and shows a running game as running with Stop', async () => {
    const { user } = renderDom(Library);

    await user.click(within(row('Celeste')).getByRole('button', { name: msg('ui.play') }));
    expect(playGame).toHaveBeenCalledWith('g-celeste');

    runningGames.set(new Set(['g-hades']));
    await tick();
    expect(within(row('Hades')).getByText(msg('games.runningLabel'))).toBeTruthy();
    await user.click(within(row('Hades')).getByRole('button', { name: msg('ui.stop') }));
    expect(stopGame).toHaveBeenCalledWith('g-hades');
  });

  it('keeps Play busy and focused during the launch: a second press sends nothing', async () => {
    let release = () => {};
    vi.mocked(playGame).mockReturnValueOnce(new Promise<void>((resolve) => (release = resolve)));
    const { user } = renderDom(Library);
    const play = within(row('Celeste')).getByRole('button', { name: msg('ui.play') });
    play.focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(playGame).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalled();
    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(play);
    release();
  });

  it('shows the size once, outside the name of its controls', () => {
    renderDom(Library);

    const size = bytesSize(SIZE);
    expect(row('Hades').textContent?.split(size).length).toBe(2);
    for (const control of within(row('Hades')).getAllByRole('button')) expect(control.getAttribute('aria-label') ?? control.textContent).not.toContain(size);
  });

  it('opens the game menu from "More" next to it, and gives focus back there on Escape', async () => {
    const { user } = renderDom(Library);
    render(GameQuickMenu);
    const more = within(row('Hades')).getByRole('button', { name: msg('library.row.more') });
    more.focus();

    await user.keyboard('{Enter}');

    const menu = await screen.findByRole('menu', { name: msg('games.installedMenuFor', { title: 'Hades' }) });
    expect(menu.contains(document.activeElement)).toBe(true);
    expect(get(gameMenu)?.source).toBe(row('Hades'));

    await user.keyboard('{Escape}');

    expect(document.activeElement).toBe(more);
  });

  it('opens the game menu on a right click of a row', async () => {
    renderDom(Library);
    render(GameQuickMenu);

    const shown = fireEvent.contextMenu(row('Inside'), { button: 2, clientX: 40, clientY: 40 });

    expect(await shown).toBe(false);
    expect(await screen.findByRole('menu', { name: msg('games.installedMenuFor', { title: 'Inside' }) })).toBeTruthy();
  });

  it('leaves the browser menu out on a row that has no game menu (a watched game)', async () => {
    watchlist.set([{ gameId: 'c-watch', title: 'Watched Game' } as WatchEntry]);
    libraryChoices.set({ ...DEFAULT_CHOICES, filter: 'watched' });
    renderDom(Library);

    expect(await fireEvent.contextMenu(row('Watched Game'), { button: 2, clientX: 40, clientY: 40 })).toBe(false);
    expect(screen.queryByRole('menu')).toBeNull();
  });
});

describe('Library: the grid', () => {
  it('leaves the browser menu out on a card that has no game menu (a watched game)', async () => {
    libraryView.set('grid');
    watchlist.set([{ gameId: 'c-watch', title: 'Watched Game' } as WatchEntry]);
    libraryChoices.set({ ...DEFAULT_CHOICES, filter: 'watched' });
    renderDom(Library);

    const card = (results() as HTMLElement).querySelector('.card') as HTMLElement;
    expect(await fireEvent.contextMenu(card, { button: 2, clientX: 40, clientY: 40 })).toBe(false);
  });

  it('is a list named by the section heading', () => {
    libraryView.set('grid');
    renderDom(Library);

    const list = screen.getByRole('list', { name: msg('games.allGamesTitle') });
    expect(within(list).getAllByRole('listitem')).toHaveLength(3);
  });
});
