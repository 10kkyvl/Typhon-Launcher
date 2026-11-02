import { describe, expect, it } from 'vitest';
import {
  EMULATOR_PROMPT_CODES,
  consoleForImage,
  consolesForImage,
  emulationPlatform,
  emulatorMissing,
  imageExtension,
  imageExtensions,
  isEmulated,
  isInstalledImage,
  needsEmulator,
  pageGame,
  platformBadge,
  type ConsoleInfo,
} from './emulation';
import { lastProfileId, playLabel, profileChoices } from '../game/profiles';

const consoles: ConsoleInfo[] = [
  { id: 'ps1', name: 'PlayStation', extensions: ['.cue', '.chd', '.pbp', '.iso', '.bin', '.m3u'] },
  { id: 'ps2', name: 'PlayStation 2', extensions: ['.iso', '.chd', '.cso', '.zso', '.bin', '.cue'] },
  { id: 'psp', name: 'PlayStation Portable', extensions: ['.iso', '.cso', '.chd', '.pbp'] },
];

describe('emulated games', () => {
  it('recognizes a game by its emulation marker', () => {
    expect(isEmulated({ emulation: { platform: 'ps2' } })).toBe(true);
    expect(isEmulated({ emulation: null })).toBe(false);
    expect(isEmulated({})).toBe(false);
    expect(isEmulated(undefined)).toBe(false);
    expect(emulationPlatform({ emulation: { platform: 'psp' } })).toBe('psp');
    expect(emulationPlatform(null)).toBe('');
    expect(platformBadge('ps1')).toBe('PS1');
  });

  it('keeps launch profiles out of an emulated game', () => {
    const profiles = [{ id: 'a', name: 'A', executable: 'x' }];
    const game = { profiles, lastProfile: 'a' };
    expect(profileChoices(game)).toHaveLength(2);
    expect(lastProfileId(game)).toBe('a');
    const emulated = { ...game, emulation: { platform: 'ps1' } };
    expect(profileChoices(emulated)).toEqual([]);
    expect(lastProfileId(emulated)).toBe('');
    expect(playLabel(emulated)).toBe('Играть');
  });
});

describe('emulator launch errors', () => {
  it.each(EMULATOR_PROMPT_CODES)('asks for an emulator on %s', (code) => {
    expect(needsEmulator(new Error(`typhon:${code}: detail`))).toBe(true);
  });

  it.each([
    'library.launch_failed',
    'library.executable_missing',
    'emulators.game_unsupported',
    'emulators.unknown_platform',
    'emulators.save_failed',
    '',
  ])('does not ask on %s', (code) => {
    expect(needsEmulator(new Error(code ? `typhon:${code}: detail` : 'boom'))).toBe(false);
  });
});

describe('console of an image', () => {
  it('reads the extension in lower case, folders aside', () => {
    expect(imageExtension('C:\\Roms.d\\Game.ISO')).toBe('.iso');
    expect(imageExtension('/games/game.tar.gz')).toBe('.gz');
    expect(imageExtension('C:\\Roms.d\\Game')).toBe('');
    expect(imageExtension('/games/.hidden')).toBe('');
  });

  it('picks the console itself when the extension fits only one', () => {
    expect(consoleForImage(consoles, 'D:\\roms\\Game.zso')).toBe('ps2');
    expect(consoleForImage(consoles, 'D:\\roms\\Game.m3u')).toBe('ps1');
  });

  it.each(['game.iso', 'game.cue', 'game.chd', 'game.bin', 'game.cso', 'game.pbp'])(
    'leaves %s to the user when several consoles accept it',
    (name) => {
      expect(consolesForImage(consoles, name).length).toBeGreaterThan(1);
      expect(consoleForImage(consoles, name)).toBe('');
    },
  );

  it('offers nothing for an unknown or missing extension', () => {
    expect(consolesForImage(consoles, 'game.exe')).toEqual([]);
    expect(consolesForImage(consoles, 'game')).toEqual([]);
    expect(consoleForImage([], 'game.iso')).toBe('');
  });

  it('survives a console whose extensions came as null', () => {
    expect(consolesForImage([{ id: 'x', name: 'X', extensions: null }], 'game.iso')).toEqual([]);
  });

  it('lists every extension once, sorted', () => {
    expect(imageExtensions(consoles)).toEqual(['.bin', '.chd', '.cso', '.cue', '.iso', '.m3u', '.pbp', '.zso']);
  });
});

describe('installed images', () => {
  it('recognises an image the launcher put into its own folder', () => {
    expect(isInstalledImage({ emulation: { platform: 'ps2' }, owned: true, installType: 'console_image' })).toBe(true);
  });

  it('leaves a manually added image alone', () => {
    expect(isInstalledImage({ emulation: { platform: 'ps2' }, owned: false })).toBe(false);
    expect(isInstalledImage({ emulation: { platform: 'ps2' } })).toBe(false);
  });

  it('is not triggered by a PC game', () => {
    expect(isInstalledImage({ owned: true, installType: 'console_image' })).toBe(false);
    expect(isInstalledImage(null)).toBe(false);
  });
});

describe('emulatorMissing', () => {
  it('hints for every state but ready', () => {
    expect(emulatorMissing({ status: 'not_set' })).toBe(true);
    expect(emulatorMissing({ status: 'missing' })).toBe(true);
    expect(emulatorMissing({ status: 'unavailable' })).toBe(true);
    expect(emulatorMissing({ status: 'ready' })).toBe(false);
  });
});

describe('pageGame', () => {
  const image = { id: 'img', emulation: { platform: 'ps2', catalogGameId: 'cat1' }, owned: true, installType: 'console_image' };
  const pc = { id: 'pc', canonicalGameId: 'cat1', emulation: null };

  it('shows the installed image on the catalog page of its game', () => {
    expect(pageGame(undefined, [image], ['cat1'])).toEqual({ game: image, viaCatalog: true });
  });

  it('finds the image through an alias of the catalog game', () => {
    expect(pageGame(undefined, [image], ['other', 'cat1']).game).toBe(image);
  });

  it('prefers the installed PC copy over the image', () => {
    expect(pageGame(pc, [pc, image], ['cat1'])).toEqual({ game: pc, viaCatalog: false });
  });

  it('ignores an image that was uninstalled, a manual one and another game', () => {
    expect(pageGame(undefined, [{ ...image, uninstalled: true }], ['cat1']).game).toBeUndefined();
    expect(pageGame(undefined, [{ id: 'm', emulation: { platform: 'ps2' } }], ['cat1']).game).toBeUndefined();
    expect(pageGame(undefined, [image], ['cat2']).game).toBeUndefined();
    expect(pageGame(undefined, [image], ['']).game).toBeUndefined();
  });

  it('a page opened by the library id of the card is not a catalog page', () => {
    expect(pageGame(image, [image], ['img']).viaCatalog).toBe(false);
  });
});
