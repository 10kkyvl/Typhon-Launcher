import { describe, expect, it, vi } from 'vitest';
import { accountService, coverService, MESSAGING_CALLS, profileService } from './profileTesting';

/**
 * The account service, the profile service and the cover upload reach the real backend with the
 * player's session, and the chat store reaches the messaging service the same way. So every test of
 * this folder that mounts a part replaces the three services and the chat store whole
 * (profileTesting.ts), each by its one line and nothing else, and fakes the three generated bindings,
 * so a call that slipped past the mocks is recorded rather than sent, and asserts that none was. A
 * second mock of one of those modules (vi.mock, vi.doMock, vi.unmock, vi.doUnmock, under any quote and
 * any spelling of its path), a function laid over the factory's answer, the real module taken by
 * vi.importActual, a module named by a path the guard cannot read, or a mock handed a function by
 * reference would put a real call back, so each is refused. A source tripwire over the test files:
 * what a mock answers is each test's to get right.
 */
const tests = import.meta.glob<string>('./*.test.ts', { query: '?raw', import: 'default', eager: true });
const services = import.meta.glob<string>(['../../lib/services/account.ts', '../../lib/services/profile.ts', '../../lib/services/profileCover.ts', '../../lib/stores/messaging.ts'], {
  query: '?raw',
  import: 'default',
  eager: true,
});

const MOUNTS = /\bimport\s+\w+\s+from\s+'\.\/\w+\.svelte'/;

/** Where the tests of this folder sit: a module path in them is read from here. */
const HERE = 'file:///src/routes/profile/';

/** The module a path names from this folder: the extension, a query and a trailing index dropped. */
function moduleOf(path: string): string {
  return new URL(path, HERE).pathname.replace(/\/index(?:\.[cm]?[jt]s)?$/, '').replace(/\.[cm]?[jt]s$/, '');
}

const LINES: [what: string, line: string, module: string][] = [
  ['the account binding', "vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));", '../../../bindings/typhon/internal/account'],
  ['the profile binding', "vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));", '../../../bindings/typhon/internal/profile'],
  ['the messaging binding', "vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));", '../../../bindings/typhon/internal/messaging'],
  ['the whole account service', "vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));", '../../lib/services/account'],
  ['the whole profile service', "vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));", '../../lib/services/profile'],
  ['the whole cover upload', "vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));", '../../lib/services/profileCover'],
  ['the whole chat store', "vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));", '../../lib/stores/messaging'],
];

