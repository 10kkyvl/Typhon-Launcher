import { screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { getMetadataView, type GameArt, type MetadataView } from '../../lib/services/metadata';
import { getCatalogGame, getRelatedReleases, getReleasesForGame, type CatalogGame } from '../../lib/services/sources';
import { libraryGames } from '../../lib/stores/library';
import { gameArt } from '../../lib/stores/metadata';
import { resetHistory } from '../../lib/stores/router';
import { stageGame, stagePictureFailed } from '../../lib/stores/stage';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import GameDetails from './GameDetails.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/metadata')>()),
  getMetadataView: vi.fn(),
  ensureMetadataFresh: vi.fn(async () => false),
}));
vi.mock('../../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/sources')>()),
  getCatalogGame: vi.fn(),
  getReleasesForGame: vi.fn(),
  getRelatedReleases: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function view(patch: Partial<MetadataView>): MetadataView {
  return {
    game: { id: 'c1', title: 'Hades' },
    cover: '/media/c1-cover.jpg',
    hero: '',
    heroIsPoster: false,
    screenshots: [],
    resolved: true,
    stale: false,
    provider: 'igdb',
    match: 'idle',
    ...patch,
  } as MetadataView;
}

const hades = {
  id: 'g1',
  title: 'Hades',
  canonicalGameId: 'c1',
  installDir: '/games/hades',
  executable: 'Hades.exe',
  installedAt: '2026-01-01T00:00:00Z',
  uninstalled: false,
  playtimeSeconds: 0,
  sizeBytes: 0,
  version: '',
} as unknown as LibraryGame;

const header = () => document.querySelector('[data-part="game-hero"]') as HTMLElement;
const title = (name = 'Hades') => screen.findByRole('heading', { level: 1, name });
const words = (h1: HTMLElement) => h1.closest('.stage-copy');
// A release is published after the current task (stores/stage.ts).
const settled = () => new Promise<void>((resolve) => queueMicrotask(resolve));

function art(map: Record<string, GameArt>) {
  gameArt.set(map);
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getCatalogGame).mockResolvedValue(null);
  vi.mocked(getReleasesForGame).mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockResolvedValue([]);
  libraryGames.set([hades]);
  art({});
});

afterEach(async () => {
  document.body.replaceChildren();
  vi.mocked(getMetadataView).mockReset();
  vi.mocked(getCatalogGame).mockReset();
  vi.mocked(getReleasesForGame).mockReset().mockResolvedValue([]);
  vi.mocked(getRelatedReleases).mockReset().mockResolvedValue([]);
  libraryGames.set([]);
  gameArt.set({});
  toasts.set([]);
  resetHistory();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  await settled();
});

describe('GameDetails on the stage: the claim', () => {
  it('stands the library game on the stage while the page shows it and gives it up on leave', async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({}));
    const { unmount } = renderDom(GameDetails, { id: 'g1' });
    await title();

    expect(get(stageGame)).toBe('g1');

    unmount();
    await settled();
    expect(get(stageGame)).toBeNull();
  });

  it('stands a catalog game outside the library on the stage by its catalog id', async () => {
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c9', title: 'Celeste' } as CatalogGame);
    vi.mocked(getMetadataView).mockResolvedValue(view({ game: { id: 'c9', title: 'Celeste' } as MetadataView['game'] }));
    renderDom(GameDetails, { id: 'c9' });
    await title('Celeste');

    expect(get(stageGame)).toBe('c9');
  });

  it('claims nothing for a game it cannot find', async () => {
    libraryGames.set([]);
    vi.mocked(getMetadataView).mockResolvedValue(view({}));
    renderDom(GameDetails, { id: 'nope' });
    await screen.findByRole('heading', { level: 1, name: msg('games.detailMissingTitle') });
    await settled();

    expect(get(stageGame)).toBeNull();
  });

  it('claims nothing for a game that failed to load', async () => {
    libraryGames.set([]);
    vi.mocked(getCatalogGame).mockRejectedValue(new Error('offline'));
    vi.mocked(getMetadataView).mockResolvedValue(view({}));
    renderDom(GameDetails, { id: 'c9' });
    await screen.findByRole('heading', { level: 1, name: msg('games.detailCatalogLoadError') });
    await settled();

    expect(get(stageGame)).toBeNull();
  });
});

