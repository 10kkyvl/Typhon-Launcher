import { cleanup, fireEvent, screen, within } from '@testing-library/svelte';
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
import { held, LIMITS, message, thread, ticket, words } from '../../lib/testing/supportTesting';
import { keptReply, keptRequest } from './drafts';
import NewTicket from './NewTicket.svelte';
import Support from './Support.svelte';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('../../lib/testing/supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('../../lib/testing/supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('../../lib/testing/supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('../../lib/testing/supportTesting')).accountOpeners(await original()));

// A text leaves what is kept for the player when its send starts, and comes back only if the send fails
// and nothing newer was kept meanwhile: leaving and coming back during a send can neither show the text
// as unsent nor send it twice.

const TICKET = ticket(7, 'Game will not start', { topic: 'launch' });
const player = (id: string) => ({ id, username: id, displayName: id }) as CurrentUser;
let players = 0;

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  unreadReplies.set(0);
  navigate('support', { ticket: '7' });
  currentUser.set(player(`sender-${++players}`));
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

const replyBox = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.replyLabel')) }) as HTMLTextAreaElement;
const subjectField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.subjectLabel')) }) as HTMLInputElement;
const bodyField = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.bodyLabel')) }) as HTMLTextAreaElement;
const logsSwitch = () => screen.getByRole('switch', { name: msg('tickets.attachLogs') });
const topicBox = () => screen.getByRole('combobox', { name: msg('tickets.topicLabel') });
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
const replies = () => vi.mocked(replyTicket).mock.calls.map((call) => call[1]);
const said = () => get(toasts).map((t) => [t.kind, t.message]);
const actions = () => get(toasts).map((t) => t.action?.label ?? null);

/**
 * Runs the action of toast `index` over a clipboard the test reads, put in place only for the run:
 * user-event sets its own on `navigator` with every render. `fails` makes the write refuse. Answers
 * what was written.
 */
async function copyFromToast(index: number, fails = false): Promise<string[]> {
  const action = get(toasts)[index]?.action;
  if (!action) throw new Error(`toast ${index} has no action`);
  const written: string[] = [];
  const writeText = async (text: string) => {
    if (fails) throw new Error('clipboard refused');
    written.push(text);
  };
  const before = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
  try {
    action.run();
    await settled();
  } finally {
    if (before) Object.defineProperty(navigator, 'clipboard', before);
    else Reflect.deleteProperty(navigator, 'clipboard');
  }
  return written;
}

const offer = () => {
  const plate = screen.queryByRole('button', { name: msg('tickets.draftRestore') })?.closest('.notice');
  return plate ? words(plate) : null;
};

async function opened() {
  const view = renderDom(TicketView, { number: 7 });
  await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
  return view;
}

async function send(user: ReturnType<typeof renderDom>['user'], text: string) {
  await user.type(replyBox(), text);
  await user.keyboard('{Control>}{Enter}{/Control}');
}

/** The form as the "New request" button of the list opens it. */
async function newRequest() {
  const view = renderDom(Support, { view: 'new' });
  await screen.findByRole('button', { name: msg('tickets.submit') });
  return view;
}

/** A request typed in the form and sent; its Create stays on its way until the test answers it. */
async function requestOnItsWay() {
  const first = held<ReturnType<typeof ticket>>();
  vi.mocked(createTicket).mockReturnValueOnce(first.promise);
  const { user } = await newRequest();
  await user.click(topicBox());
  await user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
  await user.type(subjectField(), 'Cannot sign in');
  await user.type(bodyField(), 'The code never arrives.');
  await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
  await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
  return { first, user };
}

function shownRequest() {
  return {
    topic: (topicBox().textContent ?? '').trim(),
    logs: logsSwitch().getAttribute('aria-checked'),
    subject: subjectField().value,
    body: bodyField().value,
  };
}

const SENT = { topic: msg('tickets.topicAccount'), logs: 'false', subject: 'Cannot sign in', body: 'The code never arrives.' };

describe('a reply on its way', () => {
  it('is not in the box when its request is opened again after it was sent', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(first.promise);
    const { user } = await opened();
    await send(user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    first.resolve(message(4, 'user', 'First.', 'x'));
    await settled();
    await settled();
    await opened();
    expect(replyBox().value).toBe('');
  });

  it('cannot be sent twice by opening its request again before it answers', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValueOnce(first.promise).mockResolvedValue(message(5, 'user', 'First.', 'y'));
    const { user } = await opened();
    await send(user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    const again = await opened();
    const shown = replyBox().value;
    await again.user.click(replyBox());
    await again.user.keyboard('{Control>}{Enter}{/Control}');
    await settled();
    first.resolve(message(4, 'user', 'First.', 'x'));
    await settled();
    expect({ shown, replies: replies() }).toEqual({ shown: '', replies: ['First.'] });
  });

  it('that fails after its request was left is in the box again, and a toast names the request', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(first.promise);
    const { user } = await opened();
    await send(user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    expect(said()).toEqual([['danger', msg('tickets.replyNotSentKept', { subject: 'Game will not start' })]]);
    await opened();
    expect(replyBox().value).toBe('First.');
  });

  it('that fails while its request is open again and its box untouched comes back into that box', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(first.promise);
    const { user } = await opened();
    await send(user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    await opened();
    expect(replyBox().value).toBe('');
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(replyBox().value).toBe('First.'));
    expect(said()).toEqual([['danger', msg('tickets.replyNotSentKept', { subject: 'Game will not start' })]]);
  });

  it('that fails after the player wrote another leaves the newer text, and a toast says the failed one was not sent', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(first.promise);
    const { user } = await opened();
    await send(user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    const again = await opened();
    await again.user.type(replyBox(), 'Newer.');
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    expect({ box: replyBox().value, said: said() }).toEqual({
      box: 'Newer.',
      said: [['danger', msg('tickets.replyNotSentNewer', { subject: 'Game will not start' })]],
    });
    cleanup();
    await opened();
    expect(replyBox().value).toBe('Newer.');
  });

  it('that fails after the player wrote another offers to copy it from its toast', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(first.promise);
    const { user } = await opened();
    await send(user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    const again = await opened();
    await again.user.type(replyBox(), 'Newer.');
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    const offered = actions();
    expect({ offered, written: await copyFromToast(0) }).toEqual({ offered: [msg('tickets.copyText')], written: ['First.'] });
  });

  it('sent after its request was left forgets nothing the player wrote since', async () => {
    const first = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(first.promise);
    const { user } = await opened();
    await send(user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    const again = await opened();
    await again.user.type(replyBox(), 'Next.');
    first.resolve(message(4, 'user', 'First.', 'x'));
    await settled();
    await settled();
    cleanup();
    await opened();
    expect(replyBox().value).toBe('Next.');
  });

  it('that fails while the box holds a newer reply on its way stays out of the box until that one is sent, then is in it', async () => {
    const first = held<ReturnType<typeof message>>();
    const second = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const one = await opened();
    await send(one.user, 'First.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    cleanup();
    const two = await opened();
    await send(two.user, 'Second.');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(2));
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    const meanwhile = { said: said(), box: replyBox().value };
    second.resolve(message(5, 'user', 'Second.', 'y'));
    await settled();
    await settled();
    const after = replyBox().value;
    await two.user.type(replyBox(), ' Third.');
    expect({ meanwhile, after, kept: keptReply(7) }).toEqual({
      meanwhile: { said: [['danger', msg('tickets.replyNotSentKept', { subject: 'Game will not start' })]], box: 'Second.' },
      after: 'First.',
      kept: 'First. Third.',
    });
  });

  it('that fails with its request on screen stays in the box and is kept, said in the form and not by a toast', async () => {
    vi.mocked(replyTicket).mockRejectedValue(new AccountError('network_error'));
    const { user } = await opened();
    await send(user, 'Kept.');
    await screen.findByRole('alert');
    expect({ box: replyBox().value, said: said() }).toEqual({ box: 'Kept.', said: [] });
    cleanup();
    await opened();
    expect(replyBox().value).toBe('Kept.');
  });
});

describe('a new request on its way', () => {
  it('cannot be created twice by opening the form again before it answers', async () => {
    const first = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValueOnce(first.promise).mockResolvedValue(ticket(22, 'Cannot sign in'));
    const view = renderDom(NewTicket, { initialTopic: 'account' });
    await screen.findByRole('button', { name: msg('tickets.submit') });
    if (logsSwitch().getAttribute('aria-checked') === 'true') await view.user.click(logsSwitch());
    await view.user.type(subjectField(), 'Cannot sign in');
    await view.user.type(bodyField(), 'The code never arrives.');
    await view.user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    cleanup();
    const again = renderDom(NewTicket, { initialTopic: 'account' });
    await screen.findByRole('button', { name: msg('tickets.submit') });
    const shown = subjectField().value;
    await again.user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await settled();
    first.resolve(ticket(21, 'Cannot sign in'));
    await settled();
    expect({ shown, creates: vi.mocked(createTicket).mock.calls.length }).toEqual({ shown: '', creates: 1 });
  });

  it('created after the form was left is still said, opens nothing, and forgets nothing typed since', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    expect(subjectField().value).toBe('');
    await user.type(subjectField(), 'Another matter');
    first.resolve(ticket(21, 'Cannot sign in'));
    await settled();
    await settled();
    expect({ said: said(), route: get(route).params }).toEqual({ said: [['success', msg('tickets.created', { ref: 'T-21' })]], route: { ticket: '7' } });
    cleanup();
    await newRequest();
    expect(subjectField().value).toBe('Another matter');
  });

  it('that fails after the form was left is in the form again, whole, and a toast names it', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    expect(said()).toEqual([['danger', msg('tickets.requestNotSentKept', { subject: 'Cannot sign in' })]]);
    expect(actions()).toEqual([null]);
    await newRequest();
    expect(shownRequest()).toEqual(SENT);
  });

  it('that fails while the form is open again and untouched comes back into it', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    await newRequest();
    expect(subjectField().value).toBe('');
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(shownRequest()).toEqual(SENT));
  });

  it('that fails after the player typed another keeps the newer one, and a toast says the failed one was not sent', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Another matter');
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    expect({ subject: subjectField().value, said: said() }).toEqual({
      subject: 'Another matter',
      said: [['danger', msg('tickets.requestNotSentNewer', { subject: 'Cannot sign in' })]],
    });
    cleanup();
    await newRequest();
    expect(subjectField().value).toBe('Another matter');
  });

  it('that fails while the plain form is open again with only its topic changed comes back into it, and is kept as the player goes on', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicOther') }));
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    const back = { said: said(), form: shownRequest() };
    await user.type(subjectField(), ' again');
    cleanup();
    await newRequest();
    expect({ ...back, later: subjectField().value }).toEqual({
      said: [['danger', msg('tickets.requestNotSentKept', { subject: 'Cannot sign in' })]],
      form: SENT,
      later: 'Cannot sign in again',
    });
  });

  it('that fails while the form holds text the player typed is offered beside it, not said to be back in the form', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Another matter');
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    const back = { subject: subjectField().value, said: said(), offered: offer() };
    await user.type(bodyField(), 'Still typing.');
    expect({ ...back, offeredWhileTyping: offer() !== null }).toEqual({
      subject: 'Another matter',
      said: [['danger', msg('tickets.requestNotSentNewer', { subject: 'Cannot sign in' })]],
      offered: `${msg('tickets.draftKept', { subject: 'Cannot sign in' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}`,
      offeredWhileTyping: true,
    });
  });

  it('offered beside typed text swaps with it on "Restore it": the typed text is offered in turn, the restored one kept', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Another matter');
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(offer()).not.toBeNull());
    await user.click(screen.getByRole('button', { name: msg('tickets.draftRestore') }));
    await vi.waitFor(() => expect(document.activeElement).toBe(subjectField()));
    const swapped = { form: shownRequest(), offered: offer() };
    await user.click(screen.getByRole('button', { name: msg('tickets.draftRestore') }));
    const back = { subject: subjectField().value, offered: offer() };
    await user.click(screen.getByRole('button', { name: msg('tickets.draftRestore') }));
    cleanup();
    await newRequest();
    expect({ swapped, back, later: shownRequest() }).toEqual({
      swapped: { form: SENT, offered: `${msg('tickets.draftKept', { subject: 'Another matter' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}` },
      back: { subject: 'Another matter', offered: `${msg('tickets.draftKept', { subject: 'Cannot sign in' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}` },
      later: SENT,
    });
  });

  it('offered beside typed text is still offered when the form is left and opened again', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Another matter');
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(offer()).not.toBeNull());
    cleanup();
    await newRequest();
    expect({ subject: subjectField().value, offered: offer() }).toEqual({
      subject: 'Another matter',
      offered: `${msg('tickets.draftKept', { subject: 'Cannot sign in' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}`,
    });
  });

  it('restored in place of typed text leaves that text offered, as the notice said, when the form is left and opened again', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Typed meanwhile');
    first.reject(new AccountError('network_error'));
    await vi.waitFor(() => expect(offer()).not.toBeNull());
    await user.click(screen.getByRole('button', { name: msg('tickets.draftRestore') }));
    await vi.waitFor(() => expect(subjectField().value).toBe('Cannot sign in'));
    cleanup();
    await newRequest();
    expect({ form: shownRequest(), offered: offer() }).toEqual({
      form: SENT,
      offered: `${msg('tickets.draftKept', { subject: 'Typed meanwhile' })} ${msg('tickets.draftSwapNote')} ${msg('tickets.draftRestore')}`,
    });
  });

  it('that fails while the open form sends another is kept, offered there, and in the form once that one is created', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const second = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValueOnce(second.promise);
    const { user } = await newRequest();
    await user.type(subjectField(), 'Another matter');
    await user.type(bodyField(), 'Also broken.');
    if (logsSwitch().getAttribute('aria-checked') === 'true') await user.click(logsSwitch());
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(2));
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    const meanwhile = {
      said: said(),
      offered: offer() !== null,
      restoreHeld: (screen.getByRole('button', { name: msg('tickets.draftRestore') }) as HTMLButtonElement).disabled,
      subject: subjectField().value,
    };
    second.resolve(ticket(24, 'Another matter'));
    await settled();
    await settled();
    cleanup();
    await newRequest();
    expect({ meanwhile, later: shownRequest() }).toEqual({
      meanwhile: {
        said: [['danger', msg('tickets.requestNotSentNewer', { subject: 'Cannot sign in' })]],
        offered: true,
        restoreHeld: true,
        subject: 'Another matter',
      },
      later: SENT,
    });
  });

  it('that fails with its form on screen is said there and offers nothing to the form it came from', async () => {
    vi.mocked(createTicket).mockRejectedValue(new AccountError('network_error'));
    const { user } = await newRequest();
    await user.type(subjectField(), 'Cannot sign in');
    await user.type(bodyField(), 'The code never arrives.');
    if (logsSwitch().getAttribute('aria-checked') === 'true') await user.click(logsSwitch());
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await screen.findByRole('alert');
    expect({ offered: offer(), said: said(), subject: subjectField().value }).toEqual({ offered: null, said: [], subject: 'Cannot sign in' });
  });

  it('whose text stays out of the form offers to copy it from its toast, and says it is copied', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Another matter');
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    const offered = actions();
    const written = await copyFromToast(0);
    expect({ offered, written, said: said()[1] }).toEqual({
      offered: [msg('tickets.copyText')],
      written: ['Cannot sign in\n\nThe code never arrives.'],
      said: ['success', msg('tickets.textCopied')],
    });
  });

  it('says so when the copy from its toast fails', async () => {
    const { first } = await requestOnItsWay();
    cleanup();
    const { user } = await newRequest();
    await user.type(subjectField(), 'Another matter');
    first.reject(new AccountError('network_error'));
    await settled();
    await settled();
    const written = await copyFromToast(0, true);
    expect({ written, said: said()[1] }).toEqual({ written: [], said: ['danger', msg('tickets.textCopyFailed')] });
  });

  it('takes no topic meanwhile, so nothing puts its text back in the draft and it is not created twice', async () => {
    const { first, user } = await requestOnItsWay();
    const held = (topicBox() as HTMLButtonElement).disabled;
    await user.click(topicBox());
    const other = screen.queryByRole('option', { name: msg('tickets.topicOther') });
    if (other) await user.click(other);
    const keptMeanwhile = keptRequest();
    cleanup();
    vi.mocked(createTicket).mockResolvedValue(ticket(23, 'Cannot sign in'));
    const again = await newRequest();
    const reopened = subjectField().value;
    if (reopened) {
      await again.user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
      await settled();
    }
    first.resolve(ticket(22, 'Cannot sign in'));
    await settled();
    await settled();
    expect({ held, keptMeanwhile, reopened, creates: vi.mocked(createTicket).mock.calls.length }).toEqual({ held: true, keptMeanwhile: null, reopened: '', creates: 1 });
  });

  it('keeps nothing that reaches the form meanwhile, even an input event on a held field', async () => {
    const { first } = await requestOnItsWay();
    await fireEvent.input(subjectField(), { target: { value: 'Cannot sign in at all' } });
    await settled();
    const keptMeanwhile = keptRequest();
    first.resolve(ticket(22, 'Cannot sign in'));
    await settled();
    await settled();
    expect(keptMeanwhile).toBeNull();
  });

  it('hands focus on to the busy Send when it was on the topic, held now, and the click on Send took none', async () => {
    const first = held<ReturnType<typeof ticket>>();
    vi.mocked(createTicket).mockReturnValueOnce(first.promise);
    const { user } = await newRequest();
    await user.type(subjectField(), 'Cannot sign in');
    await user.type(bodyField(), 'The code never arrives.');
    await user.click(topicBox());
    await user.click(screen.getByRole('option', { name: msg('tickets.topicAccount') }));
    expect(document.activeElement).toBe(topicBox());
    // A click in the macOS WebKit view does not focus the button it presses.
    await fireEvent.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await vi.waitFor(() => expect(createTicket).toHaveBeenCalledTimes(1));
    await settled();
    expect({ held: (topicBox() as HTMLButtonElement).disabled, focus: document.activeElement?.textContent?.trim() }).toEqual({
      held: true,
      focus: msg('tickets.submitting'),
    });
    first.resolve(ticket(22, 'Cannot sign in'));
    await settled();
    await settled();
  });

  it('is kept when the player declines the logs: nothing left, so nothing was taken', async () => {
    const { user } = await newRequest();
    await user.type(subjectField(), 'Cannot sign in');
    await user.type(bodyField(), 'The code never arrives.');
    if (logsSwitch().getAttribute('aria-checked') === 'false') await user.click(logsSwitch());
    await user.click(screen.getByRole('button', { name: msg('tickets.submit') }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('common.cancel') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    cleanup();
    await newRequest();
    expect({ subject: subjectField().value, body: bodyField().value, creates: vi.mocked(createTicket).mock.calls.length }).toEqual({
      subject: 'Cannot sign in',
      body: 'The code never arrives.',
      creates: 0,
    });
  });
});
