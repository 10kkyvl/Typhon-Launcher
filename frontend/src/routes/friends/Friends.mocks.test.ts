import { describe, expect, it, vi } from 'vitest';
import { messagingService, socialService } from './friendsTesting';

/**
 * Every call of the social service and of the messaging service goes to the real backend with the
 * player's session. So every test of this folder that mounts one of its components replaces both
 * services whole (friendsTesting.ts) and fakes both generated bindings, so a call that slipped past
 * the mocks is recorded rather than sent, and asserts that none was. The mock of a service is the
 * factory's line and nothing else: a second mock of the module (vi.mock, vi.doMock, vi.unmock, under
 * any spelling of its path), a function laid over the factory's answer, the real module taken by
 * vi.importActual, or a mock handed a function by reference would put a real call back, so each is
 * refused. A source tripwire; what a mock answers is each test's to get right.
 */
const tests = import.meta.glob<string>('./*.test.ts', { query: '?raw', import: 'default', eager: true });
const services = import.meta.glob<string>(['../../lib/services/social.ts', '../../lib/services/messaging.ts'], { query: '?raw', import: 'default', eager: true });

const MOUNTS = /\bimport\s+\w+\s+from\s+'\.\/\w+\.svelte'/;

/** Where the tests of this folder sit: a module path in them is read from here. */
const HERE = 'file:///src/routes/friends/';

const SERVICE_LINES: [what: string, line: string, module: string][] = [
  [
    'the whole social service',
    "vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));",
    '/src/lib/services/social',
  ],
  [
    'the whole messaging service',
    "vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));",
    '/src/lib/services/messaging',
  ],
  [
    'the social binding',
    "vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));",
    '/bindings/typhon/internal/social',
  ],
  [
    'the messaging binding',
    "vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));",
    '/bindings/typhon/internal/messaging',
  ],
];

/** The module a path names from this folder: the extension, a query and a trailing index dropped. */
function moduleOf(path: string): string {
  return new URL(path, HERE).pathname.replace(/\/index(?:\.[cm]?[jt]s)?$/, '').replace(/\.[cm]?[jt]s$/, '');
}

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
  const gaps = SERVICE_LINES.filter(
    ([, line, module]) => !lines.includes(line) || found.filter((call) => call.module === module).length !== 1,
  ).map(([what]) => what);
  if (found.some((call) => call.module === null)) gaps.push('a module the guard cannot read');
  if (BY_REFERENCE.test(source)) gaps.push('no real function behind a mock');
  if (!/expect\(calls\)\.toEqual\(\[\]\)/.test(source)) gaps.push('no call reached a binding');
  return gaps;
}

/** The functions of a service file that call its generated binding (`<Name>Service.`). */
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

describe('tests that mount a friends component', () => {
  it('are found', () => {
    expect(MOUNTS.test("import FriendRow from './FriendRow.svelte';")).toBe(true);
    expect(MOUNTS.test("import overlayFriends from '../../overlay/OverlayFriends.svelte?raw';")).toBe(false);
    expect(mounting.map(([path]) => path)).toEqual(expect.arrayContaining(['./AddFriendModal.dom.test.ts', './FriendRow.dom.test.ts', './Friends.parts.dom.test.ts']));
  });

  it('mock both services whole, fake both bindings and see no call reach one', () => {
    expect(mounting.map(([path, source]) => [path, missing(source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });

  it('refuse a function laid over the whole mock, or a second mock of the module', () => {
    const whole = mounting[0][1];
    const [, socialLine] = SERVICE_LINES[0];
    const spread = whole.replace(
      socialLine,
      "vi.mock('../../lib/services/social', async (original) => ({ ...(await import('./friendsTesting')).socialService(await original()), friends: (await original<typeof import('../../lib/services/social')>()).friends }));",
    );
    expect(missing(whole)).toEqual([]);
    expect(missing(spread)).toContain('the whole social service');
    for (const second of [
      "vi.doMock('../../lib/services/social', async (original) => ({ ...(await original()) }));",
      "vi.mock('../../lib/services/social.ts', async (original) => ({ ...(await original<object>()) }));",
      'vi.mock("../../lib/services/social.js", async (original) => await original());',
      'vi.doMock(`../../lib/services/../services/social`, async (original) => await original());',
      "vi.mock(import('../../lib/services/social'), async (original) => await original());",
      "vi.unmock('../../lib/services/social');",
      "vi.doUnmock('./../../lib/services/social.ts');",
      "const real = await vi.importActual<typeof import('../../lib/services/social')>('../../lib/services/social');",
    ]) {
      expect(missing(`${whole}\n${second}\n`), second).toContain('the whole social service');
    }
    expect(missing(`${whole}\nvi.mock('../../lib/stores/social', async (original) => await original());\n`)).toEqual([]);
  });

  it('refuse a module named by a path the guard cannot read, or a real function handed to a mock', () => {
    const whole = mounting[0][1];
    for (const line of [
      "const path = '../../lib/services/social';\nconst real = await vi.importActual(path);",
      'vi.doMock(socialPath, async (original) => await original());',
    ]) {
      expect(missing(`${whole}\n${line}\n`), line).toContain('a module the guard cannot read');
    }
    for (const line of ['vi.mocked(friends).mockImplementation(real.friends);', 'vi.mocked(accept).mockImplementationOnce(actual.accept);']) {
      expect(missing(`${whole}\n${line}\n`), line).toContain('no real function behind a mock');
    }
    expect(missing(`${whole}\nvi.mocked(friends).mockImplementation(async () => PAGE);\n`)).toEqual([]);
  });
});

describe('the whole mocks', () => {
  it.each([
    ['../../lib/services/social.ts', 'SocialService', socialService],
    ['../../lib/services/messaging.ts', 'MessagingService', messagingService],
  ] as const)('replace every function of %s that calls its binding, over whatever the module gives', (path, binding, factory) => {
    const calling = bindingCalls(services[path], binding);
    expect(calling.length).toBeGreaterThan(10);
    const real = Object.fromEntries(calling.map((name) => [name, () => 'real']));
    const mocked = factory(real) as Record<string, unknown>;
    expect(calling.filter((name) => !vi.isMockFunction(mocked[name]))).toEqual([]);
  });
});
