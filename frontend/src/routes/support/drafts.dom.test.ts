import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError, type CurrentUser } from '../../lib/services/account';
import { createTicket, getTicket, replyTicket, ticketLimits, unreadTickets } from '../../lib/services/support';
import { navigate, resetHistory } from '../../lib/stores/router';
import { unreadReplies } from '../../lib/stores/support';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { LIMITS, message, thread, ticket } from '../../lib/testing/supportTesting';
import { forgetReply, forgetRequest, keepReply, keepRequest, keptReply, keptRequest, onReplyBack, onRequestBack, returnReply, returnRequest, type RequestDraft } from './drafts';
import NewTicket from './NewTicket.svelte';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('../../lib/testing/supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('../../lib/testing/supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('../../lib/testing/supportTesting')).accountOpeners(await original()));

// A player of their own per test: what one test leaves kept is another account's for the next.
let players = 0;
let me = '';
const player = (id: string) => ({ id, username: id, displayName: id }) as CurrentUser;

function signIn(id: string) {
  currentUser.set(player(id));
  authState.set('authenticated');
}

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  unreadReplies.set(0);
  navigate('support', { view: 'new' });
  me = `player-${++players}`;
  signIn(me);
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(unreadTickets).mockResolvedValue(0);
  vi.mocked(getTicket).mockImplementation(async (number: number) => thread(ticket(number, `Request ${number}`), []));
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [ticketLimits, unreadTickets, getTicket, replyTicket, createTicket]) vi.mocked(mock).mockReset();
  toasts.set([]);
  unreadReplies.set(0);
  currentUser.set(null);
  authState.set('bootstrapping');
  resetHistory();
});

const subjectField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.subjectLabel')) }) as HTMLInputElement;
const bodyField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.bodyLabel')) }) as HTMLTextAreaElement;
const replyBox = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.replyLabel')) }) as HTMLTextAreaElement;
const logsSwitch = () => screen.getByRole('switch', { name: msg('tickets.attachLogs') });
const topicBox = () => screen.getByRole('combobox', { name: msg('tickets.topicLabel') });

async function newRequest() {
  const view = renderDom(NewTicket, {});
  await screen.findByRole('button', { name: msg('tickets.submit') });
  return view;
}

async function request(number: number) {
  const view = renderDom(TicketView, { number });
  await screen.findByRole('heading', { level: 1, name: `Request ${number}` });
  return view;
}

/** The form as a player leaves it: the logs switch turned off by hand, another topic, a subject and a text. */
async function typedRequest() {
  const view = await newRequest();
  await view.user.click(logsSwitch());
  await view.user.click(topicBox());
  await view.user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
  await view.user.type(subjectField(), 'Cannot sign in');
  await view.user.type(bodyField(), 'The code never arrives.');
  return view;
}

/** The send finds the session gone and the player presses "Sign in again": the shell goes, and the form with it. */
async function sessionGoneOnSend(user: ReturnType<typeof renderDom>['user'], send: () => HTMLElement) {
  await user.click(send());
  const alert = await screen.findByRole('alert');
  await vi.waitFor(() => expect(alert.textContent).toContain(msg('state.accountErrorUnauthenticated')));
  await user.click(within(alert.closest('.notice') as HTMLElement).getByRole('button', { name: msg('tickets.signInAgain') }));
  expect(get(authState)).toBe('unauthenticated');
  expect(get(currentUser)).toBeNull();
  cleanup();
}

function expectRequest(expected: { topic: string; logs: 'true' | 'false'; subject: string; body: string }) {
  expect({
    topic: (topicBox().textContent ?? '').trim(),
    logs: logsSwitch().getAttribute('aria-checked'),
    subject: subjectField().value,
    body: bodyField().value,
  }).toEqual(expected);
}

const LEFT = { topic: msg('tickets.topicAccount'), logs: 'false', subject: 'Cannot sign in', body: 'The code never arrives.' } as const;
const EMPTY = { topic: msg('tickets.topicInstall'), logs: 'true', subject: '', body: '' } as const;

