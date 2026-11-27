import { cleanup, screen } from '@testing-library/svelte';
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
import { held, LIMITS, message, thread, ticket, words } from '../../lib/testing/supportTesting';
import { keptRequest, openPrefilled, returnRequest, spareRequest, type RequestDraft } from './drafts';
import Support from './Support.svelte';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('../../lib/testing/supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('../../lib/testing/supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('../../lib/testing/supportTesting')).accountOpeners(await original()));

// What a failed send's toast says is true when it is read: "back in the form" only while a form on
// screen shows the text in its fields, "you can restore it" only while the notice on screen names that
// text, else that it is kept and where it will be. Every one of them but "back in the form" offers to
// copy the text. An empty draft slot is a free place whoever asks.

const TICKET = ticket(7, 'Game will not start', { topic: 'launch' });
const player = (id: string) => ({ id, username: id, displayName: id }) as CurrentUser;
let players = 0;
let me = '';

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  unreadReplies.set(0);
  navigate('support', { view: 'new' });
  me = `sentences-${++players}`;
  currentUser.set(player(me));
  authState.set('authenticated');
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

const BETA = { view: 'new', topic: 'launcher', subject: 'Beta 2.0: ' };
const PLAIN = { view: 'new' };
const COPY = msg('tickets.copyText');
const X = 'Cannot sign in';
const X_TEXT = `${X}\n\nThe code never arrives.`;

const replyBox = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.replyLabel')) }) as HTMLTextAreaElement;
const subjectField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.subjectLabel')) }) as HTMLInputElement;
const bodyField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.bodyLabel')) }) as HTMLTextAreaElement;
const topicBox = () => screen.getByRole('combobox', { name: msg('tickets.topicLabel') });
const settled = async () => {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await new Promise((resolve) => setTimeout(resolve, 0));
};
const said = () => get(toasts).map((t) => [t.message, t.action?.label ?? null]);
/** The words of every notice that offers a kept request, as one text. */
const offered = () =>
  screen
    .queryAllByRole('button', { name: msg('tickets.draftRestore') })
    .map((button) => (button.closest('.notice') ? words(button.closest('.notice') as HTMLElement) : ''))
    .join(' | ');
const req = (subject: string, body: string): RequestDraft => ({ topic: 'account', game: null, subject, body, attachLogs: false, logsTouched: false });
const store = () => ({ draft: keptRequest()?.subject ?? null, spare: spareRequest()?.subject ?? null });

async function open(params: Record<string, string>) {
  const view = renderDom(Support, params);
  await screen.findByRole('button', { name: msg('tickets.submit') });
  return view;
}

async function copyAll(): Promise<string[]> {
  const written: string[] = [];
  const before = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
  Object.defineProperty(navigator, 'clipboard', { value: { writeText: async (text: string) => void written.push(text) }, configurable: true });
  try {
    for (const item of get(toasts)) if (item.action?.label === COPY) item.action.run();
    await settled();
  } finally {
    if (before) Object.defineProperty(navigator, 'clipboard', before);
    else Reflect.deleteProperty(navigator, 'clipboard');
  }
  return written;
}

/** A request typed in a plain form (account topic, no logs), sent and held; the form is then left. */
async function requestOnItsWayAndLeft(nth = 1) {
  const x = held<ReturnType<typeof ticket>>();
  vi.mocked(createTicket).mockReturnValueOnce(x.promise);
  const { user } = await open(PLAIN);
  await user.click(topicBox());
  await user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
  await user.clear(subjectField());
  await user.clear(bodyField());
  await user.type(subjectField(), X);
  await user.type(bodyField(), 'The code never arrives.');
  await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
  await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(nth));
  cleanup();
  return x;
}

/** A reply typed in request 7 and sent; held; the request is then left. */
async function replyOnItsWayAndLeft(text: string) {
  const r = held<ReturnType<typeof message>>();
  vi.mocked(replyTicket).mockReturnValueOnce(r.promise);
  const view = renderDom(TicketView, { number: 7 });
  await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
  await view.user.type(replyBox(), text);
  await view.user.keyboard('{Control>}{Enter}{/Control}');
  await vi.waitFor(() => expect(replyTicket).toHaveBeenCalled());
  return { r, view };
}

/**
 * The one toast of the failed request, judged by the rule: "back in the form" is never said here (no
 * form shows the text), "you can restore it" only while a notice on screen names it, and both other
 * sentences carry Copy.
 */
