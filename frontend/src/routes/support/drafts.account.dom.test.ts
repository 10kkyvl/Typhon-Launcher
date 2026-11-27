import { cleanup, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError, type CurrentUser } from '../../lib/services/account';
import { createTicket, getTicket, replyTicket, ticketLimits, unreadTickets } from '../../lib/services/support';
import { navigate, resetHistory, route } from '../../lib/stores/router';
import { unreadReplies } from '../../lib/stores/support';
import { holdToasts, toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { held, LIMITS, message, thread, ticket } from '../../lib/testing/supportTesting';
import { keptReply, keptRequest, onReplyBack, onRequestBack, spareRequest } from './drafts';
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
  holdToasts(false);
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
    onReplyBack({ ticket: 7, empty: () => true, sending: () => false, shown: () => true, take: (text) => heard.push(text) }),
    onRequestBack({
      by: {},
      free: () => {
        heard.push('asked');
        return false;
      },
      sending: () => false,
      shown: () => true,
      take: (back) => heard.push(back),
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
    expect(said()).toEqual([['danger', msg('tickets.replyNotSentSaved', { subject: 'Game will not start' })]]);
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

  it('that is sent once B is signed in reads nothing, even with a reload held back for it', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValueOnce(first.promise);
    const { user } = await opened();
    await user.type(replyBox(), 'From A.');
    await user.keyboard('{Control>}{Enter}{/Control}');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    unreadReplies.set(1);
    await settled();
    cleanup();
    signOut();
    signIn(`${a}-b`);
    const reads = vi.mocked(getTicket).mock.calls.length;
    first.resolve(message(4, 'user', 'From A.', 'x'));
    await settled();
    expect({ said: said(), reads: vi.mocked(getTicket).mock.calls.length - reads }).toEqual({ said: [], reads: 0 });
  });

  it('that fails once B is signed in, its request still on screen, reads nothing for a reload held back for it', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValueOnce(first.promise);
    const { user } = await opened();
    await user.type(replyBox(), 'From A.');
    await user.keyboard('{Control>}{Enter}{/Control}');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    unreadReplies.set(1);
    await settled();
    signOut();
    signIn(`${a}-b`);
    const reads = vi.mocked(getTicket).mock.calls.length;
    first.reject(new AccountError('network_error'));
    await settled();
    expect({ said: said(), reads: vi.mocked(getTicket).mock.calls.length - reads }).toEqual({ said: [], reads: 0 });
  });

  it('that is sent after its request was left reads nothing, under the same account too', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValueOnce(first.promise);
    const { user } = await opened();
    await user.type(replyBox(), 'From A.');
    await user.keyboard('{Control>}{Enter}{/Control}');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    unreadReplies.set(1);
    await settled();
    cleanup();
    const reads = vi.mocked(getTicket).mock.calls.length;
    first.resolve(message(4, 'user', 'From A.', 'x'));
    await settled();
    expect({ said: said(), reads: vi.mocked(getTicket).mock.calls.length - reads }).toEqual({ said: [['success', msg('tickets.replySent')]], reads: 0 });
  });

  it('that fails with no one signed in says nothing then, and is in the box when A signs in again', async () => {
    const first = await replyOnItsWay();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    const signedOut = said();
    signIn(a);
    await opened();
    expect({ signedOut, box: replyBox().value, said: said() }).toEqual({ signedOut: [], box: 'From A.', said: [] });
  });

  it('that fails with no one signed in is not B\'s: B finds an empty box, and A finds nothing after B', async () => {
    const first = await replyOnItsWay();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    signIn(`${a}-b`);
    await opened();
    const underB = { box: replyBox().value, kept: keptReply(7), said: said() };
    cleanup();
    signOut();
    signIn(a);
    expect({ underB, underA: keptReply(7) }).toEqual({ underB: { box: '', kept: '', said: [] }, underA: '' });
  });

  it('that fails with no one signed in leaves a reply A kept in that request as it was', async () => {
    const first = await replyOnItsWay();
    const { user } = await opened();
    await user.type(replyBox(), 'Kept meanwhile.');
    cleanup();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    signIn(a);
    expect(keptReply(7)).toBe('Kept meanwhile.');
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
    expect(said()).toEqual([['danger', msg('tickets.requestNotSentSaved', { subject: 'Private matter of A' })]]);
    await newRequest();
    expect([subjectField().value, bodyField().value]).toEqual(['Private matter of A', 'Text of A.']);
  });

  it('that fails with no one signed in says nothing then, and is in the form when A signs in again', async () => {
    const first = await requestOnItsWay();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    const signedOut = said();
    signIn(a);
    await newRequest();
    expect({ signedOut, form: [subjectField().value, bodyField().value], said: said() }).toEqual({
      signedOut: [],
      form: ['Private matter of A', 'Text of A.'],
      said: [],
    });
  });

  it('that fails with no one signed in is not B\'s: B finds an empty form, and A finds nothing after B', async () => {
    const first = await requestOnItsWay();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    signIn(`${a}-b`);
    await newRequest();
    const underB = { form: [subjectField().value, bodyField().value], kept: keptRequest(), spare: spareRequest(), said: said() };
    cleanup();
    signOut();
    signIn(a);
    expect({ underB, underA: [keptRequest(), spareRequest()] }).toEqual({
      underB: { form: ['', ''], kept: null, spare: null, said: [] },
      underA: [null, null],
    });
  });

  it('that fails with no one signed in after B kept a draft and signed out: B\'s draft stays, nothing of A\'s is kept', async () => {
    const first = await requestOnItsWay();
    signOut();
    signIn(`${a}-b`);
    const { user } = await newRequest();
    await user.type(subjectField(), 'Draft of B');
    await user.type(bodyField(), 'Text of B.');
    cleanup();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    signIn(`${a}-b`);
    const underB = [keptRequest()?.subject ?? null, spareRequest()?.subject ?? null];
    signOut();
    signIn(a);
    expect({ underB, underA: [keptRequest(), spareRequest()], said: said() }).toEqual({ underB: ['Draft of B', null], underA: [null, null], said: [] });
  });

  it('that fails with no one signed in, a draft of A kept, is offered beside it when A signs in again', async () => {
    const first = await requestOnItsWay();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Typed meanwhile');
    cleanup();
    signOut();
    first.reject(new AccountError('unauthenticated'));
    await settled();
    signIn(a);
    await newRequest();
    const offered = screen.getByRole('button', { name: msg('tickets.draftRestore') }).closest('.notice')?.textContent ?? '';
    expect({ subject: subjectField().value, names: offered.includes('Private matter of A') }).toEqual({ subject: 'Typed meanwhile', names: true });
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

describe("A's support toasts once A signs out", () => {
  /** A's request fails beside text A typed: its toast names it and offers Copy. */
  async function offeredToast() {
    const first = await requestOnItsWay();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Typed meanwhile');
    first.reject(new AccountError('network_error'));
    await settled();
    const action = get(toasts)[0]?.action;
    if (!action) throw new Error('the toast offers no Copy');
    return action;
  }

  /** Runs `action` over a clipboard the test reads; answers what was written. */
  async function copied(action: { run: () => void }) {
    const written: string[] = [];
    const before = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: async (text: string) => void written.push(text) }, configurable: true });
    try {
      action.run();
      await settled();
    } finally {
      if (before) Object.defineProperty(navigator, 'clipboard', before);
      else Reflect.deleteProperty(navigator, 'clipboard');
    }
    return written;
  }

  it('are gone at the sign-out, even while a dialog holds toasts with an action, and B sees none of them', async () => {
    await offeredToast();
    const raised = said();
    holdToasts(true);
    cleanup();
    signOut();
    const signedOut = said();
    signIn(`${a}-b`);
    holdToasts(false);
    expect({ raised, signedOut, underB: said() }).toEqual({
      raised: [['danger', msg('tickets.requestNotSentOffered', { subject: 'Private matter of A' })]],
      signedOut: [],
      underB: [],
    });
  });

  it('copy nothing and say nothing under B, even when the action is run after all', async () => {
    const action = await offeredToast();
    cleanup();
    signOut();
    signIn(`${a}-b`);
    expect({ written: await copied(action), said: said() }).toEqual({ written: [], said: [] });
  });

  it('a reply not kept, and a request created, are gone at the sign-out too', async () => {
    const first = await replyOnItsWay();
    const view = await opened();
    await view.user.type(replyBox(), 'Newer.');
    first.reject(new AccountError('network_error'));
    await settled();
    cleanup();
    const created = await requestOnItsWay();
    created.resolve(ticket(41, 'Private matter of A'));
    await settled();
    const raised = said();
    signOut();
    expect({ raised, signedOut: said() }).toEqual({
      raised: [
        ['danger', msg('tickets.replyNotSentNotKept', { subject: 'Game will not start' })],
        ['success', msg('tickets.created', { ref: 'T-41' })],
      ],
      signedOut: [],
    });
  });

  it('are still watched once nothing of A is kept any more: a reply toast with Copy is gone at the sign-out', async () => {
    const first = await replyOnItsWay();
    const view = await opened();
    await view.user.type(replyBox(), 'Newer.');
    first.reject(new AccountError('network_error'));
    await settled();
    const raised = said();
    await view.user.clear(replyBox());
    const kept = keptReply(7);
    cleanup();
    signOut();
    expect({ raised, kept, signedOut: said() }).toEqual({
      raised: [['danger', msg('tickets.replyNotSentNotKept', { subject: 'Game will not start' })]],
      kept: '',
      signedOut: [],
    });
  });

  it('stay, and copy, while A is signed in', async () => {
    const action = await offeredToast();
    expect({ written: await copied(action), said: said() }).toEqual({
      written: ['Private matter of A\n\nText of A.'],
      said: [
        ['danger', msg('tickets.requestNotSentOffered', { subject: 'Private matter of A' })],
        ['success', msg('tickets.textCopied')],
      ],
    });
  });
});