// Every call that mocks, unmocks or takes the real module, with the path it is given as a literal or as
// `import('…')`; a call given anything else names a module the guard cannot read.
const MODULE_CALL = /\bvi\s*\.\s*(mock|doMock|unmock|doUnmock|importActual)\s*(?:<[^>]*>)?\s*\(\s*(?:import\s*\(\s*)?(?:(['"`])((?:(?!\2).)*)\2)?/g;
// A mock runs what the test writes in place, never a function handed over by reference.
const BY_REFERENCE = /\.mockImplementation(?:Once)?\(\s*(?!async\b|function\b|\(|[\w$]+\s*=>|vi\.fn\b)/;

function moduleCalls(source: string): { kind: string; module: string | null }[] {
  return [...source.matchAll(MODULE_CALL)].map((match) => ({ kind: match[1], module: match[3] === undefined ? null : moduleOf(match[3]) }));
}

function missing(source: string): string[] {
  const lines = source.split('\n').map((line) => line.trim());
  const found = moduleCalls(source);
  const gaps = LINES.filter(
    ([, line, module]) => !lines.includes(line) || found.filter((call) => call.module === moduleOf(module)).length !== 1,
  ).map(([what]) => what);
  if (found.some((call) => call.module === null)) gaps.push('a module the guard cannot read');
  if (BY_REFERENCE.test(source)) gaps.push('no real function behind a mock');
  if (!/expect\(calls\)\.toEqual\(\[\]\)/.test(source)) gaps.push('no call reached a binding');
  return gaps;
}

// Every module a file imports: `from '…'`, a bare `import '…'`, or `import('…')`, under any quote kind.
const IMPORTED = /(?:\bfrom\s*|\bimport\s*\(?\s*)(['"`])((?:(?!\1).)*)\1/g;
const GUARDED = new Set(LINES.map(([, , module]) => moduleOf(module)));

/**
 * The imports of a mocked module with a query or a hash: Vite loads such a path as a module of its own,
 * which the mock does not replace. `?raw` reads the source text and runs nothing.
 */
function queried(source: string): string[] {
  return [...source.matchAll(IMPORTED)]
    .map((match) => match[2])
    .filter((path) => {
      const url = new URL(path, HERE);
      return GUARDED.has(moduleOf(path)) && ((url.search !== '' && url.search !== '?raw') || url.hash !== '');
    });
}

/** The functions of a source file that call its generated binding (`<binding>.`). */
function bindingCalls(source: string, binding: string): string[] {
  const names: string[] = [];
  let current = '';
  for (const line of source.split('\n')) {
    const declared = /^export (?:async )?function (\w+)/.exec(line);
    if (declared) current = declared[1];
    if (current && line.includes(`${binding}.`) && !names.includes(current)) names.push(current);
  }
  return names;
}

const mounting = Object.entries(tests).filter(([, source]) => MOUNTS.test(source));

// Exported from the chat store and reaching nothing: the drafts live in localStorage, the rest is state.
const LOCAL_MESSAGING = ['setChatError', 'beginSend', 'sendLive', 'sessionWatch', 'sendProgress', 'endSend', 'setAttachmentDrafts', 'draftKey', 'getDraft', 'setDraft'];

describe('tests that mount a profile part', () => {
  it('are found', () => {
    expect(MOUNTS.test("import Profile from './Profile.svelte';")).toBe(true);
    expect(MOUNTS.test("import raw from './Profile.svelte?raw';")).toBe(false);
    expect(mounting.map(([path]) => path)).toEqual(expect.arrayContaining(['./Profile.pins.dom.test.ts', './HiddenBadge.dom.test.ts', './ProfileAppearancePanel.frames.test.ts']));
    expect(mounting.length).toBeGreaterThanOrEqual(14);
  });

  it('mock the three services and the chat store whole, fake the three bindings and see no call reach one', () => {
    expect(mounting.map(([path, source]) => [path, missing(source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });

  it('refuse a function laid over a whole mock, a second mock of a module, or a way around them', () => {
    const whole = `${LINES.map(([, line]) => line).join('\n')}\nexpect(calls).toEqual([]);`;
    const [, account] = LINES[3];
    const spread = whole.replace(account, account.replace(/=> (.*)\);$/, '=> ({ ...$1, updateProfile: realUpdate }));'));
    expect(missing(whole)).toEqual([]);
    expect(missing(spread)).toContain('the whole account service');
    expect(missing(`${whole}\nvi.doMock('../../lib/services/profile', async (original) => ({ ...(await original()) }));`)).toContain('the whole profile service');
    expect(missing(`${whole}\nconst real = await vi.importActual('../../lib/services/account');`)).toContain('the whole account service');
    expect(missing(whole.replace('expect(calls).toEqual([]);', ''))).toContain('no call reached a binding');
  });

  it('refuse a second mock of a module under any spelling of its path, or the real module taken', () => {
    const whole = `${LINES.map(([, line]) => line).join('\n')}\nexpect(calls).toEqual([]);`;
    for (const second of [
      'vi.mock("../../lib/services/account", async (original) => await original());',
      'vi.mock(`../../lib/services/profileCover.ts`, async (original) => await original());',
      "vi.mock('../../lib/services/profile.js', async (original) => await original());",
      "vi.doMock('../../lib/services/../services/account?x=1', async (original) => await original());",
      "vi.mock(import('../../lib/stores/messaging'), async (original) => await original());",
      "vi.unmock('./../../lib/services/profileCover');",
      "vi.doUnmock('../../../bindings/typhon/internal/account/index.ts');",
      "const real = await vi.importActual<typeof import('../../lib/services/account')>('../../lib/services/account');",
    ]) {
      expect(missing(`${whole}\n${second}\n`).some((gap) => gap.startsWith('the ')), second).toBe(true);
    }
    expect(missing(`${whole}\nvi.mock('../../lib/stores/profileAccent', async (actual) => ({ ...(await actual<object>()) }));\n`)).toEqual([]);
  });

  it('refuse a module named by a path the guard cannot read, or a real function handed to a mock', () => {
    const whole = `${LINES.map(([, line]) => line).join('\n')}\nexpect(calls).toEqual([]);`;
    for (const line of ["const path = '../../lib/services/account';\nconst real = await vi.importActual(path);", 'vi.doMock(coverPath, async (original) => await original());']) {
      expect(missing(`${whole}\n${line}\n`), line).toContain('a module the guard cannot read');
    }
    for (const line of ['vi.mocked(updateProfile).mockImplementation(real.updateProfile);', 'vi.mocked(uploadProfileCover).mockImplementationOnce(actual.uploadProfileCover);']) {
      expect(missing(`${whole}\n${line}\n`), line).toContain('no real function behind a mock');
    }
    expect(missing(`${whole}\nvi.mocked(updateProfile).mockImplementation(async () => player());\n`)).toEqual([]);
  });
});

describe('every test of this folder', () => {
  it('imports no mocked module by a path with a query, which would load the real one', () => {
    // The guard's own self-tests name such paths as text.
    const others = Object.entries(tests).filter(([path]) => path !== './Profile.mocks.test.ts');
    expect(others.length).toBeGreaterThanOrEqual(20);
    expect(others.map(([path, source]) => [path, queried(source)]).filter(([, found]) => found.length > 0)).toEqual([]);
  });

  it('refuses a query on any mocked module under any import form, and lets ?raw and other modules through', () => {
    for (const line of [
      "const real = await import('../../lib/services/account?real');",
      'const real = (await import("../../lib/services/account?real" as string)) as typeof account;',
      'import * as real from `../../lib/services/profileCover.ts?x=1`;',
      "import { getProfileSnapshot } from '../../lib/services/profile?import';",
      "import '../../lib/stores/messaging?side';",
      "const binding = await import('../../../bindings/typhon/internal/account/index.js?v=2');",
      "const real = await import('../../lib/services/account#real');",
    ]) {
      expect(queried(line), line).toHaveLength(1);
    }
    for (const line of [
      "import source from '../../lib/services/account.ts?raw';",
      "import { updateProfile } from '../../lib/services/account';",
      "import raw from './Profile.svelte?raw';",
      "const art = await import('../../lib/stores/metadata?x=1');",
    ]) {
      expect(queried(line), line).toEqual([]);
    }
  });
});

describe('the whole mocks', () => {
  it.each([
    ['../../lib/services/account.ts', 'AccountService', accountService, 10],
    ['../../lib/services/profile.ts', 'ProfileService', profileService, 3],
    ['../../lib/services/profileCover.ts', 'AccountService', coverService, 1],
  ] as const)('replace every function of %s that calls its binding', (path, binding, factory, least) => {
    const calling = bindingCalls(services[path], binding);
    expect(calling.length).toBeGreaterThanOrEqual(least);
    const real = Object.fromEntries(calling.map((name) => [name, () => 'real']));
    const mocked = factory(real) as Record<string, unknown>;
    expect(calling.filter((name) => !vi.isMockFunction(mocked[name]))).toEqual([]);
  });

  it('replace every function of the chat store that is not local', () => {
    const exported = [...services['../../lib/stores/messaging.ts'].matchAll(/^export (?:async )?function (\w+)/gm)].map(([, name]) => name);
    expect(exported.filter((name) => !LOCAL_MESSAGING.includes(name)).sort()).toEqual([...MESSAGING_CALLS].sort());
  });
});
