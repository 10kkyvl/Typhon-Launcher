import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { readFilters, type PickFilters } from '../game/randomPick';
import { launchGame, type LibraryCollection, type LibraryGame } from '../services/library';
import { collections } from '../stores/collections';
import { libraryGenres } from '../stores/genres';
import { libraryGames } from '../stores/library';
import { user } from '../testing/dom';
import RandomGameModal from './RandomGameModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/library')>()),
  launchGame: vi.fn(async () => true),
}));

// The spin is held by the test: it ends when the test says so.
let finish: (() => void) | undefined;
vi.mock('../game/randomPick', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../game/randomPick')>()),
  reducedMotion: () => false,
  runSpin: (frames: LibraryGame[], onFrame: (game: LibraryGame) => void, onDone: (game: LibraryGame) => void) => {
    onFrame(frames[0]);
    finish = () => onDone(frames[frames.length - 1]);
    return () => {};
  },
}));

const STORAGE_KEY = 'typhon.randomPick';

const hades = { id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe', canonicalGameId: 'cat-hades' } as LibraryGame;
const celeste = { id: 'g2', title: 'Celeste', uninstalled: false, installDir: '/games/celeste', executable: 'Celeste.exe', canonicalGameId: 'cat-celeste' } as LibraryGame;
const secret = { id: 'g3', title: 'Secret', uninstalled: false, installDir: '/games/secret', executable: 'Secret.exe', canonicalGameId: 'cat-secret', hidden: true } as LibraryGame;
const evenings: LibraryCollection = { id: 'c1', name: 'Evenings', tags: ['coop'], createdAt: '', updatedAt: '' };
const weekend: LibraryCollection = { id: 'c2', name: 'Weekend', status: 'backlog', createdAt: '', updatedAt: '' };

let app: HTMLDivElement;

function save(filters: Partial<PickFilters>) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ source: 'installed', collectionId: '', stale: false, genre: '', ...filters }));
}

function openDialog() {
  render(RandomGameModal, { target: app, props: { onclose: vi.fn() } });
  return screen.getByRole('dialog', { name: msg('games.randomTitle') });
}

async function settled() {
  await tick();
  await new Promise((resolve) => setTimeout(resolve, 0));
  await tick();
}

// The filters are kept in localStorage; this environment has none, so each test gets its own.
function memoryStorage(): Storage {
  const items = new Map<string, string>();
  return {
    get length() {
      return items.size;
    },
    clear: () => items.clear(),
    getItem: (key) => items.get(key) ?? null,
    key: (index) => [...items.keys()][index] ?? null,
    removeItem: (key) => void items.delete(key),
    setItem: (key, value) => void items.set(key, String(value)),
  };
}

beforeEach(() => {
  vi.stubGlobal('localStorage', memoryStorage());
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  libraryGames.set([]);
  libraryGenres.set({});
  collections.set([]);
  vi.unstubAllGlobals();
  finish = undefined;
  vi.clearAllMocks();
  document.body.replaceChildren();
});