function judged(toast: (string | null)[] | undefined, notice: string) {
  const [words, action] = toast ?? [null, null];
  const sentence =
    words === msg('tickets.requestNotSentSaved', { subject: X })
      ? 'kept'
      : words === msg('tickets.requestNotSentOffered', { subject: X })
        ? notice.includes(X)
          ? 'offered, named'
          : 'offered, NOT named'
        : `other: ${words}`;
  return { sentence, copy: action === COPY };
}

describe('a request whose send fails with no form on screen', () => {
  it('kept as the draft, the spare taken, is not said to be back in the form: kept, with Copy, and the plain way in opens with it', async () => {
    const x = await requestOnItsWayAndLeft();
    expect(returnRequest(req('Spare matter', 'Spare body.'), {}, me)).toBe('kept');
    openPrefilled({});
    expect(store()).toEqual({ draft: null, spare: 'Spare matter' });
    x.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    const copied = await copyAll();
    await open(PLAIN);
    expect({ toast, copied, form: subjectField().value, store: store() }).toEqual({
      toast: [[msg('tickets.requestNotSentSaved', { subject: X }), COPY]],
      copied: [X_TEXT],
      form: X,
      store: { draft: X, spare: 'Spare matter' },
    });
  });

  it('kept as the draft, the spare free, is not said to be back in the form: kept, with Copy, and the Beta badge offers it', async () => {
    const x = await requestOnItsWayAndLeft();
    x.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    await open(BETA);
    expect({ toast, subject: subjectField().value, named: offered().includes(X) }).toEqual({
      toast: [[msg('tickets.requestNotSentSaved', { subject: X }), COPY]],
      subject: 'Beta 2.0: ',
      named: true,
    });
  });
});

describe('a request whose send fails beside a form that does not show it', () => {
  it('takes the empty draft slot beside an untouched prefilled form whose way in moved the draft to the spare', async () => {
    const x = await requestOnItsWayAndLeft();
    const { user } = await open(PLAIN);
    await user.type(subjectField(), 'Old matter');
    await user.type(bodyField(), 'Old body.');
    cleanup();
    await open(BETA);
    const opened = store();
    x.reject(new AccountError('network_error'));
    await settled();
    const after = { store: store(), toast: judged(said()[0], offered()), toasts: said().length, copied: await copyAll() };
    expect({ opened, after }).toEqual({
      opened: { draft: null, spare: 'Old matter' },
      after: { store: { draft: X, spare: 'Old matter' }, toast: after.toast, toasts: 1, copied: [X_TEXT] },
    });
    expect(after.toast.copy).toBe(true);
    expect(after.toast.sentence).toBe('offered, named');
  });

  it('kept as the draft beside a form that sends its own, the spare taken: "you can restore it" only if the notice names it, and Copy', async () => {
    const x = await requestOnItsWayAndLeft();
    expect(returnRequest(req('Spare matter', 'Spare body.'), {}, me)).toBe('kept');
    openPrefilled({});
    const t = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValueOnce(t.promise);
    const f = await open(PLAIN);
    await f.user.click(topicBox());
    await f.user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
    await f.user.type(bodyField(), ' more');
    expect(returnRequest(req('Other spare', 'Other body.'), {}, me)).toBe('offered');
    await f.user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(2));
    x.reject(new AccountError('network_error'));
    await settled();
    const toast = judged(said()[0], offered());
    expect({ store: store(), copy: toast.copy, toasts: said().length }).toEqual({ store: { draft: X, spare: 'Other spare' }, copy: true, toasts: 1 });
    expect(toast.sentence).toBe('offered, named');
    t.reject(new AccountError('network_error'));
    await settled();
  });

  it('taken into a form whose fields are not on screen yet (its limits still read) is not said to be back in the form', async () => {
    const x = await requestOnItsWayAndLeft();
    const limits = held<typeof LIMITS>();
    vi.mocked(ticketLimits).mockReturnValueOnce(limits.promise);
    renderDom(Support, PLAIN);
    x.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    limits.resolve(LIMITS);
    await screen.findByRole('button', { name: msg('tickets.submit') });
    expect({ toast, form: subjectField().value }).toEqual({ toast: [[msg('tickets.requestNotSentSaved', { subject: X }), COPY]], form: X });
  });

  it('taken into a form whose limits read failed is not said to be back in the form: kept, with Copy, and in the fields after Retry', async () => {
    const x = await requestOnItsWayAndLeft();
    vi.mocked(ticketLimits).mockRejectedValueOnce(new AccountError('network_error'));
    const { user } = renderDom(Support, PLAIN);
    await screen.findByRole('button', { name: msg('common.retry') });
    x.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    const kept = store();
    await user.click(screen.getByRole('button', { name: msg('common.retry') }));
    await screen.findByRole('button', { name: msg('tickets.submit') });
    expect({ toast, kept, form: [subjectField().value, bodyField().value] }).toEqual({
      toast: [[msg('tickets.requestNotSentSaved', { subject: X }), COPY]],
      kept: { draft: X, spare: null },
      form: [X, 'The code never arrives.'],
    });
  });

  it('offered by a prefilled form whose notice is not on screen yet (its limits still read) is not said to be restorable there', async () => {
    const x = await requestOnItsWayAndLeft();
    const limits = held<typeof LIMITS>();
    vi.mocked(ticketLimits).mockReturnValueOnce(limits.promise);
    renderDom(Support, BETA);
    x.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    limits.resolve(LIMITS);
    await screen.findByRole('button', { name: msg('tickets.submit') });
    expect({ toast, store: store(), named: offered().includes(X) }).toEqual({
      toast: [[msg('tickets.requestNotSentSaved', { subject: X }), COPY]],
      store: { draft: null, spare: X },
      named: true,
    });
  });

  it('taken into a form on screen that holds nothing is back in the form, with no Copy', async () => {
    const x = await requestOnItsWayAndLeft();
    await open(PLAIN);
    x.reject(new AccountError('network_error'));
    await settled();
    expect({ toast: said(), form: subjectField().value }).toEqual({ toast: [[msg('tickets.requestNotSentKept', { subject: X }), null]], form: X });
  });
});

