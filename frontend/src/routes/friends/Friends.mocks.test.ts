import { describe, expect, it, vi } from 'vitest';
import { messagingService, socialService } from './friendsTesting';

/**
 * Every call of the social service and of the messaging service goes to the real backend with the
 * player's session. So every test of this folder that mounts one of its components replaces both
 * services whole (friendsTesting.ts) and fakes both generated bindings, so a call that slipped past
 * the mocks is recorded rather than sent, and asserts that none was. The mock of a service is the
 * factory's line and nothing else: a second mock of the module, or a function laid over the factory's
 * answer, would put a real call back, so either is refused. A source tripwire; what a mock answers is
 * each test's to get right.
 */
const tests = import.meta.glob<string>('./*.test.ts', { query: '?raw', import: 'default', eager: true });
const services = import.meta.glob<string>(['../../lib/services/social.ts', '../../lib/services/messaging.ts'], { query: '?raw', import: 'default', eager: true });

const MOUNTS = /\bimport\s+\w+\s+from\s+'\.\/\w+\.svelte'/;

const SERVICE_LINES: [string, string, RegExp][] = [
  [
    'the whole social service',
    "vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));",
    /\bvi\.(?:do)?[mM]ock\(\s*['"][^'"]*lib\/services\/social['"]/g,
  ],
  [
    'the whole messaging service',
    "vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));",
    /\bvi\.(?:do)?[mM]ock\(\s*['"][^'"]*lib\/services\/messaging['"]/g,
  ],
  [
    'the social binding',
    "vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));",
    /\bvi\.(?:do)?[mM]ock\(\s*['"][^'"]*bindings\/typhon\/internal\/social['"]/g,
  ],
  [
    'the messaging binding',
    "vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));",
    /\bvi\.(?:do)?[mM]ock\(\s*['"][^'"]*bindings\/typhon\/internal\/messaging['"]/g,
  ],
];

function missing(source: string): string[] {
  const lines = source.split('\n').map((line) => line.trim());
  const gaps = SERVICE_LINES.filter(([, line, any]) => !lines.includes(line) || (source.match(any) ?? []).length !== 1).map(([what]) => what);
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
    const twice = `${whole}\nvi.doMock('../../lib/services/social', async (original) => ({ ...(await original()) }));\n`;
    expect(missing(whole)).toEqual([]);
    expect(missing(spread)).toContain('the whole social service');
    expect(missing(twice)).toContain('the whole social service');
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
