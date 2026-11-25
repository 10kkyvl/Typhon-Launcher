import { describe, expect, it } from 'vitest';

/**
 * The support screens send a player's text, replies and log bundles to the real support backend, and
 * the screens' own reads (the list, a request, the unread count) reach it too. So every test that
 * mounts a support screen replaces the whole support service (Limits, List, Get, Resolve, Unread,
 * Create, Reply), the log upload and the account service's openers with mocks (lib/testing/supportTesting.ts),
 * and fakes the three generated bindings, so a call that slipped past the mocks is recorded rather
 * than sent. A source tripwire over the test files: what a mock answers is each test's to get right.
 */
const tests = import.meta.glob<string>(['../../**/*.test.ts', '!../../**/node_modules/**'], { query: '?raw', import: 'default', eager: true });

const SCREEN = /\bimport\s+\w+\s+from\s+'[^']*\/(?:Support|TicketList|TicketView|NewTicket|AttachLogs|LogsConfirm|SupportRouteHarness)\.svelte'/;

const FIXTURES = String.raw`\(await import\('[^']*\/supportTesting'\)\)`;

/**
 * The module is mocked once, by exactly the factory of the fixtures and nothing around it: a spread, an
 * Object.assign, a second vi.mock or vi.doMock after it (or an unmock) could put one real function back
 * over the whole mock, and vi.importActual hands the test the real module. `\1` is the factory's own
 * parameter, the original module.
 */
function only(path: string, call: string): (source: string) => boolean {
  const any = new RegExp(String.raw`vi\.(?:mock|doMock|unmock|doUnmock)\(\s*'[^']*${path}'`, 'g');
  const exact = new RegExp(String.raw`^vi\.mock\('[^']*${path}', async \((\w*)\) => ${FIXTURES}\.${call}\);$`, 'm');
  const actual = new RegExp(String.raw`importActual\b[^(]*\(\s*'[^']*${path}'`);
  return (source) => (source.match(any) ?? []).length === 1 && exact.test(source) && !actual.test(source);
}

