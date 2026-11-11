import { tick } from 'svelte';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { playGame, type LibraryGame } from '../services/library';
import { closeGameMenu } from '../stores/gameMenu';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import GameMenuHarness from '../testing/GameMenuHarness.svelte';
import Modal from './Modal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  steamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
  playGame: vi.fn(async () => {}),
}));

const hades = { id: 'g1', title: 'Hades', uninstalled: false, executable: 'Hades.exe' } as LibraryGame;
const celeste = { id: 'g2', title: 'Celeste', uninstalled: false, executable: 'Celeste.exe' } as LibraryGame;

const menuName = (title: string) => msg('games.installedMenuFor', { title });
const menu = () => screen.queryByRole('menu');
const items = () => screen.getAllByRole('menuitem');
const link = (title: string) => screen.getByRole('link', { name: title });

function box(left: number, top: number, width: number, height: number): () => DOMRect {
  return () => DOMRect.fromRect({ x: left, y: top, width, height });
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([hades, celeste]);
});

afterEach(() => {
  closeGameMenu();
  libraryGames.set([]);
  toasts.set([]);
  vi.mocked(playGame).mockClear();
  document.documentElement.removeAttribute('style');
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

function setup(props: { cards?: boolean } = {}) {
  render(GameMenuHarness, { target: app, props });
  return user();
}

async function rightClick(keys: ReturnType<typeof user>, target: Element, clientX: number, clientY: number) {
  await keys.pointer({ keys: '[MouseRight]', target, coords: { clientX, clientY } });
}

describe('game context menu by pointer', () => {
  it('a right-click opens the menu at the pointer with focus on it, out of the page, named for the game', async () => {
    const keys = setup();

    await rightClick(keys, link('Hades'), 120, 80);

    const opened = screen.getByRole('menu');
    expect(document.activeElement).toBe(opened);
    expect([opened.style.left, opened.style.top]).toEqual(['120px', '80px']);
    expect(app.contains(opened)).toBe(false);
    expect(screen.getByRole('menu', { name: menuName('Hades') })).toBe(opened);
    expect(items().every((item) => item.tabIndex === -1)).toBe(true);
  });

  it('opens below the title bar and towards the inside near the window edges', async () => {
    const root = document.documentElement;
    root.style.fontSize = '10px';
    root.style.setProperty('--titlebar-h', '4rem');
    root.style.setProperty('--popup-edge', '0.8rem');
    const rect = HTMLElement.prototype.getBoundingClientRect;
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      return this.getAttribute('role') === 'menu' ? DOMRect.fromRect({ width: 200, height: 300 }) : rect.call(this);
    });
    const keys = setup();

    await rightClick(keys, link('Hades'), window.innerWidth - 10, 5);

    const opened = screen.getByRole('menu');
    expect([opened.style.left, opened.style.top]).toEqual([`${window.innerWidth - 210}px`, '48px']);
  });

  it('a right-click on another game moves the menu to that game and that point', async () => {
    const keys = setup();
    await rightClick(keys, link('Hades'), 100, 100);

    await rightClick(keys, link('Celeste'), 300, 200);

    expect(screen.getAllByRole('menu')).toHaveLength(1);
    const opened = screen.getByRole('menu', { name: menuName('Celeste') });
    expect([opened.style.left, opened.style.top]).toEqual(['300px', '200px']);
    expect(document.activeElement).toBe(opened);
  });

  it('a right-click on a game swallows the browser menu, also inside the open menu', async () => {
    setup();

    expect(await fireEvent.contextMenu(link('Hades'), { clientX: 40, clientY: 40, button: 2 })).toBe(false);
    expect(await fireEvent.contextMenu(items()[0], { clientX: 50, clientY: 50, button: 2 })).toBe(false);
  });

  it('Escape closes it and focus goes to the game it was opened on, also when the right-click moved no focus', async () => {
    const keys = setup();
    // WebKit moves no focus on a right-click; fireEvent dispatches the bare event.
    await fireEvent.contextMenu(link('Hades'), { clientX: 120, clientY: 80, button: 2 });
    expect(document.activeElement).toBe(menu());

    await keys.keyboard('{Escape}');

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(link('Hades'));
  });

  it('a press outside closes it, a press on an icon (an <svg>) too', async () => {
    const keys = setup();
    await rightClick(keys, link('Hades'), 120, 80);

    await keys.click(screen.getByRole('button', { name: 'Outside' }));
    expect(menu()).toBeNull();

    await rightClick(keys, link('Hades'), 120, 80);
    const icon = screen.getByRole('button', { name: 'Help' }).querySelector('path') as Element;
    await keys.pointer({ keys: '[MouseLeft]', target: icon });
    expect(menu()).toBeNull();
  });

  it.each([
    ['the page scrolls', () => document.dispatchEvent(new Event('scroll'))],
    ['the window resizes', () => window.dispatchEvent(new Event('resize'))],
    ['the window loses focus', () => window.dispatchEvent(new Event('blur'))],
  ])('closes when %s', async (_case, happen) => {
    const keys = setup();
    await rightClick(keys, link('Hades'), 120, 80);
    expect(menu()).not.toBeNull();

    happen();
    await tick();

    expect(menu()).toBeNull();
  });

  it('stays open when its own list scrolls', async () => {
    const keys = setup();
    await rightClick(keys, link('Hades'), 120, 80);

    (menu() as HTMLElement).dispatchEvent(new Event('scroll'));
    await tick();

    expect(menu()).not.toBeNull();
  });

  it('a pick runs the item and closes the menu', async () => {
    const keys = setup();
    await rightClick(keys, link('Hades'), 120, 80);

    await keys.click(screen.getByRole('menuitem', { name: msg('games.play') }));

    expect(vi.mocked(playGame)).toHaveBeenCalledExactlyOnceWith('g1');
    expect(menu()).toBeNull();
  });
});

