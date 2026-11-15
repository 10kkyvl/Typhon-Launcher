import { describe, expect, it } from 'vitest';
import { entryCover, entryHeroArt, gameEntry } from '../library/entries';
import type { LibraryGame } from '../services/library';
import type { GameArt } from '../services/metadata';
import { backdropSources, wideArt } from './source';

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

const art: Record<string, GameArt> = {
  c1: { cover: '/media/c1-cover.jpg', hero: '/media/c1-hero.jpg', heroIsPoster: false },
  c7: { cover: '/media/c7-cover.jpg', hero: '/media/c7-hero.jpg', heroIsPoster: false },
};

// The library card, the backdrop and the stage show one game's art by one key (I-17). An emulated image
// links to its catalog game for the game page alone (internal/library/emulated.go, Emulation.CatalogGameID,
// deliberately not CanonicalGameID): its card keeps its own cover, and the backdrop and the stage follow it.
describe('the art key of a library game, as the card, the backdrop and the stage read it', () => {
  const pc = game('lib-1', { canonicalGameId: 'c1', cover: '/media/own-1.jpg' });
  const image = game('e1', { cover: '/media/e1-cover.jpg', emulation: { platform: 'ps2', catalogGameId: 'c7' } } as Partial<LibraryGame>);
  const games = [pc, image];

  it('a game with a catalog match: the card, the backdrop and the stage take its catalog art', () => {
    const entry = gameEntry(pc);
    expect(wideArt(pc.id, games, art)).toEqual({ state: 'yes', src: entryHeroArt(entry, art) });
    expect(backdropSources(pc.id, games, art)).toEqual([entryHeroArt(entry, art), entryCover(entry, art), pc.cover]);
  });

  it('an emulated image: none of them takes its catalog game art, the card and the backdrop keep its own cover', () => {
    const entry = gameEntry(image);
    expect(entry.catalogId).toBeUndefined();
    expect(entryHeroArt(entry, art)).toBe('');
    expect(wideArt(image.id, games, art)).toEqual({ state: 'no' });
    expect(backdropSources(image.id, games, art)).toEqual([entryCover(entry, art)]);
    expect(entryCover(entry, art)).toBe('/media/e1-cover.jpg');
  });
});
