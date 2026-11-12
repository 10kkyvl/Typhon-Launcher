import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { setNotInterested } from '../../lib/services/recommendations';
import { queryCatalogGames, type CatalogGame } from '../../lib/services/sources';
import { libraryGames } from '../../lib/stores/library';
import { resetHistory } from '../../lib/stores/router';
import { catalogView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Catalog from './Catalog.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/recommendations', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/recommendations')>()),
  setNotInterested: vi.fn(),
}));
vi.mock('../../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/sources')>()),
  queryCatalogGames: vi.fn(),
}));

const page = (items: Partial<CatalogGame>[], total = items.length) => ({
  items: items as CatalogGame[],
  total,
  page: 1,
  pageSize: 60,
});

// The accessible name of the one button whose name holds `title`.
async function nameOf(title: string): Promise<string> {
  let found = '';
  await screen.findByRole('button', {
    name: (name) => {
      if (!name.includes(title)) return false;
      found = name;
      return true;
    },
  });
  return found;
}

const times = (text: string, word: string) => text.split(word).length - 1;

beforeEach(() => {
  // The page keeps its state for the way back; each test starts on a fresh history.
  resetHistory();
  vi.mocked(queryCatalogGames).mockReset();
});

afterEach(() => {
  catalogView.set('grid');
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('Catalog: loading', () => {
  it('the first page shows the base spinner named by what is loading, over the skeleton', async () => {
    vi.mocked(queryCatalogGames).mockReturnValue(new Promise(() => {}));
    renderDom(Catalog);

    expect((await screen.findByText(msg('games.catalogLoadingLabel'))).closest('.spinner[role="status"]')).not.toBeNull();
  });
});

describe('Catalog: messages about the page', () => {
  it('general picks in place of personal ones are said on the base notice, as a status', async () => {
    vi.mocked(queryCatalogGames).mockResolvedValue({ ...page([{ id: 'c-celeste', title: 'Celeste' }]), personalizationFallback: true });
    renderDom(Catalog);

    const notice = (await screen.findByText(msg('games.recommendationFallback'))).closest('.notice');
    expect(notice?.getAttribute('data-tone')).toBe('info');
    expect(notice?.querySelector('[role="status"]')?.textContent).toContain(msg('games.recommendationFallback'));
  });

  it('a game put away is said on the base notice, with its undo as the action', async () => {
    catalogView.set('list');
    vi.mocked(queryCatalogGames).mockResolvedValue(page([{ id: 'c-celeste', title: 'Celeste' }]));
    vi.mocked(setNotInterested).mockResolvedValue(undefined);
    const { user } = renderDom(Catalog);
    await user.click(await screen.findByRole('button', { name: msg('games.recommendationNotInterested') }));

    const text = await screen.findByText(msg('games.recommendationDismissed', { title: 'Celeste' }));
    const notice = text.closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('success');
    const undo = within(notice).getByRole('button', { name: msg('games.recommendationUndo') });
    await vi.waitFor(() => expect(undo.hasAttribute('disabled')).toBe(false));
    undo.focus();
    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(setNotInterested).toHaveBeenLastCalledWith('c-celeste', false));
  });
});

type Answer = Awaited<ReturnType<typeof queryCatalogGames>>;

function deferred() {
  let resolve!: (value: Answer) => void;
  const promise = new Promise<Answer>((res) => (resolve = res));
  return { promise, resolve };
}

// Presses the Retry inside `notice` from the keyboard and checks it holds the notice and the focus.
async function retryHolds(notice: HTMLElement, keys: ReturnType<typeof renderDom>['user']) {
  const retry = within(notice).getByRole('button', { name: msg('games.catalogRetry') });
  retry.focus();
  await keys.keyboard('{Enter}');
  expect(retry.getAttribute('aria-busy')).toBe('true');
  expect(notice.isConnected).toBe(true);
  expect(document.activeElement).toBe(retry);
}

describe('Catalog: retrying', () => {
  it('a failed first page: the danger notice keeps its busy Retry and the focus, then focus goes to the first game', async () => {
    const again = deferred();
    catalogView.set('list');
    vi.mocked(queryCatalogGames).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(Catalog);
    const notice = (await screen.findByText(msg('games.catalogUnavailableTitle'))).closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');

    await retryHolds(notice, user);
    expect(screen.queryByText(msg('games.catalogLoadingLabel'))).toBeNull();

    again.resolve(page([{ id: 'c-celeste', title: 'Celeste' }]));
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement?.closest('.list-entry')?.textContent).toContain('Celeste'));
  });

  it('an offline page: the warning notice keeps its busy Retry and the focus until the answer', async () => {
    const again = deferred();
    vi.mocked(queryCatalogGames).mockResolvedValueOnce({ ...page([{ id: 'c-celeste', title: 'Celeste' }]), offline: true }).mockReturnValueOnce(again.promise);
    const { user } = renderDom(Catalog);
    const notice = (await screen.findByText(msg('games.catalogOffline'))).closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('warning');

    await retryHolds(notice, user);

    again.resolve(page([{ id: 'c-celeste', title: 'Celeste' }]));
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
  });

  it('an offline page whose Retry fails: the same notice says the catalog is unavailable and keeps the focus', async () => {
    let fail!: (err: unknown) => void;
    vi.mocked(queryCatalogGames)
      .mockResolvedValueOnce({ ...page([{ id: 'c-celeste', title: 'Celeste' }]), offline: true })
      .mockReturnValueOnce(new Promise<Answer>((_resolve, reject) => (fail = reject)));
    const { user } = renderDom(Catalog);
    const notice = (await screen.findByText(msg('games.catalogOffline'))).closest('.notice') as HTMLElement;
    await retryHolds(notice, user);
    const retry = document.activeElement;

    fail(new Error('offline'));

    await vi.waitFor(() => expect(notice.getAttribute('data-tone')).toBe('danger'));
    expect(notice.isConnected).toBe(true);
    expect(notice.textContent).toContain(msg('games.catalogUnavailableTitle'));
    expect(document.activeElement).toBe(retry);
    await vi.waitFor(() => expect(retry?.hasAttribute('aria-busy')).toBe(false));
  });
});

describe('Catalog: a game already installed', () => {
  afterEach(() => libraryGames.set([]));

  it('says so with the base status badge in its row', async () => {
    catalogView.set('list');
    libraryGames.set([
      { id: 'g-celeste', title: 'Celeste', canonicalGameId: 'c-celeste', installDir: 'C:\\Games\\c', executable: 'c.exe', sizeBytes: 0, uninstalled: false, tags: [] } as unknown as LibraryGame,
    ]);
    vi.mocked(queryCatalogGames).mockResolvedValue(page([{ id: 'c-celeste', title: 'Celeste' }]));
    renderDom(Catalog);

    const mark = await screen.findByText(msg('games.gameInstalledWord'));
    expect(mark.closest('.badge.success')).not.toBeNull();
  });
});

describe('Catalog: a row of the list view', () => {
  it('is named by its title once: the cover says nothing of its own', async () => {
    catalogView.set('list');
    vi.mocked(queryCatalogGames).mockResolvedValue(page([{ id: 'c-celeste', title: 'Celeste', releaseYear: 2018 }]));
    renderDom(Catalog);

    const name = await nameOf('Celeste');
    expect(name.startsWith('Celeste')).toBe(true);
    expect(times(name, 'Celeste')).toBe(1);
  });
});
