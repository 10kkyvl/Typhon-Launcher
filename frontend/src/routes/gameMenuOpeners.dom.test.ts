import { tick } from 'svelte';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import GameQuickMenu from '../lib/components/GameQuickMenu.svelte';
import { msg } from '../lib/i18n';
import type { LibraryGame } from '../lib/services/library';
import type { CatalogGame } from '../lib/services/sources';
import { closeGameMenu, GAME_MENU_ID } from '../lib/stores/gameMenu';
import { libraryGames } from '../lib/stores/library';
import { installedView, libraryView } from '../lib/stores/ui';
import { user } from '../lib/testing/dom';
import Catalog from './catalog/Catalog.svelte';
import Installed from './installed/Installed.svelte';
import Library from './library/Library.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../lib/testing/dom')).wailsRuntime());
vi.mock('../lib/services/backend', () => ({ inWails: false }));
vi.mock('../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/services/library')>()),
  steamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
}));
vi.mock('../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/services/sources')>()),
  queryCatalogGames: vi.fn(async (query: { page?: number }) => ({
    items: [
      { id: 'c-hades', title: 'Hades' },
      { id: 'c-celeste', title: 'Celeste' },
    ] as CatalogGame[],
    total: 2,
    page: query.page ?? 1,
    pageSize: 60,
  })),
}));

// Real pages with the game menu mounted next to them, as AppShell has it: every opener must reach the
// menu of its own game. Hades was played (the Library hero), both are installed and in the catalog.
const games = [
  { id: 'g-hades', title: 'Hades', canonicalGameId: 'c-hades', lastPlayed: '2026-11-01T10:00:00Z' },
  { id: 'g-celeste', title: 'Celeste', canonicalGameId: 'c-celeste', lastPlayed: null },
].map((game, i) => ({ ...game, installDir: `C:\\Games\\${i}`, executable: 'game.exe', sizeBytes: 0, uninstalled: false, tags: [] }) as unknown as LibraryGame);

const menuFor = (title: string) => msg('games.installedMenuFor', { title });
const RIGHT_CLICK = { button: 2, clientX: 120, clientY: 80 };

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set(games);
});

afterEach(() => {
  closeGameMenu();
  libraryGames.set([]);
  installedView.set('list');
  libraryView.set('grid');
  document.body.replaceChildren();
});

function mount(page: typeof Installed | typeof Library | typeof Catalog) {
  render(page, { target: app });
  render(GameQuickMenu, { target: app });
  return user();
}

// The game menu, and only it, is open, named for `title`.
function gameMenuOf(title: string): HTMLElement {
  const menus = screen.getAllByRole('menu');
  expect(menus.map((menu) => menu.id)).toEqual([GAME_MENU_ID]);
  expect(screen.getByRole('menu', { name: menuFor(title) })).toBe(menus[0]);
  return menus[0];
}

async function opensByPointerAndKeyboard(keys: ReturnType<typeof user>, control: HTMLElement, title: string) {
  await fireEvent.contextMenu(control, RIGHT_CLICK);
  const byPointer = gameMenuOf(title);
  expect(document.activeElement).toBe(byPointer);
  await keys.keyboard('{Escape}');
  expect(screen.queryByRole('menu')).toBeNull();

  control.focus();
  await keys.keyboard('{Shift>}{F10}{/Shift}');
  gameMenuOf(title);
  expect(document.activeElement).toBe(screen.getAllByRole('menuitem')[0]);
  await keys.keyboard('{Escape}');
  expect(document.activeElement).toBe(control);
}

describe('the game menu from the Installed page', () => {
  it('a row: right-click and Shift+F10 open the menu of that row’s game', async () => {
    const keys = mount(Installed);
    const row = screen.getByRole('button', { name: menuFor('Celeste') }).closest('[role="presentation"]') as HTMLElement;

    await opensByPointerAndKeyboard(keys, within(row).getByRole('button', { name: msg('games.openFolder') }), 'Celeste');
  });

  it('a card: right-click and Shift+F10 open the menu of that card’s game', async () => {
    installedView.set('grid');
    const keys = mount(Installed);

    await opensByPointerAndKeyboard(keys, screen.getByRole('button', { name: 'Celeste' }), 'Celeste');
  });

  it('a right-click on a row’s own "…" while its menu is open leaves one menu open: the game menu, holding focus', async () => {
    const keys = mount(Installed);
    const more = screen.getByRole('button', { name: menuFor('Hades') });
    await keys.click(more);
    expect(screen.getAllByRole('menu')).toHaveLength(1);
    expect(more.getAttribute('aria-expanded')).toBe('true');

    await keys.pointer({ keys: '[MouseRight]', target: more, coords: { clientX: 300, clientY: 200 } });
    await tick();

    const opened = gameMenuOf('Hades');
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(opened);
    await keys.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(within(opened).getAllByRole('menuitem')[0]);
  });
});

describe('the game menu from the Library page', () => {
  it('a card: right-click and Shift+F10 open the menu of that card’s game', async () => {
    const keys = mount(Library);
    const cover = screen.getAllByRole('button', { name: 'Celeste' })[0];

    await opensByPointerAndKeyboard(keys, cover, 'Celeste');
  });

  it('the carousel: right-click and Shift+F10 open the menu of the game it shows', async () => {
    const keys = mount(Library);
    const carousel = screen.getByRole('region', { name: msg('library.carousel.label') });

    await opensByPointerAndKeyboard(keys, within(carousel).getByRole('button', { name: 'Hades' }), 'Hades');
  });

  it('a list row: right-click and Shift+F10 open the menu of that row’s game', async () => {
    libraryView.set('list');
    const keys = mount(Library);
    const list = screen.getByRole('list', { name: msg('games.allGamesTitle') });
    const title = within(list).getByRole('button', { name: 'Celeste' });

    await opensByPointerAndKeyboard(keys, title, 'Celeste');
  });
});

describe('the game menu from the Catalog page', () => {
  it('a card’s "…" opens the menu of the library game it stands for and says so while it is open', async () => {
    const keys = mount(Catalog);
    const more = await screen.findByRole('button', { name: menuFor('Celeste') });
    expect(more.getAttribute('aria-haspopup')).toBe('menu');
    expect(more.getAttribute('aria-expanded')).toBe('false');

    await keys.click(more);
    const opened = gameMenuOf('Celeste');
    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(more.getAttribute('aria-controls')).toBe(opened.id);

    await keys.keyboard('{Escape}');
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(more);

    await keys.keyboard('{Enter}');
    gameMenuOf('Celeste');
    expect(document.activeElement).toBe(screen.getAllByRole('menuitem')[0]);
    expect(screen.getByRole('button', { name: menuFor('Hades') }).getAttribute('aria-expanded')).toBe('false');
  });
});
