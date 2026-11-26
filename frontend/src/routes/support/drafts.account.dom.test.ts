import { cleanup, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError, type CurrentUser } from '../../lib/services/account';
import { createTicket, getTicket, replyTicket, ticketLimits, unreadTickets } from '../../lib/services/support';
import { navigate, resetHistory, route } from '../../lib/stores/router';
import { unreadReplies } from '../../lib/stores/support';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { held, LIMITS, message, thread, ticket } from '../../lib/testing/supportTesting';
import { keptReply, keptRequest, onReplyBack, onRequestBack } from './drafts';
import Support from './Support.svelte';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('../../lib/testing/supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('../../lib/testing/supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('../../lib/testing/supportTesting')).accountOpeners(await original()));

// A send started under one account and answered after another signed in: nothing of the first
// account's (its text, its request's subject or reference) reaches the second.

const TICKET = ticket(7, 'Game will not start', { topic: 'launch' });
const player = (id: string) => ({ id, username: id, displayName: id }) as CurrentUser;
let players = 0;
let a = '';

function signIn(id: string) {
  currentUser.set(player(id));
  authState.set('authenticated');
}

function signOut() {
  currentUser.set(null);
  authState.set('unauthenticated');
}

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  unreadReplies.set(0);
  navigate('support', { ticket: '7' });
  a = `a-${++players}`;
  signIn(a);
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(unreadTickets).mockResolvedValue(0);
  vi.mocked(getTicket).mockResolvedValue(thread(TICKET, []));
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [ticketLimits, getTicket, replyTicket, createTicket, unreadTickets]) vi.mocked(mock).mockReset();
  toasts.set([]);
  currentUser.set(null);
  authState.set('bootstrapping');
  resetHistory();
});

const replyBox = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.replyLabel')) }) as HTMLTextAreaElement;
const subjectField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.subjectLabel')) }) as HTMLInputElement;
const bodyField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.bodyLabel')) }) as HTMLTextAreaElement;
const settled = async () => {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await new Promise((resolve) => setTimeout(resolve, 0));
};
const said = () => get(toasts).map((t) => [t.kind, t.message]);

async function opened() {
  const view = renderDom(TicketView, { number: 7 });
  await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
  return view;
}

async function newRequest() {
  const view = renderDom(Support, { view: 'new' });
  await screen.findByRole('button', { name: msg('tickets.submit') });
  return view;
}

/** A's reply on its way, the request left. */
async function replyOnItsWay() {
  const first = held<ReturnType<typeof message>>();
  vi.mocked(replyTicket).mockReturnValueOnce(first.promise);
  const { user } = await opened();
  await user.type(replyBox(), 'From A.');
  await user.keyboard('{Control>}{Enter}{/Control}');
  await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
  cleanup();
  return first;
}

/** A's new request on its way, the form left. */
async function requestOnItsWay() {
  const first = held<ReturnType<typeof ticket>>();
  vi.mocked(createTicket).mockReturnValueOnce(first.promise);
  const { user } = await newRequest();
  await user.type(subjectField(), 'Private matter of A');
  await user.type(bodyField(), 'Text of A.');
  if (screen.getByRole('switch', { name: msg('tickets.attachLogs') }).getAttribute('aria-checked') === 'true') {
    await user.click(screen.getByRole('switch', { name: msg('tickets.attachLogs') }));
  }
  await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
  await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
  cleanup();
  return first;
}

function hearing() {
  const heard: unknown[] = [];
  const offs = [
    onReplyBack((...back) => heard.push(back)),
    onRequestBack((back) => {
      heard.push(back);
      return false;
    }),
  ];
  return { heard, off: () => offs.forEach((off) => off()) };
}

describe('a reply of A that fails once B is signed in', () => {
  it('is not in B\'s draft or box, no form hears of it, and no toast says it', async () => {
    const first = await replyOnItsWay();
    signOut();
    signIn(`${a}-b`);
    await opened();
    const { heard, off } = hearing();
    try {
      first.reject(new AccountError('network_error'));
      await settled();
      expect({ box: replyBox().value, kept: keptReply(7), heard, said: said() }).toEqual({ box: '', kept: '', heard: [], said: [] });
    } finally {
      off();
    }
  });

  it('comes back when A signed out and in again as A before it failed', async () => {
    const first = await replyOnItsWay();
    signOut();
    signIn(a);
    first.reject(new AccountError('network_error'));
    await settled();
    expect(said()).toEqual([['danger', msg('tickets.replyNotSentKept', { subject: 'Game will not start' })]]);
    await opened();
    expect(replyBox().value).toBe('From A.');
  });

  it('that is sent once B is signed in says nothing', async () => {
    const first = await replyOnItsWay();
    signOut();
    signIn(`${a}-b`);
    const reads = vi.mocked(getTicket).mock.calls.length;
    first.resolve(message(4, 'user', 'From A.', 'x'));
    await settled();
    expect({ said: said(), reads: vi.mocked(getTicket).mock.calls.length - reads }).toEqual({ said: [], reads: 0 });
  });

  it('that fails with no one signed in is kept for no one and says nothing', async () => {
    const first = await replyOnItsWay();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    expect(said()).toEqual([]);
    signIn(`${a}-b`);
    expect(keptReply(7)).toBe('');
  });
});

describe('a new request of A that fails once B is signed in', () => {
  it('is not in B\'s draft or form, no form hears of it, and no toast names it', async () => {
    const first = await requestOnItsWay();
    signOut();
    signIn(`${a}-b`);
    await newRequest();
    const { heard, off } = hearing();
    try {
      first.reject(new AccountError('network_error'));
      await settled();
      expect({ subject: subjectField().value, body: bodyField().value, kept: keptRequest(), heard, said: said() }).toEqual({
        subject: '',
        body: '',
        kept: null,
        heard: [],
        said: [],
      });
    } finally {
      off();
    }
  });

  it('comes back when A signed out and in again as A before it failed', async () => {
    const first = await requestOnItsWay();
    signOut();
    signIn(a);
    first.reject(new AccountError('network_error'));
    await settled();
    expect(said()).toEqual([['danger', msg('tickets.requestNotSentKept', { subject: 'Private matter of A' })]]);
    await newRequest();
    expect([subjectField().value, bodyField().value]).toEqual(['Private matter of A', 'Text of A.']);
  });

  it('that is created once B is signed in says nothing of it and opens nothing', async () => {
    const first = await requestOnItsWay();
    signOut();
    signIn(`${a}-b`);
    navigate('library');
    first.resolve(ticket(41, 'Private matter of A'));
    await settled();
    expect({ said: said(), route: get(route).name }).toEqual({ said: [], route: 'library' });
  });

  it('that is created while A is still signed in is said as before', async () => {
    const first = await requestOnItsWay();
    first.resolve(ticket(41, 'Private matter of A'));
    await settled();
    expect(said()).toEqual([['success', msg('tickets.created', { ref: 'T-41' })]]);
  });
});
