import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { setHidden, setStatus, type LibraryGame } from '../services/library';
import { closeGameMenu, openGameMenu } from '../stores/gameMenu';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import GameMenuHarness from '../testing/GameMenuHarness.svelte';
import GameCard from './GameCard.svelte';
import GameQuickMenu from './GameQuickMenu.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  steamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
  setHidden: vi.fn(),
  setStatus: vi.fn(),
}));

const RIGHT_CLICK = { button: 2, clientX: 120, clientY: 80 };

const hades = { id: 'g1', title: 'Hades', uninstalled: false, executable: 'Hades.exe' } as LibraryGame;
const celeste = { id: 'g2', title: 'Celeste', uninstalled: false, executable: 'Celeste.exe' } as LibraryGame;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  closeGameMenu();
  libraryGames.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

// The list the menu was opened from drops the game (hidden from "All", shown again from "Hidden").
const leave = () => libraryGames.update((list) => list.filter((game) => game.id !== 'g1'));
const mark = (patch: Partial<LibraryGame>) => libraryGames.update((list) => list.map((game) => (game.id === 'g1' ? { ...game, ...patch } : game)));

async function pick(item: string | RegExp, how: 'pointer' | 'keyboard') {
  const keys = user();
  const link = screen.getByRole('link', { name: 'Hades' });
  if (how === 'pointer') {
    await fireEvent.contextMenu(link, RIGHT_CLICK);
    await keys.click(await screen.findByRole('menuitem', { name: item }));
  } else {
    link.focus();
    await keys.keyboard('{Shift>}{F10}{/Shift}');
    const target = screen.getByRole('menuitem', { name: item });
    const steps = screen.getAllByRole('menuitem').indexOf(target);
    if (steps > 0) await keys.keyboard(`{ArrowDown>${steps}}{/ArrowDown}`);
    await keys.keyboard('{Enter}');
  }
  return link;
}

async function settled() {
  await tick();
  await new Promise((resolve) => setTimeout(resolve, 0));
  await tick();
}

describe.each([
  ['hide', () => hades, () => msg('games.actionHide')],
  ['unhide', () => ({ ...hades, hidden: true }), () => msg('games.actionUnhide')],
] as const)('%s from the game menu', (_name, game, item) => {
  it.each(['pointer', 'keyboard'] as const)('game leaves the list before the call answers (menu by %s): focus to the next game', async (how) => {
    libraryGames.set([game(), celeste]);
    vi.mocked(setHidden).mockImplementationOnce(async () => {
      leave();
      return game();
    });
    render(GameMenuHarness, { target: app });

    await pick(item(), how);
    await settled();

    expect(document.activeElement).toBe(screen.getByRole('link', { name: 'Celeste' }));
  });

  it.each(['pointer', 'keyboard'] as const)('game leaves the list after the call answers (menu by %s): focus to the next game', async (how) => {
    libraryGames.set([game(), celeste]);
    vi.mocked(setHidden).mockResolvedValueOnce(game());
    render(GameMenuHarness, { target: app });

    const link = await pick(item(), how);
    await settled();
    expect(document.activeElement).toBe(link);

    leave();
    await settled();

    expect(document.activeElement).toBe(screen.getByRole('link', { name: 'Celeste' }));
  });

  it('the only game leaves: focus to the page heading', async () => {
    libraryGames.set([game()]);
    vi.mocked(setHidden).mockImplementationOnce(async () => {
      leave();
      return game();
    });
    render(GameMenuHarness, { target: app });

    await pick(item(), 'pointer');
    await settled();

    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' }));
  });

  it('the game stays in the list: focus stays on its control', async () => {
    libraryGames.set([game(), celeste]);
    vi.mocked(setHidden).mockImplementationOnce(async (_id, on) => {
      mark({ hidden: on });
      return game();
    });
    render(GameMenuHarness, { target: app });

    const link = await pick(item(), 'pointer');
    await settled();

    expect(link.isConnected).toBe(true);
    expect(document.activeElement).toBe(link);
  });
});

