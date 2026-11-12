import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { queryCatalogGames, type CatalogGame } from '../../lib/services/sources';
import { catalogView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Catalog from './Catalog.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
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
