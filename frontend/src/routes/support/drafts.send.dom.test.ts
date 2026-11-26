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
import { held, LIMITS, message, thread, ticket } from '../../lib/testing/supportTesting';
import { keptRequest } from './drafts';
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
