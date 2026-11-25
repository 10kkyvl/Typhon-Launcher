// First: services/library reaches this store back through stores/cloudsaves, and a store loaded
// inside that cycle would keep the real service instead of the mock below.
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get, type Writable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { launchesInFlight, playGame, selectGameExecutable, setExecutable, stopGame, type LibraryGame } from '../../lib/services/library';
import type { MoveJob } from '../../lib/services/relocate';
import { moves, moveTarget } from '../../lib/stores/relocate';
import { toasts } from '../../lib/stores/toasts';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

// The row's one action: it follows the game's state on the same control, so focus stays on it, and
// it is busy from the press to the answer, so a second press sends nothing. The menu's Move, and
// what a move in progress turns off.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  playGame: vi.fn(async () => {}),
  stopGame: vi.fn(async () => {}),
  selectGameExecutable: vi.fn(async () => ''),
  setExecutable: vi.fn(async () => ({})),
  // A launch another screen started (the game page, Big Picture) is in flight until Go answers it.
  launchesInFlight: (await import('svelte/store')).writable<ReadonlySet<string>>(new Set()),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

function game(id: string, title: string, extra: Partial<LibraryGame> = {}): LibraryGame {
  return {
    id,
    title,
    installDir: `C:\\Games\\${title}`,
    executable: `C:\\Games\\${title}\\game.exe`,
    sizeBytes: 0,
    uninstalled: false,
    tags: [],
    lastPlayed: null,
    ...extra,
  } as unknown as LibraryGame;
}

const hades = game('g-hades', 'Hades');
const celeste = game('g-celeste', 'Celeste');
const row = (id: string) => document.querySelector(`[data-game-row="${id}"]`) as HTMLElement;
const toastTexts = () => get(toasts).map((item) => `${item.kind}:${item.message}`);
const inFlight = launchesInFlight as Writable<ReadonlySet<string>>;

beforeEach(() => {
  libraryLoaded.set(true);
  libraryGames.set([hades, celeste]);
  installedView.set('list');
});

afterEach(() => {
  cleanup();
  libraryGames.set([]);
  libraryLoaded.set(false);
  runningGames.set(new Set());
  installedView.set('list');
  moves.set([]);
  moveTarget.set(null);
  toasts.set([]);
  inFlight.set(new Set());
  vi.clearAllMocks();
  document.body.replaceChildren();
});

describe('Installed: the row’s one action keeps focus as the game’s state changes', () => {
  it.each(['list', 'grid'] as const)('in the %s, Play becomes Stop on the same control when the game starts, and back', async (view) => {
    installedView.set(view);
    const { user } = renderDom(Installed);
    const play = within(row('g-hades')).getByRole('button', { name: msg('games.play') });
    play.focus();

    await user.keyboard('{Enter}');
    expect(playGame).toHaveBeenCalledWith('g-hades');
    runningGames.set(new Set(['g-hades']));
    await tick();

    expect(play.isConnected).toBe(true);
    expect(document.activeElement).toBe(play);
    expect(play.getAttribute('aria-label') ?? play.textContent?.trim()).toBe(msg('games.installedStopButton'));

    runningGames.set(new Set());
    await tick();
    expect(document.activeElement).toBe(play);
    expect(play.getAttribute('aria-label') ?? play.textContent?.trim()).toBe(msg('games.play'));
  });

  it('in the list, “Set file” becomes Play on the same control once the file is saved', async () => {
    libraryGames.set([game('g-celeste', 'Celeste', { executable: '' })]);
    vi.mocked(selectGameExecutable).mockResolvedValue('C:\\Games\\Celeste\\Celeste.exe');
    const { user } = renderDom(Installed);
    const action = within(row('g-celeste')).getByRole('button', { name: msg('games.installedSetExeButton') });
    action.focus();

    await user.keyboard('{Enter}');
    await vi.waitFor(() => expect(setExecutable).toHaveBeenCalled());
    libraryGames.set([celeste]);
    await tick();

    expect(document.activeElement).toBe(action);
    expect(action.textContent?.trim()).toBe(msg('games.play'));
  });
});

describe('Installed: the row’s action is busy from the press to the answer', () => {
  it.each(['list', 'grid'] as const)('in the %s, a second press while Play waits sends nothing; focus stays', async (view) => {
    installedView.set(view);
    let answer!: () => void;
    vi.mocked(playGame).mockReturnValue(new Promise<void>((resolve) => (answer = resolve)));
    const { user } = renderDom(Installed);
    const play = within(row('g-hades')).getByRole('button', { name: msg('games.play') });
    play.focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(playGame).toHaveBeenCalledTimes(1);
    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(play.hasAttribute('disabled')).toBe(false);
    expect(document.activeElement).toBe(play);

    answer();
    await vi.waitFor(() => expect(play.hasAttribute('aria-busy')).toBe(false));
  });

  it.each(['list', 'grid'] as const)('in the %s, two presses in one task, before the busy state is drawn, send one Play', async (view) => {
    installedView.set(view);
    vi.mocked(playGame).mockReturnValue(new Promise<void>(() => {}));
    renderDom(Installed);
    const play = within(row('g-hades')).getByRole('button', { name: msg('games.play') });

    play.click();
    play.click();
    await tick();

    expect(playGame).toHaveBeenCalledTimes(1);
  });

  it.each(['list', 'grid'] as const)('in the %s, a launch another screen started keeps the control busy, and a press sends nothing', async (view) => {
    installedView.set(view);
    inFlight.set(new Set(['g-hades']));
    const { user } = renderDom(Installed);
    const play = within(row('g-hades')).getByRole('button', { name: msg('games.play') });

    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(within(row('g-celeste')).getByRole('button', { name: msg('games.play') }).hasAttribute('aria-busy')).toBe(false);
    await user.click(play);
    expect(playGame).not.toHaveBeenCalled();

    inFlight.set(new Set());
    await tick();
    expect(play.hasAttribute('aria-busy')).toBe(false);
    await user.click(play);
    expect(playGame).toHaveBeenCalledWith('g-hades');
  });

  it('a failed Stop is said in the catalogue’s words for its code, never Go’s text', async () => {
    runningGames.set(new Set(['g-hades']));
    vi.mocked(stopGame).mockRejectedValueOnce(new Error('typhon:library.moving: raw go text'));
    const { user } = renderDom(Installed);

    await user.click(within(row('g-hades')).getByRole('button', { name: msg('games.installedStopButton') }));

    await vi.waitFor(() => expect(toastTexts()).toContain(`danger:${msg('library.moving')}`));
    expect(toastTexts().join(' ')).not.toContain('raw go text');
  });

  it('a failed Stop without a known code says the screen’s own words', async () => {
    runningGames.set(new Set(['g-hades']));
    vi.mocked(stopGame).mockRejectedValueOnce(new Error('raw go text'));
    const { user } = renderDom(Installed);

    await user.click(within(row('g-hades')).getByRole('button', { name: msg('games.installedStopButton') }));

    await vi.waitFor(() => expect(toastTexts()).toContain(`danger:${msg('games.errorStopFailed')}`));
  });
});

