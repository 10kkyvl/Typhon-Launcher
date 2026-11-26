import { describe, expect, it, vi } from 'vitest';
import { accountService, coverService, MESSAGING_CALLS, profileService } from './profileTesting';

/**
 * The account service, the profile service and the cover upload reach the real backend with the
 * player's session, and the chat store reaches the messaging service the same way. So every test of
 * this folder that mounts a part replaces the three services and the chat store whole
 * (profileTesting.ts), each by its one line and nothing else, and fakes the three generated bindings,
 * so a call that slipped past the mocks is recorded rather than sent, and asserts that none was. A
 * second mock of a module, a function laid over the factory's answer, or a way around the mocks would
 * put a real call back, so each is refused. A source tripwire over the test files: what a mock
 * answers is each test's to get right.
 */
const tests = import.meta.glob<string>('./*.test.ts', { query: '?raw', import: 'default', eager: true });
const services = import.meta.glob<string>(['../../lib/services/account.ts', '../../lib/services/profile.ts', '../../lib/services/profileCover.ts', '../../lib/stores/messaging.ts'], {
  query: '?raw',
  import: 'default',
  eager: true,
});

const MOUNTS = /\bimport\s+\w+\s+from\s+'\.\/\w+\.svelte'/;

const LINES: [what: string, line: string, module: string][] = [
  ['the account binding', "vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));", '../../../bindings/typhon/internal/account'],
  ['the profile binding', "vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));", '../../../bindings/typhon/internal/profile'],
  ['the messaging binding', "vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));", '../../../bindings/typhon/internal/messaging'],
  ['the whole account service', "vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));", '../../lib/services/account'],
  ['the whole profile service', "vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));", '../../lib/services/profile'],
  ['the whole cover upload', "vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));", '../../lib/services/profileCover'],
  ['the whole chat store', "vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));", '../../lib/stores/messaging'],
];

function missing(source: string): string[] {
  const lines = new Set(source.split('\n').map((line) => line.trim()));
  const gaps = LINES.filter(([, line]) => !lines.has(line)).map(([what]) => what);
  for (const [, , module] of LINES) {
    const mocks = source.split(`ock('${module}'`).length - 1;
    if (mocks !== 1) gaps.push(`one mock of ${module}, not ${mocks}`);
  }
  if (/\bimportActual\b|\bdoMock\b|\bspyOn\b|\bunmock\b/.test(source)) gaps.push('no way around the mocks');
  if (!/expect\(calls\)\.toEqual\(\[\]\)/.test(source)) gaps.push('no call reached a binding');
  return gaps;
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
    expect(missing(`${whole}\nvi.doMock('../../lib/services/profile', async (original) => ({ ...(await original()) }));`)).toContain('one mock of ../../lib/services/profile, not 2');
    expect(missing(`${whole}\nconst real = await vi.importActual('../../lib/services/account');`)).toContain('no way around the mocks');
    expect(missing(whole.replace('expect(calls).toEqual([]);', ''))).toContain('no call reached a binding');
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
