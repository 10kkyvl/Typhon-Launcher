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

const NEEDED: [string, RegExp][] = [
  ['the whole support service', /vi\.mock\('[^']*lib\/services\/support',[^\n]*\bsupportService\(/],
  ['the log upload', /vi\.mock\('[^']*lib\/services\/logsUpload',[^\n]*\blogsUpload\(\)/],
  ['the account openers', /vi\.mock\('[^']*lib\/services\/account',[^\n]*\baccountOpeners\(/],
  ['the support binding', /vi\.mock\('[^']*bindings\/typhon\/internal\/support',[^\n]*\bfakeService\([^\n]*'support'\)/],
  ['the diagnostics binding', /vi\.mock\('[^']*bindings\/typhon\/internal\/diagnostics',[^\n]*\bfakeService\([^\n]*'diagnostics'\)/],
  ['the account binding', /vi\.mock\('[^']*bindings\/typhon\/internal\/account',[^\n]*\bfakeService\([^\n]*'account'\)/],
  ['no call reached a binding', /expect\(calls\)\.toEqual\(\[\]\)/],
];

const missing = (source: string) => NEEDED.filter(([, pattern]) => !pattern.test(source)).map(([what]) => what);

const mounting = Object.entries(tests).filter(([, source]) => SCREEN.test(source));

describe('tests that mount a support screen', () => {
  it('are found', () => {
    expect(SCREEN.test("import TicketList from './TicketList.svelte';")).toBe(true);
    expect(SCREEN.test("import ticketList from './TicketList.svelte?raw';")).toBe(false);
    expect(mounting.map(([path]) => path)).toContain('./TicketView.dom.test.ts');
    expect(mounting.length).toBeGreaterThanOrEqual(8);
  });

  it('mock the whole support service, the log upload and the account openers, and fake the bindings', () => {
    expect(missing("vi.mock('../../lib/services/support', async (importOriginal) => ({ ...(await importOriginal()), listTickets: vi.fn() }));")).toContain('the whole support service');
    expect(mounting.map(([path, source]) => [path, missing(source)]).filter(([, gaps]) => gaps.length > 0)).toEqual([]);
  });
});