describe('a reply whose send fails with its box not on screen', () => {
  it('kept for its request, no request open, says it is kept, with Copy, and is in the box on opening it', async () => {
    const { r } = await replyOnItsWayAndLeft('First.');
    cleanup();
    r.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    const copied = await copyAll();
    renderDom(TicketView, { number: 7 });
    await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
    expect({ toast, copied, box: replyBox().value }).toEqual({
      toast: [[msg('tickets.replyNotSentSaved', { subject: 'Game will not start' }), COPY]],
      copied: ['First.'],
      box: 'First.',
    });
  });

  it('kept until the reply on its way from the box is done offers Copy meanwhile', async () => {
    const { r } = await replyOnItsWayAndLeft('First.');
    cleanup();
    const second = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValueOnce(second.promise);
    const two = renderDom(TicketView, { number: 7 });
    await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
    await two.user.type(replyBox(), 'Second.');
    await two.user.keyboard('{Control>}{Enter}{/Control}');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(2));
    r.reject(new AccountError('network_error'));
    await settled();
    expect(said()).toEqual([[msg('tickets.replyNotSentAfter', { subject: 'Game will not start' }), COPY]]);
    expect(await copyAll()).toEqual(['First.']);
    second.reject(new AccountError('network_error'));
    await settled();
  });

  it('taken into a box that is not drawn yet (its limits still read) is not said to be back in the box', async () => {
    const { r } = await replyOnItsWayAndLeft('First.');
    cleanup();
    const limits = held<typeof LIMITS>();
    vi.mocked(ticketLimits).mockReturnValueOnce(limits.promise);
    renderDom(TicketView, { number: 7 });
    await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
    r.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    limits.resolve(LIMITS);
    await vi.waitFor(() => expect(replyBox().value).toBe('First.'));
    expect(toast).toEqual([[msg('tickets.replyNotSentSaved', { subject: 'Game will not start' }), COPY]]);
  });

  it('taken into a box whose request is still being read (its limits read) is not said to be back in the box', async () => {
    const { r } = await replyOnItsWayAndLeft('First.');
    cleanup();
    const t = held<ReturnType<typeof thread>>();
    vi.mocked(getTicket).mockReturnValueOnce(t.promise);
    renderDom(TicketView, { number: 7 });
    await settled();
    r.reject(new AccountError('network_error'));
    await settled();
    const toast = said();
    t.resolve(thread(TICKET, []));
    await vi.waitFor(() => expect(replyBox().value).toBe('First.'));
    expect(toast).toEqual([[msg('tickets.replyNotSentSaved', { subject: 'Game will not start' }), COPY]]);
  });

  it('taken into the open box that holds nothing is back in the box, with no Copy', async () => {
    const { r } = await replyOnItsWayAndLeft('First.');
    cleanup();
    renderDom(TicketView, { number: 7 });
    await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
    await screen.findByRole('textbox', { name: new RegExp(msg('tickets.replyLabel')) });
    r.reject(new AccountError('network_error'));
    await settled();
    expect({ toast: said(), box: replyBox().value }).toEqual({ toast: [[msg('tickets.replyNotSentKept', { subject: 'Game will not start' }), null]], box: 'First.' });
  });
});
