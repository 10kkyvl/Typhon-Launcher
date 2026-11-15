import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Theme } from '../../../bindings/typhon/internal/theme';
import { fromHex, gameToneOf, toneVars, type RGB } from '../../lib/art/tone';
import type { PixelRead } from '../../lib/backdrop/pixels';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { getMetadataView, type MetadataView } from '../../lib/services/metadata';
import { getCatalogGame, getRelatedReleases, getReleasesForGame } from '../../lib/services/sources';
import { libraryGames } from '../../lib/stores/library';
import { gameArt, loadArt } from '../../lib/stores/metadata';
import { resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { applyTheme, clearTheme } from '../../lib/theme/apply';
import GameDetails from './GameDetails.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

const backend = vi.hoisted(() => ({ art: {} as Record<string, unknown> }));
vi.mock('../../lib/services/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/metadata')>()),
  getMetadataView: vi.fn(),
  ensureMetadataFresh: vi.fn(async () => false),
  getGameArt: async (ids: string[]) => Object.fromEntries(ids.filter((id) => id in backend.art).map((id) => [id, backend.art[id]])),
  ensureArt: async (ids: string[]) => ids,
}));
vi.mock('../../lib/services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/sources')>()),
  getCatalogGame: vi.fn(),
  getReleasesForGame: vi.fn(),
  getRelatedReleases: vi.fn(),
}));

const canvas = vi.hoisted(() => {
  const pictures: Record<string, PixelRead> = {};
  const state = { pictures, read: vi.fn(async (url: string, _side: number): Promise<PixelRead> => state.pictures[url] ?? { ok: false, reason: 'load' }) };
  return state;
});
vi.mock('../../lib/backdrop/pixels', async (original) => ({
  ...(await original<typeof import('../../lib/backdrop/pixels')>()),
  readPixels: (url: string, side: number) => canvas.read(url, side),
}));

vi.mock('../../lib/theme/resolveColors', () => ({
  resolveColors: (_vars: Record<string, string>, names: readonly string[]) =>
    Object.fromEntries(
      names.map((name) => {
        const value = document.documentElement.style.getPropertyValue(name).trim();
        return [name, /^#[0-9a-f]{6}$/i.test(value) ? [...fromHex(value), 1] : undefined];
      }),
    ),
}));

const DARK = { '--bg': '#0e0f11', '--text': '#edeef0' };
const RED: RGB = [200, 40, 30];
const picture = (color: RGB): PixelRead => ({ ok: true, data: Uint8ClampedArray.from(Array.from({ length: 16 }, () => [...color, 255]).flat()), width: 4, height: 4 });

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

const view = { game: { id: 'c1', title: 'Hades' }, cover: '/media/c1.jpg', hero: '', heroIsPoster: false, screenshots: ['/media/s1.jpg'], resolved: true, stale: false, provider: 'igdb', match: 'idle' } as unknown as MetadataView;

const settle = async () => {
  for (let i = 0; i < 5; i++) await Promise.resolve();
  await tick();
};

const vars = (node: Element | null) =>
  node instanceof HTMLElement ? Object.fromEntries([...node.style].filter((name) => name.startsWith('--')).map((name) => [name, node.style.getPropertyValue(name)])) : null;
const header = () => document.querySelector('[data-part="game-hero"]') as HTMLElement;
const tabsWrap = () => document.querySelector('[data-part="game-tabs"]') as HTMLElement;
const play = () => within(document.querySelector('[data-part="game-actions"]') as HTMLElement).getByRole('button', { name: msg('games.play') });

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
  vi.mocked(getMetadataView).mockResolvedValue(view);
  canvas.read.mockClear();
  canvas.pictures = { '/media/c1.jpg': picture(RED) };
  backend.art = {};
  applyTheme({ id: 't', name: 't', base: 'dark', tokens: DARK, builtIn: true, updatedAt: '' } satisfies Theme);
  libraryGames.set([hades]);
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
  clearTheme();
  resetHistory();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  await settle();
});

describe("the game page in its game's colour", () => {
  it("paints the header's Play and the tab marker in the game's colour once it is known, and only then", async () => {
    renderDom(GameDetails, { id: 'g1' });
    await screen.findByRole('heading', { level: 1, name: 'Hades' });
    await settle();
    // The art store has not answered: the theme's accent stays, nothing is read.
    expect(canvas.read).not.toHaveBeenCalled();
    expect(document.querySelector('.game-tone')).toBeNull();
    expect(play().classList.contains('play')).toBe(true);

    backend.art = { c1: { cover: '/media/c1.jpg', hero: '', heroIsPoster: false } };
    await loadArt(['c1']);
    await settle();

    const tone = toneVars(gameToneOf(RED, { bg: DARK['--bg'], text: DARK['--text'] }));
    const scope = header().closest('.game-tone');
    expect(vars(scope)).toEqual(tone);
    expect(scope?.contains(play())).toBe(true);
    expect(vars(tabsWrap())).toEqual(tone);
    expect(tabsWrap().classList.contains('game-tone')).toBe(true);
    const strip = screen.getByRole('tablist').parentElement as HTMLElement;
    expect(strip.style.getPropertyValue('--tab-marker')).toBe('var(--game-accent)');
    expect(canvas.read).toHaveBeenCalledTimes(1);
  });

  it('leaves nothing painted once the page goes', async () => {
    backend.art = { c1: { cover: '/media/c1.jpg', hero: '', heroIsPoster: false } };
    await loadArt(['c1']);
    const { unmount } = renderDom(GameDetails, { id: 'g1' });
    await screen.findByRole('heading', { level: 1, name: 'Hades' });
    await settle();
    expect(document.querySelector('.game-tone')).not.toBeNull();
    unmount();
    expect(document.querySelector('.game-tone')).toBeNull();
  });
});
