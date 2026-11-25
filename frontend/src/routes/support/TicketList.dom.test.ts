import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { listTickets, unreadTickets } from '../../lib/services/support';
import { refreshUnread, unreadReplies } from '../../lib/stores/support';
import { resetHistory, route } from '../../lib/stores/router';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { page, ticket, words } from '../../lib/testing/supportTesting';
import TicketList from './TicketList.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('../../lib/testing/supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('../../lib/testing/supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('../../lib/testing/supportTesting')).accountOpeners(await original()));

const OPEN = ticket(3, 'Game will not start', { status: 'open', topic: 'launch', updatedAt: new Date().toISOString() });
const ANSWERED = ticket(2, 'Download stuck at 99%', { status: 'waiting_user', topic: 'install', unread: true });
const RESOLVED = ticket(1, 'Cannot sign in', { status: 'resolved', topic: 'account' });

beforeEach(() => {
  resetBindings();
  unreadReplies.set(0);
  vi.mocked(listTickets).mockResolvedValue(page([OPEN, ANSWERED, RESOLVED]));
});

afterEach(() => {
  // Unmounted first: a screen still mounted would answer the resets below with reads of its own.
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(listTickets).mockReset();
  vi.mocked(unreadTickets).mockReset();
  unreadReplies.set(0);
  authState.set('bootstrapping');
  resetHistory();
});

const row = (subject: string) => screen.getByRole('button', { name: new RegExp(subject) });

describe('TicketList: the requests, one row each', () => {
  it('names each row by its subject and says its reference, topic, last change and status', async () => {
    renderDom(TicketList);
    const open = await screen.findByRole('button', { name: /Game will not start/ });

    expect(words(open)).toContain('T-3');
    expect(words(open)).toContain(msg('tickets.topicLaunch'));
    expect(words(open)).toContain(msg('tickets.updated', { when: msg('format.today') }));
    expect(words(open)).toContain(msg('tickets.statusOpen'));
    expect(words(row('Download stuck'))).toContain(msg('tickets.statusWaitingUser'));
    expect(words(row('Cannot sign in'))).toContain(msg('tickets.statusResolved'));
  });

  it('marks only a request support has answered since it was read', async () => {
    renderDom(TicketList);
    await screen.findByRole('button', { name: /Game will not start/ });

    expect(within(row('Download stuck')).getByRole('img', { name: msg('tickets.unreadMark') })).toBeTruthy();
    expect(within(row('Game will not start')).queryByRole('img')).toBeNull();
    expect(within(row('Cannot sign in')).queryByRole('img')).toBeNull();
  });

  it('lists the requests in the order Go sends them, as a list', async () => {
    renderDom(TicketList);
    await screen.findByRole('button', { name: /Game will not start/ });

    const list = screen.getByRole('list');
    expect(within(list).getAllByRole('listitem').map((item) => within(item).getByRole('button'))).toEqual([
      row('Game will not start'),
      row('Download stuck'),
      row('Cannot sign in'),
    ]);
  });
});

describe('TicketList: where its buttons lead', () => {
  it('opens a request by its number', async () => {
    const { user } = renderDom(TicketList);
    await user.click(await screen.findByRole('button', { name: /Download stuck/ }));

    expect(get(route)).toMatchObject({ name: 'support', params: { ticket: '2' } });
  });

  it('opens the new request form from the header', async () => {
    const { user } = renderDom(TicketList);
    await screen.findByRole('button', { name: /Game will not start/ });

    await user.click(screen.getByRole('button', { name: msg('tickets.new') }));

    expect(get(route)).toMatchObject({ name: 'support', params: { view: 'new' } });
  });

  it('opens the service status page, and says so when it cannot', async () => {
    const { openStatusPage } = await import('../../lib/services/account');
    vi.mocked(openStatusPage).mockRejectedValueOnce(new AccountError('network_error'));
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { toasts } = await import('../../lib/stores/toasts');
    toasts.set([]);
    const { user } = renderDom(TicketList);
    await screen.findByRole('button', { name: /Game will not start/ });

    await user.click(screen.getByRole('button', { name: msg('tickets.statusPage') }));

    expect(openStatusPage).toHaveBeenCalledTimes(1);
    expect(get(toasts).map((item) => [item.kind, item.message])).toEqual([['danger', msg('tickets.statusOpenFailed')]]);
    error.mockRestore();
    toasts.set([]);
  });
});

describe('TicketList: an empty list', () => {
  it('says there are no requests yet, with the way to write one, once a read found none', async () => {
    vi.mocked(listTickets).mockResolvedValue(page([]));
    const { user } = renderDom(TicketList);

    const empty = await screen.findByRole('heading', { level: 2, name: msg('tickets.emptyTitle') });
    expect(empty).toBeTruthy();
    expect(screen.queryByRole('list')).toBeNull();
    const buttons = screen.getAllByRole('button', { name: msg('tickets.new') });
    await user.click(buttons[buttons.length - 1]);
    expect(get(route)).toMatchObject({ name: 'support', params: { view: 'new' } });
  });

  it('does not say so while the first read runs or after it failed', async () => {
    let fail: (err: unknown) => void = () => {};
    vi.mocked(listTickets).mockReturnValue(new Promise((_ok, reject) => (fail = reject)));
    renderDom(TicketList);
    expect(screen.queryByText(msg('tickets.emptyTitle'))).toBeNull();

    fail(new AccountError('network_error'));
    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('state.accountErrorNetwork')));
    expect(screen.queryByText(msg('tickets.emptyTitle'))).toBeNull();
  });
});

