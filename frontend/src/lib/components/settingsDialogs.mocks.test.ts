import { describe, expect, it } from 'vitest';
import { CALL_IN_PLACE, NO_REAL_MODULE, PLAIN_RUNTIME, gaps, importOf, wrappers } from '../testing/mounts';

/**
 * The dialogs of settings, themes, cloud saves and legal texts reach services that act: cloudForget
 * deletes the player's remote saves (CloudDisableModal), cloudResolveConflict overwrites saves on one
 * side (CloudConflictModal), importThemeText writes a theme file, setEmulatorPath writes the settings,
 * and the legal documents and the sent-data log read the player's real data. So every test file that
 * mounts one of them replaces each service the dialog reaches whole, through its factory in
 * lib/testing/settingsDialogsTesting.ts, fakes that service's generated binding so a call that slipped
 * past the mock is recorded rather than sent, and asserts that none was. The mock of a service is the
 * factory's line and nothing else: a second mock of the module (vi.mock, vi.doMock, vi.unmock, under
 * any spelling of its path and any quotes), a function laid over the factory's answer, the real module
 * taken by vi.importActual, or a mock handed a function by reference would put a real call back, so
 * each is refused. A source tripwire; what a mock answers is each test's to get right.
 *
 * A test of a component that mounts one of these dialogs at any depth (the settings page and its tabs,
 * the app, Big Picture's shell) runs the dialog's code too. One that presses a control of a dialog that
 * acts or sends (`HELD`) is held to the same rule, for every service the dialogs its components reach
 * call. The rest run on lib/testing/dom.ts's runtime, mocked once by a factory that takes no original
 * module and with no real module behind any mock: a binding that slips past their mocks has no `Call`
 * to send through (one given in place is a `vi.fn`), and a `Browser` or `Clipboard` they add cannot be
 * the real one.
 */
const tests = import.meta.glob<string>(['../../**/*.test.ts', '!../../**/node_modules/**'], { query: '?raw', import: 'default', eager: true });
const components = import.meta.glob<string>(['../../**/*.svelte', '!../../**/node_modules/**'], { query: '?raw', import: 'default', eager: true });

type Service = 'cloudsaves' | 'settings' | 'theme' | 'telemetryLog' | 'legal' | 'emulators';

/** Each dialog and the services it reaches, its caller's included where the dialog only calls back. */
const DIALOGS: Record<string, Service[]> = {
  CloudConsentModal: ['settings', 'cloudsaves'],
  CloudDisableModal: ['settings', 'cloudsaves'],
  CloudConflictModal: ['cloudsaves'],
  ThemeTextImportModal: ['theme'],
  SentDataModal: ['telemetryLog'],
  LegalDocumentModal: ['legal'],
  EmulatorsCard: ['emulators'],
};

const SERVICES: Record<Service, { module: string; factory: string; binding: string; prefix: string }> = {
  cloudsaves: { module: '/src/lib/services/cloudsaves', factory: 'cloudsavesService', binding: '/bindings/typhon/internal/cloudsaves', prefix: 'cloudsaves' },
  settings: { module: '/src/lib/services/settings', factory: 'settingsService', binding: '/bindings/typhon/internal/settings', prefix: 'settings' },
  theme: { module: '/src/lib/services/theme', factory: 'themeService', binding: '/bindings/typhon/internal/theme', prefix: 'theme' },
  telemetryLog: { module: '/src/lib/services/telemetryLog', factory: 'telemetryLogService', binding: '/bindings/typhon/internal/telemetrylog', prefix: 'telemetrylog' },
  legal: { module: '/src/lib/services/legal', factory: 'legalService', binding: '/bindings/typhon/internal/legal', prefix: 'legal' },
  emulators: { module: '/src/lib/services/emulators', factory: 'emulatorsService', binding: '/bindings/typhon/internal/emulators', prefix: 'emulators' },
};

const HELPER = '/src/lib/testing/settingsDialogsTesting';

const MOUNTS = new RegExp(String.raw`\bimport\s+\w+\s+from\s+(['"\x60])[^'"\x60?]*\/(${Object.keys(DIALOGS).join('|')})\.svelte\1`, 'g');

