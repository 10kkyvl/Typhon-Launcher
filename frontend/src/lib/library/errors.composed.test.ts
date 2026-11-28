import { afterEach, describe, expect, it } from 'vitest';
import { locale, msg, type MessageKey } from '../i18n';
import { errEmulators } from '../i18n/catalog/ru/errEmulators';
import { errLibrary } from '../i18n/catalog/ru/errLibrary';
import { actionErrorText, isClauseCode } from './errors';

// Every caller that composes "{action}: {reason}" and the actions it passes, read from its source, so
// a new action or a new caller is met by the table below without being listed by hand.
const sources = import.meta.glob<string>(['../../**/*.svelte', '../../**/*.ts', '!../../**/*.test.ts', '!../../**/node_modules/**'], {
  query: '?raw',
  import: 'default',
  eager: true,
});
const COMPOSES = /\b(?:actionErrorText|propertiesErrorText)\b/;
const DEFINED = ['./errors.ts', '../game/propertiesErrors.ts'];
const callers = Object.entries(sources).filter(([path, source]) => COMPOSES.test(source) && !DEFINED.includes(path));
// `errorText` is the properties dialog's name for propertiesErrorText.
const CALL = /\b(?:actionErrorText|errorText)\(\s*[\w.]+\s*,\s*msg\(([^)]*)\)/g;
const actions = [...new Set(callers.flatMap(([, source]) => [...source.matchAll(CALL)].flatMap((call) => [...call[1].matchAll(/'([\w.]+)'/g)].map((key) => key[1]))))].sort();

const words: Record<string, string> = { ...errLibrary, ...errEmulators };
const clauses = Object.keys(words).filter(isClauseCode);
const failure = (code: string) => new Error(`typhon:${code}: detail`);
const normal = (text: string) => text.trim().replace(/\.$/, '').toLocaleLowerCase();
const colons = (text: string) => (text.match(/:/g) ?? []).length;

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
        'emul.openSiteFailedFor',
        'emul.biosOpenFailedFor',
        'emul.addFailedFor',
        'emul.coverFailedFor',
        'modals.compatEnableFailed',
        'modals.compatDisableFailed',
        'modals.propertiesExeSaveFailed',
      ]),
    );
    expect(clauses).toEqual(expect.arrayContaining(['emulators.save_failed', 'emulators.windows_program', 'library.no_executable']));
  });

  for (const lang of ['ru', 'en'] as const) {
    it(`${lang}: says each thing once and never stacks two colons`, () => {
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
          if (text.includes('..')) wrong.push(`periods: ${text}`);
          if (said(action) !== 1) wrong.push(`action not said once: ${text}`);
          if (!normal(text).includes(normal(reason).split(':')[0])) wrong.push(`reason lost: ${text}`);
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
    ['en', 'library.no_executable', 'modals.propertiesExeSaveFailed', 'Could not save the executable: Executable file not found'],
  ] as const)('%s %s after %s reads "%s"', (lang, code, action, expected) => {
    locale.set(lang);
    expect(actionErrorText(failure(code), msg(action))).toBe(expected);
  });
});