describe('TicketList: a failed read says why by its code', () => {
  it.each([
    ['network_error', msg('state.accountErrorNetwork')],
    ['rate_limited', msg('state.accountErrorRateLimited')],
    ['ticket_not_found', msg('errSupport.notFound')],
  ])('%s', async (code, text) => {
    vi.mocked(listTickets).mockRejectedValue(new AccountError(code));
    renderDom(TicketList);

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(text));
  });

  it('an unknown failure says the list could not be read', async () => {
    vi.mocked(listTickets).mockRejectedValue(new AccountError('something_new'));
    renderDom(TicketList);

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('tickets.loadFailed')));
    expect(alert.textContent).not.toContain('something_new');
  });
});

describe('TicketList: truthful states', () => {
  it('draws no list while the first read has found nothing to list', async () => {
    vi.mocked(listTickets).mockRejectedValue(new AccountError('network_error'));
    renderDom(TicketList);
    await screen.findByRole('alert');
    expect(screen.queryByRole('list')).toBeNull();
  });

  it('a session gone: says sign-in is needed and offers it instead of a Retry that cannot help', async () => {
    authState.set('authenticated');
    vi.mocked(listTickets).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = renderDom(TicketList);

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('state.accountErrorUnauthenticated')));
    const notice = alert.closest('.notice') as HTMLElement;
    expect(within(notice).queryByRole('button', { name: msg('common.retry') })).toBeNull();

    await user.click(within(notice).getByRole('button', { name: msg('tickets.signInAgain') }));

    expect(get(authState)).toBe('unauthenticated');
  });
});

describe('TicketList: the unread count', () => {
  it('reads the list again when the count changes, keeping the rows meanwhile', async () => {
    renderDom(TicketList);
    await screen.findByRole('button', { name: /Game will not start/ });
    vi.mocked(listTickets).mockResolvedValue(page([{ ...OPEN, unread: true }, ANSWERED, RESOLVED]));

    unreadReplies.set(2);

    await vi.waitFor(() => expect(within(row('Game will not start')).getByRole('img', { name: msg('tickets.unreadMark') })).toBeTruthy());
    expect(listTickets).toHaveBeenCalledTimes(2);
  });

  it('keeps what it shows and says nothing on screen when the background poll fails', async () => {
    authState.set('authenticated');
    vi.mocked(unreadTickets).mockRejectedValue(new AccountError('network_error'));
    renderDom(TicketList);
    await screen.findByRole('button', { name: /Game will not start/ });

    await refreshUnread();
    await tick();

    expect(screen.queryByRole('alert')).toBeNull();
    expect(row('Game will not start')).toBeTruthy();
    expect(listTickets).toHaveBeenCalledTimes(1);
  });
});
