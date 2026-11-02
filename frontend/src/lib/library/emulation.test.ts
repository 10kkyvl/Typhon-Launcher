import { describe, expect, it } from 'vitest';
import {
  EMULATOR_PROMPT_CODES,
  consoleForImage,
  consolesForImage,
  emulationPlatform,
  imageExtension,
  imageExtensions,
  isEmulated,
  needsEmulator,
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
