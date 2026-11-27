import { describe, expect, it } from 'vitest';
import { wailsRuntime } from '../testing/dom';
import { ANY_RUNTIME, NEEDED_PLAIN, PLAIN, REAL_MODULE, gaps, importOf, wrappers } from '../testing/mounts';

/**
 * The game and library dialogs reach Go through calls that act: `library.PlayGame` starts a real game
 * (the random pick), the review report, the metadata search and the log export leave the device, and
 * the rest write the library. So every test that imports one of them runs on the Wails runtime of
 * lib/testing/gameDialogsTesting.ts, whose `Call` records what a binding sends instead of sending it,
 * and asserts that nothing was sent: a call that slipped past the test's own mocks, through a real
 * service function or a binding, fails the test rather than reaching Go. The runtime is mocked once,
 * by exactly that factory; a second mock, an unmock or a real module taken with vi.importActual could
 * put the real runtime back, so each is refused. An import counts in any quote kind, and as `import(`.
 *
 * A test of a component that mounts a dialog at any depth (the game page, the library, the game menu,
 * the review card, the shell) runs the dialog's code too. Those that open a dialog's flow (`HELD`)
 * are held to the same rule. The rest run on lib/testing/dom.ts's runtime, which has no `Call`,
 * `Browser` or `Clipboard`, so a binding that slips past their mocks throws before anything is sent;
 * such a test may give it a `Call` only as a `vi.fn` written in place. A source tripwire: what a mock
 * answers is each test's to get right.
 */
const tests = import.meta.glob<string>(['../../**/*.test.ts', '!../../**/node_modules/**'], { query: '?raw', import: 'default', eager: true });
const components = import.meta.glob<string>(['../../**/*.svelte', '!../../**/node_modules/**'], { query: '?raw', import: 'default', eager: true });

const DIALOGS = [
  'GameStatusModal',
  'GameTagsModal',
  'CollectionModal',
  'SaveBackupsModal',
  'SavesCandidatesModal',
  'RandomGameModal',
  'GameCrashModal',
  'MetadataMatchModal',
  'ReportReviewModal',
  'GameDialogs',
  'GameDialogsHarness',
  // The dialogs of sources and updates: adding or refreshing a source fetches its feed, removing one
  // deletes its releases, a match rewrites a release's game, and the update dialogs start a game's
  // download or the launcher's restart.
  'ReleaseMatchModal',
  'AddSourceModal',
  'SourceDetailsModal',
  'SourcesNoticeModal',
  'ReleaseNotesModal',
  'UpdateRestartModal',
  'UpdateCard',
  // The dialogs of the account: one deletes the account for good, one uploads the avatar, and the
  // sync consent is a legal act that lets the library leave the device.
  'DeleteAccountModal',
  'AvatarCropModal',
  'SocialConsentScreen',
  // The dialogs of game properties, emulators and runtimes: a wrapper switch rewrites the game's files,
  // a runtime installer runs as administrator, closing the open emulator ends its process, and the rest
  // write the library or the settings.
  'GamePropertiesModal',
  'RedistModal',
  'EmulatorNeededModal',
  'BiosNeededModal',
  'EmulatorBusyModal',
  'AddImageModal',
  // The dialogs of download, install and storage: they start real downloads and installers, delete
  // game files and move game folders between disks.
  'RemoveGameModal',
];

const MOUNTS = importOf(DIALOGS);

const WRAPPERS = wrappers(DIALOGS, components);
const WRAPS = importOf(WRAPPERS);

// Tests of a wrapper that open a dialog's flow: the status and tags dialogs from the game menu, the
// report from a review card, the restart dialog from the sidebar and the banner in a seamless restart,
// and the sync consent given on each of the three screens that ask for it.
const HELD = [
  './ContextMenu.dom.test.ts',
  './GameQuickMenu.dom.test.ts',
  './GameQuickMenu.focus.dom.test.ts',
  '../../routes/game/ReviewCard.dom.test.ts',
  './UpdateRestartModal.handoff.dom.test.ts',
  '../../routes/friends/Friends.consent.dom.test.ts',
  '../../routes/activity/Activity.consent.dom.test.ts',
  '../../routes/user/User.consent.dom.test.ts',
];

