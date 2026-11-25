import { vi } from 'vitest';
import type { LogUploadStatus } from '../../lib/services/logsUpload';
import type { Limits, Message, Ticket, TicketPage, TicketThread } from '../../lib/services/support';

// Fixtures and stand-ins of the support screens' dom tests. Each test file mocks the modules itself
// (vi.mock is hoisted per file) through the factories here, so no test can reach the support backend:
//
//   vi.mock('../../lib/services/support', async (original) => (await import('./supportTesting')).supportService(await original()));
//   vi.mock('../../lib/services/logsUpload', async () => (await import('./supportTesting')).logsUpload());
//   vi.mock('../../lib/services/account', async (original) => (await import('./supportTesting')).accountOpeners(await original()));
//
// Ticket text here is made up.

/** Every call of Go's `support.Service` the screens reach, each a mock the test answers. */
export function supportService<T extends object>(original: T) {
  return {
    ...original,
    ticketLimits: vi.fn(),
    listTickets: vi.fn(),
    getTicket: vi.fn(),
    createTicket: vi.fn(),
    replyTicket: vi.fn(),
    resolveTicket: vi.fn(),
    unreadTickets: vi.fn(),
  };
}

type UploadHandler = (status: LogUploadStatus) => void;
const uploadHandlers = new Set<UploadHandler>();

/** The diagnostics upload: its status events come from `emitUpload`, and nothing is ever sent. */
export function logsUpload() {
  return {
    onLogUploadStatus: vi.fn((handler: UploadHandler) => {
      uploadHandlers.add(handler);
      return () => uploadHandlers.delete(handler);
    }),
    sendLogs: vi.fn(async () => {
      throw new Error('sendLogs is not for a test');
    }),
  };
}

export function emitUpload(status: LogUploadStatus): void {
  for (const handler of uploadHandlers) handler(status);
}

/** The pages of the website the account service opens in the browser, and the guest's way out. */
export function accountOpeners<T extends object>(original: T) {
  return {
    ...original,
    logout: vi.fn(async () => {}),
    openSupportSite: vi.fn(async () => {}),
    openStatusPage: vi.fn(async () => {}),
    openForgotPassword: vi.fn(async () => {}),
  };
}

/**
 * A generated binding whose `Service` records every call instead of reaching Go: the screens' tests
 * assert that `calls` (lib/testing/fakeBinding) stays empty, so a call that slipped past the mocks
 * above is seen, not sent.
 *
 *   vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('./supportTesting')).fakeService(await original(), 'support'));
 */
export async function fakeService<T extends object>(original: T, prefix: string) {
  const { fakeBinding } = await import('../../lib/testing/fakeBinding');
  return { ...original, Service: fakeBinding(prefix) };
}

export const LIMITS: Limits = { maxSubjectRunes: 120, maxBodyRunes: 5000 };

export function ticket(number: number, subject: string, patch: Partial<Ticket> = {}): Ticket {
  return {
    number,
    ref: `T-${number}`,
    topic: 'launcher',
    subject,
    status: 'open',
    gameIgdbId: null,
    client: null,
    unread: false,
    createdAt: '2026-11-01T10:00:00Z',
    updatedAt: '2026-11-01T10:00:00Z',
    resolvedAt: null,
    ...patch,
  };
}

export function message(id: number, author: 'user' | 'staff', body: string, createdAt: string, patch: Partial<Message> = {}): Message {
  return { id, author, body, logBundleId: null, createdAt, ...patch };
}

export function page(tickets: Ticket[], next = ''): TicketPage {
  return { tickets, next };
}

export function thread(of: Ticket, messages: Message[] = []): TicketThread {
  return { ticket: of, messages };
}

/** A promise the test settles by hand, so the state while a request runs can be looked at. */
export function held<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}

export const words = (el: Element | null | undefined) => (el?.textContent ?? '').replace(/\s+/g, ' ').trim();
