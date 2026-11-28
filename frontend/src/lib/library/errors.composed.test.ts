import { afterEach, describe, expect, it } from 'vitest';
import { locale, msg, type MessageKey } from '../i18n';
import { errEmulators } from '../i18n/catalog/ru/errEmulators';
import { errLibrary } from '../i18n/catalog/ru/errLibrary';
import { actionErrorText, isClauseCode, lateErrorText } from './errors';

// Every caller that composes an action and a reason, and the actions it passes, read from its source, so
// a new action or a new caller is met by the tables below without being listed by hand.
const sources = import.meta.glob<string>(['../../**/*.svelte', '../../**/*.ts', '!../../**/*.test.ts', '!../../**/node_modules/**'], {
  query: '?raw',
  import: 'default',
  eager: true,
});
const COMPOSES = /\b(?:actionErrorText|lateErrorText|propertiesErrorText)\b/;
const DEFINED = ['./errors.ts', '../game/propertiesErrors.ts'];
const callers = Object.entries(sources).filter(([path, source]) => COMPOSES.test(source) && !DEFINED.includes(path));
// `errorText` is the properties dialog's name for propertiesErrorText.
const keysPassedTo = (call: RegExp) =>
  [...new Set(callers.flatMap(([, source]) => [...source.matchAll(call)].flatMap((found) => [...found[1].matchAll(/'([\w.]+)'/g)].map((key) => key[1]))))].sort();
const actions = keysPassedTo(/\b(?:actionErrorText|errorText)\(\s*[\w.]+\s*,\s*msg\(([^)]*)\)/g);
const lateActions = keysPassedTo(/\blateErrorText\(\s*[\w.]+\s*,\s*msg\(([^)]*)\)/g);

const words: Record<string, string> = { ...errLibrary, ...errEmulators };
const clauses = Object.keys(words).filter(isClauseCode);
const withWords = Object.keys(words).filter((code) => /^(library|emulators)\./.test(code));
const failure = (code: string) => new Error(`typhon:${code}: detail`);
const normal = (text: string) => text.trim().replace(/\.$/, '').toLocaleLowerCase();
const colons = (text: string) => (text.match(/:/g) ?? []).length;
// The proper names the error and action words hold, read from their catalogs: such a name may follow a
// colon with its capital.
const PROPER = ['BIOS', 'CrossOver', 'Discord', 'IGDB', 'PCSX2', 'PS3', 'PS4', 'RPCS3', 'Steam', 'Typhon', 'Vita3K', 'Windows'];
const capitalAfterColon = (text: string) =>
  [...text.matchAll(/:\s+(\S+)/g)].some(([, word]) => /^\p{Lu}/u.test(word) && !PROPER.includes(word.replace(/[^\p{L}\p{N}]+$/u, '')));

afterEach(() => locale.set('ru'));

