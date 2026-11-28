import { describe, expect, it } from 'vitest';
import { wholeService } from './blocksTesting';

/**
 * The game picker reads Go's profile service and, on a retry, Go's library service, and the blocks'
 * settings and the picker of block kinds import the account service, whose functions reach the real
 * backend with the player's session. So every test of this folder that mounts a part replaces those
 * three services whole (every function a mock: blocksTesting.ts), each by its one line and nothing
 * else, fakes their three generated bindings, so a call that slipped past the mocks is recorded
 * rather than sent, and asserts that none was. A second mock of one of those modules (vi.mock,
 * vi.doMock, vi.unmock, vi.doUnmock, under any quote), the real module taken by vi.importActual, a
 * module named by a path the guard cannot read, or a mock handed a function by reference would put a
 * real call back, so each is refused. The parts' own imports are read too: a part that starts to call
 * another service or store has it added here first. A source tripwire over the files: what a mock
 * answers is each test's to get right.
 */
const tests = import.meta.glob<string>('./*.test.ts', { query: '?raw', import: 'default', eager: true });
const parts = import.meta.glob<string>(['./*.svelte', './*.ts', '!./*.test.ts', '!./blocksTesting.ts'], { query: '?raw', import: 'default', eager: true });

const MOUNTS = /\bimport\s+\w+\s+from\s+'\.\/\w+\.svelte'/;

const FIXTURE = "(await import('./blocksTesting'))";
const LINES: [what: string, line: string, module: string][] = [
  // As inside Wails: a call that slipped past a mock goes on to its binding, whose fake records it.
  ['the services as inside Wails', "vi.mock('../../services/backend', () => ({ inWails: true }));", '../../services/backend'],
  ['the account binding', `vi.mock('../../../../bindings/typhon/internal/account', async (original) => ${FIXTURE}.fakeService(await original(), 'account'));`, '../../../../bindings/typhon/internal/account'],
  ['the library binding', `vi.mock('../../../../bindings/typhon/internal/library', async (original) => ${FIXTURE}.fakeService(await original(), 'library'));`, '../../../../bindings/typhon/internal/library'],
  ['the profile binding', `vi.mock('../../../../bindings/typhon/internal/profile', async (original) => ${FIXTURE}.fakeService(await original(), 'profile'));`, '../../../../bindings/typhon/internal/profile'],
  ['the whole account service', `vi.mock('../../services/account', async (original) => ${FIXTURE}.wholeService(await original()));`, '../../services/account'],
  ['the whole library service', `vi.mock('../../services/library', async (original) => ${FIXTURE}.wholeService(await original()));`, '../../services/library'],
  ['the whole profile service', `vi.mock('../../services/profile', async (original) => ${FIXTURE}.wholeService(await original()));`, '../../services/profile'],
];

/** Where the tests of this folder sit: a module path in them is read from here. */
const HERE = 'file:///src/lib/components/profile/';
const moduleOf = (path: string) => new URL(path, HERE).pathname.replace(/\/index(?:\.[cm]?[jt]s)?$/, '').replace(/\.[cm]?[jt]s$/, '');