describe('Installed: Move in the row’s menu', () => {
  const menu = (title: string) => screen.getByRole('button', { name: msg('games.installedMenuFor', { title }) });
  const item = (key: Parameters<typeof msg>[0]) => screen.queryByRole('menuitem', { name: (name) => name.startsWith(msg(key)) });

  it('opens the move dialog for that game', async () => {
    const { user } = renderDom(Installed);

    await user.click(menu('Celeste'));
    await user.click(item('games.actionMove') as HTMLElement);

    expect(get(moveTarget)).toEqual({ gameId: 'g-celeste', title: 'Celeste' });
  });

  it.each([
    ['an emulated game', game('g-x', 'X', { emulation: { platform: 'ps2' } })],
    ['an installed disc image', game('g-x', 'X', { emulation: { platform: 'ps2' }, owned: true, installType: 'console_image' })],
    ['a game of another launcher', game('g-x', 'X', { executable: '', external: { provider: 'steam', appId: '1', launchUri: 'steam://run/1' } })],
  ])('is not offered for %s, which Go does not move', async (_name, one) => {
    libraryGames.set([one]);
    const { user } = renderDom(Installed);

    await user.click(menu('X'));

    expect(item('games.actionMove')).toBeNull();
  });

  it('is not offered while the game runs', async () => {
    runningGames.set(new Set(['g-hades']));
    const { user } = renderDom(Installed);

    await user.click(menu('Hades'));

    expect(item('games.actionMove')).toBeNull();
  });

  it('while the game is being moved, Move and both removals are off and say why; another game’s are not', async () => {
    moves.set([{ id: 'm1', scope: 'game', stage: 'copy', gameId: 'g-hades', title: 'Hades' } as unknown as MoveJob]);
    const { user } = renderDom(Installed);
    expect(row('g-hades').textContent).toContain(msg('installed.moving'));

    await user.click(menu('Hades'));
    for (const key of ['games.actionMove', 'games.actionUninstall', 'games.actionRemoveLibrary'] as const) {
      const off = item(key) as HTMLElement;
      expect(off.getAttribute('aria-disabled')).toBe('true');
      expect(off.textContent?.trim()).toBe(msg('installed.menuMoving', { action: msg(key) }));
    }
    await user.keyboard('{Escape}');

    await user.click(menu('Celeste'));
    for (const key of ['games.actionMove', 'games.actionUninstall', 'games.actionRemoveLibrary'] as const) {
      expect((item(key) as HTMLElement).getAttribute('aria-disabled')).toBeNull();
    }
  });

  it('with two games being moved, each row says so and has Move and both removals off, not only the first listed', async () => {
    moves.set([
      { id: 'm1', scope: 'game', stage: 'copy', gameId: 'g-hades', title: 'Hades' } as unknown as MoveJob,
      { id: 'm2', scope: 'game', stage: 'copy', gameId: 'g-celeste', title: 'Celeste' } as unknown as MoveJob,
    ]);
    const { user } = renderDom(Installed);

    for (const [id, title] of [['g-hades', 'Hades'], ['g-celeste', 'Celeste']] as const) {
      expect(row(id).textContent, title).toContain(msg('installed.moving'));
      await user.click(menu(title));
      for (const key of ['games.actionMove', 'games.actionUninstall', 'games.actionRemoveLibrary'] as const) {
        expect((item(key) as HTMLElement).getAttribute('aria-disabled'), `${title} ${key}`).toBe('true');
      }
      await user.keyboard('{Escape}');
    }
  });

  it('while the whole library is being moved, every row says so and has Move and both removals off with the reason', async () => {
    moves.set([{ id: 'm1', scope: 'library', stage: 'copy', gameId: '', title: '' } as unknown as MoveJob]);
    const { user } = renderDom(Installed);

    for (const [id, title] of [['g-hades', 'Hades'], ['g-celeste', 'Celeste']] as const) {
      expect(row(id).textContent, title).toContain(msg('installed.moving'));
      await user.click(menu(title));
      for (const key of ['games.actionMove', 'games.actionUninstall', 'games.actionRemoveLibrary'] as const) {
        const off = item(key) as HTMLElement;
        expect(off.getAttribute('aria-disabled'), `${title} ${key}`).toBe('true');
        expect(off.textContent?.trim()).toBe(msg('installed.menuMoving', { action: msg(key) }));
      }
      await user.keyboard('{Escape}');
    }
  });
});
