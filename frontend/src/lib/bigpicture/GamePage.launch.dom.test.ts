import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { getMetadataView, type MetadataView } from '../services/metadata';
import { getCatalogGame, type CatalogGame } from '../services/sources';
import { libraryGames } from '../stores/library';
import { user } from '../testing/dom';
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

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'c1', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame]);
  vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c1', title: 'Hades' } as CatalogGame);
  vi.mocked(getMetadataView).mockResolvedValue({
    game: { id: 'c1', title: 'Hades' } as CatalogGame,
    cover: '',
    hero: '',
    screenshots: [],
    resolved: true,
    stale: false,
    provider: 'igdb',
    match: 'idle',
    storeLinks: {},
  } as MetadataView);
});

afterEach(() => {
  app.remove();
  libraryGames.set([]);
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getMetadataView).mockReset();
});

function renderPage(onlaunch: (id: string) => Promise<boolean>) {
  render(GamePage, { target: app, props: { id: 'g1', onback: vi.fn(), ondownloads: vi.fn(), onlaunch, requestText: vi.fn(async () => null) } });
}

// The launch Big Picture hands the page waits while a question (the save conflict) is open.
describe('Big Picture game page: a launch that waits on a question', () => {
  it('shows no error while the launch waits, and after it was given up Play works again', async () => {
    const asked = deferred<boolean>();
    const onlaunch = vi.fn().mockReturnValueOnce(asked.promise).mockReturnValueOnce(new Promise(() => {}));
    renderPage(onlaunch);
    const keys = user();
    const play = await screen.findByRole('button', { name: msg('bp.game.launch') });

    await keys.click(play);
    expect(onlaunch).toHaveBeenCalledWith('g1');
    expect(screen.queryByRole('alert')).toBeNull();

    asked.resolve(false);
    await vi.waitFor(() => expect(play).toHaveProperty('disabled', false));
    expect(screen.queryByRole('alert')).toBeNull();

    await keys.click(play);
    expect(onlaunch).toHaveBeenCalledTimes(2);
  });

  it('shows a conflict that held the launch back again in its own words', async () => {
    renderPage(vi.fn(async () => {
      throw new Error('typhon:cloud.conflict_unresolved: diverged');
    }));
    const keys = user();

    await keys.click(await screen.findByRole('button', { name: msg('bp.game.launch') }));

    expect((await screen.findByRole('alert')).textContent?.trim()).toBe(msg('cloud.conflict_unresolved'));
  });
});