/** Where a test file sits, as a URL its module paths are read against. */
const fileUrl = (path: string) => new URL(path, 'file:///src/lib/components/');

/** The module a path names from the file at `at`: the extension, a query and a trailing index dropped. */
function moduleOf(path: string, at: URL): string {
  return new URL(path, at).pathname.replace(/\?.*$/, '').replace(/\/index(?:\.[cm]?[jt]s)?$/, '').replace(/\.[cm]?[jt]s$/, '');
}

// Every call that mocks, unmocks or takes the real module, with the path it is given as a literal or as
// `import('…')`; a call given anything else names a module the guard cannot read.
const MODULE_CALL = /\bvi\s*\.\s*(mock|doMock|unmock|doUnmock|importActual)\s*(?:<[^>]*>)?\s*\(\s*(?:import\s*\(\s*)?(?:(['"`])((?:(?!\2).)*)\2)?/g;
// The factory's line: the module mocked by a function of the helper given the original module whole.
const WHOLE = /\bvi\.mock\(\s*(['"`])([^'"`]+)\1\s*,\s*async\s*\(\s*(\w+)\s*\)\s*=>\s*\(\s*await\s+import\(\s*(['"`])([^'"`]+)\4\s*\)\s*\)\s*\.\s*(\w+)\(\s*await\s+\3\(\s*\)\s*(?:,\s*(['"`])([\w-]+)\7\s*)?\)\s*\);/g;
// A mock runs what the test writes in place, never a function handed over by reference.
const BY_REFERENCE = /\.mockImplementation(?:Once)?\(\s*(?!async\b|function\b|\(|[\w$]+\s*=>|vi\.fn\b)/;

function mounted(source: string): string[] {
  return [...new Set([...source.matchAll(MOUNTS)].map((match) => match[2]))];
}

/** The components that mount a dialog, at any depth, by file name, and the sources of each. */
const WRAPPERS = wrappers(Object.keys(DIALOGS), components);
const WRAPS = importOf(WRAPPERS);
const sourcesOf = (name: string) => Object.entries(components).filter(([path]) => path.endsWith(`/${name}.svelte`)).map(([, source]) => source);

/** The dialogs a source mounts itself or through the components it imports, at any depth. */
function reached(source: string): string[] {
  const dialogs = new Set<string>();
  const seen = new Set<string>();
  const queue = [source];
  while (queue.length > 0) {
    const next = queue.pop() ?? '';
    for (const dialog of Object.keys(DIALOGS)) if (importOf([dialog]).test(next)) dialogs.add(dialog);
    for (const name of WRAPPERS) {
      if (seen.has(name) || !importOf([name]).test(next)) continue;
      seen.add(name);
      queue.push(...sourcesOf(name));
    }
  }
  return [...dialogs].sort();
}

function missing(path: string, source: string, dialogs = mounted(source)): string[] {
  const at = fileUrl(path);
  const needed = [...new Set(dialogs.flatMap((dialog) => DIALOGS[dialog]))];
  const found = [...source.matchAll(MODULE_CALL)].map((match) => (match[3] === undefined ? null : moduleOf(match[3], at)));
  const whole = [...source.matchAll(WHOLE)].map((match) => ({
    module: moduleOf(match[2], at),
    helper: moduleOf(match[5], at),
    factory: match[6],
    prefix: match[8],
  }));
  const gaps: string[] = [];
  for (const name of needed) {
    const { module, factory, binding, prefix } = SERVICES[name];
    const service = whole.filter((line) => line.module === module && line.helper === HELPER && line.factory === factory && line.prefix === undefined);
    if (service.length !== 1 || found.filter((call) => call === module).length !== 1) gaps.push(`the whole ${name} service`);
    const fake = whole.filter((line) => line.module === binding && line.helper === HELPER && line.factory === 'fakeService' && line.prefix === prefix);
    if (fake.length !== 1 || found.filter((call) => call === binding).length !== 1) gaps.push(`the ${name} binding faked`);
  }
  if (found.includes(null)) gaps.push('a module the guard cannot read');
  if (BY_REFERENCE.test(source)) gaps.push('no real function behind a mock');
  if (!/expect\(calls\)\.toEqual\(\[\]\)/.test(source)) gaps.push('no call reached a binding');
  return gaps;
}

// The glob's keys are relative to this folder: each is read as its path from the root.
const files = Object.entries(tests).map(([key, source]) => [fileUrl(key).pathname, source] as const);
// This file names the dialogs in its own examples.
const SELF = '/src/lib/components/settingsDialogs.mocks.test.ts';
const all = files.filter(([path, source]) => path !== SELF && mounted(source).length > 0);
// The dialogs' own tests: named after a dialog, or the cloud dialogs' shared file.
const own = (path: string) => /\/(?:CloudDialogs|CloudConsentModal|CloudDisableModal|ThemeTextImportModal|SentDataModal|LegalDocumentModal|EmulatorsCard)\.[\w.]*test\.ts$/.test(path);
const mounting = all.filter(([path]) => own(path));
/**
 * Tests of other screens that mount the conflict dialog as the second step of their own flow (a launch
 * a conflict holds back). They are their screens' to hold to a guard; a new file here is a decision,
 * so the list is pinned.
 */
const ELSEWHERE = [
  '/src/lib/components/CloudConflictBanner.dom.test.ts',
  '/src/lib/components/GameSaves.resolve.dom.test.ts',
  '/src/lib/components/InstallModal.play.dom.test.ts',
  '/src/lib/components/RandomGameModal.launch.dom.test.ts',
  '/src/lib/components/busyDialogs.dom.test.ts',
];

/**
 * Tests of a component that mounts a dialog and press a control of one that acts or sends: Delete in
 * the cloud saves dialog of the settings page.
 */
const HELD = ['/src/routes/settings/DownloadsTab.dom.test.ts'];
/**
 * The words of the controls that act or send. A test of a component that names one presses it, or
 * looks for it to press: it is in HELD, or in PRESSED_ELSEWHERE with the reason it is not.
 */
const PRESSES = [
  'cloudsaves.consentEnable',
  'cloudsaves.disableDelete',
  'cloudsaves.disableKeep',
  'cloudsaves.conflictKeepMine',
  'cloudsaves.conflictTakeRemote',
  'cloudsaves.conflictPlayHere',
  'emul.detectApply',
  'settings.appearanceImportText',
];
/**
 * Big Picture's own flow of a launch held by a conflict: its test replaces the cloud saves binding by a
 * Service of three vi.fn written in place, so a choice reaches nothing; the file is Big Picture's.
 */
const PRESSED_ELSEWHERE = ['/src/lib/components/BigPictureShell.conflict.dom.test.ts'];

// The runtime's factory takes no original module: with no importActual either, the real runtime cannot
// come back through a Browser or Clipboard the test adds.
const FACTORY_TAKES_NOTHING = ['the runtime mocked by a factory that takes nothing', (source: string) => /\bvi\.mock\(\s*(['"`])@wailsio\/runtime\1\s*,\s*async\s*\(\s*\)\s*=>/.test(source)] as const;
const NEEDED_WEAK = [PLAIN_RUNTIME, FACTORY_TAKES_NOTHING, CALL_IN_PLACE, NO_REAL_MODULE];

const wrapping = files.filter(([path, source]) => path !== SELF && mounted(source).length === 0 && WRAPS.test(source));
const held = wrapping.filter(([path]) => HELD.includes(path));
const weak = wrapping.filter(([path]) => !HELD.includes(path));

const SOUND_PATH = '/src/lib/components/ThemeTextImportModal.dom.test.ts';
const THEME_LINE = "vi.mock('../services/theme', async (original) => (await import('../testing/settingsDialogsTesting')).themeService(await original()));";
const SOUND = [
  "import ThemeTextImportModal from './ThemeTextImportModal.svelte';",
  THEME_LINE,
  "vi.mock('../../../bindings/typhon/internal/theme', async (original) => (await import('../testing/settingsDialogsTesting')).fakeService(await original(), 'theme'));",
  "vi.mock('../stores/toasts', async (original) => await original());",
  'afterEach(() => {',
  '  expect(calls).toEqual([]);',
  '});',
  'vi.mocked(importThemeText).mockImplementation(async () => THEME);',
].join('\n');

describe('tests that mount a settings, theme, cloud or legal dialog', () => {
  it('are found', () => {
    expect(mounted("import CloudDisableModal from './CloudDisableModal.svelte';")).toEqual(['CloudDisableModal']);
    expect(mounted('import Card from "../../routes/settings/EmulatorsCard.svelte";')).toEqual(['EmulatorsCard']);
    expect(mounted("import source from './SentDataModal.svelte?raw';")).toEqual([]);
    expect(mounting.map(([path]) => path)).toEqual(
      expect.arrayContaining([
        '/src/lib/components/CloudConsentModal.dom.test.ts',
        '/src/lib/components/CloudDisableModal.dom.test.ts',
        '/src/lib/components/CloudDialogs.dom.test.ts',
        '/src/lib/components/ThemeTextImportModal.dom.test.ts',
        '/src/lib/components/SentDataModal.dom.test.ts',
        '/src/lib/components/LegalDocumentModal.dom.test.ts',
        '/src/routes/settings/EmulatorsCard.dom.test.ts',
      ]),
    );
  });

  it('are the dialogs’ own, but for the pinned flows of other screens that reach the conflict dialog', () => {
    expect(all.filter(([path]) => !own(path)).map(([path]) => path).sort()).toEqual(ELSEWHERE);
  });

  it('take a sound file as sound', () => {
    expect(missing(SOUND_PATH, SOUND)).toEqual([]);
  });

  it('mock each service the dialog reaches whole, fake its binding and see no call reach one', () => {
    expect(mounting.map(([path, source]) => [path, missing(path, source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });

  it('refuse a function laid over the whole mock', () => {
    const spread = SOUND.replace(
      THEME_LINE,
      "vi.mock('../services/theme', async (original) => ({ ...(await import('../testing/settingsDialogsTesting')).themeService(await original()), importThemeText: (await original<typeof import('../services/theme')>()).importThemeText }));",
    );
    expect(missing(SOUND_PATH, spread)).toContain('the whole theme service');
  });

  it.each([
    "vi.doMock('../services/theme', async (original) => ({ ...(await original()) }));",
    "vi.mock('../services/theme.ts', async (original) => ({ ...(await original<object>()) }));",
    'vi.mock("../services/theme.js", async (original) => await original());',
    'vi.doMock(`../services/../services/theme`, async (original) => await original());',
    "vi.mock(import('../services/theme'), async (original) => await original());",
    "vi.unmock('../services/theme');",
    "vi.doUnmock('./../services/theme.ts');",
    "vi.doUnmock('../../lib/services/theme/index.ts');",
    "const real = await vi.importActual<typeof import('../services/theme')>('../services/theme');",
  ])('refuse a second mock of the service, under any spelling: %s', (second) => {
    expect(missing(SOUND_PATH, `${SOUND}\n${second}\n`)).toContain('the whole theme service');
  });

  it('refuse a whole mock given another file’s factory or the factory of another service', () => {
    for (const line of [
      "vi.mock('../services/theme', async (original) => (await import('../../routes/friends/friendsTesting')).socialService(await original()));",
      "vi.mock('../services/theme', async (original) => (await import('../testing/settingsDialogsTesting')).legalService(await original()));",
    ]) {
      expect(missing(SOUND_PATH, SOUND.replace(THEME_LINE, line)), line).toContain('the whole theme service');
    }
  });

  it('refuse a file whose binding is not faked, or that does not see whether a call reached it', () => {
    const unfaked = SOUND.split('\n').filter((line) => !line.includes('bindings/typhon/internal/theme')).join('\n');
    expect(missing(SOUND_PATH, unfaked)).toContain('the theme binding faked');
    expect(missing(SOUND_PATH, SOUND.replace('  expect(calls).toEqual([]);\n', ''))).toContain('no call reached a binding');
  });

  it('refuse a module named by a path the guard cannot read, or a real function handed to a mock', () => {
    for (const line of ["const path = '../services/theme';\nconst real = await vi.importActual(path);", 'vi.doMock(themePath, async (original) => await original());']) {
      expect(missing(SOUND_PATH, `${SOUND}\n${line}\n`), line).toContain('a module the guard cannot read');
    }
    expect(missing(SOUND_PATH, `${SOUND}\nvi.mocked(importThemeText).mockImplementation(real.importThemeText);\n`)).toContain('no real function behind a mock');
  });

  it('ask a file of another folder for its own paths, read from where it sits', () => {
    const card = [
      "import EmulatorsCard from './EmulatorsCard.svelte';",
      "vi.mock('../../lib/services/emulators', async (original) => (await import('../../lib/testing/settingsDialogsTesting')).emulatorsService(await original()));",
      "vi.mock('../../../bindings/typhon/internal/emulators', async (original) => (await import('../../lib/testing/settingsDialogsTesting')).fakeService(await original(), 'emulators'));",
      'expect(calls).toEqual([]);',
    ].join('\n');
    expect(missing('/src/routes/settings/EmulatorsCard.dom.test.ts', card)).toEqual([]);
    expect(missing('/src/routes/settings/parts/EmulatorsCard.dom.test.ts', card)).toContain('the whole emulators service');
  });
});

describe('tests of a component that mounts a settings, theme, cloud or legal dialog', () => {
  it('know the components that mount one, at any depth, and the dialogs each reaches', () => {
    expect(WRAPPERS).toEqual(expect.arrayContaining(['App', 'DownloadsTab', 'Settings']));
    expect(reached("import Settings from './Settings.svelte';")).toEqual(
      expect.arrayContaining(['CloudConsentModal', 'CloudDisableModal', 'EmulatorsCard', 'LegalDocumentModal', 'SentDataModal']),
    );
    expect(reached("import Card from './Card.svelte';")).toEqual([]);
  });

  it('are found', () => {
    expect(wrapping.map(([path]) => path)).toEqual(
      expect.arrayContaining([
        '/src/routes/settings/DownloadsTab.dom.test.ts',
        '/src/routes/settings/AboutTab.rows.dom.test.ts',
        '/src/routes/settings/GeneralTab.rows.dom.test.ts',
        '/src/routes/settings/Settings.about.dom.test.ts',
        '/src/routes/settings/Settings.switch.dom.test.ts',
        '/src/routes/settings/appearance.dom.test.ts',
        '/src/App.startup.dom.test.ts',
        '/src/App.handoff.dom.test.ts',
      ]),
    );
    expect(held.map(([path]) => path)).toEqual(HELD);
  });

  it('that press a control which acts or sends are held to the whole mocks', () => {
    const pressing = weak.filter(([, source]) => PRESSES.some((key) => source.includes(`'${key}`)));
    expect(pressing.map(([path]) => path).sort()).toEqual(PRESSED_ELSEWHERE);
  });

  it('mock each service the dialogs they reach call whole, fake its binding and see no call reach one', () => {
    expect(held.map(([path, source]) => [path, missing(path, source, reached(source))]).filter(([, found]) => found.length > 0)).toEqual([]);
  });

  it('refuse another runtime, a factory that takes the real one, a Call that is not a vi.fn in place, or a real module', () => {
    const plain = "vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());";
    expect(gaps(NEEDED_WEAK, plain)).toEqual([]);
    expect(gaps(NEEDED_WEAK, "vi.mock('@wailsio/runtime', async () => ({ ...(await import('../testing/dom')).wailsRuntime(), Clipboard: { SetText: vi.fn(async () => {}) } }));")).toEqual([]);
    expect(gaps(NEEDED_WEAK, "vi.mock('@wailsio/runtime', async (original) => ({ ...(await original()), ...(await import('../testing/dom')).wailsRuntime() }));")).toContain(
      'the runtime mocked by a factory that takes nothing',
    );
    expect(gaps(NEEDED_WEAK, `${plain}\nconst extra = { Call: real.Call };`)).toContain('a Call only as a vi.fn in place');
    expect(gaps(NEEDED_WEAK, `${plain}\nvi.doUnmock('@wailsio/runtime');`)).toContain('the runtime of lib/testing/dom.ts, once');
    expect(gaps(NEEDED_WEAK, `${plain}\nconst real = await vi.importActual('@wailsio/runtime');`)).toContain('no real module behind a mock');
  });

  it('run on the runtime that cannot send', () => {
    expect(weak.map(([path, source]) => [path, gaps(NEEDED_WEAK, source)]).filter(([, found]) => found.length > 0)).toEqual([]);
  });
});
