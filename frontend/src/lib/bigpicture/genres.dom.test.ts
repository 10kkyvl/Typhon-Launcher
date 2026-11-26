import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { getMetadataView, type MetadataView } from '../services/metadata';
import { getCatalogGame, queryCatalogGames, type CatalogGame, type CatalogPage as Page } from '../services/sources';
import CatalogPage from './components/CatalogPage.svelte';
import GamePage from './components/GamePage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/metadata')>()),
  getMetadataView: vi.fn(),
  ensureMetadataFresh: vi.fn(async () => false),
}));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  getCatalogGame: vi.fn(),
  getReleasesForGame: vi.fn(async () => []),
  queryCatalogGames: vi.fn(),
}));

// The catalog names genres in English; the desktop screens say them in the player's language, and
// Big Picture must say the same words.
const action = msg('games.genreAction');

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  document.body.append(app);
});

afterEach(() => {
  app.remove();
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getMetadataView).mockReset();
  vi.mocked(queryCatalogGames).mockReset();
});

describe('Big Picture genres', () => {
  it('says a catalog genre filter in the interface language', async () => {
    vi.mocked(queryCatalogGames).mockResolvedValue({ items: [], total: 0, facets: [{ label: 'Action', count: 3 }] } as unknown as Page);
    render(CatalogPage, { target: app, props: { onback: vi.fn(), ongame: vi.fn(), requestText: vi.fn(async () => null) } });

    const chip = await screen.findByRole('button', { name: new RegExp(`^${action}`) });

    expect(action).not.toBe('Action');
    expect(chip.textContent).not.toContain('Action');
  });

  it("says a game's genres in the interface language", async () => {
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades', genres: ['Action'] } as CatalogGame);
    vi.mocked(getMetadataView).mockResolvedValue({
      game: { id: 'c1', title: 'Hades', genres: ['Action'] } as CatalogGame,
      cover: '', hero: '', heroIsPoster: false, screenshots: [], resolved: true, stale: false, provider: 'igdb', match: 'idle',
    } as unknown as MetadataView);
    render(GamePage, {
      target: app,
      props: { id: 'c1', onback: vi.fn(), ondownloads: vi.fn(), onlaunch: vi.fn(async () => false), requestText: vi.fn(async () => null) },
    });

    expect(await screen.findByText(action)).toBeTruthy();
    expect(screen.queryByText('Action')).toBeNull();
  });
});