describe('a new request left unsent', () => {
  it('is in the form again after "Sign in again" and the sign-in of the same player', async () => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = await typedRequest();
    await sessionGoneOnSend(user, () => screen.getByRole('button', { name: msg('tickets.submit') }));

    signIn(me);
    await newRequest();
    expectRequest(LEFT);
  });

  it('is gone once it is sent', async () => {
    vi.mocked(createTicket).mockResolvedValue(ticket(21, 'Cannot sign in'));
    const { user } = await typedRequest();
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    cleanup();

    await newRequest();
    expectRequest(EMPTY);
  });

  it('is gone once the player cancels the form', async () => {
    const { user } = await typedRequest();
    await user.click(screen.getByRole('button', { name: msg('common.cancel') }));
    cleanup();

    await newRequest();
    expectRequest(EMPTY);
  });

  it('is kept when the player only leaves the form, and the form opens with it', async () => {
    await typedRequest();
    cleanup();

    await newRequest();
    expectRequest(LEFT);
  });

  it('is not shown to another account, and is dropped when that account signs in', async () => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = await typedRequest();
    await sessionGoneOnSend(user, () => screen.getByRole('button', { name: msg('tickets.submit') }));

    signIn(`${me}-other`);
    await newRequest();
    expectRequest(EMPTY);
    cleanup();

    signIn(me);
    await newRequest();
    expectRequest(EMPTY);
  });

  it('is never shown to a guest, nor kept from what a guest types', async () => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = await typedRequest();
    await sessionGoneOnSend(user, () => screen.getByRole('button', { name: msg('tickets.submit') }));

    authState.set('guest');
    const guest = await newRequest();
    expectRequest(EMPTY);
    await guest.user.type(subjectField(), 'Typed as a guest');
    cleanup();
    await newRequest();
    expectRequest(EMPTY);
    cleanup();

    // What the player typed signed in is still theirs once they sign in again.
    signIn(me);
    await newRequest();
    expectRequest(LEFT);
  });
});

