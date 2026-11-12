import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { setNotInterested } from '../../lib/services/recommendations';
import { queryCatalogGames, type CatalogGame } from '../../lib/services/sources';
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
