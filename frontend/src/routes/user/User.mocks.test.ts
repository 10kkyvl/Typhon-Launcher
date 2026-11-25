import { describe, expect, it } from 'vitest';
import { sourcesUnder } from '../../lib/testing/sources';
import { MESSAGING_CALLS, SOCIAL_CALLS } from './userTesting';

/**
 * Every call of the social service goes to the real backend with the player's session, and the chat
 * store reaches the messaging service the same way. So every test of this folder that mounts a part
 * replaces both whole (userTesting.ts), each by its one line and nothing after it, and fakes the two
 * generated bindings, so a call that slipped past the mocks is recorded and fails the test rather than
 * being sent. A source tripwire over the test files: what a mock answers is each test's to get right.
 */
const tests = import.meta.glob<string>('./*.test.ts', { query: '?raw', import: 'default', eager: true });

const MOUNTS = /\bimport\s+\w+\s+from\s+'[^']*\.svelte'/;

const LINES: [string, string][] = [
  ['the social binding', "vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));"],
  ['the messaging binding', "vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));"],
  ['the whole social service', "vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));"],
  ['the whole chat store', "vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));"],
];

const MOCKED = ['bindings/typhon/internal/social', 'bindings/typhon/internal/messaging', 'lib/services/social', 'lib/stores/messaging'];

function missing(source: string): string[] {
  const lines = new Set(source.split('\n').map((line) => line.trim()));
  const gaps = LINES.filter(([, line]) => !lines.has(line)).map(([what]) => what);
  for (const path of MOCKED) {
    const mocks = source.split(`mock('${path.startsWith('bindings') ? '../../../' : '../../'}${path}'`).length - 1;
    if (mocks !== 1) gaps.push(`one mock of ${path}, not ${mocks}`);
  }
  if (/\bimportActual\b|\bdoMock\b|\bspyOn\b/.test(source)) gaps.push('no way around the mocks');
  if (!/expect\(calls\)\.toEqual\(\[\]\)/.test(source)) gaps.push('no call reached a binding');
  return gaps;
}

const mounting = Object.entries(tests).filter(([, source]) => MOUNTS.test(source));

// Exported from the chat store and reaching nothing: the drafts live in localStorage, the rest is state.
const LOCAL_MESSAGING = ['setChatError', 'beginSend', 'sendLive', 'sessionWatch', 'sendProgress', 'endSend', 'setAttachmentDrafts', 'draftKey', 'getDraft', 'setDraft'];

const exported = (source: string) => [...source.matchAll(/^export (?:async )?function (\w+)/gm)].map(([, name]) => name);

describe("the user page's tests", () => {
  it('are found', () => {
    expect(mounting.map(([path]) => path)).toContain('./User.pins.dom.test.ts');
    expect(mounting.length).toBeGreaterThanOrEqual(8);
  });

  it('replace the social service and the chat store whole and fake both bindings', () => {
    const wrapped = LINES[2][1].replace(/=> (.*)\);$/, '=> ({ ...$1, profile: realProfile }));');
    expect(missing(LINES.map(([, line]) => line).join('\n') + '\nexpect(calls).toEqual([]);')).toEqual([]);
    expect(missing(wrapped)).toContain('the whole social service');
    expect(missing(`${LINES[2][1]}\nvi.mock('../../lib/services/social', () => ({}));`)).toContain('one mock of lib/services/social, not 2');
    expect(mounting.map(([path, source]) => [path, missing(source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });

  it('mock every call the social service sends to Go', () => {
    const source = sourcesUnder('lib/services/social.ts')['lib/services/social.ts'];
    const calling = exported(source).filter((name) => new RegExp(`function ${name}\\b[^]*?\\n}`).exec(source)?.[0].includes('SocialService.'));
    expect(calling.length).toBeGreaterThan(15);
    expect([...SOCIAL_CALLS].sort()).toEqual(calling.sort());
  });

  it('mock every function of the chat store that is not local', () => {
    const source = sourcesUnder('lib/stores/messaging.ts')['lib/stores/messaging.ts'];
    expect(exported(source).filter((name) => !LOCAL_MESSAGING.includes(name)).sort()).toEqual([...MESSAGING_CALLS].sort());
  });
});
