import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Theme } from '../../../bindings/typhon/internal/theme';
import { fromHex, gameToneOf, toneVars, type RGB } from '../art/tone';
import type { PixelRead } from '../backdrop/pixels';
import { msg } from '../i18n';
import { loadArt } from '../stores/metadata';
import { applyTheme, clearTheme } from '../theme/apply';
import GameCard from './GameCard.svelte';

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
const picture = (color: RGB): PixelRead => ({ ok: true, data: Uint8ClampedArray.from(Array.from({ length: 16 }, () => [...color, 255]).flat()), width: 4, height: 4 });

const settle = async () => {
  for (let i = 0; i < 5; i++) await Promise.resolve();
  await tick();
};

const vars = (node: HTMLElement) => Object.fromEntries([...node.style].filter((name) => name.startsWith('--')).map((name) => [name, node.style.getPropertyValue(name)]));

let round = 0;

/** `count` installed games the art store has answered for, each with a readable cover, mounted as cards. */
async function cards(count: number) {
  round += 1;
  const ids = Array.from({ length: count }, (_, i) => `r${round}-g${i}`);
  backend.art = Object.fromEntries(ids.map((id) => [id, { cover: `/media/${id}.jpg`, hero: `/media/${id}-hero.jpg`, heroIsPoster: false }]));
  for (const id of ids) canvas.pictures[`/media/${id}.jpg`] = picture(RED);
  await loadArt(ids);
  for (const id of ids) render(GameCard, { props: { id, title: `Game ${id}`, cover: `/media/${id}.jpg`, installed: true, onplay: vi.fn() } });
  await settle();
  return ids;
}

const cardOf = (title: string) => screen.getByRole('button', { name: title }).closest('.card') as HTMLElement;
const playOf = (title: string) => cardOf(title).querySelector('button.play') as HTMLElement;

beforeEach(() => {
  canvas.read.mockClear();
  applyTheme({ id: 't', name: 't', base: 'dark', tokens: DARK, builtIn: true, updatedAt: '' } satisfies Theme);
});

afterEach(() => {
  clearTheme();
  document.body.replaceChildren();
});

describe("GameCard: the round Play in the game's colour", () => {
  it('reads no picture for a grid of cards until one is hovered, and then that one only', async () => {
    const ids = await cards(50);
    expect(canvas.read).not.toHaveBeenCalled();
    expect(document.querySelectorAll('button.play.game-tone')).toHaveLength(0);

    await fireEvent.pointerEnter(cardOf(`Game ${ids[7]}`));
    await settle();

    expect(canvas.read).toHaveBeenCalledTimes(1);
    expect(canvas.read.mock.calls[0][0]).toBe(`/media/${ids[7]}.jpg`);
    const play = playOf(`Game ${ids[7]}`);
    expect(play.classList.contains('game-tone')).toBe(true);
    expect(vars(play)).toEqual(toneVars(gameToneOf(RED, { bg: DARK['--bg'], text: DARK['--text'] })));
    expect(document.querySelectorAll('button.play.game-tone')).toHaveLength(1);
  });

  it('reads its colour when focus enters the card from the keyboard', async () => {
    const ids = await cards(3);
    screen.getByRole('button', { name: `Game ${ids[1]}` }).focus();
    await settle();
    expect(canvas.read).toHaveBeenCalledTimes(1);
    expect(playOf(`Game ${ids[1]}`).classList.contains('game-tone')).toBe(true);
  });

  it('keeps the colour on Play as it turns to Stop and back', async () => {
    const [id] = await cards(1);
    const view = render(GameCard, { props: { id, title: 'Twin', cover: `/media/${id}.jpg`, installed: true, onplay: vi.fn() } });
    await fireEvent.pointerEnter(cardOf('Twin'));
    await settle();
    expect(playOf('Twin').classList.contains('game-tone')).toBe(true);

    await view.rerender({ id, title: 'Twin', cover: `/media/${id}.jpg`, installed: true, running: true, onplay: vi.fn() });
    await settle();
    const stop = screen.getAllByRole('button', { name: msg('ui.stop') })[0];
    expect(stop.classList.contains('game-tone')).toBe(true);
    expect(stop.classList.contains('running')).toBe(true);
    expect(canvas.read).toHaveBeenCalledTimes(1);
  });
});
