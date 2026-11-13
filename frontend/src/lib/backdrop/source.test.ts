import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import type { GameArt } from '../services/metadata';
import { artRequest, backdropSources, pageAmbient, playedArtRequests, rememberedGame } from './source';

function game(id: string, extra: Partial<LibraryGame> = {}): LibraryGame {
  return {
    id,
    title: id,
    executable: '',
    installDir: '',
    cover: '',
    version: '',
    sizeBytes: 0,
    lastPlayed: null,
    playtimeSeconds: 0,
    installedAt: '',
    ...extra,
  };
}

const art = (cover: string, hero: string): GameArt => ({ cover, hero, heroIsPoster: false });

describe('the art behind a game', () => {
  it('tries the hero, then the cover of the canonical game, then the library cover', () => {
    const games = [game('lib-1', { canonicalGameId: 'c1', cover: '/media/own.jpg' })];
    expect(backdropSources('lib-1', games, { c1: art('/media/cover.jpg', '/media/hero.jpg') })).toEqual([
      '/media/hero.jpg',
      '/media/cover.jpg',
      '/media/own.jpg',
    ]);
    expect(backdropSources('lib-1', games, { c1: art('/media/cover.jpg', '') })).toEqual(['/media/cover.jpg', '/media/own.jpg']);
    expect(backdropSources('lib-1', games, { c1: art('', '') })).toEqual(['/media/own.jpg']);
    expect(backdropSources('lib-1', games, {})).toEqual(['/media/own.jpg']);
  });

  it('lists a picture once even when two sources name it', () => {
    const games = [game('lib-1', { canonicalGameId: 'c1', cover: '/media/same.jpg' })];
    expect(backdropSources('lib-1', games, { c1: art('/media/same.jpg', '/media/same.jpg') })).toEqual(['/media/same.jpg']);
  });

  it('maps a library id through its canonical id, never through another game that shares the art key', () => {
    const games = [game('lib-1', { canonicalGameId: 'c1' }), game('c2')];
    const map = { c1: art('', '/media/one.jpg'), 'lib-1': art('', '/media/wrong.jpg'), c2: art('', '/media/two.jpg') };
    expect(backdropSources('lib-1', games, map)).toEqual(['/media/one.jpg']);
    expect(backdropSources('c2', games, map)).toEqual([]);
  });

  it('takes the art store directly for a game outside the library', () => {
    expect(backdropSources('catalog-7', [], { 'catalog-7': art('/media/c.jpg', '/media/h.jpg') })).toEqual(['/media/h.jpg', '/media/c.jpg']);
    expect(backdropSources('catalog-7', [], {})).toEqual([]);
  });

  it('treats blank placeholders as no art', () => {
    const games = [game('lib-1', { canonicalGameId: 'c1', cover: ' ' })];
    expect(backdropSources('lib-1', games, { c1: art('—', 'null') })).toEqual([]);
  });
});

describe('asking for missing art', () => {
  it('asks for the canonical game of a library game the store does not know yet, once it knows it does not', () => {
    const games = [game('lib-1', { canonicalGameId: 'c1' }), game('lib-2')];
    expect(artRequest('lib-1', games, {})).toBe('c1');
    expect(artRequest('lib-1', games, { c1: art('', '') })).toBeNull();
    expect(artRequest('lib-2', games, {})).toBeNull();
    expect(artRequest('catalog-7', games, {})).toBe('catalog-7');
  });

  it('asks only for the few most recently played games', () => {
    const games = Array.from({ length: 12 }, (_, i) =>
      game(`lib-${i}`, { canonicalGameId: `c${i}`, lastPlayed: new Date(Date.UTC(2026, 0, i + 1)).toISOString() }),
    );
    games.push(game('never', { canonicalGameId: 'cn' }));
    expect(playedArtRequests(games, { c11: art('', '/media/h.jpg') })).toEqual(['c10', 'c9', 'c8', 'c7']);
  });
});

describe('the remembered game', () => {
  const at = (day: number) => new Date(Date.UTC(2026, 0, day)).toISOString();

  it('is the most recently played game that has art', () => {
    const games = [
      game('old', { lastPlayed: at(1), cover: '/media/old.jpg' }),
      game('newest-bare', { lastPlayed: at(9), canonicalGameId: 'cb' }),
      game('recent', { lastPlayed: at(5), canonicalGameId: 'cr' }),
    ];
    expect(rememberedGame(games, { cr: art('', '/media/r.jpg') })).toBe('recent');
    expect(rememberedGame(games, {})).toBe('old');
  });

  it('is none before any game is played, and never a hidden one', () => {
    expect(rememberedGame([game('a', { cover: '/media/a.jpg' })], {})).toBeNull();
    expect(rememberedGame([game('h', { cover: '/media/h.jpg', lastPlayed: at(3), hidden: true })], {})).toBeNull();
    expect(rememberedGame([game('bad', { cover: '/media/b.jpg', lastPlayed: 'not a date' })], {})).toBeNull();
  });
});

describe('the game a game page shows behind the window', () => {
  it('is the library game by its own id', () => {
    expect(pageAmbient('lib-1', 'c1', true)).toBe('lib-1');
  });

  it('is a catalog game by its canonical id, before the art store knows it', () => {
    expect(pageAmbient(undefined, 'catalog-7', true)).toBe('catalog-7');
    expect(artRequest('catalog-7', [], {})).toBe('catalog-7');
    expect(backdropSources('catalog-7', [], {})).toEqual([]);
  });

  it('is nothing for a page without a game', () => {
    expect(pageAmbient(undefined, undefined, true)).toBeNull();
    expect(pageAmbient(undefined, '', true)).toBeNull();
  });

  it('is nothing for a game the page could not find or load: its id is not a game to ask art for', () => {
    expect(pageAmbient(undefined, 'route-id', false)).toBeNull();
    expect(pageAmbient('lib-1', 'c1', false)).toBeNull();
  });
});