describe('game context menu from the keyboard', () => {
  it.each([
    ['Shift+F10', '{Shift>}{F10}{/Shift}'],
    ['the ContextMenu key', '{ContextMenu}'],
  ])('%s on a game opens its menu next to it with focus on the first item', async (_key, press) => {
    const keys = setup();
    link('Hades').getBoundingClientRect = box(300, 200, 80, 20);
    link('Hades').focus();

    await keys.keyboard(press);

    const opened = screen.getByRole('menu', { name: menuName('Hades') });
    expect(document.activeElement).toBe(items()[0]);
    expect(items()[0].textContent?.trim()).toBe(msg('games.play'));
    expect([opened.style.left, opened.style.top]).toEqual(['300px', '224px']);
  });

  it('a contextmenu event without a pointer position opens next to the focused game, not in the corner', async () => {
    setup();
    link('Celeste').getBoundingClientRect = box(300, 200, 80, 20);
    link('Celeste').focus();

    fireEvent.contextMenu(link('Celeste'), { clientX: 0, clientY: 0, button: 0 });
    await tick();

    const opened = screen.getByRole('menu', { name: menuName('Celeste') });
    expect([opened.style.left, opened.style.top]).toEqual(['300px', '224px']);
    expect(document.activeElement).toBe(items()[0]);
  });

  it('Shift+F10 held opens the menu once and runs nothing', async () => {
    const keys = setup();
    link('Hades').focus();

    await keys.keyboard('{Shift>}{F10>6}{/F10}{/Shift}');

    expect(screen.getAllByRole('menu')).toHaveLength(1);
    expect(document.activeElement).toBe(items()[0]);
    expect(vi.mocked(playGame)).not.toHaveBeenCalled();
  });

  it('arrows move with wrap, Home and End jump, a typed letter finds the next item starting with it', async () => {
    const keys = setup();
    link('Hades').focus();
    await keys.keyboard('{Shift>}{F10}{/Shift}');
    const all = items();

    await keys.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(all[1]);
    await keys.keyboard('{ArrowUp}{ArrowUp}');
    expect(document.activeElement).toBe(all.at(-1));
    await keys.keyboard('{Home}');
    expect(document.activeElement).toBe(all[0]);
    await keys.keyboard('{End}');
    expect(document.activeElement).toBe(all.at(-1));

    // From the first item, a letter goes to the next item (wrapping) whose label starts with it.
    const letter = msg('games.actionProperties')[0].toLowerCase();
    const expected = [...all.slice(1), all[0]].find((item) => item.textContent?.trim().toLowerCase().startsWith(letter));
    await keys.keyboard('{Home}');
    await keys.keyboard(letter);
    expect(expected).toBeDefined();
    expect(document.activeElement).toBe(expected);
  });

  it('Escape closes it and focus returns to the control it was opened from', async () => {
    const keys = setup();
    link('Celeste').focus();
    await keys.keyboard('{Shift>}{F10}{/Shift}{ArrowDown}');
    expect(document.activeElement).toBe(items()[1]);

    await keys.keyboard('{Escape}');

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(link('Celeste'));
  });

  it('Enter runs the focused item, closes the menu and leaves focus on the game', async () => {
    const keys = setup();
    link('Hades').focus();
    await keys.keyboard('{Shift>}{F10}{/Shift}');

    await keys.keyboard('{Enter}');

    expect(vi.mocked(playGame)).toHaveBeenCalledExactlyOnceWith('g1');
    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(link('Hades'));
  });

  it('Tab closes it and focus moves on from the game', async () => {
    const keys = setup();
    link('Hades').focus();
    await keys.keyboard('{Shift>}{F10}{/Shift}');
    expect(document.activeElement).toBe(items()[0]);

    await keys.tab();

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(link('Celeste'));
  });

  it('a keyboard open on a game scrolled out of sight brings it into view first, so the menu stays', async () => {
    const keys = setup();
    let top = -60;
    link('Hades').getBoundingClientRect = () => DOMRect.fromRect({ x: 300, y: top, width: 80, height: 20 });
    const reveal = vi.fn(() => {
      top = 300;
    });
    link('Hades').scrollIntoView = reveal;
    link('Hades').focus();

    await keys.keyboard('{Shift>}{F10}{/Shift}');
    await tick();

    expect(reveal).toHaveBeenCalledExactlyOnceWith({ block: 'nearest' });
    expect(menu()).not.toBeNull();
    expect((menu() as HTMLElement).style.top).toBe('324px');
  });
});

