import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Theme } from '../../../bindings/typhon/internal/theme';
import { fromHex, gameToneOf, toneVars, type GameTone, type RGB } from '../art/tone';
import type { PixelRead } from '../backdrop/pixels';
import type { LibraryGame } from '../services/library';
import GameScopeHarness from '../testing/GameScopeHarness.svelte';
import { applyTheme, clearTheme } from '../theme/apply';
import { libraryGames } from './library';
import { gameArt, loadArt } from './metadata';
import { gameScope, gameTone } from './gameTone';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// What the backend answers for art; a game left out of the answer has none.
const backend = vi.hoisted(() => ({ art: {} as Record<string, unknown> }));
vi.mock('../services/metadata', async (original) => ({
  ...(await original<typeof import('../services/metadata')>()),
  getGameArt: async (ids: string[]) => Object.fromEntries(ids.filter((id) => id in backend.art).map((id) => [id, backend.art[id]])),
  ensureArt: async (ids: string[]) => ids,
}));

// The canvas is the boundary: each picture answers with its pixels or why they cannot be read.
const canvas = vi.hoisted(() => {
  const pictures: Record<string, PixelRead> = {};
  const state = { pictures, read: vi.fn(async (url: string, _side: number): Promise<PixelRead> => state.pictures[url] ?? { ok: false, reason: 'load' }) };
  return state;
});
vi.mock('../backdrop/pixels', async (original) => ({
  ...(await original<typeof import('../backdrop/pixels')>()),
  readPixels: (url: string, side: number) => canvas.read(url, side),
}));

// jsdom resolves no colour on a canvas: the root's own value stands for what the browser computes.
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
const LIGHT = { '--bg': '#eef1f4', '--text': '#10161d' };
const theme = (tokens: Record<string, string>): Theme => ({ id: 't', name: 't', base: 'dark', tokens, builtIn: true, updatedAt: '' });

const RED: RGB = [200, 40, 30];
const BLUE: RGB = [30, 90, 220];
const GREY: RGB = [120, 120, 120];
const picture = (color: RGB): PixelRead => ({ ok: true, data: Uint8ClampedArray.from(Array.from({ length: 16 }, () => [...color, 255]).flat()), width: 4, height: 4 });

function game(id: string, extra: Partial<LibraryGame> = {}): LibraryGame {
  return { id, title: id, executable: '', installDir: '', cover: '', version: '', sizeBytes: 0, lastPlayed: null, playtimeSeconds: 0, installedAt: '', ...extra };
}

/** Every value a subscriber is handed, in order. */
function record(id: string) {
  const seen: (GameTone | null)[] = [];
  const stop = gameTone(id).subscribe((tone) => seen.push(tone));
  return { seen, stop };
}

const settle = async () => {
  for (let i = 0; i < 5; i++) await Promise.resolve();
  await tick();
};

const readsOf = (url: string) => canvas.read.mock.calls.filter(([u]) => u === url).length;
const toneIn = (color: RGB, tokens: Record<string, string>) => gameToneOf(color, { bg: tokens['--bg'], text: tokens['--text'] });

let n = 0;
// The colours are kept for the session, so every test names its own games.
const fresh = () => `t${++n}`;

beforeEach(() => {
  canvas.pictures = {};
  canvas.read.mockClear();
  backend.art = {};
  applyTheme(theme(DARK));
});

afterEach(() => {
  libraryGames.set([]);
  clearTheme();
  document.body.replaceChildren();
});

