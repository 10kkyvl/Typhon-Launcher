import { afterEach, describe, expect, it, vi } from 'vitest';
import { locale, msg, type MessageKey } from '../i18n';
import { errEmulators } from '../i18n/catalog/ru/errEmulators';
import { errLibrary } from '../i18n/catalog/ru/errLibrary';
import { actionErrorText, isClauseCode, lateErrorText } from './errors';

// Clause words the catalog does not hold yet: a lower-case reason with a sentence ending inside, and
// one that only ends with a period.
const SHAPES: Record<string, string> = {
  'emulators.shape_inner_period': 'путь не задан. Выберите файл ещё раз',
  'emulators.shape_trailing_period': 'путь не задан.',
};
vi.mock('../i18n', async (importOriginal) => {
  const real = await importOriginal<typeof import('../i18n')>();
  return {
    ...real,
    hasMessage: (key: string): key is MessageKey => key in SHAPES || real.hasMessage(key),
    msg: (key: MessageKey, params?: Parameters<typeof real.msg>[1]) => SHAPES[key] ?? real.msg(key, params),
  };
});

afterEach(() => locale.set('ru'));

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

  it('says a reason that opens with a capital as its own sentence after the action', () => {
    locale.set('en');
    expect(actionErrorText(failure('emulators.not_exe'), msg('emul.saveFailed'))).toBe('Could not save the emulator path. An emulator executable (.exe) is required');
    locale.set('ru');
    expect(actionErrorText(failure('library.crossover_unavailable'), msg('emul.saveFailed'))).toBe(
      'Не удалось сохранить путь к эмулятору. CrossOver не установлен или недоступен',
    );
  });

  it('says a lower-case reason with a sentence ending inside as its own sentence, one with only a final period after a colon', () => {
    expect(actionErrorText(failure('emulators.shape_inner_period'), msg('emul.saveFailed'))).toBe(
      'Не удалось сохранить путь к эмулятору. Путь не задан. Выберите файл ещё раз',
    );
    expect(actionErrorText(failure('emulators.shape_trailing_period'), msg('emul.saveFailed'))).toBe('Не удалось сохранить путь к эмулятору: путь не задан.');
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

describe('lateErrorText', () => {
  const failure = (code: string) => new Error(`typhon:${code}: detail`);

  it('says a sentence code after the action, which names the target', () => {
    locale.set('en');
    expect(lateErrorText(failure('emulators.bios_dir_unknown'), msg('emul.biosOpenFailedFor', { emulator: 'PCSX2' }))).toBe(
      'Could not open the BIOS folder of PCSX2. The BIOS folder of this emulator could not be determined',
    );
    locale.set('ru');
    expect(lateErrorText(failure('emulators.already_running'), msg('emul.saveFailedFor', { emulator: 'PCSX2' }))).toBe(
      'Не удалось указать эмулятор PCSX2. Эмулятор уже открыт: закройте его, чтобы запустить игру',
    );
  });

  it("says another domain's sentence after the action too", () => {
    expect(lateErrorText(failure('wrappers.running'), msg('emul.addFailedFor', { title: 'Okami' }))).toBe(
      msg('common.failedReasonSentence', { action: msg('emul.addFailedFor', { title: 'Okami' }), reason: msg('wrappers.running') }),
    );
  });

  it('says a clause code as actionErrorText does', () => {
    const action = msg('emul.saveFailedFor', { emulator: 'PCSX2' });
    expect(lateErrorText(failure('emulators.not_exe'), action)).toBe('Не удалось указать эмулятор PCSX2: нужен исполняемый файл эмулятора (.exe)');
    expect(lateErrorText(failure('emulators.shape_inner_period'), action)).toBe('Не удалось указать эмулятор PCSX2. Путь не задан. Выберите файл ещё раз');
  });

  it('says the action alone for a reason that says the same, a code with no words and an error with no code', () => {
    expect(lateErrorText(failure('emulators.save_failed'), msg('emul.saveFailed'))).toBe(msg('emul.saveFailed'));
    expect(lateErrorText(failure('emulators.not_found'), msg('emul.saveFailedFor', { emulator: 'PCSX2' }))).toBe(msg('emul.saveFailedFor', { emulator: 'PCSX2' }));
    expect(lateErrorText(new Error('disk'), msg('emul.saveFailedFor', { emulator: 'PCSX2' }))).toBe(msg('emul.saveFailedFor', { emulator: 'PCSX2' }));
  });
});