describe('actionErrorText over every clause code and every action its callers pass', () => {
  it('reads the callers: the emulator prompts, the image dialog, the properties dialog and its compatibility switch, the emulators card', () => {
    expect(callers.map(([path]) => path).sort()).toEqual([
      '../components/AddImageModal.svelte',
      '../components/BiosNeededModal.svelte',
      '../components/EmulatorBusyModal.svelte',
      '../components/EmulatorNeededModal.svelte',
      '../components/GamePropertiesModal.svelte',
      '../game/compatSwitch.svelte.ts',
      '../../routes/settings/EmulatorsCard.svelte',
    ].sort());
    expect(actions).toEqual(
      expect.arrayContaining([
        'emul.saveFailed',
        'emul.busyCloseFailed',
        'emul.saveFailedFor',
        'emul.biosOpenFailed',
        'emul.addFailed',
        'modals.compatEnableFailed',
        'modals.compatDisableFailed',
        'modals.propertiesExeSaveFailed',
      ]),
    );
    expect(clauses).toEqual(expect.arrayContaining(['emulators.save_failed', 'emulators.windows_program', 'library.no_executable']));
  });

  // A late answer arrives after its dialog is gone: each of the four dialogs says it naming its target.
  it('reads the late toasts of the four emulator dialogs, each with an action that names its target', () => {
    expect(lateActions).toEqual(
      ['emul.addFailedFor', 'emul.biosOpenFailedFor', 'emul.busyCloseFailed', 'emul.coverFailedFor', 'emul.openSiteFailedFor', 'emul.saveFailedFor'].sort(),
    );
  });

  for (const lang of ['ru', 'en'] as const) {
    it(`${lang}: says each thing once, keeps the reason, never stacks two colons and puts no capital after one`, () => {
      locale.set(lang);
      const wrong: string[] = [];
      for (const key of actions) {
        const action = msg(key as MessageKey, { emulator: 'RPCS3', title: 'Okami' });
        for (const code of clauses) {
          const reason = msg(code as MessageKey);
          const text = actionErrorText(failure(code), action);
          const parts = text.split(/[.:]\s+/).map(normal);
          const said = (part: string) => parts.filter((other) => other === normal(part)).length;
          if (new Set(parts).size !== parts.length) wrong.push(`repeats: ${text}`);
          if (colons(text) > 1) wrong.push(`colons: ${text}`);
          if (capitalAfterColon(text)) wrong.push(`capital after a colon: ${text}`);
          if (text.includes('..')) wrong.push(`periods: ${text}`);
          if (said(action) !== 1) wrong.push(`action not said once: ${text}`);
          if (!normal(text).includes(normal(reason))) wrong.push(`reason lost: ${text}`);
        }
      }
      expect(wrong).toEqual([]);
    });
  }

  for (const lang of ['ru', 'en'] as const) {
    it(`${lang}: a late failure opens with its action and says every reason, a sentence's too, after it`, () => {
      locale.set(lang);
      const wrong: string[] = [];
      for (const key of lateActions) {
        const action = msg(key as MessageKey, { emulator: 'RPCS3', title: 'Okami' });
        for (const code of withWords) {
          const reason = msg(code as MessageKey);
          const text = lateErrorText(failure(code), action);
          const parts = text.split(/[.:]\s+/).map(normal);
          if (!text.startsWith(action.replace(/\.$/, ''))) wrong.push(`action not first: ${text}`);
          if (parts.filter((part) => part === normal(action)).length !== 1) wrong.push(`action not said once: ${text}`);
          if (new Set(parts).size !== parts.length) wrong.push(`repeats: ${text}`);
          if (colons(text) > 1) wrong.push(`colons: ${text}`);
          if (capitalAfterColon(text)) wrong.push(`capital after a colon: ${text}`);
          if (text.includes('..')) wrong.push(`periods: ${text}`);
          if (!normal(text).includes(normal(reason))) wrong.push(`reason lost: ${text}`);
        }
      }
      expect(wrong).toEqual([]);
    });
  }

  it.each([
    ['ru', 'emulators.save_failed', 'emul.saveFailed', 'Не удалось сохранить путь к эмулятору'],
    ['en', 'emulators.save_failed', 'emul.saveFailed', 'Could not save the emulator path'],
    [
      'ru',
      'emulators.windows_program',
      'emul.saveFailed',
      'Не удалось сохранить путь к эмулятору. На этой системе программа для Windows не подходит: нужна версия эмулятора для неё',
    ],
    [
      'en',
      'emulators.windows_program',
      'emul.saveFailed',
      'Could not save the emulator path. A Windows program does not work on this system: use the emulator build made for it',
    ],
    ['ru', 'emulators.not_exe', 'emul.saveFailed', 'Не удалось сохранить путь к эмулятору: нужен исполняемый файл эмулятора (.exe)'],
    ['en', 'emulators.not_exe', 'emul.saveFailed', 'Could not save the emulator path. An emulator executable (.exe) is required'],
    ['en', 'library.no_executable', 'modals.propertiesExeSaveFailed', 'Could not save the executable. Executable file not found'],
    ['ru', 'library.crossover_unavailable', 'modals.propertiesExeSaveFailed', 'Не удалось сохранить исполняемый файл. CrossOver не установлен или недоступен'],
    ['en', 'library.moving', 'emul.saveFailed', 'Could not save the emulator path. The game is being moved. Wait until the move finishes'],
    [
      'ru',
      'library.state_unreadable',
      'emul.saveFailed',
      'Не удалось сохранить путь к эмулятору: файлы библиотеки не удалось прочитать, изменение не сохранено; перезапустите лаунчер',
    ],
  ] as const)('%s %s after %s reads "%s"', (lang, code, action, expected) => {
    locale.set(lang);
    expect(actionErrorText(failure(code), msg(action))).toBe(expected);
  });
});
