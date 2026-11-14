// First: services/library reaches this store back through stores/cloudsaves (see Library.state.dom.test.ts).
import { libraryGames, libraryLoaded, runningGames } from '../../lib/stores/library';
import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import GameQuickMenu from '../../lib/components/GameQuickMenu.svelte';
import { msg } from '../../lib/i18n';
import { deleteCollection, setHidden, type LibraryCollection } from '../../lib/services/library';
import { unwatchGame, type Entry as WatchEntry } from '../../lib/services/watchlist';
import { collections, collectionsState } from '../../lib/stores/collections';
import { closeGameMenu } from '../../lib/stores/gameMenu';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { libraryView } from '../../lib/stores/ui';
import { watchAlertsLoaded, watchlist } from '../../lib/stores/watchlist';
import { renderDom, user as keys } from '../../lib/testing/dom';
import Library from './Library.svelte';
import { game, results, shownTitles } from './libraryTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
  getCollections: vi.fn(async () => []),
  allTags: vi.fn(async () => []),
  steamStatus: vi.fn(async () => ({ supported: false, installed: false, added: false })),
  setHidden: vi.fn(),
  deleteCollection: vi.fn(),
}));
vi.mock('../../lib/services/watchlist', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/watchlist')>()),
  unwatchGame: vi.fn(async () => {}),
}));

const alpha = game('g-alpha', 'Alpha');
const bravo = game('g-bravo', 'Bravo');
const charlie = game('g-charlie', 'Charlie');

/** The control focus is on names this game: its title, or a control of its card or row. */
function focusedTitle(): string | undefined {
  const list = results();
  const item = list ? [...list.children].find((child) => child.contains(document.activeElement)) : undefined;
  return item?.querySelector('[id$="-title"]')?.textContent?.trim();
}

/** Opens the menu of the game whose title has focus from the keyboard and picks `item`. */
async function pickFromMenu(title: string, item: string) {
  const user = keys();
  const index = shownTitles().indexOf(title);
  const link = within((results() as HTMLElement).children[index] as HTMLElement).getAllByRole('button')[0];
  link.focus();
  await user.keyboard('{Shift>}{F10}{/Shift}');
  const target = await screen.findByRole('menuitem', { name: item });
  const steps = screen.getAllByRole('menuitem').indexOf(target);
  if (steps > 0) await user.keyboard(`{ArrowDown>${steps}}{/ArrowDown}`);
  await user.keyboard('{Enter}');
}

const settle = async () => {
  await tick();
  await new Promise((resolve) => setTimeout(resolve, 0));
  await tick();
};

const hiding = (id: string, hidden: boolean) =>
  vi.mocked(setHidden).mockImplementationOnce(async () => {
    libraryGames.update((list) => list.map((one) => (one.id === id ? { ...one, hidden } : one)));
    return game(id, id);
  });

beforeEach(() => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  libraryLoaded.set(true);
  libraryGames.set([alpha, bravo, charlie]);
  collections.set([]);
  collectionsState.set('ready');
  vi.mocked(setHidden).mockReset();
  vi.mocked(unwatchGame).mockReset();
  vi.mocked(deleteCollection).mockReset();
});

afterEach(() => {
  closeGameMenu();
  libraryGames.set([]);
  runningGames.set(new Set());
  watchlist.set([]);
  watchAlertsLoaded.set(false);
  document.body.replaceChildren();
});

describe.each(['list', 'grid'] as const)('Library (%s): a game that leaves the list from the menu', (view) => {
  beforeEach(() => libraryView.set(view));

  it('Hide hands focus to the next game', async () => {
    renderDom(Library);
    render(GameQuickMenu);
    hiding('g-bravo', true);

    await pickFromMenu('Bravo', msg('games.actionHide'));
    await settle();

    expect(shownTitles()).toEqual(['Alpha', 'Charlie']);
    expect(focusedTitle()).toBe('Charlie');
  });

  it('Hide of the last game hands focus to the one before it', async () => {
    renderDom(Library);
    render(GameQuickMenu);
    hiding('g-charlie', true);

    await pickFromMenu('Charlie', msg('games.actionHide'));
    await settle();

    expect(focusedTitle()).toBe('Bravo');
  });

  it('Unhide of the only hidden game hands focus to the first control of the list section', async () => {
    libraryGames.set([alpha, { ...bravo, hidden: true }]);
    libraryChoices.set({ ...DEFAULT_CHOICES, filter: 'hidden' });
    renderDom(Library);
    render(GameQuickMenu);
    hiding('g-bravo', false);

    await pickFromMenu('Bravo', msg('games.actionUnhide'));
    await settle();

    expect(results()).toBeNull();
    expect(document.activeElement).toBe(screen.getByLabelText(msg('games.librarySearchPlaceholder')));
  });
});