describe("a game's colour", () => {
  it('stays unknown while the art store has not answered: no picture is read and no stand-in colour is handed out', async () => {
    const c = fresh();
    libraryGames.set([game(`lib-${c}`, { canonicalGameId: c, cover: `/media/${c}-own.jpg` })]);
    canvas.pictures[`/media/${c}.jpg`] = picture(RED);
    const { seen, stop } = record(`lib-${c}`);
    await settle();
    expect(canvas.read).not.toHaveBeenCalled();

    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: '', heroIsPoster: false } };
    await loadArt([c]);
    await settle();

    expect(seen.at(-1)).toEqual(toneIn(RED, DARK));
    expect(seen.slice(0, -1).every((tone) => tone === null)).toBe(true);
    stop();
  });

  it("is read once from the game's picture however many controls ask, the same for its catalog id and its library id", async () => {
    const c = fresh();
    libraryGames.set([game(`lib-${c}`, { canonicalGameId: c })]);
    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: `/media/${c}-hero.jpg`, heroIsPoster: false } };
    canvas.pictures[`/media/${c}.jpg`] = picture(BLUE);
    await loadArt([c]);
    const a = record(`lib-${c}`);
    const b = record(`lib-${c}`);
    const fromCatalog = record(c);
    await settle();

    expect(canvas.read).toHaveBeenCalledTimes(1);
    expect(readsOf(`/media/${c}.jpg`)).toBe(1);
    expect(a.seen.at(-1)).toEqual(toneIn(BLUE, DARK));
    expect(b.seen.at(-1)).toEqual(a.seen.at(-1));
    expect(fromCatalog.seen.at(-1)).toEqual(a.seen.at(-1));
    [a, b, fromCatalog].forEach((r) => r.stop());
  });

  it('a picture that does not load moves on to the next one', async () => {
    const c = fresh();
    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: `/media/${c}-hero.jpg`, heroIsPoster: false } };
    canvas.pictures[`/media/${c}-hero.jpg`] = picture(RED);
    await loadArt([c]);
    const { seen, stop } = record(c);
    await settle();
    expect(seen.at(-1)).toEqual(toneIn(RED, DARK));
    stop();
  });

  // The colour comes only from a picture that was read: what a screen draws without one (a generated
  // picture, letters, a cover the canvas could not fetch) is not something the store can see.
  it('has no colour for a game with no picture: never a colour, the theme accent stays', async () => {
    const none = fresh();
    await loadArt([none]);
    const bare = record(none);
    await settle();
    expect(canvas.read).not.toHaveBeenCalled();
    expect(bare.seen.every((tone) => tone === null)).toBe(true);
    bare.stop();
  });

  it('has no colour when every picture of the game fails to load', async () => {
    const c = fresh();
    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: `/media/${c}-hero.jpg`, heroIsPoster: false } };
    await loadArt([c]);
    const failed = record(c);
    await settle();
    expect(canvas.read).toHaveBeenCalledTimes(2);
    expect(failed.seen.every((tone) => tone === null)).toBe(true);
    failed.stop();
  });

  it('keeps the theme accent for a picture it shows but cannot read: tainted, grey, or no canvas', async () => {
    const ids = [fresh(), fresh(), fresh()];
    const answers: PixelRead[] = [{ ok: false, reason: 'tainted' }, picture(GREY), { ok: false, reason: 'unavailable' }];
    backend.art = Object.fromEntries(ids.map((id) => [id, { cover: `/media/${id}.jpg`, hero: '', heroIsPoster: false }]));
    ids.forEach((id, i) => (canvas.pictures[`/media/${id}.jpg`] = answers[i]));
    await loadArt(ids);
    const subs = ids.map(record);
    await settle();
    expect(canvas.read).toHaveBeenCalledTimes(3);
    expect(subs.map((s) => s.seen.every((tone) => tone === null))).toEqual([true, true, true]);
    subs.forEach((s) => s.stop());
  });

  it('a theme change clamps the colour again against the new page without reading the picture again', async () => {
    const c = fresh();
    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: '', heroIsPoster: false } };
    canvas.pictures[`/media/${c}.jpg`] = picture(RED);
    await loadArt([c]);
    const { seen, stop } = record(c);
    await settle();
    expect(seen.at(-1)).toEqual(toneIn(RED, DARK));

    applyTheme(theme(LIGHT));
    await settle();

    expect(seen.at(-1)).toEqual(toneIn(RED, LIGHT));
    expect(toneIn(RED, LIGHT)).not.toEqual(toneIn(RED, DARK));
    expect(canvas.read).toHaveBeenCalledTimes(1);
    stop();
  });

  it('reads a new picture of the game again, and a failed one only when the art changes', async () => {
    const c = fresh();
    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: '', heroIsPoster: false } };
    await loadArt([c]);
    const { seen, stop } = record(c);
    await settle();
    expect(readsOf(`/media/${c}.jpg`)).toBe(1);

    // Other stores moving (the library, a theme) re-derive; they do not read again.
    libraryGames.set([game('other')]);
    applyTheme(theme(DARK));
    const again = record(c);
    await settle();
    expect(readsOf(`/media/${c}.jpg`)).toBe(1);

    canvas.pictures[`/media/${c}-new.jpg`] = picture(BLUE);
    gameArt.update((art) => ({ ...art, [c]: { cover: `/media/${c}-new.jpg`, hero: '', heroIsPoster: false } }));
    await settle();

    expect(readsOf(`/media/${c}-new.jpg`)).toBe(1);
    expect(seen.at(-1)).toEqual(toneIn(BLUE, DARK));
    stop();
    again.stop();
  });

  it('drops the answer for a picture the game no longer has, however late it comes', async () => {
    const c = fresh();
    const pending = new Map<string, (read: PixelRead) => void>();
    const held = (url: string) => new Promise<PixelRead>((resolve) => pending.set(url, resolve));
    canvas.read.mockImplementationOnce(held).mockImplementationOnce(held);
    backend.art = { [c]: { cover: `/media/${c}-a.jpg`, hero: '', heroIsPoster: false } };
    await loadArt([c]);
    const { seen, stop } = record(c);
    await settle();
    expect([...pending.keys()]).toEqual([`/media/${c}-a.jpg`]);

    gameArt.update((art) => ({ ...art, [c]: { cover: `/media/${c}-b.jpg`, hero: '', heroIsPoster: false } }));
    await settle();
    expect([...pending.keys()]).toEqual([`/media/${c}-a.jpg`, `/media/${c}-b.jpg`]);

    pending.get(`/media/${c}-b.jpg`)?.(picture(BLUE));
    await settle();
    pending.get(`/media/${c}-a.jpg`)?.(picture(RED));
    await settle();

    expect(seen.at(-1)).toEqual(toneIn(BLUE, DARK));
    expect(seen.some((tone) => tone !== null && tone.fill === toneIn(RED, DARK).fill)).toBe(false);
    stop();
  });

  it('is nothing for no game', () => {
    expect(get(gameTone(null))).toBeNull();
  });
});

