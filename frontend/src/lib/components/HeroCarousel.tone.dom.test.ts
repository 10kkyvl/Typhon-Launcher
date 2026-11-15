import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Theme } from '../../../bindings/typhon/internal/theme';
import { fromHex, gameToneOf, toneVars, type RGB } from '../art/tone';
import type { PixelRead } from '../backdrop/pixels';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { libraryGames } from '../stores/library';
import { loadArt } from '../stores/metadata';
import { renderDom } from '../testing/dom';
import { applyTheme, clearTheme } from '../theme/apply';
import HeroCarousel, { type HeroSlide } from './HeroCarousel.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

const backend = vi.hoisted(() => ({ art: {} as Record<string, unknown> }));
vi.mock('../services/metadata', async (original) => ({
  ...(await original<typeof import('../services/metadata')>()),
  getGameArt: async (ids: string[]) => Object.fromEntries(ids.filter((id) => id in backend.art).map((id) => [id, backend.art[id]])),
  ensureArt: async (ids: string[]) => ids,
}));

const canvas = vi.hoisted(() => {
  const pictures: Record<string, PixelRead> = {};
  const state = { pictures, read: vi.fn(async (url: string, _side: number): Promise<PixelRead> => state.pictures[url] ?? { ok: false, reason: 'load' }) };
  return state;
});
vi.mock('../backdrop/pixels', async (original) => ({
  ...(await original<typeof import('../backdrop/pixels')>()),
  readPixels: (url: string, side: number) => canvas.read(url, side),
}));

vi.mock('../theme/resolveColors', () => ({
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
const BLUE: RGB = [30, 90, 220];
const picture = (color: RGB): PixelRead => ({ ok: true, data: Uint8ClampedArray.from(Array.from({ length: 16 }, () => [...color, 255]).flat()), width: 4, height: 4 });
const toneIn = (color: RGB) => toneVars(gameToneOf(color, { bg: DARK['--bg'], text: DARK['--text'] }));

const settle = async () => {
  for (let i = 0; i < 5; i++) await Promise.resolve();
  await tick();
};

const vars = (node: HTMLElement) => Object.fromEntries([...node.style].filter((name) => name.startsWith('--')).map((name) => [name, node.style.getPropertyValue(name)]));

const game = (id: string, canonicalGameId: string) => ({ id, title: id, canonicalGameId, executable: '', installDir: '', cover: '', version: '', sizeBytes: 0, lastPlayed: null, playtimeSeconds: 0, installedAt: '' }) as LibraryGame;
const slide = (id: string, patch: Partial<HeroSlide> = {}): HeroSlide => ({ id, title: `Game ${id}`, art: {}, staged: true, ...patch });
const slideOf = (title: string) => screen.getAllByRole('group', { hidden: true }).find((group) => group.textContent?.includes(title)) as HTMLElement;

beforeEach(async () => {
  canvas.read.mockClear();
  applyTheme({ id: 't', name: 't', base: 'dark', tokens: DARK, builtIn: true, updatedAt: '' } satisfies Theme);
  libraryGames.set([game('a', 'ca'), game('b', 'cb')]);
  backend.art = { ca: { cover: '/media/ca.jpg', hero: '', heroIsPoster: false }, cb: { cover: '/media/cb.jpg', hero: '', heroIsPoster: false } };
  canvas.pictures = { '/media/ca.jpg': picture(RED), '/media/cb.jpg': picture(BLUE) };
  await loadArt(['ca', 'cb']);
});

afterEach(() => {
  libraryGames.set([]);
  clearTheme();
  document.body.replaceChildren();
});

describe("HeroCarousel: each slide in its game's colour", () => {
  it("paints each slide's Play in its own game's colour, read without a hover", async () => {
    renderDom(HeroCarousel, { slides: [slide('a'), slide('b')], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn() });
    await settle();

    expect(canvas.read).toHaveBeenCalledTimes(2);
    const first = slideOf('Game a');
    const second = slideOf('Game b');
    expect(first.classList.contains('game-tone')).toBe(true);
    expect(vars(first)).toEqual(toneIn(RED));
    expect(vars(second)).toEqual(toneIn(BLUE));
    const play = within(first).getByRole('button', { name: msg('ui.play') });
    expect(play.classList.contains('play')).toBe(true);
  });

  it('Stop on a running game is the same launching button, in the same colour', async () => {
    renderDom(HeroCarousel, { slides: [slide('a', { running: true })], onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn() });
    await settle();
    const stop = within(slideOf('Game a')).getByRole('button', { name: msg('ui.stop') });
    expect(stop.classList.contains('play')).toBe(true);
    expect(stop.closest('.game-tone')).toBe(slideOf('Game a'));
  });
});