// Every call that mocks, unmocks or takes the real module, with the path it is given as a literal or
// as `import('…')`; a call given anything else names a module the guard cannot read.
const MODULE_CALL = /\bvi\s*\.\s*(mock|doMock|unmock|doUnmock|importActual)\s*(?:<[^>]*>)?\s*\(\s*(?:import\s*\(\s*)?(?:(['"`])((?:(?!\2).)*)\2)?/g;
// A mock runs what the test writes in place, never a function handed over by reference.
const BY_REFERENCE = /\.mockImplementation(?:Once)?\(\s*(?!async\b|function\b|\(|[\w$]+\s*=>|vi\.fn\b)/;

function missing(source: string): string[] {
  const lines = source.split('\n').map((line) => line.trim());
  const found = [...source.matchAll(MODULE_CALL)].map((match) => (match[3] === undefined ? null : moduleOf(match[3])));
  const gaps = LINES.filter(([, line, module]) => !lines.includes(line) || found.filter((call) => call === moduleOf(module)).length !== 1).map(([what]) => what);
  if (found.some((call) => call === null)) gaps.push('a module the guard cannot read');
  if (BY_REFERENCE.test(source)) gaps.push('no real function behind a mock');
  if (!/expect\(calls\)\.toEqual\(\[\]\)/.test(source)) gaps.push('no call reached a binding');
  return gaps;
}

/**
 * What the parts take from lib/services and lib/stores for their values (a type is erased): the
 * services mocked above, the account's words, which run nothing, and the library store, whose reads
 * go through the library service.
 */
const MOCKED = new Set(['services/account', 'services/library', 'services/profile', 'services/accountMessages', 'stores/library']);

function reached(source: string): string[] {
  return [...source.matchAll(/^\s*import\s+(?!type\b)[^;]*?from\s+'\.\.\/\.\.\/((?:services|stores)\/[\w]+)'/gm)].map((match) => match[1]);
}

const mounting = Object.entries(tests).filter(([, source]) => MOUNTS.test(source));

describe('tests that mount a profile block', () => {
  it('are found', () => {
    expect(MOUNTS.test("import BlockGrid from './BlockGrid.svelte';")).toBe(true);
    expect(MOUNTS.test("import source from './PinnedBlock.svelte?raw';")).toBe(false);
    expect(mounting.map(([path]) => path)).toEqual(expect.arrayContaining(['./BlockGrid.test.ts', './BlockGrid.dom.test.ts', './BlockPicker.test.ts', './BlockPicker.dom.test.ts', './GamePicker.dom.test.ts']));
    expect(mounting.length).toBeGreaterThanOrEqual(9);
  });

  it('run the services as inside Wails, mock the three whole, fake the three bindings and see no call reach one', () => {
    expect(mounting.map(([path, source]) => [path, missing(source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });

  it('refuse a function laid over a whole mock, a second mock of a module, or a way around them', () => {
    const whole = `${LINES.map(([, line]) => line).join('\n')}\nexpect(calls).toEqual([]);`;
    expect(missing(whole)).toEqual([]);
    expect(missing(whole.replace(LINES[6][1], LINES[6][1].replace(/=> (.*)\);$/, '=> ({ ...$1, igdbIdsOf: real }));')))).toContain('the whole profile service');
    expect(missing(`${whole}\nvi.doMock('../../services/library', async (original) => ({ ...(await original()) }));`)).toContain('the whole library service');
    expect(missing(`${whole}\nvi.unmock("../../services/account");`)).toContain('the whole account service');
    expect(missing(`${whole}\nconst real = await vi.importActual('../../services/profile');`)).toContain('the whole profile service');
    expect(missing(`${whole}\nconst path = '../../services/profile';\nawait vi.importActual(path);`)).toContain('a module the guard cannot read');
    expect(missing(`${whole}\nvi.mocked(igdbIdsOf).mockImplementation(realIds);`)).toContain('no real function behind a mock');
    expect(missing(whole.replace('expect(calls).toEqual([]);', ''))).toContain('no call reached a binding');
  });

  it('cover every service and store the parts call', () => {
    expect(reached("  import { igdbIdsOf } from '../../services/profile';\n  import type { GameRef } from '../../services/social';")).toEqual(['services/profile']);
    const unguarded = Object.entries(parts).flatMap(([path, source]) => reached(source).filter((module) => !MOCKED.has(module)).map((module) => `${path}: ${module}`));
    expect(unguarded).toEqual([]);
  });
});

describe('the whole mock of a service', () => {
  it('makes every function a mock that answers nothing, and keeps its constants and classes', async () => {
    class Refusal extends Error {}
    const real = { call: () => 'real', LIMIT: 12, KINDS: ['a'], Refusal };
    const mocked = wholeService(real);

    expect(await mocked.call()).toBeUndefined();
    expect([mocked.LIMIT, mocked.KINDS, mocked.Refusal]).toEqual([12, ['a'], Refusal]);
  });
});