describe('RandomGameModal: the saved genre and collection', () => {
  it('opened before genres and collections arrive: both stay saved, and are chosen once they arrive', async () => {
    save({ source: 'collection', collectionId: 'c2', genre: 'Roguelike' });
    libraryGames.set([hades, celeste]);
    const dialog = openDialog();
    await settled();

    expect(readFilters()).toMatchObject({ collectionId: 'c2', genre: 'Roguelike' });

    libraryGenres.set({ 'cat-hades': ['Roguelike'], 'cat-celeste': ['Platformer'] });
    collections.set([evenings, weekend]);
    await settled();

    expect(readFilters()).toMatchObject({ collectionId: 'c2', genre: 'Roguelike' });
    expect(within(dialog).getByRole('combobox', { name: msg('games.randomGenreLabel') }).textContent?.trim()).toBe('Roguelike');
    expect(within(dialog).getByRole('combobox', { name: msg('games.randomSourceCollection') }).textContent?.trim()).toBe('Weekend');
  });

  it('after the genres of some games failed to load: the saved genre stays saved, and is left out of the pick', async () => {
    save({ genre: 'Roguelike' });
    libraryGames.set([hades, celeste]);
    // Celeste's catalog data came; Hades's request failed and is retried later.
    libraryGenres.set({ 'cat-celeste': ['Platformer'] });
    const dialog = openDialog();
    const keys = user();
    await settled();

    expect(readFilters().genre).toBe('Roguelike');

    await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSpin') }));
    finish?.();

    expect(await within(dialog).findByRole('button', { name: msg('games.randomAgain') })).toBeTruthy();
    expect(within(dialog).queryByText(msg('games.randomEmpty'))).toBeNull();
    expect(readFilters().genre).toBe('Roguelike');
  });

  it('a saved genre no game has any more is dropped once every game is known', async () => {
    save({ genre: 'Horror' });
    libraryGames.set([hades, celeste]);
    openDialog();
    await settled();
    expect(readFilters().genre).toBe('Horror');

    libraryGenres.set({ 'cat-hades': ['Roguelike'], 'cat-celeste': ['Platformer'] });
    await settled();

    expect(readFilters().genre).toBe('');
  });

  it('a saved collection the loaded list no longer has gives way to the first one', async () => {
    save({ source: 'collection', collectionId: 'gone' });
    libraryGames.set([hades]);
    collections.set([evenings, weekend]);
    openDialog();
    await settled();

    expect(readFilters().collectionId).toBe('c1');
  });
});

describe('RandomGameModal: the genre list', () => {
  it('offers no genre that only hidden games have', async () => {
    libraryGames.set([hades, celeste, secret]);
    libraryGenres.set({ 'cat-hades': ['Roguelike'], 'cat-celeste': ['Platformer'], 'cat-secret': ['Horror', 'Roguelike'] });
    const dialog = openDialog();
    const keys = user();

    await keys.click(within(dialog).getByRole('combobox', { name: msg('games.randomGenreLabel') }));

    const offered = screen.getAllByRole('option').map((option) => option.textContent?.trim());
    expect(offered).toEqual([msg('games.randomGenreAny'), 'Platformer', 'Roguelike']);
  });
});

describe('RandomGameModal: the result footer while it spins again', () => {
  it('keeps Filters and Play focusable, refusing a press, and focus where it is', async () => {
    libraryGames.set([hades, celeste]);
    const dialog = openDialog();
    const keys = user();
    await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSpin') }));
    finish?.();
    const again = await within(dialog).findByRole('button', { name: msg('games.randomAgain') });
    const filters = within(dialog).getByRole('button', { name: msg('games.randomFilters') });
    const play = within(dialog).getByRole('button', { name: msg('games.randomPlay') });
    again.focus();

    await keys.keyboard('{Enter}');
    expect(again.getAttribute('aria-busy')).toBe('true');

    for (const control of [filters, play]) {
      expect(control).toHaveProperty('disabled', false);
      expect(control.getAttribute('aria-disabled')).toBe('true');
    }
    await keys.tab();
    expect(document.activeElement).toBe(play);
    await keys.keyboard('{Enter}');
    expect(launchGame).not.toHaveBeenCalled();
    expect(play.isConnected).toBe(true);

    await keys.tab({ shift: true });
    await keys.tab({ shift: true });
    expect(document.activeElement).toBe(filters);
    await keys.keyboard('{Enter}');
    expect(filters.isConnected).toBe(true);
    expect(document.activeElement).toBe(filters);

    finish?.();
    await vi.waitFor(() => expect(filters.getAttribute('aria-disabled')).toBeNull());
    expect(play.getAttribute('aria-disabled')).toBeNull();
    expect(document.activeElement).toBe(filters);
  });
});