describe('a reply left unsent', () => {
  it('is in its own request again after "Sign in again", and only there', async () => {
    vi.mocked(replyTicket).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = await request(7);
    await user.type(replyBox(), 'Still closes.');
    await sessionGoneOnSend(user, () => screen.getByRole('button', { name: msg('tickets.reply') }));

    signIn(me);
    await request(8);
    expect(replyBox().value).toBe('');
    cleanup();
    await request(7);
    expect(replyBox().value).toBe('Still closes.');
  });

  it('is gone once it is sent', async () => {
    vi.mocked(replyTicket).mockResolvedValue(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z'));
    const { user } = await request(7);
    await user.type(replyBox(), 'Still closes.');
    await user.click(screen.getByRole('button', { name: msg('tickets.reply') }));
    await vi.waitFor(() => expect(replyBox().value).toBe(''));
    cleanup();

    await request(7);
    expect(replyBox().value).toBe('');
  });

  it('is not shown to another account', async () => {
    const { user } = await request(7);
    await user.type(replyBox(), 'Still closes.');
    cleanup();

    signIn(`${me}-other`);
    await request(7);
    expect(replyBox().value).toBe('');
  });
});

describe('the kept drafts themselves', () => {
  const as = (id: string | null) => currentUser.set(id ? player(id) : null);
  const form = {};
  const DRAFT: RequestDraft = {
    topic: 'launch',
    game: { igdbId: 42, title: 'A game' },
    subject: 'Will not start',
    body: 'Closes after the logo.',
    attachLogs: false,
    logsTouched: true,
  };

  it('give the player back the request they left, game and logs switch included, as a copy', () => {
    const game = { igdbId: 42, title: 'A game' };
    keepRequest({ ...DRAFT, game }, form);
    game.title = 'Changed after';
    expect(keptRequest()).toEqual(DRAFT);

    const back = keptRequest();
    if (back) back.subject = 'Changed by a reader';
    expect(keptRequest()?.subject).toBe(DRAFT.subject);
  });

  it('are dropped as soon as another account signs in, before anything is read', () => {
    keepRequest(DRAFT, form);
    keepReply(7, 'Still closes.');
    as(null);
    as(`${me}-other`);
    as(me);
    expect(keptRequest()).toBeNull();
    expect(keptReply(7)).toBe('');
  });

  it('keep a reply per request, and forget each on its own', () => {
    keepReply(7, 'For seven.');
    keepReply(8, 'For eight.');
    keepRequest(DRAFT, form);
    forgetReply(7);
    expect([keptReply(7), keptReply(8)]).toEqual(['', 'For eight.']);
    forgetRequest(form);
    expect(keptRequest()).toBeNull();
    expect(keptReply(8)).toBe('For eight.');
  });

  // Counted through the store's own subscribe: a write that subscribed again would leave one more
  // live subscription for every key the player presses.
  it('watch the account once however often the player types, and stop when nothing is kept', () => {
    const real = currentUser.subscribe;
    let live = 0;
    const spy = vi.spyOn(currentUser, 'subscribe').mockImplementation((run, invalidate) => {
      live += 1;
      const stop = real(run, invalidate);
      return () => {
        live -= 1;
        stop();
      };
    });
    try {
      for (let i = 1; i <= 5; i++) {
        keepReply(7, `Text ${i}`);
        keepRequest({ ...DRAFT, body: `Text ${i}` }, form);
      }
      expect(live).toBe(1);
      forgetReply(7);
      forgetRequest(form);
      expect(live).toBe(0);
    } finally {
      spy.mockRestore();
    }
  });

  it('forget a request only for the form that wrote it', () => {
    keepRequest(DRAFT, form);
    forgetRequest({});
    expect(keptRequest()).toEqual(DRAFT);
  });

  it('take the text of a failed send back only while nothing newer is kept, and tell an open form', () => {
    const heard: RequestDraft[] = [];
    const off = onRequestBack((back) => heard.push(back));
    try {
      expect(returnRequest(DRAFT, form, me)).toBe(true);
      expect(keptRequest()).toEqual(DRAFT);
      const newer = { ...DRAFT, subject: 'Written after' };
      keepRequest(newer, {});
      expect(returnRequest(DRAFT, form, me)).toBe(false);
      expect(keptRequest()).toEqual(newer);
      expect(heard).toEqual([DRAFT]);
    } finally {
      off();
    }
    keepReply(7, 'Written after');
    expect([returnReply(7, 'Failed', me), keptReply(7)]).toEqual([false, 'Written after']);
    expect([returnReply(8, 'Failed', me), keptReply(8)]).toEqual([true, 'Failed']);
  });

  it('take nothing back for a guest, nor for an account other than the one the send started under', () => {
    const heard: unknown[] = [];
    const off = [onRequestBack((back) => heard.push(back)), onReplyBack((...back) => heard.push(back))];
    try {
      as(null);
      expect([returnRequest(DRAFT, form, me), returnReply(7, 'Failed', me)]).toEqual([false, false]);
      as(`${me}-other`);
      expect([returnRequest(DRAFT, form, me), returnReply(7, 'Failed', me), keptRequest(), keptReply(7)]).toEqual([false, false, null, '']);
      expect([returnRequest(DRAFT, form, null), returnReply(7, 'Failed', null)]).toEqual([false, false]);
      expect(heard).toEqual([]);
    } finally {
      for (const stop of off) stop();
    }
    as(me);
    expect([keptRequest(), keptReply(7)]).toEqual([null, '']);
  });

  it('keep nothing for a form with nothing typed in it, and an emptied form forgets only what it wrote', () => {
    keepRequest({ ...DRAFT, game: null, subject: '  ', body: '' }, form);
    expect(keptRequest()).toBeNull();
    keepRequest(DRAFT, form);
    keepRequest({ ...DRAFT, game: null, subject: '', body: '' }, form);
    expect(keptRequest()).toBeNull();
    keepRequest(DRAFT, form);
    keepRequest({ ...DRAFT, game: null, subject: '', body: '' }, {});
    expect(keptRequest()).toEqual(DRAFT);
    forgetRequest(form);
    keepReply(7, 'Text');
    keepReply(7, '');
    expect(keptReply(7)).toBe('');
  });
});
