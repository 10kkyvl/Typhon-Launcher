import { beforeEach, describe, expect, it, vi } from 'vitest';
import { answer, calls, resetBindings } from '../testing/fakeBinding';

const FAKE = '../testing/fakeBinding';

vi.mock('../../../bindings/typhon/internal/support', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('support'),
}));

type Api = typeof import('./support');

async function load(inWails: boolean): Promise<Api> {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./support');
}

beforeEach(() => {
  resetBindings();
});

describe('support service in the desktop app', () => {
  it('gives an empty page when Go sent null lists', async () => {
    answer('support.List', { tickets: null, next: '' });
    const api = await load(true);

    await expect(api.listTickets()).resolves.toEqual({ tickets: [], next: '' });
    expect(calls.map((call) => call.key)).toEqual(['support.List']);
  });

  it('passes the cursor through', async () => {
    answer('support.List', { tickets: [{ number: 1 }], next: 'c2' });
    const api = await load(true);

    const page = await api.listTickets('c1');

    expect(calls[0].args).toEqual(['c1']);
    expect(page).toEqual({ tickets: [{ number: 1 }], next: 'c2' });
  });

  it('gives a thread with no messages as an empty list', async () => {
    answer('support.Get', { ticket: { number: 7 }, messages: null });
    const api = await load(true);

    await expect(api.getTicket(7)).resolves.toEqual({ ticket: { number: 7 }, messages: [] });
  });

  it('sends the draft exactly as the Go input expects it', async () => {
    answer('support.Create', { number: 3, ref: 'T-3' });
    const api = await load(true);
    const input = { topic: 'launch' as const, subject: 's', body: 'b', gameIgdbId: 12, attachLogs: true };

    await api.createTicket(input);

    expect(calls).toEqual([{ key: 'support.Create', args: [input] }]);
  });

  it('forwards reply, resolve, limits and unread to the right calls', async () => {
    answer('support.Reply', { id: 1 });
    answer('support.Resolve', { number: 4 });
    answer('support.Limits', { maxSubjectRunes: 120, maxBodyRunes: 5000 });
    answer('support.Unread', 2);
    const api = await load(true);

    await api.replyTicket(4, 'text', true);
    await api.resolveTicket(4);
    await expect(api.ticketLimits()).resolves.toEqual({ maxSubjectRunes: 120, maxBodyRunes: 5000 });
    await expect(api.unreadTickets()).resolves.toBe(2);

    expect(calls.map((call) => call.key)).toEqual([
      'support.Reply',
      'support.Resolve',
      'support.Limits',
      'support.Unread',
    ]);
    expect(calls[0].args).toEqual([4, 'text', true]);
  });

  it('turns a ticket code into an account error with its field', async () => {
    answer('support.Create', () => Promise.reject(new Error('ticket_invalid_subject')));
    const api = await load(true);
    const { AccountError } = await import('./account');

    const failure = await api
      .createTicket({ topic: 'other', subject: '', body: 'b', gameIgdbId: null, attachLogs: false })
      .catch((err: unknown) => err);

    expect(failure).toBeInstanceOf(AccountError);
    expect(failure).toMatchObject({ code: 'ticket_invalid_subject', field: 'subject' });
  });

  it('keeps a coded log-upload failure readable instead of flattening it to a server error', async () => {
    const failure = new Error('typhon:diagnostics.log_upload_rate_limited: 429');
    answer('support.Create', () => Promise.reject(failure));
    const api = await load(true);

    await expect(
      api.createTicket({ topic: 'other', subject: 's', body: 'b', gameIgdbId: null, attachLogs: true }),
    ).rejects.toBe(failure);
  });

  it('does not hide a failed unread check as zero', async () => {
    answer('support.Unread', () => Promise.reject(new Error('network_error')));
    const api = await load(true);

    await expect(api.unreadTickets()).rejects.toMatchObject({ code: 'network_error' });
  });
});

describe('support service in a browser preview', () => {
  it('reads as empty without calling the backend', async () => {
    const api = await load(false);

    await expect(api.listTickets()).resolves.toEqual({ tickets: [], next: '' });
    await expect(api.unreadTickets()).resolves.toBe(0);
    expect(calls).toEqual([]);
  });

  it('refuses to write', async () => {
    const api = await load(false);

    await expect(api.getTicket(1)).rejects.toMatchObject({ code: 'unauthenticated' });
    await expect(api.replyTicket(1, 'x', false)).rejects.toMatchObject({ code: 'unauthenticated' });
    await expect(api.resolveTicket(1)).rejects.toMatchObject({ code: 'unauthenticated' });
    await expect(api.ticketLimits()).rejects.toMatchObject({ code: 'unauthenticated' });
    expect(calls).toEqual([]);
  });
});
