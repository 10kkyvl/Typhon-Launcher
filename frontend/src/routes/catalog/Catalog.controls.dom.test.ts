import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { setNotInterested } from '../../lib/services/recommendations';
import { queryCatalogGames, type CatalogGame } from '../../lib/services/sources';
import { resetHistory } from '../../lib/stores/router';
import { catalogView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Catalog from './Catalog.svelte';

// A failed page asks whether it was cancelled (CancelError), which the shared runtime stub lacks.
vi.mock('@wailsio/runtime', async () => ({
  ...(await import('../../lib/testing/dom')).wailsRuntime(),
  CancelError: class CancelError extends Error {},
}));
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
  it('a failed first page: the danger notice keeps its busy Retry and the focus until the answer', async () => {
    const again = deferred();
    vi.mocked(queryCatalogGames).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(Catalog);
    const notice = (await screen.findByText(msg('games.catalogUnavailableTitle'))).closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');

    await retryHolds(notice, user);
    expect(screen.queryByText(msg('games.catalogLoadingLabel'))).toBeNull();

    again.resolve(page([{ id: 'c-celeste', title: 'Celeste' }]));
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
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

  it('a failed next page: its notice retries that page, busy and focused until the answer', async () => {
    const again = deferred();
    // The second page fails (its warm-up included) until the Retry asks for it again.
    let secondFails = true;
    vi.mocked(queryCatalogGames).mockImplementation(async (query) => {
      if ((query.page ?? 1) === 1) return page([{ id: 'c-celeste', title: 'Celeste' }], 2);
      if (secondFails) throw new Error('offline');
      return again.promise;
    });
    const { user } = renderDom(Catalog);
    await user.click(await screen.findByRole('button', { name: msg('games.catalogShowMore') }));
    const notice = (await screen.findByText(msg('games.catalogMoreFailed'))).closest('.notice') as HTMLElement;
    expect(screen.queryByRole('button', { name: msg('games.catalogShowMore') })).toBeNull();
    secondFails = false;

    await retryHolds(notice, user);

    again.resolve({ ...page([{ id: 'c-hades', title: 'Hades' }], 2), page: 2 });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(vi.mocked(queryCatalogGames).mock.calls.at(-1)?.[0]).toMatchObject({ page: 2 });
    expect(await screen.findByText('Hades')).toBeTruthy();
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
