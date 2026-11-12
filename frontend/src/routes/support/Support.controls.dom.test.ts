import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { getTicket, listTickets, ticketLimits, type TicketThread } from '../../lib/services/support';
import { sendNewTicket } from '../../lib/support/draft';
import { renderDom } from '../../lib/testing/dom';
import NewTicket from './NewTicket.svelte';
import TicketList from './TicketList.svelte';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/support', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/support')>()),
  ticketLimits: vi.fn(),
  getTicket: vi.fn(),
  listTickets: vi.fn(),
}));
vi.mock('../../lib/support/draft', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/support/draft')>()),
  sendNewTicket: vi.fn(),
}));

const LIMITS = { maxSubjectRunes: 120, maxBodyRunes: 5000 };

const THREAD: TicketThread = {
  ticket: {
    number: 7,
    ref: 'T-7',
    topic: 'launcher',
    subject: 'Crash on start',
    status: 'open',
    gameIgdbId: null,
    client: null,
    unread: false,
    createdAt: '2026-11-01T10:00:00Z',
    updatedAt: '2026-11-01T10:00:00Z',
    resolvedAt: null,
  },
  messages: [],
};

// A promise the test settles by hand, so the state while a request runs can be looked at.
function held<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(getTicket).mockResolvedValue(THREAD);
  vi.mocked(listTickets).mockResolvedValue({ tickets: [], next: '' });
});

afterEach(() => {
  vi.mocked(ticketLimits).mockReset();
  vi.mocked(getTicket).mockReset();
  vi.mocked(listTickets).mockReset();
  vi.mocked(sendNewTicket).mockReset();
});

// The notice holding `text` and its Retry: after a press the notice is still there, its Retry busy and
// focused, until the answer comes.
async function retryHolds(text: string, user: ReturnType<typeof renderDom>['user']) {
  const alert = await screen.findByRole('alert');
  await vi.waitFor(() => expect(alert.textContent).toContain(text));
  const notice = alert.closest('.notice') as HTMLElement;
  const retry = within(notice).getByRole('button', { name: msg('common.retry') });

  await user.click(retry);

  expect(notice.isConnected).toBe(true);
  expect(retry.isConnected).toBe(true);
  expect(retry.getAttribute('aria-busy')).toBe('true');
  expect(document.activeElement).toBe(retry);
  return notice;
}

describe('Support: Retry in a load-error notice', () => {
  it('keeps the list failure and its focused Retry on screen while the list reloads', async () => {
    const again = held<{ tickets: []; next: string }>();
    vi.mocked(listTickets).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(TicketList);

    const notice = await retryHolds(msg('tickets.loadFailed'), user);

    again.resolve({ tickets: [], next: '' });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
  });

  it('keeps the request failure and its focused Retry on screen while the request reloads', async () => {
    const again = held<TicketThread>();
    vi.mocked(getTicket).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(TicketView, { number: 7 });

    const notice = await retryHolds(msg('tickets.ticketLoadFailed'), user);

    again.resolve(THREAD);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
  });

  it('keeps a failure of the limits and its focused Retry on the reply form while they reload', async () => {
    const again = held<typeof LIMITS>();
    vi.mocked(ticketLimits).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(TicketView, { number: 7 });

    const notice = await retryHolds(msg('tickets.limitsFailed'), user);

    again.reject(new Error('offline'));
    await vi.waitFor(() => expect(within(notice).getByRole('button', { name: msg('common.retry') }).hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
  });

  it('keeps a failure of the limits and its focused Retry on the new request form while they reload', async () => {
    const again = held<typeof LIMITS>();
    vi.mocked(ticketLimits).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(NewTicket, { initialTopic: 'launcher' });

    const notice = await retryHolds(msg('tickets.limitsFailed'), user);

    again.resolve(LIMITS);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(screen.getByRole('button', { name: msg('tickets.submit') })).toBeTruthy();
  });
});

describe('Support: loading', () => {
  it('shows the base spinner named by what is loading', () => {
    vi.mocked(listTickets).mockReturnValue(new Promise(() => {}));
    const { container } = renderDom(TicketList);
    const status = screen.getByRole('status');
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('tickets.loading'));
    expect(container.querySelector('p.muted')).toBeNull();
  });
});

describe('Support: field messages', () => {
  it('stand outside the labels of the new request form, tied to their fields', async () => {
    const { user } = renderDom(NewTicket, { initialTopic: 'launcher' });
    await user.click(await screen.findByRole('button', { name: msg('tickets.submit') }));

    const alerts = screen.getAllByRole('alert');
    expect(alerts).toHaveLength(2);
    for (const alert of alerts) {
      expect(alert.closest('label')).toBeNull();
      expect(document.querySelector(`[aria-describedby="${alert.id}"]`)).toBeTruthy();
    }
    expect(screen.getByLabelText(msg('tickets.subjectLabel'), { exact: false }).getAttribute('aria-invalid')).toBe('true');
  });

  it('stands outside the label of the reply, tied to it', async () => {
    const { user } = renderDom(TicketView, { number: 7 });
    await user.click(await screen.findByRole('button', { name: msg('tickets.reply') }));

    const alert = screen.getByRole('alert');
    expect(alert.closest('label')).toBeNull();
    const reply = screen.getByLabelText(msg('tickets.replyLabel'), { exact: false });
    expect(reply.getAttribute('aria-describedby')).toBe(alert.id);
  });
});

describe('Support: sending a new request', () => {
  it('keeps the focused submit busy while the request is sent', async () => {
    const sent = held<{ sent: false }>();
    vi.mocked(sendNewTicket).mockReturnValue(sent.promise);
    const { user } = renderDom(NewTicket, { initialTopic: 'launcher', initialSubject: 'Crash' });
    const submit = await screen.findByRole('button', { name: msg('tickets.submit') });
    await user.type(screen.getByLabelText(msg('tickets.bodyLabel'), { exact: false }), 'It crashes on start.');

    await user.click(submit);

    expect(submit.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(submit);
    sent.resolve({ sent: false });
    await vi.waitFor(() => expect(submit.hasAttribute('aria-busy')).toBe(false));
  });
});
