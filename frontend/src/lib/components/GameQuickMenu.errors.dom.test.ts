import { fireEvent, render, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../i18n';
import {
  createShortcut,
  locateSaves,
  playGame,
  playGameProfile,
  removeShortcut,
  setHidden,
  stopGame,
  type LibraryGame,
} from '../services/library';
import { unshare } from '../services/lan';
import { openFolder, openGameFolder } from '../services/settings';
import { closeGameMenu } from '../stores/gameMenu';
import { shares } from '../stores/lan';
import { libraryGames, runningGames } from '../stores/library';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import GameMenuHarness from '../testing/GameMenuHarness.svelte';
import type { Settings } from '../services/settings';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  steamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
  playGame: vi.fn(),
  playGameProfile: vi.fn(),
  stopGame: vi.fn(),
  createShortcut: vi.fn(),
  removeShortcut: vi.fn(),
  setFavorite: vi.fn(),
  setHidden: vi.fn(),
  locateSaves: vi.fn(),
}));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  openGameFolder: vi.fn(),
  openFolder: vi.fn(),
}));
vi.mock('../services/lan', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/lan')>()),
  unshare: vi.fn(),
}));

const RIGHT_CLICK = { button: 2, clientX: 120, clientY: 80 };

const hades = {
  id: 'g1',
  title: 'Hades',
  uninstalled: false,
  installDir: '/games/hades',
  executable: 'Hades.exe',
  profiles: [{ id: 'p1', name: 'Speedrun', executable: 'Hades.exe' }],
} as LibraryGame;

// What Go sends: a code the launcher has words for, a code it has none for, and prose with no code at
// all (Russian from Go, English from the runtime).
const CODED = 'library.not_installed' as const;
const failures = {
  coded: () => new Error(`typhon:${CODED}: игра не установлена`),
  'coded without text': () => new Error('typhon:library.no_such_code_here: что-то пошло не так'),
  'russian prose': () => new Error('не удалось открыть: доступ запрещён'),
  'english prose': () => new Error('exec: "explorer": executable file not found in $PATH'),
};
type Failure = keyof typeof failures;

const expected = (failure: Failure, fallback: MessageKey) => (failure === 'coded' ? msg(CODED) : msg(fallback));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  closeGameMenu();
  libraryGames.set([]);
  runningGames.set(new Set());
  settings.set(null);
  shares.set([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

async function pick(item: string) {
  render(GameMenuHarness, { target: app });
  await fireEvent.contextMenu(screen.getByRole('link', { name: 'Hades' }), RIGHT_CLICK);
  await user().click(await screen.findByRole('menuitem', { name: item }));
}

const shown = () => get(toasts).map((toast) => [toast.message, toast.kind]);

interface Case {
  action: string;
  item: () => string;
  game?: Partial<LibraryGame>;
  running?: boolean;
  fail: (err: Error) => void;
  fallback: MessageKey;
}

const CASES: Case[] = [
  { action: 'play', item: () => msg('games.play'), fail: (err) => vi.mocked(playGame).mockRejectedValueOnce(err), fallback: 'games.errorPlayFailed' },
  {
    action: 'profile play',
    item: () => msg('games.actionPlayProfile', { name: 'Speedrun' }),
    fail: (err) => vi.mocked(playGameProfile).mockRejectedValueOnce(err),
    fallback: 'games.errorPlayFailed',
  },
  { action: 'stop', item: () => msg('games.stop'), running: true, fail: (err) => vi.mocked(stopGame).mockRejectedValueOnce(err), fallback: 'games.errorStopFailed' },
  { action: 'open folder', item: () => msg('games.openFolder'), fail: (err) => vi.mocked(openGameFolder).mockRejectedValueOnce(err), fallback: 'library.menuFolderFailed' },
  {
    action: 'create shortcut',
    item: () => msg('games.actionShortcutCreate'),
    fail: (err) => vi.mocked(createShortcut).mockRejectedValueOnce(err),
    fallback: 'games.detailShortcutCreateError',
  },
  {
    action: 'remove shortcut',
    item: () => msg('games.actionShortcutRemove'),
    game: { shortcutPath: '/desktop/Hades.lnk' },
    fail: (err) => vi.mocked(removeShortcut).mockRejectedValueOnce(err),
    fallback: 'games.detailShortcutRemoveError',
  },
  { action: 'find the saves', item: () => msg('games.actionOpenSaves'), fail: (err) => vi.mocked(locateSaves).mockRejectedValueOnce(err), fallback: 'library.menuSavesFailed' },
  {
    action: 'open the saves',
    item: () => msg('games.actionOpenSaves'),
    fail: (err) => {
      vi.mocked(locateSaves).mockResolvedValueOnce({ path: '/saves/hades', candidates: [], unreadable: 0 });
      vi.mocked(openFolder).mockRejectedValueOnce(err);
    },
    fallback: 'library.menuSavesFailed',
  },
  { action: 'hide', item: () => msg('games.actionHide'), fail: (err) => vi.mocked(setHidden).mockRejectedValueOnce(err), fallback: 'games.hideFailed' },
];

describe('a failed action of the game menu', () => {
  describe.each(CASES)('$action', ({ item, game, running, fail, fallback }) => {
    it.each(Object.keys(failures) as Failure[])('%s: the launcher says it in its own words', async (failure) => {
      libraryGames.set([{ ...hades, ...game }]);
      if (running) runningGames.set(new Set(['g1']));
      fail(failures[failure]());

      await pick(item());

      await vi.waitFor(() => expect(shown()).toEqual([[expected(failure, fallback), 'danger']]));
    });
  });

  // The store owns the unshare request and its errors: the menu only asks. Its text must not be raw either.
  it.each(Object.keys(failures) as Failure[])('stop sharing over LAN, %s: no raw message', async (failure) => {
    libraryGames.set([hades]);
    settings.set({ lanSharing: true } as Settings);
    shares.set([{ gameId: 'g1' } as never]);
    const err = failures[failure]();
    vi.mocked(unshare).mockRejectedValueOnce(err);

    await pick(msg('games.actionLanUnshare'));

    await vi.waitFor(() => expect(shown()).toHaveLength(1));
    const [[text, kind]] = shown();
    expect(kind).toBe('danger');
    expect(text).not.toContain(err.message.replace(/^typhon:[\w.]+: /, ''));
    expect(text).not.toContain('typhon:');
  });

  it('a failed launch reads the same from the menu as from a card', async () => {
    libraryGames.set([hades]);
    vi.mocked(playGame).mockRejectedValueOnce(failures['russian prose']());

    await pick(msg('games.play'));

    // Library.svelte toggleRun: libraryErrorText(err, msg('games.errorPlayFailed')).
    await vi.waitFor(() => expect(shown()).toEqual([[msg('games.errorPlayFailed'), 'danger']]));
  });
});