describe('a status change that takes the game out of the list, after its dialog closed', () => {
  it('focus goes to the next game', async () => {
    libraryGames.set([hades, celeste]);
    vi.mocked(setStatus).mockResolvedValueOnce(hades);
    render(GameMenuHarness, { target: app });

    const link = await pick(new RegExp(msg('games.actionStatus', { status: '' }).trim().split(':')[0]), 'pointer');
    const dialog = await screen.findByRole('dialog', { name: msg('modals.gameStatusTitle') });
    await user().click(screen.getByRole('button', { name: msg('games.statusPlaying') }));
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(document.activeElement).toBe(link);

    leave();
    await settled();

    expect(document.activeElement).toBe(screen.getByRole('link', { name: 'Celeste' }));
  });
});

describe('a menu its caller opened without naming the game element (no source)', () => {
  it('has no home to hand focus to: focus stays where it fell', async () => {
    libraryGames.set([hades, celeste]);
    vi.mocked(setHidden).mockImplementationOnce(async () => {
      leave();
      return hades;
    });
    render(GameMenuHarness, { target: app });
    const link = screen.getByRole('link', { name: 'Hades' });
    link.focus();
    openGameMenu('g1', { at: { anchor: link }, focus: 'first', trigger: null, source: null });
    await tick();

    await user().click(screen.getByRole('menuitem', { name: msg('games.actionHide') }));
    await settled();

    expect(link.isConnected).toBe(false);
    expect(document.activeElement).toBe(document.body);
  });
});

// Library cards as the page draws them: a source badge on the cover before the card's controls.
function cards() {
  const main = document.createElement('main');
  const heading = document.createElement('h1');
  heading.textContent = 'Library';
  heading.tabIndex = -1;
  const grid = document.createElement('div');
  main.append(heading, grid);
  app.append(main);
  const unmount: Record<string, () => void> = {};
  for (const game of [hades, celeste]) {
    const view = render(GameCard, { target: grid, props: { id: game.id, title: game.title, source: 'steam', installed: true, onplay: vi.fn() } });
    unmount[game.id] = view.unmount;
  }
  render(GameQuickMenu, { target: app });
  const card = (title: string) => screen.getByRole('button', { name: title }).closest('.card') as HTMLElement;
  const play = (title: string) => card(title).querySelector<HTMLElement>('button.play') as HTMLElement;
  return { card, play, unmount };
}

describe('library cards that carry a source badge before their controls', () => {
  it('Escape on a menu opened by pointer puts focus on the card\'s Play, not on its badge', async () => {
    libraryGames.set([hades, celeste]);
    const { card, play } = cards();

    await fireEvent.contextMenu(card('Hades'), RIGHT_CLICK);
    expect(await screen.findByRole('menu')).toBeTruthy();
    await user().keyboard('{Escape}');

    expect(screen.queryByRole('menu')).toBeNull();
    expect(document.activeElement).toBe(play('Hades'));
  });

  it('Hide puts focus on the next card\'s Play, not on its badge', async () => {
    libraryGames.set([hades, celeste]);
    const { card, play, unmount } = cards();
    vi.mocked(setHidden).mockImplementationOnce(async () => {
      leave();
      unmount.g1();
      return hades;
    });

    await fireEvent.contextMenu(card('Hades'), RIGHT_CLICK);
    await user().click(await screen.findByRole('menuitem', { name: msg('games.actionHide') }));
    await settled();

    expect(document.activeElement).toBe(play('Celeste'));
  });
});

describe('a player who moved on before the game left the list', () => {
  it('keeps focus where the player put it', async () => {
    libraryGames.set([hades, celeste]);
    let answer!: () => void;
    vi.mocked(setHidden).mockImplementationOnce(() => new Promise<LibraryGame>((resolve) => (answer = () => resolve(hades))));
    render(GameMenuHarness, { target: app });

    const link = await pick(msg('games.actionHide'), 'pointer');
    await settled();
    expect(document.activeElement).toBe(link);

    const elsewhere = screen.getByRole('button', { name: 'Outside' });
    elsewhere.focus();
    leave();
    answer();
    await settled();

    expect(link.isConnected).toBe(false);
    expect(document.activeElement).toBe(elsewhere);
  });
});
