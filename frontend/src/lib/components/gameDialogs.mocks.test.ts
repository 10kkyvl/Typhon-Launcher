import { describe, expect, it } from 'vitest';

/**
 * The game and library dialogs reach Go through calls that act: `library.PlayGame` starts a real game
 * (the random pick), the review report, the metadata search and the log export leave the device, and
 * the rest write the library. So every test that mounts one of them runs on the Wails runtime of
 * lib/testing/gameDialogsTesting.ts, whose `Call` records what a binding sends instead of sending it,
 * and asserts that nothing was sent: a call that slipped past the test's own mocks, through a real
 * service function or a binding, fails the test rather than reaching Go. The runtime is mocked once,
 * by exactly that factory; a second mock, an unmock or a real module taken with vi.importActual could
 * put the real runtime back, so each is refused. A source tripwire: what a mock answers is each
 * test's to get right.
 */
const tests = import.meta.glob<string>(['../../**/*.test.ts', '!../../**/node_modules/**'], { query: '?raw', import: 'default', eager: true });

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
];

const MOUNTS = new RegExp(String.raw`\bimport\s+\w+\s+from\s+'[^']*\/(?:${DIALOGS.join('|')})\.svelte'`);

const RUNTIME = /^vi\.mock\('@wailsio\/runtime', async \(\) => \(await import\('(?:\.\.\/)+(?:lib\/)?testing\/gameDialogsTesting'\)\)\.goRuntime\(\)\);$/m;
const ANY_RUNTIME = /\bvi\.(?:mock|doMock|unmock|doUnmock)\(\s*['"]@wailsio\/runtime['"]/g;

// A mock runs what the test writes in place, never a function handed over by reference (the real one
// taken from vi.importActual), and no module is taken whole by a path the guard cannot read.
const BY_REFERENCE = /\.mockImplementation(?:Once)?\(\s*(?!async\b|function\b|\(|[\w$]+\s*=>|vi\.fn\b)/;

const NEEDED: [string, (source: string) => boolean][] = [
  ['the recording runtime, once', (source) => RUNTIME.test(source) && (source.match(ANY_RUNTIME) ?? []).length === 1],
  ['nothing sent to Go', (source) => /const sent = sentToGo\(\);/.test(source) && /expect\(sent\)\.toEqual\(\[\]\);/.test(source)],
  ['no real module behind a mock', (source) => !/\bimportActual\b/.test(source) && !/\bvi\.(?:unmock|doUnmock)\(/.test(source)],
  ['no real function behind a mock', (source) => !BY_REFERENCE.test(source)],
];

const missing = (source: string) => NEEDED.filter(([, holds]) => !holds(source)).map(([what]) => what);

const mounting = Object.entries(tests).filter(([, source]) => MOUNTS.test(source));

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
  it('are found', () => {
    expect(MOUNTS.test("import GameCrashModal from './GameCrashModal.svelte';")).toBe(true);
    expect(MOUNTS.test("import source from './GameCrashModal.svelte?raw';")).toBe(false);
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
