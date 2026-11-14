import { describe, expect, it } from 'vitest';
import { pageGame } from '../library/emulation';
import { gamePageSource as detailsSource } from '../testing/gamePageParts';
import { catalogLoadKey, pageTitle, releaseLoadKey, sameGameIds } from './pageLoad';
import { consoleOrigin, firstWebUrl } from './releases';

const image = { id: 'img', title: 'Special Edition', emulation: { platform: 'ps2', catalogGameId: 'B' }, owned: true, installType: 'console_image' };

describe('catalogLoadKey', () => {
  it('asks the catalog for a page the library does not hold', () => {
    expect(catalogLoadKey('A', [])).toBe('A');
  });

  it('does not change when the card is found through the catalog entry', () => {
    expect(catalogLoadKey('A', [])).toBe(catalogLoadKey('A', [image]));
  });

  it('asks for nothing when the library holds the page by id or by the catalog id', () => {
    expect(catalogLoadKey('img', [image])).toBe('');
    expect(catalogLoadKey('A', [{ id: 'x', canonicalGameId: 'A' }])).toBe('');
  });

  it('settles after one load when the entry leads to an image card', () => {
    const games = [image];
    let catalog: { id: string; aliasIds: string[] } | null = null;
    let last: string | undefined;
    let loads = 0;
    for (let step = 0; step < 20; step++) {
      const key = catalogLoadKey('A', games);
      if (key !== last) {
        last = key;
        if (key) {
          catalog = null;
          loads++;
          catalog = { id: 'A', aliasIds: ['B'] };
        } else {
          catalog = null;
        }
      }
      pageGame(undefined, games, ['A', ...(catalog?.aliasIds ?? [])]);
    }
    expect(loads).toBe(1);
    expect(pageGame(undefined, games, ['A', ...(catalog?.aliasIds ?? [])]).game).toBe(image);
  });

  it('the page effect keys on it and not on the card it finds', () => {
    expect(detailsSource).toContain('catalogLoadKey(id, $libraryGames)');
    expect(detailsSource).not.toMatch(/const known = Boolean\(localGame\)/);
  });
});

describe('releaseLoadKey', () => {
  const base = { id: 'A', canonicalId: 'A', sideloaded: false, external: false };

  it('stays the same when the page starts showing the installed image', () => {
    const before = releaseLoadKey({ ...base, title: pageTitle(undefined, false, 'Special Edition') });
    const after = releaseLoadKey({ ...base, title: pageTitle(image, true, 'Special Edition') });
    expect(after).toBe(before);
  });

  it('changes for a card of the library, an external game and a sideloaded image', () => {
    const keys = new Set([
      releaseLoadKey({ ...base, title: 'T' }),
      releaseLoadKey({ ...base, title: 'T', external: true }),
      releaseLoadKey({ ...base, title: 'T', sideloaded: true }),
    ]);
    expect(keys.size).toBe(3);
  });
});

describe('sameGameIds', () => {
  it('takes the editions of the same game and leaves remakes and ports out', () => {
    const rel = (id: string, relation: string) => ({ game: { id, relation } });
    expect(sameGameIds([rel('B', 'edition'), rel('C', 'remake'), rel('D', 'base'), rel('E', 'port')])).toEqual(['B', 'D']);
  });
});

describe('the cover of an image download', () => {
  it('takes the first real web address and skips the local cache', () => {
    expect(firstWebUrl('wails://cache/c.jpg', 'https://img/c.jpg', 'http://x/y.jpg')).toBe('https://img/c.jpg');
    expect(firstWebUrl(undefined, '', 'C:\covers\c.png')).toBe('');
  });

  it('sends the cover and the page game with the origin', () => {
    expect(consoleOrigin({ platform: 'ps2', title: 'T' }, 'Game', 'https://img/c.jpg', 'A')).toEqual({
      platform: 'ps2',
      title: 'Game',
      cover: 'https://img/c.jpg',
      gameId: 'A',
    });
  });

  it('wires both pages to the web address of the catalog', () => {
    expect(detailsSource).toContain('firstWebUrl(catalogGame?.coverUrl, metaView?.cover, localGame?.cover)');
  });
});
