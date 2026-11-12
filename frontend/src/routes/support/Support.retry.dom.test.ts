import { fireEvent, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { getTicket, listTickets, ticketLimits, type Ticket, type TicketPage, type TicketThread } from '../../lib/services/support';
import { unreadReplies } from '../../lib/stores/support';
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

const LIMITS = { maxSubjectRunes: 120, maxBodyRunes: 5000 };

function ticket(number: number, subject: string): Ticket {
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
  } as Ticket;
}

const THREAD: TicketThread = { ticket: ticket(7, 'Crash on start'), messages: [] };

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
  unreadReplies.set(0);
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(getTicket).mockResolvedValue(THREAD);
});

afterEach(() => {
  unreadReplies.set(0);
  vi.mocked(ticketLimits).mockReset();
  vi.mocked(getTicket).mockReset();
  vi.mocked(listTickets).mockReset();
});

const row = (subject: string) => screen.getByRole('button', { name: new RegExp(subject) });

async function failureNotice(text: string): Promise<HTMLElement> {
  const alert = await screen.findByRole('alert');
  await vi.waitFor(() => expect(alert.textContent).toContain(text));
  return alert.closest('.notice') as HTMLElement;
}

// A list whose first page loads and whose second page fails once, then answers with `second`.
function pagedList(second: Promise<TicketPage>) {
  let more = 0;
  vi.mocked(listTickets).mockImplementation(async (cursor = '') => {
    if (cursor === '') return { tickets: [ticket(1, 'First request')], next: 'c2' };
    more += 1;
    if (more === 1) throw new Error('offline');
    return second;
  });
}

describe('TicketList: Retry after "load more" failed', () => {
  it('asks again for the page that failed, keeps the pages on screen, and moves focus to the first new request', async () => {
    const second = held<TicketPage>();
    pagedList(second.promise);
    const { user } = renderDom(TicketList);
    await user.click(await screen.findByRole('button', { name: msg('tickets.loadMore') }));
    const notice = await failureNotice(msg('tickets.loadFailed'));

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));

    expect(vi.mocked(listTickets).mock.calls.map(([cursor]) => cursor)).toEqual(['', 'c2', 'c2']);
    expect(row('First request')).toBeTruthy();
    second.resolve({ tickets: [ticket(2, 'Second request')], next: '' });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(row('First request')).toBeTruthy();
    expect(document.activeElement).toBe(row('Second request'));
  });

  it('keeps the notice and its focused Retry when a reply arrives while the retry runs, and takes the retry\'s answer', async () => {
    const second = held<TicketPage>();
    pagedList(second.promise);
    const { user } = renderDom(TicketList);
    await user.click(await screen.findByRole('button', { name: msg('tickets.loadMore') }));
    const notice = await failureNotice(msg('tickets.loadFailed'));
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    await user.click(retry);

    unreadReplies.set(1);
    await vi.waitFor(() => expect(vi.mocked(listTickets).mock.calls.length).toBeGreaterThanOrEqual(3));

    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    second.resolve({ tickets: [ticket(2, 'Second request')], next: '' });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(row('First request')).toBeTruthy();
    expect(row('Second request')).toBeTruthy();
  });
});

describe('TicketList: Retry after the first page failed', () => {
  it('moves focus to the first request once the list loads', async () => {
    vi.mocked(listTickets)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ tickets: [ticket(1, 'First request')], next: '' });
    const { user } = renderDom(TicketList);
    const notice = await failureNotice(msg('tickets.loadFailed'));

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(document.activeElement).toBe(row('First request'));
  });
});

describe('TicketView: Retry while a reply arrives', () => {
  it('keeps the notice and its focused Retry, takes the retry\'s answer and moves focus to the request\'s title', async () => {
    const again = held<TicketThread>();
    const answered = { ...THREAD, ticket: { ...THREAD.ticket, subject: 'Crash on start, answered' } };
    vi.mocked(getTicket)
      .mockResolvedValueOnce(THREAD)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(again.promise)
      .mockResolvedValue(answered);
    const { user } = renderDom(TicketView, { number: 7 });
    await screen.findByRole('heading', { level: 1, name: 'Crash on start' });
    unreadReplies.set(1);
    const notice = await failureNotice(msg('tickets.ticketLoadFailed'));
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    await user.click(retry);

    unreadReplies.set(2);
    await Promise.resolve();

    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    again.resolve(answered);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    const title = screen.getByRole('heading', { level: 1 });
    expect(title.textContent).toBe('Crash on start, answered');
    expect(document.activeElement).toBe(title);
  });
});

