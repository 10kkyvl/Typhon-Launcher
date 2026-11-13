import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { getMetadataView, type MetadataView } from '../services/metadata';
import { getCatalogGame, type CatalogGame } from '../services/sources';
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
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  document.body.append(app);
  vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
  vi.mocked(getMetadataView).mockResolvedValue({
    game: { id: 'c1', title: 'Hades' } as CatalogGame,
    cover: '',
    hero: '',
    heroIsPoster: false,
    screenshots: [],
    resolved: true,
    stale: false,
    provider: 'igdb',
    match: 'idle',
    storeLinks: { steam: 'https://store.steampowered.com/app/1145360' },
  } as MetadataView);
});

afterEach(() => {
  app.remove();
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getMetadataView).mockReset();
});

describe('Big Picture game page: the store links', () => {
  // The D-pad reaches a control only by its Big Picture mark, so the page has to hand StoreLinks its prefix.
  it('marks each store chip for Big Picture navigation with the page prefix and the store', async () => {
    render(GamePage, {
      target: app,
      props: { id: 'c1', onback: vi.fn(), ondownloads: vi.fn(), onlaunch: vi.fn(async () => false), requestText: vi.fn(async () => null) },
    });

    const steam = await screen.findByRole('link', { name: msg('games.storeSteam') });

    expect(steam.getAttribute('data-bp-focus')).toBe('game:store:steam');
  });
});
