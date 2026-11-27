import { vi, type Mock } from 'vitest';

// Stand-ins of the services behind the dialogs of settings, themes, cloud saves and legal texts, for
// the tests that mount one of them (lib/components/settingsDialogs.mocks.test.ts holds the files to
// it). Those services act: `cloudForget` deletes the player's remote saves, `cloudResolveConflict`
// overwrites saves on one side, `importThemeText` writes a theme file, `setEmulatorPath` writes the
// settings, and the legal documents and the sent-data log read the player's real data. Each test file
// mocks a service whole through the factory here, and fakes its generated binding, so a call that
// slipped past the mocks is recorded rather than sent, and asserts that none was:
//
//   vi.mock('../services/cloudsaves', async (original) => (await import('../testing/settingsDialogsTesting')).cloudsavesService(await original()));
//   vi.mock('../../../bindings/typhon/internal/cloudsaves', async (original) => (await import('../testing/settingsDialogsTesting')).fakeService(await original(), 'cloudsaves'));
//   afterEach(() => { ...; expect(calls).toEqual([]); resetBindings(); });
//
// Every function of a factory answers nothing until the test says what it answers: a call the test
// did not expect fails it.

const unanswered = (name: string) => async (): Promise<never> => {
  throw new Error(`${name} is not answered by this test`);
};

type Answer = Mock<(...args: unknown[]) => Promise<unknown>>;

function every<T extends object, K extends string>(original: T, names: readonly K[]) {
  return { ...original, ...(Object.fromEntries(names.map((name) => [name, vi.fn<(...args: unknown[]) => Promise<unknown>>(unanswered(name))])) as Record<K, Answer>) };
}

export const CLOUDSAVES_CALLS = ['loadCloudStatus', 'cloudSyncNow', 'cloudResolveConflict', 'cloudAllowLaunchOnce', 'cloudPullRemote', 'cloudDeleteRemote', 'cloudForget'] as const;
export const SETTINGS_CALLS = [
  'getSettings',
  'saveSettingsPatch',
  'selectFolder',
  'openFolder',
  'proposeLibraryPath',
  'setupLibrary',
  'libraryFolders',
  'addLibraryFolder',
  'removeLibraryFolder',
  'saveConsent',
  'saveLegalAcceptance',
  'openGameFolder',
] as const;
export const THEME_CALLS = [
  'listThemes',
  'getTheme',
  'saveTheme',
  'deleteTheme',
  'applyTheme',
  'confirmTheme',
  'activeTheme',
  'importTheme',
  'importThemeText',
  'exportTheme',
  'resetTheme',
  'selectThemeFile',
  'selectExportPath',
  'selectSkillFolder',
  'saveAISkill',
] as const;
export const TELEMETRY_LOG_CALLS = ['listSentData'] as const;
export const LEGAL_CALLS = ['listLegalDocuments', 'legalVersion', 'getLegalDocument'] as const;
export const EMULATORS_CALLS = [
  'listPlatforms',
  'imageConsoles',
  'imageTitle',
  'biosFolder',
  'openBiosFolder',
  'closeEmulator',
  'setEmulatorPath',
  'detectEmulators',
  'openOfficialPage',
  'selectEmulatorFile',
  'selectImageFile',
  'gameInEmulator',
  'missingImages',
] as const;

/** Every call of lib/services/cloudsaves that reaches Go. */
export const cloudsavesService = <T extends object>(original: T) => every(original, CLOUDSAVES_CALLS);
/** Every call of lib/services/settings that reaches Go. */
export const settingsService = <T extends object>(original: T) => every(original, SETTINGS_CALLS);
/** Every call of lib/services/theme that reaches Go. */
export const themeService = <T extends object>(original: T) => every(original, THEME_CALLS);
/** Every call of lib/services/telemetryLog that reaches Go. */
export const telemetryLogService = <T extends object>(original: T) => every(original, TELEMETRY_LOG_CALLS);
/** Every call of lib/services/legal that reaches Go. */
export const legalService = <T extends object>(original: T) => every(original, LEGAL_CALLS);
/** Every call of lib/services/emulators that reaches Go. */
export const emulatorsService = <T extends object>(original: T) => every(original, EMULATORS_CALLS);

/** A generated binding whose `Service` records every call in lib/testing/fakeBinding instead of sending it. */
export async function fakeService<T extends object>(original: T, prefix: string) {
  const { fakeBinding } = await import('./fakeBinding');
  return { ...original, Service: fakeBinding(prefix) };
}

/** A promise the test settles by hand, so the state while a request runs can be looked at. */
export function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}
