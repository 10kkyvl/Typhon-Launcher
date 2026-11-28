import { describe, expect, it } from 'vitest';
import { msg } from '../i18n';
import { errEmulators } from '../i18n/catalog/ru/errEmulators';
import { errLibrary } from '../i18n/catalog/ru/errLibrary';
import { actionErrorText, isClauseCode } from './errors';

// Go's codes of the two domains, with their Russian words (errLibrary also holds other domains').
const words: Record<string, string> = { ...errLibrary, ...errEmulators };
const goCodes = Object.keys(words).filter((key) => /^(library|emulators)\./.test(key));
// A proper name may open a clause: "CrossOver не установлен".
const opensLowerCase = (text: string) => /^(\p{Ll}|CrossOver )/u.test(text);

describe('isClauseCode', () => {
  it('finds the codes it sorts', () => {
    expect(goCodes.length).toBeGreaterThan(60);
    expect(goCodes).toEqual(expect.arrayContaining(['emulators.not_exe', 'emulators.already_running', 'library.game_not_found']));
  });

  it("calls a library or emulator code a clause exactly when its Russian words are one", () => {
    const wrong = goCodes.filter((code) => isClauseCode(code) !== opensLowerCase(words[code]));
    expect(wrong).toEqual([]);
  });

  it('says the emulator reasons that are whole sentences alone', () => {
    expect(goCodes.filter((code) => code.startsWith('emulators.') && !isClauseCode(code)).sort()).toEqual([
      'emulators.already_running',
      'emulators.bios_dir_unknown',
      'emulators.bios_missing',
      'emulators.pkg_installing',
      'emulators.pkg_invalid',
    ]);
  });

  it('takes no other domain for a clause', () => {
    expect(isClauseCode('wrappers.running')).toBe(false);
    expect(isClauseCode('steam.running')).toBe(false);
    expect(isClauseCode('')).toBe(false);
  });
});

describe('actionErrorText', () => {
  const failure = (code: string) => new Error(`typhon:${code}: detail`);

  it('says a clause after the action', () => {
    expect(actionErrorText(failure('emulators.not_exe'), msg('emul.saveFailed'))).toBe(
      msg('common.failedReason', { action: msg('emul.saveFailed'), reason: msg('emulators.not_exe') }),
    );
  });

  it('says a sentence alone', () => {
    expect(actionErrorText(failure('emulators.already_running'), msg('emul.saveFailed'))).toBe(msg('emulators.already_running'));
    expect(actionErrorText(failure('wrappers.running'), msg('emul.saveFailed'))).toBe(msg('wrappers.running'));
  });

  it('falls to the action for a code with no words and for an error with no code', () => {
    expect(actionErrorText(failure('emulators.not_found'), msg('emul.saveFailed'))).toBe(msg('emul.saveFailed'));
    expect(actionErrorText(new Error('disk'), msg('emul.saveFailed'))).toBe(msg('emul.saveFailed'));
  });
});