describe('the "…" menu button of a game card', () => {
  const button = () => screen.getByRole('button', { name: menuName('Hades') });

  it('says it opens a menu and whether it is open; a click toggles the menu, anchored to the button', async () => {
    const keys = setup({ cards: true });
    expect(button().getAttribute('aria-haspopup')).toBe('menu');
    expect(button().getAttribute('aria-expanded')).toBe('false');
    button().getBoundingClientRect = box(400, 100, 30, 30);

    await keys.click(button());

    const opened = screen.getByRole('menu', { name: menuName('Hades') });
    expect(button().getAttribute('aria-expanded')).toBe('true');
    expect(button().getAttribute('aria-controls')).toBe(opened.id);
    expect([opened.style.left, opened.style.top]).toEqual(['400px', '134px']);

    await keys.click(button());
    expect(menu()).toBeNull();
    expect(button().getAttribute('aria-expanded')).toBe('false');
  });

  it.each([
    ['Enter', '{Enter}'],
    ['Space', ' '],
    ['ArrowDown', '{ArrowDown}'],
  ])('%s opens it with focus on the first item; Escape closes it and focus returns to the button', async (_key, press) => {
    const keys = setup({ cards: true });
    button().focus();

    await keys.keyboard(press);
    expect(document.activeElement).toBe(items()[0]);
    expect(button().getAttribute('aria-expanded')).toBe('true');

    await keys.keyboard('{Escape}');
    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(button());
  });

  it('the card itself: a right-click opens the menu at the pointer, Shift+F10 on its cover next to the cover', async () => {
    const keys = setup({ cards: true });
    const cover = screen.getAllByRole('button', { name: 'Hades' })[0];

    await rightClick(keys, cover, 60, 70);
    expect([(menu() as HTMLElement).style.left, (menu() as HTMLElement).style.top]).toEqual(['60px', '70px']);
    await keys.keyboard('{Escape}');
    expect(menu()).toBeNull();

    cover.getBoundingClientRect = box(200, 150, 100, 140);
    cover.focus();
    await keys.keyboard('{Shift>}{F10}{/Shift}');
    const opened = screen.getByRole('menu', { name: menuName('Hades') });
    expect([opened.style.left, opened.style.top]).toEqual(['200px', '294px']);
    expect(document.activeElement).toBe(items()[0]);
  });

  it('Enter held on the button opens the menu once and runs nothing', async () => {
    const keys = setup({ cards: true });
    button().focus();

    await keys.keyboard('{Enter>6}');
    await keys.keyboard('{/Enter}');

    expect(screen.getAllByRole('menu')).toHaveLength(1);
    expect(document.activeElement).toBe(items()[0]);
    expect(vi.mocked(playGame)).not.toHaveBeenCalled();
  });
});

describe('game context menu and dialogs', () => {
  it('a dialog that opens while the menu is open closes the menu first', async () => {
    const keys = setup();
    await rightClick(keys, link('Hades'), 120, 80);

    render(Modal, { target: app, props: { open: true, title: 'The game closed' } });

    expect(menu()).toBeNull();
  });

  it('a dialog from an item after a keyboard open gives focus back to the game when it closes', async () => {
    const keys = setup();
    link('Celeste').focus();
    await keys.keyboard('{Shift>}{F10}{/Shift}');
    const tags = items().findIndex((item) => item.textContent?.trim() === msg('games.actionTags'));
    await keys.keyboard(`{ArrowDown>${tags}}{/ArrowDown}{Enter}`);
    expect(await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') })).toBeTruthy();

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(link('Celeste'));
  });

  it('a dialog from an item of the "…" menu gives focus back to the button', async () => {
    const keys = setup({ cards: true });
    const more = screen.getByRole('button', { name: menuName('Celeste') });
    await keys.click(more);
    await keys.click(screen.getByRole('menuitem', { name: msg('games.actionTags') }));
    expect(await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') })).toBeTruthy();

    await keys.keyboard('{Escape}');

    expect(document.activeElement).toBe(more);
  });

  it('a right-click while another control has focus: the dialog from an item gives focus to the game, not to that control', async () => {
    const keys = setup();
    const outside = screen.getByRole('button', { name: 'Outside' });
    outside.focus();
    // WebKit moves no focus on a right-click; fireEvent dispatches the bare event.
    fireEvent.contextMenu(link('Hades'), { clientX: 120, clientY: 80, button: 2 });
    await tick();
    await keys.click(screen.getByRole('menuitem', { name: msg('games.actionTags') }));
    expect(await screen.findByRole('dialog', { name: msg('modals.gameTagsTitle') })).toBeTruthy();

    await keys.keyboard('{Escape}');

    expect(document.activeElement).toBe(link('Hades'));
  });
});