describe('the scope a game paints its controls in', () => {
  async function coloured(color: RGB) {
    const c = fresh();
    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: '', heroIsPoster: false } };
    canvas.pictures[`/media/${c}.jpg`] = picture(color);
    await loadArt([c]);
    return c;
  }

  const vars = (node: HTMLElement) => Object.fromEntries([...node.style].filter((name) => name.startsWith('--')).map((name) => [name, node.style.getPropertyValue(name)]));

  it('sets the colour and the marker class once the colour is known, and nothing before', async () => {
    const c = fresh();
    render(GameScopeHarness, { props: { id: c } });
    const node = screen.getByTestId('scope');
    await settle();
    expect(node.classList.contains('game-tone')).toBe(false);
    expect(vars(node)).toEqual({});

    backend.art = { [c]: { cover: `/media/${c}.jpg`, hero: '', heroIsPoster: false } };
    canvas.pictures[`/media/${c}.jpg`] = picture(RED);
    await loadArt([c]);
    await settle();
    expect(node.classList.contains('game-tone')).toBe(true);
    expect(vars(node)).toEqual(toneVars(toneIn(RED, DARK)));
  });

  it('a change of game takes the old colour away and brings the new one', async () => {
    const red = await coloured(RED);
    const blue = await coloured(BLUE);
    const { rerender } = render(GameScopeHarness, { props: { id: red } });
    const node = screen.getByTestId('scope');
    await settle();
    expect(vars(node)).toEqual(toneVars(toneIn(RED, DARK)));

    await rerender({ id: blue });
    await settle();
    expect(vars(node)).toEqual(toneVars(toneIn(BLUE, DARK)));

    await rerender({ id: null });
    await settle();
    expect(vars(node)).toEqual({});
    expect(node.classList.contains('game-tone')).toBe(false);
  });

  it('clears the node when it is destroyed', async () => {
    const red = await coloured(RED);
    const node = document.createElement('div');
    const destroy = gameScope(red)(node);
    await settle();
    expect(node.classList.contains('game-tone')).toBe(true);
    expect(typeof destroy).toBe('function');
    (destroy as () => void)();
    expect(vars(node)).toEqual({});
    expect(node.classList.contains('game-tone')).toBe(false);
  });
});