// A mock runs what the test writes in place, never a function handed over by reference (the real one
// taken from vi.importActual), and no module is taken whole by a path the guard cannot read.
const BY_REFERENCE = /\.mockImplementation(?:Once)?\(\s*(?!async\b|function\b|\(|[\w$]+\s*=>|vi\.fn\b)/;
const ACTUAL_BY_NAME = /importActual(?:<[^>]*>)?\(\s*(?!['"`])/;

const NEEDED: [string, (source: string) => boolean][] = [
  ['the whole support service', only(String.raw`lib\/services\/support`, String.raw`supportService\(await \1\(\)\)`)],
  ['the log upload', only(String.raw`lib\/services\/logsUpload`, String.raw`logsUpload\(\)`)],
  ['the account openers', only(String.raw`lib\/services\/account`, String.raw`accountOpeners\(await \1\(\)\)`)],
  ['the support binding', only(String.raw`bindings\/typhon\/internal\/support`, String.raw`fakeService\(await \1\(\), 'support'\)`)],
  ['the diagnostics binding', only(String.raw`bindings\/typhon\/internal\/diagnostics`, String.raw`fakeService\(await \1\(\), 'diagnostics'\)`)],
  ['the account binding', only(String.raw`bindings\/typhon\/internal\/account`, String.raw`fakeService\(await \1\(\), 'account'\)`)],
  ['no call reached a binding', (source) => /expect\(calls\)\.toEqual\(\[\]\)/.test(source)],
  ['no real function behind a mock', (source) => !BY_REFERENCE.test(source) && !ACTUAL_BY_NAME.test(source)],
];

const missing = (source: string) => NEEDED.filter(([, holds]) => !holds(source)).map(([what]) => what);

const mounting = Object.entries(tests).filter(([, source]) => SCREEN.test(source));

describe('tests that mount a support screen', () => {
  it('are found', () => {
    expect(SCREEN.test("import TicketList from './TicketList.svelte';")).toBe(true);
    expect(SCREEN.test("import ticketList from './TicketList.svelte?raw';")).toBe(false);
    expect(mounting.map(([path]) => path)).toContain('./TicketView.dom.test.ts');
    expect(mounting.length).toBeGreaterThanOrEqual(8);
  });

  it('refuse a mock that puts one real function back over the whole one', () => {
    const fixtures = "(await import('../../lib/testing/supportTesting'))";
    const whole = `vi.mock('../../lib/services/support', async (original) => ${fixtures}.supportService(await original()));`;
    expect(missing(whole)).not.toContain('the whole support service');
    for (const mock of [
      "vi.mock('../../lib/services/support', async (importOriginal) => ({ ...(await importOriginal()), listTickets: vi.fn() }));",
      `vi.mock('../../lib/services/support', async (original) => ({ ...${fixtures}.supportService(await original()), getTicket: (await original<typeof import('../../lib/services/support')>()).getTicket }));`,
      `vi.mock('../../lib/services/support', async (original) => ({ ...${fixtures}.supportService(await original()), unreadTickets: (await original<typeof import('../../lib/services/support')>()).unreadTickets }));`,
      `vi.mock('../../lib/services/support', async (original) => Object.assign(${fixtures}.supportService(await original()), { getTicket: (await original<typeof import('../../lib/services/support')>()).getTicket }));`,
      `${whole}\nvi.mock('../../lib/services/support', async (original) => await original());`,
    ]) {
      expect(missing(mock), mock).toContain('the whole support service');
    }
    expect(missing(`vi.mock('../../lib/services/account', async (original) => ({ ...${fixtures}.accountOpeners(await original()), logout: vi.fn() }));`)).toContain('the account openers');
    expect(missing(`vi.mock('../../lib/services/logsUpload', async () => ({ ...${fixtures}.logsUpload(), sendLogs: vi.fn() }));`)).toContain('the log upload');
  });

  // A file that passes every check above, and the six modules it mocks.
  const fixtures = "(await import('../../lib/testing/supportTesting'))";
  const MODULES: [what: string, path: string][] = [
    ['the whole support service', '../../lib/services/support'],
    ['the log upload', '../../lib/services/logsUpload'],
    ['the account openers', '../../lib/services/account'],
    ['the support binding', '../../../bindings/typhon/internal/support'],
    ['the diagnostics binding', '../../../bindings/typhon/internal/diagnostics'],
    ['the account binding', '../../../bindings/typhon/internal/account'],
  ];
  const sound = [
    `vi.mock('../../lib/services/support', async (original) => ${fixtures}.supportService(await original()));`,
    `vi.mock('../../lib/services/logsUpload', async () => ${fixtures}.logsUpload());`,
    `vi.mock('../../lib/services/account', async (original) => ${fixtures}.accountOpeners(await original()));`,
    `vi.mock('../../../bindings/typhon/internal/support', async (original) => ${fixtures}.fakeService(await original(), 'support'));`,
    `vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => ${fixtures}.fakeService(await original(), 'diagnostics'));`,
    `vi.mock('../../../bindings/typhon/internal/account', async (original) => ${fixtures}.fakeService(await original(), 'account'));`,
    'vi.mocked(getTicket).mockImplementation(async (number: number) => thread(ticket(number, `Request ${number}`), []));',
    'expect(calls).toEqual([]);',
  ].join('\n');

  it('take a sound file as sound', () => {
    expect(missing(sound)).toEqual([]);
  });

  it.each(MODULES)('refuse a vi.doMock that mocks %s again', (what, path) => {
    expect(missing(`${sound}\nvi.doMock('${path}', async (original) => await original());`)).toContain(what);
  });

  it.each(MODULES)('refuse a vi.importActual of %s', (what, path) => {
    expect(missing(`${sound}\nconst real = await vi.importActual<typeof import('${path}')>('${path}');`)).toContain(what);
  });

  it('refuse a mock given a real function to run, by reference rather than written in the test', () => {
    for (const line of [
      'vi.mocked(getTicket).mockImplementation(real.getTicket);',
      'vi.mocked(createTicket).mockImplementationOnce(actual.createTicket);',
      "const path = '../../lib/services/support';\nconst real = await vi.importActual(path);",
    ]) {
      expect(missing(`${sound}\n${line}`), line).toContain('no real function behind a mock');
    }
  });

  it('mock the whole support service, the log upload and the account openers, and fake the bindings', () => {
    expect(mounting.map(([path, source]) => [path, missing(source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });
});