describe('Library: unwatching a watched game', () => {
  const watched = (gameId: string, title: string) => ({ gameId, title }) as WatchEntry;

  beforeEach(() => {
    libraryView.set('grid');
    watchAlertsLoaded.set(true);
    watchlist.set([watched('c-one', 'One'), watched('c-two', 'Two')]);
    libraryChoices.set({ ...DEFAULT_CHOICES, filter: 'watched' });
  });

  const unwatchOn = (title: string) =>
    within((results() as HTMLElement).children[shownTitles().indexOf(title)] as HTMLElement).getByRole('button', { name: msg('games.unwatchAction') });

  it('is busy while the call runs: a second press sends nothing', async () => {
    let release = () => {};
    vi.mocked(unwatchGame).mockReturnValueOnce(new Promise<void>((resolve) => (release = resolve)));
    const { user } = renderDom(Library);
    const unwatch = unwatchOn('One');
    unwatch.focus();

    await user.keyboard('{Enter}');
    await user.keyboard('{Enter}');

    expect(unwatchGame).toHaveBeenCalledTimes(1);
    expect(unwatch.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(unwatch);
    release();
  });

  it('hands focus to the next watched game', async () => {
    const { user } = renderDom(Library);
    unwatchOn('One').focus();

    await user.keyboard('{Enter}');
    await settle();

    expect(shownTitles()).toEqual(['Two']);
    expect(focusedTitle()).toBe('Two');
  });

  it('of the last one, hands focus to the first game of "All" it returns to', async () => {
    watchlist.set([watched('c-one', 'One')]);
    const { user } = renderDom(Library);
    unwatchOn('One').focus();

    await user.keyboard('{Enter}');
    await settle();

    expect(shownTitles()).toEqual(['Alpha', 'Bravo', 'Charlie']);
    expect(focusedTitle()).toBe('Alpha');
  });
});

describe('Library: the remembered game', () => {
  it('moves to its neighbour when typing filters it out', async () => {
    libraryView.set('list');
    const { user } = renderDom(Library);
    const before = screen.getByRole('button', { name: msg('games.collectionAdd') });
    before.focus();
    await user.tab();
    await user.keyboard('{ArrowDown}');
    expect(focusedTitle()).toBe('Bravo');

    await user.type(screen.getByLabelText(msg('games.librarySearchPlaceholder')), 'l');
    expect(shownTitles()).toEqual(['Alpha', 'Charlie']);
    before.focus();
    await user.tab();

    expect(focusedTitle()).toBe('Charlie');
  });
});

describe('Library: deleting the active collection', () => {
  it('returns focus to a live control of the collections row', async () => {
    libraryView.set('list');
    const cozy = { id: 'k1', name: 'Cozy', tags: ['cozy'], genres: [], status: '', createdAt: '', updatedAt: '' } as LibraryCollection;
    const quick = { id: 'k2', name: 'Quick', tags: ['quick'], genres: [], status: '', createdAt: '', updatedAt: '' } as LibraryCollection;
    collections.set([cozy, quick]);
    libraryChoices.set({ ...DEFAULT_CHOICES, collection: 'k1' });
    vi.mocked(deleteCollection).mockImplementationOnce(async () => {
      collections.update((list) => list.filter((one) => one.id !== 'k1'));
    });
    const { user } = renderDom(Library);
    const row = screen.getByRole('group', { name: msg('games.collectionsLabel') });

    await user.click(within(row).getByRole('button', { name: msg('games.collectionEditLabel') }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('common.delete') }));
    const confirm = await screen.findByRole('alertdialog').catch(() => screen.getAllByRole('dialog').at(-1) as HTMLElement);
    await user.click(within(confirm).getByRole('button', { name: msg('common.delete') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await settle();

    expect(deleteCollection).toHaveBeenCalledWith('k1');
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement?.isConnected).toBe(true);
    expect(row.contains(document.activeElement)).toBe(true);
  });
});
