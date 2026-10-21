import { describe, expect, it } from 'vitest';
import {
  destinationInRoot,
  gamesInFolder,
  gamesOnVolume,
  insidePath,
  rootOfDestination,
  sameVolume,
  volumeKey,
} from './folders';

const game = (installDir: string) => ({ installDir });

describe('gamesInFolder', () => {
  const cases: [string, string, string[], number][] = [
    ['empty list', 'D:\\Games', [], 0],
    ['empty folder path', '', ['D:\\Games\\A'], 0],
    ['direct child', 'D:\\Games', ['D:\\Games\\A', 'D:\\Games\\B'], 2],
    ['case is ignored on a drive path', 'd:\\games', ['D:\\GAMES\\A', 'D:\\Games\\B'], 2],
    ['nested deeper', 'D:\\Games', ['D:\\Games\\Publisher\\A'], 1],
    ['sibling with the same prefix', 'D:\\Games', ['D:\\Games2\\A'], 0],
    ['other volume', 'D:\\Games', ['E:\\Games\\A'], 0],
    ['trailing separator on the folder', 'D:\\Games\\', ['D:\\Games\\A'], 1],
    ['forward slashes', 'D:/Games', ['D:\\Games\\A'], 1],
    ['game installed straight into the folder', 'D:\\Games', ['D:\\Games'], 1],
    ['game without a directory', 'D:\\Games', [''], 0],
    ['posix path is case sensitive', '/home/u/Games', ['/home/u/games/A', '/home/u/Games/A'], 1],
    ['unc share', '\\\\NAS\\Games', ['\\\\nas\\games\\A'], 1],
  ];
  it.each(cases)('%s', (_name, folder, dirs, want) => {
    expect(gamesInFolder(folder, dirs.map(game))).toBe(want);
  });
});

describe('insidePath', () => {
  it('does not match an empty path', () => {
    expect(insidePath('', 'D:\\Games')).toBe(false);
    expect(insidePath('D:\\Games', '')).toBe(false);
  });
  it('treats a drive root as containing its folders', () => {
    expect(insidePath('D:\\', 'D:\\Games\\A')).toBe(true);
    expect(insidePath('/', '/home/u')).toBe(true);
  });
});

describe('volumeKey', () => {
  const cases: [string, string, string][] = [
    ['drive letter lower-cased', 'D:\\Games\\A', 'd:'],
    ['drive root', 'c:\\', 'c:'],
    ['forward slashes', 'C:/Games', 'c:'],
    ['unc share', '\\\\NAS\\Games\\A', '//nas/games'],
    ['mounted volume', '/Volumes/Big/Games', '/volumes/big'],
    ['posix root', '/home/u/Games', '/'],
    ['empty', '', ''],
    ['relative', 'Games\\A', ''],
  ];
  it.each(cases)('%s', (_name, path, want) => {
    expect(volumeKey(path)).toBe(want);
  });
});

describe('sameVolume', () => {
  it('compares drive letters regardless of case', () => {
    expect(sameVolume('C:\\Typhon', 'c:\\Games\\A')).toBe(true);
    expect(sameVolume('C:\\Typhon', 'D:\\Games\\A')).toBe(false);
  });
  it('never matches an unknown volume', () => {
    expect(sameVolume('', '')).toBe(false);
    expect(sameVolume('C:\\Typhon', '')).toBe(false);
  });
});

describe('gamesOnVolume', () => {
  it('keeps only games on the volume of the library', () => {
    const games = [game('C:\\Typhon\\Games\\A'), game('D:\\Games\\B'), game('c:\\Other\\C')];
    expect(gamesOnVolume('C:\\Typhon', games)).toEqual([games[0], games[2]]);
  });
  it('returns nothing for an empty list', () => {
    expect(gamesOnVolume('C:\\Typhon', [])).toEqual([]);
  });
  it('returns nothing when the library volume is unknown', () => {
    expect(gamesOnVolume('', [game('C:\\A')])).toEqual([]);
  });
});

describe('rootOfDestination', () => {
  const roots = ['C:\\Typhon\\Games', 'D:\\Library', 'D:\\Library\\Deep'];
  const cases: [string, string, string][] = [
    ['inside the main folder', 'C:\\Typhon\\Games\\Half-Life', 'C:\\Typhon\\Games'],
    ['inside an extra folder, other case', 'd:\\library\\Half-Life', 'D:\\Library'],
    ['the longest root wins', 'D:\\Library\\Deep\\Half-Life', 'D:\\Library\\Deep'],
    ['a custom folder', 'E:\\Anywhere\\Half-Life', ''],
    ['the root itself is not a game folder', 'D:\\Library', ''],
    ['empty destination', '', ''],
    ['sibling with the same prefix', 'D:\\Library2\\Half-Life', ''],
  ];
  it.each(cases)('%s', (_name, destination, want) => {
    expect(rootOfDestination(roots, destination)).toBe(want);
  });
  it('has nothing to match with no roots', () => {
    expect(rootOfDestination([], 'C:\\Games\\A')).toBe('');
  });
});

describe('destinationInRoot', () => {
  const cases: [string, string, string, string][] = [
    ['keeps the folder name and its case', 'D:\\Library', 'C:\\Typhon\\Games\\Half-Life', 'D:\\Library\\Half-Life'],
    ['trailing separators', 'D:\\Library\\', 'C:\\Typhon\\Games\\Half-Life\\', 'D:\\Library\\Half-Life'],
    ['drive root', 'D:\\', 'C:\\Games\\Half-Life', 'D:\\Half-Life'],
    ['posix', '/mnt/games', '/home/u/Games/Half-Life', '/mnt/games/Half-Life'],
    ['empty destination keeps the root', 'D:\\Library', '', 'D:\\Library'],
  ];
  it.each(cases)('%s', (_name, root, destination, want) => {
    expect(destinationInRoot(root, destination)).toBe(want);
  });
});