describe('Support: where focus goes when the limits load on Retry', () => {
  it('on a request: to the reply field', async () => {
    vi.mocked(ticketLimits).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(LIMITS);
    const { user } = renderDom(TicketView, { number: 7 });
    const notice = await failureNotice(msg('tickets.limitsFailed'));

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(document.activeElement).toBe(screen.getByLabelText(msg('tickets.replyLabel'), { exact: false }));
  });

  it('on a new request: to the first control of the form', async () => {
    vi.mocked(ticketLimits).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(LIMITS);
    const { user, container } = renderDom(NewTicket, { initialTopic: 'launcher' });
    const notice = await failureNotice(msg('tickets.limitsFailed'));

    await user.click(within(notice).getByRole('button', { name: msg('common.retry') }));

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    const form = container.querySelector('.form') as HTMLElement;
    expect(form.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toBe(form.querySelector('button, input, textarea, [tabindex]'));
  });
});

// The notice of `text`, its live region as it is now, and its Retry.
async function failed(text: string) {
  const notice = await failureNotice(text);
  const region = notice.querySelector('.body') as HTMLElement;
  return { notice, region, before: region.firstElementChild, retry: within(notice).getByRole('button', { name: msg('common.retry') }) };
}

// After a Retry that failed again: the notice stays with its focused Retry, and its words come into its
// live region again as a new node, so they are said again.
async function saidAgain(seen: Awaited<ReturnType<typeof failed>>, text: string) {
  await vi.waitFor(() => expect(seen.retry.hasAttribute('aria-busy')).toBe(false));
  expect(seen.notice.isConnected).toBe(true);
  expect(document.activeElement).toBe(seen.retry);
  await vi.waitFor(() => expect(seen.region.firstElementChild).not.toBe(seen.before));
  expect(seen.region.firstElementChild?.textContent).toContain(text);
}

describe('Support: a Retry that fails again says so again', () => {
  it('the request list\'s first page', async () => {
    vi.mocked(listTickets).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(TicketList);
    const seen = await failed(msg('tickets.loadFailed'));

    await user.click(seen.retry);

    await saidAgain(seen, msg('tickets.loadFailed'));
  });

  it('the request list\'s later page', async () => {
    vi.mocked(listTickets).mockImplementation(async (cursor = '') => {
      if (cursor === '') return { tickets: [ticket(1, 'First request')], next: 'c2' };
      throw new Error('offline');
    });
    const { user } = renderDom(TicketList);
    await user.click(await screen.findByRole('button', { name: msg('tickets.loadMore') }));
    const seen = await failed(msg('tickets.loadFailed'));

    await user.click(seen.retry);

    await saidAgain(seen, msg('tickets.loadFailed'));
    expect(row('First request')).toBeTruthy();
  });

  it('a request', async () => {
    vi.mocked(getTicket).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(TicketView, { number: 7 });
    const seen = await failed(msg('tickets.ticketLoadFailed'));

    await user.click(seen.retry);

    await saidAgain(seen, msg('tickets.ticketLoadFailed'));
  });

  it('the limits of a reply', async () => {
    vi.mocked(ticketLimits).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(TicketView, { number: 7 });
    const seen = await failed(msg('tickets.limitsFailed'));

    await user.click(seen.retry);

    await saidAgain(seen, msg('tickets.limitsFailed'));
  });

  it('the limits of a new request', async () => {
    vi.mocked(ticketLimits).mockRejectedValue(new Error('offline'));
    const { user } = renderDom(NewTicket, { initialTopic: 'launcher' });
    const seen = await failed(msg('tickets.limitsFailed'));

    await user.click(seen.retry);

    await saidAgain(seen, msg('tickets.limitsFailed'));
  });
});

describe('TicketList: a reply while a Retry runs', () => {
  it('starts no refresh of its own over the retry', async () => {
    const again = held<TicketPage>();
    vi.mocked(listTickets)
      .mockResolvedValueOnce({ tickets: [ticket(1, 'First request')], next: '' })
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(again.promise)
      .mockResolvedValue({ tickets: [ticket(1, 'First request')], next: '' });
    const { user } = renderDom(TicketList);
    await screen.findByRole('button', { name: /First request/ });
    unreadReplies.set(1);
    const seen = await failed(msg('tickets.loadFailed'));
    await user.click(seen.retry);
    expect(listTickets).toHaveBeenCalledTimes(3);

    unreadReplies.set(2);
    await Promise.resolve();

    expect(listTickets).toHaveBeenCalledTimes(3);
    expect(seen.notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(seen.retry);
    again.resolve({ tickets: [ticket(1, 'First request')], next: '' });
    await vi.waitFor(() => expect(seen.notice.isConnected).toBe(false));
  });
});

describe('Support: a reply that came while a refresh was held back', () => {
  it('reloads the request list\'s first page once the Retry answers, keeping the later page', async () => {
    const second = held<TicketPage>();
    let firstPages = 0;
    let more = 0;
    vi.mocked(listTickets).mockImplementation(async (cursor = '') => {
      if (cursor === '') {
        firstPages += 1;
        return { tickets: [{ ...ticket(1, 'First request'), unread: firstPages > 1 }], next: 'c2' };
      }
      more += 1;
      if (more === 1) throw new Error('offline');
      return second.promise;
    });
    const { user } = renderDom(TicketList);
    await user.click(await screen.findByRole('button', { name: msg('tickets.loadMore') }));
    const seen = await failed(msg('tickets.loadFailed'));
    await user.click(seen.retry);

    unreadReplies.set(1);
    await Promise.resolve();
    expect(firstPages).toBe(1);
    second.resolve({ tickets: [ticket(2, 'Second request')], next: '' });

    await vi.waitFor(() => expect(firstPages).toBe(2));
    await vi.waitFor(() => expect(within(row('First request')).getByRole('img', { name: msg('tickets.unreadMark') })).toBeTruthy());
    expect(row('Second request')).toBeTruthy();
  });

  it('reloads a request once its Retry answers', async () => {
    const again = held<TicketThread>();
    const answered = { ...THREAD, messages: [{ id: 'r1', author: 'staff', body: 'We are on it.', createdAt: '2026-11-02T10:00:00Z', logBundleId: null }] } as unknown as TicketThread;
    vi.mocked(getTicket)
      .mockResolvedValueOnce(THREAD)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(again.promise)
      .mockResolvedValue(answered);
    const { user } = renderDom(TicketView, { number: 7 });
    await screen.findByRole('heading', { level: 1, name: 'Crash on start' });
    unreadReplies.set(1);
    const seen = await failed(msg('tickets.ticketLoadFailed'));
    await user.click(seen.retry);

    unreadReplies.set(2);
    await Promise.resolve();
    expect(getTicket).toHaveBeenCalledTimes(3);
    again.resolve(THREAD);

    await vi.waitFor(() => expect(getTicket).toHaveBeenCalledTimes(4));
    expect(await screen.findByText('We are on it.')).toBeTruthy();
  });
});


describe('TicketList: "Load more" that brings the last page', () => {
  it('hands focus to the first request it brought', async () => {
    vi.mocked(listTickets).mockImplementation(async (cursor = '') =>
      cursor === '' ? { tickets: [ticket(1, 'First request')], next: 'c2' } : { tickets: [ticket(2, 'Second request')], next: '' },
    );
    const { user } = renderDom(TicketList);
    (await screen.findByRole('button', { name: msg('tickets.loadMore') })).focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(screen.queryByRole('button', { name: msg('tickets.loadMore') })).toBeNull());
    expect(document.activeElement).toBe(row('Second request'));
  });

  // A pointer press where a click does not focus a button (the macOS WebKit view).
  it('leaves focus alone after a press that held none', async () => {
    vi.mocked(listTickets).mockImplementation(async (cursor = '') =>
      cursor === '' ? { tickets: [ticket(1, 'First request')], next: 'c2' } : { tickets: [ticket(2, 'Second request')], next: '' },
    );
    renderDom(TicketList);
    const more = await screen.findByRole('button', { name: msg('tickets.loadMore') });
    expect(document.activeElement).toBe(document.body);

    await fireEvent.click(more);

    await vi.waitFor(() => expect(row('Second request')).toBeTruthy());
    await tick();
    expect(document.activeElement).toBe(document.body);
  });

  it('keeps focus on "Load more" while pages remain', async () => {
    vi.mocked(listTickets).mockImplementation(async (cursor = '') =>
      cursor === '' ? { tickets: [ticket(1, 'First request')], next: 'c2' } : { tickets: [ticket(2, 'Second request')], next: 'c3' },
    );
    const { user } = renderDom(TicketList);
    const more = await screen.findByRole('button', { name: msg('tickets.loadMore') });
    more.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(row('Second request')).toBeTruthy());
    await tick();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('tickets.loadMore') }));
  });
});