describe('GameDetails on the stage: the header', () => {
  it('with wide art has no picture and no cover of its own: its words stand on the stage', async () => {
    art({ c1: { cover: '/media/c1-cover.jpg', hero: '/media/c1-hero.jpg', heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: '/media/c1-hero.jpg' }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    expect(header().querySelectorAll('img')).toHaveLength(0);
    expect(screen.queryByRole('img', { name: 'Hades' })).toBeNull();
    expect(words(h1)).not.toBeNull();
    expect(h1.getAttribute('tabindex')).toBe('-1');
    // The breadcrumb and the actions stand on the same plate as the title.
    expect(words(h1)?.querySelector('nav')).not.toBeNull();
    expect(words(h1)?.querySelector('[data-part="game-actions"]')).not.toBeNull();
  });

  it('knows the art from its own metadata while the art store has not answered', async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: '/media/c1-hero.jpg' }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    await vi.waitFor(() => expect(words(h1)).not.toBeNull());
    expect(header().querySelectorAll('img')).toHaveLength(0);
  });

  it('without wide art shows the cover beside the words and keeps the claim', async () => {
    art({ c1: { cover: '/media/c1-cover.jpg', hero: '', heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: '' }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    await vi.waitFor(() => expect(screen.getByRole('img', { name: 'Hades' })).toBeTruthy());
    expect(words(h1)).toBeNull();
    expect(get(stageGame)).toBe('g1');
  });

  it('does not wait on the art store for a game with no art at all', async () => {
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: '', cover: '' }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    await vi.waitFor(() => expect(header().querySelector('.cover .artwork')).not.toBeNull());
    expect(words(h1)).toBeNull();
  });

  it('takes a poster for a cover, not for wide art', async () => {
    art({ c1: { cover: '/media/c1-cover.jpg', hero: '/media/c1-cover.jpg', heroIsPoster: true } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: '/media/c1-cover.jpg', heroIsPoster: true }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    await vi.waitFor(() => expect(screen.getByRole('img', { name: 'Hades' })).toBeTruthy());
    expect(words(h1)).toBeNull();
  });

  it('waits in its skeleton while its metadata is read, then stands on the stage', async () => {
    const read = deferred<MetadataView>();
    vi.mocked(getMetadataView).mockReturnValue(read.promise);
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    expect(header().querySelector('.cover-skeleton')).not.toBeNull();
    expect(header().querySelectorAll('img')).toHaveLength(0);
    expect(words(h1)).toBeNull();

    read.resolve(view({ hero: '/media/c1-hero.jpg' }));
    await vi.waitFor(() => expect(words(h1)).not.toBeNull());
    expect(header().querySelector('.cover-skeleton')).toBeNull();
  });

  it('turns to the cover when the stage cannot load the picture, without loading it itself', async () => {
    const src = '/media/c1-hero-broken.jpg';
    art({ c1: { cover: '/media/c1-cover.jpg', hero: src, heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: src }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();
    await vi.waitFor(() => expect(words(h1)).not.toBeNull());

    stagePictureFailed(src);

    await vi.waitFor(() => expect(screen.getByRole('img', { name: 'Hades' })).toBeTruthy());
    expect(words(h1)).toBeNull();
    expect([...document.querySelectorAll('img')].map((img) => img.getAttribute('src'))).not.toContain(src);
  });

  it('shows the cover, not the skeleton, when its metadata cannot be read and the art store has not answered', async () => {
    vi.mocked(getMetadataView).mockRejectedValue(new Error('typhon:metadata.read_failed: disk'));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    await vi.waitFor(() => expect(screen.getByRole('img', { name: 'Hades' })).toBeTruthy());
    expect(header().querySelector('.cover-skeleton')).toBeNull();
    expect(words(h1)).toBeNull();
  });

  it("stands on the stage when the stage has wide art its metadata view does not name", async () => {
    art({ c1: { cover: '/media/c1-cover.jpg', hero: '/media/c1-store-hero.jpg', heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: '' }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();

    await vi.waitFor(() => expect(words(h1)).not.toBeNull());
    expect(screen.queryByRole('img', { name: 'Hades' })).toBeNull();
  });

  it('keeps the breadcrumb and the title the same nodes, focus included, when the header turns to the cover', async () => {
    const src = '/media/c1-hero-focus.jpg';
    art({ c1: { cover: '/media/c1-cover.jpg', hero: src, heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: src }));
    renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();
    await vi.waitFor(() => expect(words(h1)).not.toBeNull());
    const back = screen.getByRole('button', { name: msg('games.installedStatusWord') });
    back.focus();

    stagePictureFailed(src);

    await vi.waitFor(() => expect(screen.getByRole('img', { name: 'Hades' })).toBeTruthy());
    expect(back.isConnected).toBe(true);
    expect(h1.isConnected).toBe(true);
    expect(document.activeElement).toBe(back);
  });
});

describe('GameDetails on the stage: the screenshots', () => {
  const first = '/media/c1-shot-1.jpg';
  const shots = [first, '/media/c1-shot-2.jpg'].map((url, n) => ({ id: `s${n + 1}`, url })) as MetadataView['screenshots'];

  async function gallery(user: ReturnType<typeof renderDom>['user']): Promise<string[]> {
    await user.click(await screen.findByRole('tab', { name: msg('games.detailTabScreenshots') }));
    await vi.waitFor(() => expect(document.querySelectorAll('.shots .shot').length).toBeGreaterThan(0));
    return [...document.querySelectorAll('.shots .shot img')].map((img) => img.getAttribute('src') ?? '');
  }

  it('keeps the hero picture in the gallery when the page shows the cover', async () => {
    art({ c1: { cover: '/media/c1-cover.jpg', hero: '', heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: first, screenshots: shots }));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();
    await vi.waitFor(() => expect(screen.getByRole('img', { name: 'Hades' })).toBeTruthy());
    expect(words(h1)).toBeNull();

    expect(await gallery(user)).toEqual([first, '/media/c1-shot-2.jpg']);
  });

  it('leaves the hero picture out of the gallery while it stands on the stage', async () => {
    art({ c1: { cover: '/media/c1-cover.jpg', hero: first, heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: first, screenshots: shots }));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();
    await vi.waitFor(() => expect(words(h1)).not.toBeNull());

    expect(await gallery(user)).toEqual(['/media/c1-shot-2.jpg']);
  });

  it('leaves out the picture the stage shows when the metadata view names none', async () => {
    art({ c1: { cover: '/media/c1-cover.jpg', hero: first, heroIsPoster: false } });
    vi.mocked(getMetadataView).mockResolvedValue(view({ hero: '', screenshots: shots }));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const h1 = await title();
    await vi.waitFor(() => expect(words(h1)).not.toBeNull());

    expect(await gallery(user)).toEqual(['/media/c1-shot-2.jpg']);
  });
});

// An emulated image links to its catalog game for the game page alone (internal/library/emulated.go,
// Emulation.CatalogGameID): its library card keeps its own cover, and the backdrop and the stage key it the
// same way, so its page reads the catalog game's metadata but stands on no catalog art.
describe('GameDetails on the stage: an emulated image', () => {
  const okami = {
    ...hades,
    id: 'e1',
    title: 'Okami',
    canonicalGameId: '',
    cover: '/media/e1-cover.jpg',
    executable: '',
    owned: true,
    installType: 'console_image',
    emulation: { platform: 'ps2', catalogGameId: 'c7' },
  } as unknown as LibraryGame;

  beforeEach(() => {
    libraryGames.set([okami]);
    art({ c7: { cover: '/media/c7-cover.jpg', hero: '/media/c7-hero.jpg', heroIsPoster: false } });
    vi.mocked(getCatalogGame).mockResolvedValue({ id: 'c7', title: 'Okami' } as CatalogGame);
    vi.mocked(getMetadataView).mockResolvedValue(
      view({ game: { id: 'c7', title: 'Okami' } as MetadataView['game'], cover: '/media/c7-cover.jpg', hero: '/media/c7-hero.jpg' }),
    );
  });

  it.each([
    ['its catalog page', 'c7'],
    ['its own card', 'e1'],
  ])('opened from %s shows its cover on an empty stage claimed by the card', async (_, id) => {
    renderDom(GameDetails, { id });
    const h1 = await title('Okami');

    await vi.waitFor(() => expect(screen.getByRole('img', { name: 'Okami' })).toBeTruthy());
    expect(words(h1)).toBeNull();
    expect(get(stageGame)).toBe('e1');
  });
});
