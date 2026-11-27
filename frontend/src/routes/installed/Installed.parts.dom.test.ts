import { tick } from 'svelte';
import { cleanup, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { compatStatuses } from '../../lib/services/compat';
import { inspectRemoval } from '../../lib/services/install';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { loadArt } from '../../lib/stores/metadata';
import { refreshMissingImages } from '../../lib/stores/missingImages';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

// The page keeps what its parts share and hands it down. A prop passed without `bind:`, a callback
// not wired or a read moved into a part that is not always mounted breaks nothing a type check sees:
// these tests go through such seams the way a player does: the search and the sorting, both removals,
// the add dialog's fresh fields, the three reads and what makes each run again. Not held here: choose
// folder, Find games and its progress words, Add image, the broken badge from the compatibility log,
// covers following the art store.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
const backend = vi.hoisted(() => ({ inWails: false }));
vi.mock('../../lib/services/backend', () => backend);
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/install')>()),
  inspectRemoval: vi.fn(),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

const games = [
  { id: 'g-hades', title: 'Hades', lastPlayed: '2026-11-01T10:00:00Z', canonicalGameId: 'c-hades' },
  { id: 'g-celeste', title: 'Celeste', lastPlayed: null },
].map((game, i) => ({ ...game, installDir: `C:\\Games\\${i}`, executable: 'game.exe', sizeBytes: 0, uninstalled: false, tags: [] }) as unknown as LibraryGame);

const rowTitles = () => [...document.querySelectorAll('.row .title')].map((title) => title.textContent);
const shownOf = (shown: number) => msg('installed.shownOf', { shown, count: games.length });

beforeEach(() => {
  libraryGames.set(games);
  libraryLoaded.set(true);
  installedView.set('list');
});

afterEach(() => {
  libraryLoaded.set(false);
  // Unmounted first: a page left mounted would answer the next test's store changes with reads of its own.
  cleanup();
  libraryGames.set([]);
  runningGames.set(new Set());
  installedView.set('list');
  backend.inWails = false;
  vi.mocked(compatStatuses).mockClear();
  vi.mocked(loadArt).mockClear();
  vi.mocked(refreshMissingImages).mockClear();
  vi.mocked(inspectRemoval).mockReset();
  document.body.replaceChildren();
});

describe('Installed: the header controls and the list', () => {
  it('the search typed in the header filters the rows and the shown-of line', async () => {
    const { user } = renderDom(Installed);
    expect(rowTitles()).toEqual(['Hades', 'Celeste']);

    await user.type(screen.getByRole('textbox', { name: msg('games.installedSearchPlaceholder') }), 'cel');

    expect(rowTitles()).toEqual(['Celeste']);
    expect(screen.getByText(shownOf(1))).toBeTruthy();
  });

  it('the sorting picked in the header reorders the rows and is said on its chip', async () => {
    const { user } = renderDom(Installed);
    const chip = (sort: string) => msg('games.installedSortByLabel', { sort });

    await user.click(screen.getByRole('button', { name: chip(msg('games.recentLabel')) }));
    await user.click(screen.getByRole('menuitemradio', { name: msg('games.sortAlpha') }));

    expect(rowTitles()).toEqual(['Celeste', 'Hades']);
    expect(screen.getByRole('button', { name: chip(msg('games.sortAlpha')) })).toBeTruthy();
  });
});

describe('Installed: removing a game from a row’s menu', () => {
  it.each([
    ['games.actionUninstall', 'modals.removeGameModeDiskLabel'],
    ['games.actionRemoveLibrary', 'modals.removeGameModeLibraryLabel'],
  ] as const)('%s opens the page’s dialog for that game with %s chosen', async (item, mode) => {
    vi.mocked(inspectRemoval).mockResolvedValue({ owned: true, dirMissing: false, method: 'files', running: false, busy: false } as Awaited<ReturnType<typeof inspectRemoval>>);
    const { user } = renderDom(Installed);

    await user.click(screen.getByRole('button', { name: msg('games.installedMenuFor', { title: 'Celeste' }) }));
    await user.click(screen.getByRole('menuitem', { name: msg(item) }));

    const dialog = await screen.findByRole('dialog');
    expect(inspectRemoval).toHaveBeenCalledWith('g-celeste');
    const chosen = within(dialog).getAllByRole('radio').filter((radio) => radio.getAttribute('aria-checked') === 'true');
    expect(chosen.map((radio) => document.getElementById(radio.getAttribute('aria-labelledby') ?? '')?.textContent)).toEqual([msg(mode)]);
  });
});

describe('Installed: the add-game dialog', () => {
  it('opens with both fields empty every time', async () => {
    const { user } = renderDom(Installed);
    backend.inWails = true;
    const open = async () => {
      await user.click(screen.getByRole('button', { name: msg('games.installedAddGameButton') }));
      return within(await screen.findByRole('dialog')).getAllByRole('textbox') as HTMLInputElement[];
    };

    const [executable, title] = await open();
    await user.type(executable, 'C:\\Games\\Hades\\Hades.exe');
    await user.type(title, 'Hades');
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

    expect((await open()).map((field) => field.value)).toEqual(['', '']);
  });
});

describe('Installed: what the page reads', () => {
  const reads = () => [vi.mocked(compatStatuses), vi.mocked(loadArt), vi.mocked(refreshMissingImages)].map((read) => read.mock.calls.length);

  it.each([
    ['the list', 'list', games],
    ['the grid', 'grid', games],
    ['no games', 'list', []],
  ] as const)('with %s: the compatibility log, the art and the missing images, once each and in that order', async (_name, view, list) => {
    libraryGames.set([...list]);
    installedView.set(view);
    renderDom(Installed);
    await tick();

    expect(reads()).toEqual([1, 1, 1]);
    expect(loadArt).toHaveBeenCalledWith(list.length > 0 ? ['c-hades'] : []);
    const order = [compatStatuses, loadArt, refreshMissingImages].map((read) => vi.mocked(read).mock.invocationCallOrder[0]);
    expect(order).toEqual([...order].sort((a, b) => a - b));
  });

  it('the compatibility log again when a game starts, all three again when the list changes', async () => {
    renderDom(Installed);
    await tick();

    runningGames.set(new Set(['g-hades']));
    await tick();
    expect(reads()).toEqual([2, 1, 1]);

    libraryGames.set(games.slice(0, 1));
    await tick();
    expect(reads()).toEqual([3, 2, 2]);
  });
});