// Tests of the whole settings page, which mounts the sources notice and, through the About tab's
// banner, the restart dialog. Each runs on lib/testing/dom.ts's runtime, which has no Call, so a
// binding that slips past its mocks throws before anything is sent; each breaks one rule of the
// wrapper check, named here, and belongs to the settings pages. No other test joins them.
const SETTINGS_PAGE: Record<string, string> = {
  '../../routes/settings/AboutTab.rows.dom.test.ts': 'no Browser or Clipboard given to it',
  '../../routes/settings/DownloadsTab.dom.test.ts': 'no real module behind a mock',
};

const RUNTIME = /^vi\.mock\('@wailsio\/runtime', async \(\) => \(await import\('(?:\.\.\/)+(?:lib\/)?testing\/gameDialogsTesting'\)\)\.goRuntime\(\)\);$/m;

// A mock runs what the test writes in place, never a function handed over by reference (the real one
// taken from vi.importActual), and no module is taken whole by a path the guard cannot read.
const BY_REFERENCE = /\.mockImplementation(?:Once)?\(\s*(?!async\b|function\b|\(|[\w$]+\s*=>|vi\.fn\b)/;

const NEEDED: [string, (source: string) => boolean][] = [
  ['the recording runtime, once', (source) => RUNTIME.test(source) && (source.match(ANY_RUNTIME) ?? []).length === 1],
  ['nothing sent to Go', (source) => /const sent = sentToGo\(\);/.test(source) && /expect\(sent\)\.toEqual\(\[\]\);/.test(source)],
  ['no real module behind a mock', REAL_MODULE],
  ['no real function behind a mock', (source) => !BY_REFERENCE.test(source)],
];

const missing = (source: string) => NEEDED.filter(([, holds]) => !holds(source)).map(([what]) => what);

const missingPlain = (source: string) => gaps(NEEDED_PLAIN, source);

const mounting = Object.entries(tests).filter(([path, source]) => MOUNTS.test(source) || HELD.includes(path));
// The source guards that quote a wrapper's import line in their own self-tests: they mount nothing.
// By path, so a test that mounts one cannot leave the check by being named like a guard.
const GUARDS = ['./settingsDialogs.mocks.test.ts', '../../routes/profile/Profile.mocks.test.ts'];
const wrappingOf = (entries: [string, string][]) =>
  entries.filter(([path, source]) => !GUARDS.includes(path) && !MOUNTS.test(source) && !HELD.includes(path) && WRAPS.test(source));
const wrapping = wrappingOf(Object.entries(tests));

const SOUND = [
  "import GameTagsModal from './GameTagsModal.svelte';",
  "vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());",
  "vi.mock('../services/library', async (importOriginal) => ({ ...(await importOriginal<typeof import('../services/library')>()), setTags: vi.fn() }));",
  'afterEach(() => {',
  '  const sent = sentToGo();',
  '  resetBindings();',
  '  expect(sent).toEqual([]);',
  '});',
  'vi.mocked(setTags).mockImplementation(async (id: string) => ({ id }) as LibraryGame);',
].join('\n');

describe('tests that mount a game or library dialog', () => {
  it.each([
    "import GameCrashModal from './GameCrashModal.svelte';",
    'import GameCrashModal from "./GameCrashModal.svelte";',
    'import GameCrashModal from `./GameCrashModal.svelte`;',
    "import GameCrashModal, { type Props } from './GameCrashModal.svelte';",
    "const { default: GameCrashModal } = await import('./GameCrashModal.svelte');",
    'const dialog = await import ("../components/GameCrashModal.svelte");',
  ])('are found by %s', (line) => {
    expect(MOUNTS.test(line)).toBe(true);
  });

  it.each([
    "import source from './GameCrashModal.svelte?raw';",
    'import source from "./GameCrashModal.svelte?raw";',
    "import NotADialog from './GameCrashModalHost.svelte';",
  ])('leave out %s', (line) => {
    expect(MOUNTS.test(line)).toBe(false);
  });

  it('are found', () => {
    expect(mounting.map(([path]) => path)).toEqual(
      expect.arrayContaining([
        './GameStatusModal.dom.test.ts',
        './GameTagsModal.dom.test.ts',
        './CollectionModal.dom.test.ts',
        './SaveBackupsModal.dom.test.ts',
        './SavesCandidatesModal.dom.test.ts',
        './RandomGameModal.launch.dom.test.ts',
        './GameCrashModal.dom.test.ts',
        './MetadataMatchModal.dom.test.ts',
        '../../routes/game/ReportReviewModal.dom.test.ts',
        '../../routes/game/GameDialogs.dom.test.ts',
        './ReleaseMatchModal.dom.test.ts',
        './AddSourceModal.dom.test.ts',
        './SourceDetailsModal.dom.test.ts',
        './SourceDetailsModal.controls.dom.test.ts',
        './SourceDetailsModal.actions.dom.test.ts',
        './SourcesNoticeModal.dom.test.ts',
        './ReleaseNotesModal.dom.test.ts',
        './UpdateRestartModal.dom.test.ts',
        './UpdateCard.dom.test.ts',
        './UpdateCard.test.ts',
        './DeleteAccountModal.dom.test.ts',
        './AvatarCropModal.dom.test.ts',
        './SocialConsentScreen.dom.test.ts',
        './GamePropertiesModal.dom.test.ts',
        './GamePropertiesModal.controls.dom.test.ts',
        './GamePropertiesModal.kit.dom.test.ts',
        './RedistModal.dom.test.ts',
        './RedistModal.install.dom.test.ts',
        './EmulatorPrompts.dom.test.ts',
        './EmulatorNeededModal.dom.test.ts',
        './BiosNeededModal.dom.test.ts',
        './EmulatorBusyModal.dom.test.ts',
        './AddImageModal.dom.test.ts',
        './addDialogs.dom.test.ts',
        './RemoveGameModal.dom.test.ts',
        ...HELD,
      ]),
    );
  });

  it('take a sound file as sound', () => {
    expect(missing(SOUND)).toEqual([]);
  });

  it('refuse the runtime of lib/testing/dom.ts, which records nothing', () => {
    const plain = SOUND.replace(
      "vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());",
      "vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());",
    );
    expect(missing(plain)).toContain('the recording runtime, once');
  });

  it.each([
    ['a second mock', "vi.doMock('@wailsio/runtime', async (original) => await original());"],
    ['a second mock in other quotes', 'vi.doMock("@wailsio/runtime", async (original) => await original());'],
    ['an unmock', "vi.unmock('@wailsio/runtime');"],
  ])('refuse %s of the runtime', (_case, line) => {
    expect(missing(`${SOUND}\n${line}`)).toContain('the recording runtime, once');
  });

  it('refuse a file that does not assert nothing was sent', () => {
    expect(missing(SOUND.replace('  expect(sent).toEqual([]);\n', ''))).toContain('nothing sent to Go');
  });

  it.each([
    ['a real module', "const real = await vi.importActual<typeof import('../services/library')>('../services/library');"],
    ['an unmocked module', "vi.doUnmock('../services/library');"],
  ])('refuse %s behind a mock', (_case, line) => {
    expect(missing(`${SOUND}\n${line}`)).toContain('no real module behind a mock');
  });

  it('refuse a mock given a real function to run by reference', () => {
    expect(missing(`${SOUND}\nvi.mocked(launchGame).mockImplementationOnce(real.launchGame);`)).toContain('no real function behind a mock');
  });

  it('run on the recording runtime and see nothing sent to Go', () => {
    expect(mounting.map(([path, source]) => [path, missing(source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });
});

describe('tests of a component that mounts a game or library dialog', () => {
  it('know the components that mount one, at any depth', () => {
    expect(WRAPPERS).toEqual(
      expect.arrayContaining([
        'App',
        'GameDetails',
        'GameQuickMenu',
        'GameReviews',
        'GameSaves',
        'Library',
        'ReviewCard',
        'Sources',
        'Settings',
        'UpdateBanner',
        'SidebarStatus',
        'GameSections',
        'ProfileSettingsModal',
        'AvatarEditor',
        'Friends',
        'Activity',
        'UserProfile',
        'Installed',
        'BigPictureHarness',
      ]),
    );
    expect(WRAPPERS).not.toContain('GameCard');
  });

  it('are found', () => {
    expect(wrapping.map(([path]) => path)).toEqual(
      expect.arrayContaining([
        '../../routes/game/GameReviews.dom.test.ts',
        '../../routes/game/GameDetails.data.dom.test.ts',
        '../../routes/library/Library.state.dom.test.ts',
        '../../routes/gameMenuOpeners.dom.test.ts',
      ]),
    );
  });

  it('are read whatever they are called: one named like a guard is still held to the rule', () => {
    const hidden = [
      "import { it } from 'vitest';",
      "import Sources from './Sources.svelte';",
      "it('mounts the sources page on the real runtime', () => {",
      '  render(Sources);',
      '});',
    ].join('\n');
    const path = '../../routes/sources/Sources.mocks.test.ts';

    expect(wrappingOf([[path, hidden]]).map(([found]) => found)).toEqual([path]);
    expect(missingPlain(hidden)).toContain('the runtime of lib/testing/dom.ts, once');
  });

  it('leave out only the source guards that quote a wrapper’s import, each named by its path', () => {
    for (const path of GUARDS) {
      expect(tests[path], path).toMatch(/import\.meta\.glob<string>\([^)]*\.test\.ts[\s\S]*?query: '\?raw'/);
      expect(WRAPS.test(tests[path]), path).toBe(true);
    }
  });

  it('rely on a runtime that has no Call, Browser or Clipboard', () => {
    expect(Object.keys(wailsRuntime()).filter((key) => ['Call', 'Browser', 'Clipboard'].includes(key))).toEqual([]);
  });

  it('refuse another runtime, a Browser or Clipboard, or a Call that is not a vi.fn in place', () => {
    const plain = "vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());";
    expect(missingPlain(plain)).toEqual([]);
    expect(missingPlain(`${plain}\nconst extra = { Call: { ByID: vi.fn(() => new Promise(() => {})) } };`)).toEqual([]);
    expect(missingPlain(`${plain}\nconst extra = { Browser: { OpenURL: vi.fn() } };`)).toContain('no Browser or Clipboard given to it');
    expect(missingPlain(`${plain}\nconst extra = { Call: real.Call };`)).toContain('a Call only as a vi.fn in place');
    expect(missingPlain(`${plain}\nvi.doUnmock('@wailsio/runtime');`)).toContain('the runtime of lib/testing/dom.ts, once');
    expect(missingPlain("vi.mock('@wailsio/runtime', async () => ({ Events: { On: vi.fn() } }));")).toContain('the runtime of lib/testing/dom.ts, once');
  });

  it('run on the recording runtime, or on the one that cannot send', () => {
    expect(
      wrapping
        .filter(([path]) => !(path in SETTINGS_PAGE))
        .map(([path, source]) => [path, missingPlain(source)])
        .filter(([, gaps]) => gaps.length > 0),
    ).toEqual([]);
  });

  it('leave the settings page’s tests only the one gap each is named for, on the runtime that cannot send', () => {
    const named = Object.keys(SETTINGS_PAGE);
    expect(wrapping.map(([path]) => path)).toEqual(expect.arrayContaining(named));
    for (const path of named) {
      const source = tests[path];
      expect([path, missingPlain(source)]).toEqual([path, [SETTINGS_PAGE[path]]]);
      expect(source).toMatch(PLAIN);
      expect(source).not.toMatch(/\bCall\b/);
    }
  });
});
