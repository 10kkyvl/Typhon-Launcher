import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { getTicket, replyTicket, resolveTicket, ticketLimits } from '../../lib/services/support';
import { resetHistory, route } from '../../lib/stores/router';
import { unreadReplies } from '../../lib/stores/support';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { clockTime, longDate, shortDate } from '../../lib/utils/format';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import { emitUpload, held, LIMITS, message, thread, ticket, words } from './supportTesting';
import TicketView from './TicketView.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/support', async (original) => (await import('./supportTesting')).fakeService(await original(), 'support'));
vi.mock('../../../bindings/typhon/internal/diagnostics', async (original) => (await import('./supportTesting')).fakeService(await original(), 'diagnostics'));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./supportTesting')).fakeService(await original(), 'account'));
vi.mock('../../lib/services/support', async (original) => (await import('./supportTesting')).supportService(await original()));
vi.mock('../../lib/services/logsUpload', async () => (await import('./supportTesting')).logsUpload());
vi.mock('../../lib/services/account', async (original) => (await import('./supportTesting')).accountOpeners(await original()));

const TICKET = ticket(7, 'Game will not start', { topic: 'launch' });
const MESSAGES = [
  message(1, 'user', 'It closes right after the logo.', '2026-11-01T10:00:00Z', { logBundleId: 'b1' }),
  message(2, 'staff', 'Which version of the launcher?', '2026-11-01T12:30:00Z'),
  message(3, 'user', 'The latest one.', '2026-11-01T13:00:00Z'),
];

beforeEach(() => {
  resetBindings();
  toasts.set([]);
  unreadReplies.set(0);
  vi.mocked(ticketLimits).mockResolvedValue(LIMITS);
  vi.mocked(getTicket).mockResolvedValue(thread(TICKET, MESSAGES));
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [ticketLimits, getTicket, replyTicket, resolveTicket]) vi.mocked(mock).mockReset();
  unreadReplies.set(0);
  toasts.set([]);
  authState.set('bootstrapping');
  resetHistory();
});

const said = () => get(toasts).map((item) => [item.kind, item.message]);
const replyBox = () => screen.getByRole('textbox', { name: new RegExp(msg('tickets.replyLabel')) }) as HTMLTextAreaElement;
const replyButton = () => screen.getByRole('button', { name: msg('tickets.reply') });

async function opened() {
  const view = renderDom(TicketView, { number: 7 });
  await screen.findByRole('heading', { level: 1, name: 'Game will not start' });
  return view;
}

describe('TicketView: the request', () => {
  it('is headed by its subject, with its reference, topic and status', async () => {
    await opened();
    const meta = words(screen.getByRole('heading', { level: 1 }).parentElement);
    expect(meta).toContain('T-7');
    expect(meta).toContain(msg('tickets.topicLaunch'));
    expect(meta).toContain(msg('tickets.statusOpen'));
    expect(getTicket).toHaveBeenCalledWith(7);
  });

  it('shows the messages as a list in the order they were written, each with who wrote it', async () => {
    await opened();
    const items = within(screen.getByRole('list')).getAllByRole('listitem');
    expect(items.map((item) => words(item))).toEqual([
      expect.stringMatching(new RegExp(`^${msg('tickets.you')}.*It closes right after the logo\\.`)),
      expect.stringMatching(new RegExp(`^${msg('tickets.staff')}.*Which version of the launcher\\?`)),
      expect.stringMatching(new RegExp(`^${msg('tickets.you')}.*The latest one\\.`)),
    ]);
    expect(words(items[0])).toContain(msg('tickets.logsAttached'));
    expect(words(items[1])).not.toContain(msg('tickets.logsAttached'));
  });

  it('names the thread, and says when each message was written: words on screen, the exact time to assistive tech and on hover', async () => {
    const { user } = await opened();
    const list = screen.getByRole('list', { name: msg('tickets.thread') });
    const items = within(list).getAllByRole('listitem');
    const written = new Date('2026-11-01T12:30:00Z');
    const exact = msg('tickets.messageTime', { date: longDate(written), time: clockTime(written) });
    const time = items[1].querySelector('time');

    expect(time?.getAttribute('datetime')).toBe('2026-11-01T12:30:00Z');
    expect(time?.querySelector('[aria-hidden="true"]')?.textContent).toBe(msg('tickets.messageTime', { date: shortDate(written), time: clockTime(written) }));
    expect(time?.querySelector('.sr-only')?.textContent).toBe(exact);
    expect(document.querySelector('[title]')).toBeNull();
    await user.hover(time as HTMLElement);
    await vi.waitFor(() => expect(document.querySelector('[data-part="tooltip"]')?.textContent?.trim()).toBe(exact));
  });

  it('says only the clock time for a message of today, and nothing for a time it cannot read', async () => {
    const now = new Date();
    vi.mocked(getTicket).mockResolvedValue(thread(TICKET, [message(1, 'staff', 'Hello.', now.toISOString()), message(2, 'user', 'Hi.', 'not a date')]));
    await opened();
    const items = within(screen.getByRole('list', { name: msg('tickets.thread') })).getAllByRole('listitem');
    expect(items[0].querySelector('time [aria-hidden="true"]')?.textContent).toBe(clockTime(now));
    expect(items[1].querySelector('time')).toBeNull();
    expect(words(items[1])).toBe(`${msg('tickets.you')} Hi.`);
  });

  it('goes back to the list', async () => {
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.back') }));
    expect(get(route)).toMatchObject({ name: 'support', params: {} });
  });

  it('says a request that is not there by its code', async () => {
    vi.mocked(getTicket).mockRejectedValue(new AccountError('ticket_not_found'));
    renderDom(TicketView, { number: 7 });
    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('errSupport.notFound')));
    expect(screen.queryByRole('heading', { level: 1 })).toBeNull();
  });
});

describe('TicketView: a reply', () => {
  it('sends the trimmed text without logs, then clears the box, says so and reads the request again', async () => {
    vi.mocked(replyTicket).mockResolvedValue(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z'));
    const { user } = await opened();
    await user.type(replyBox(), '  Still closes.  ');

    await user.click(replyButton());

    await vi.waitFor(() => expect(replyBox().value).toBe(''));
    expect(replyTicket).toHaveBeenCalledWith(7, 'Still closes.', false);
    expect(said()).toEqual([['success', msg('tickets.replySent')]]);
    expect(getTicket).toHaveBeenCalledTimes(2);
  });

  it('counts the text against the limit Go gives', async () => {
    vi.mocked(ticketLimits).mockResolvedValue({ maxSubjectRunes: 10, maxBodyRunes: 12 });
    const { user } = await opened();
    expect(screen.getByText(msg('tickets.counter', { count: 0, max: 12 }))).toBeTruthy();

    await user.type(replyBox(), 'Thirteen char');
    await user.click(replyButton());

    expect(replyBox().getAttribute('aria-invalid')).toBe('true');
    expect(screen.getByText(msg('tickets.errBodyTooLong', { max: 12 }))).toBeTruthy();
    expect(replyTicket).not.toHaveBeenCalled();
  });

  it('keeps the text when the reply is refused', async () => {
    vi.mocked(replyTicket).mockRejectedValue(new AccountError('ticket_invalid_body'));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');

    await user.click(replyButton());

    await vi.waitFor(() => expect(replyBox().getAttribute('aria-invalid')).toBe('true'));
    expect(replyBox().value).toBe('Still closes.');
    expect(screen.getByText(msg('errSupport.invalidBody'))).toBeTruthy();
  });

  it('asks before attaching logs, and sends nothing when the player declines', async () => {
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(screen.getByRole('switch', { name: msg('tickets.attachLogs') }));

    await user.click(replyButton());
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    expect(words(dialog)).toContain(msg('tickets.logsConfirmText'));
    expect(words(dialog)).toContain(msg('tickets.logsConfirmNote'));
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(replyTicket).not.toHaveBeenCalled();
    expect(replyBox().value).toBe('Still closes.');
  });

  it('sends the logs with the reply once the player agrees', async () => {
    vi.mocked(replyTicket).mockResolvedValue(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z', { logBundleId: 'b2' }));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(screen.getByRole('switch', { name: msg('tickets.attachLogs') }));

    await user.click(replyButton());
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    await user.click(within(dialog).getByRole('button', { name: msg('tickets.logsConfirmButton') }));

    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledWith(7, 'Still closes.', true));
    await vi.waitFor(() => expect(screen.getByRole('switch', { name: msg('tickets.attachLogs') }).getAttribute('aria-checked')).toBe('false'));
  });

  it('names the upload of the logs and says its value in words, and the consent cancelled gives focus back to the box', async () => {
    const sent = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(sent.promise);
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(screen.getByRole('switch', { name: msg('tickets.attachLogs') }));
    await user.click(replyBox());

    await user.keyboard('{Control>}{Enter}{/Control}');
    await screen.findByRole('dialog', { name: msg('tickets.logsConfirmTitle') });
    await user.keyboard('{Escape}');
    await vi.waitFor(() => expect(document.activeElement).toBe(replyBox()));
    expect(replyTicket).not.toHaveBeenCalled();

    await user.keyboard('{Control>}{Enter}{/Control}');
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: msg('tickets.logsConfirmButton') }));
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledWith(7, 'Still closes.', true));
    emitUpload({ state: 'sending', sentBytes: 768, totalBytes: 1024 });
    const bar = await screen.findByRole('progressbar', { name: msg('settings.aboutLogsLabel') });
    expect(bar.getAttribute('aria-valuetext')).toBe(msg('settings.aboutLogsSendingProgress', { percent: 75 }));
    sent.resolve(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z'));
    await vi.waitFor(() => expect(screen.queryByRole('progressbar')).toBeNull());
  });

  it('says a resolved request reopens with a reply', async () => {
    vi.mocked(getTicket).mockResolvedValue(thread({ ...TICKET, status: 'resolved' }, MESSAGES));
    await opened();
    expect(screen.getByText(msg('tickets.reopenNote'))).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('tickets.resolve') })).toBeNull();
    expect(replyButton()).toBeTruthy();
  });
});

describe('TicketView: the reply is the base multi-line field', () => {
  it('is named by its label alone and says how it is sent', async () => {
    await opened();
    const box = screen.getByRole('textbox', { name: msg('tickets.replyLabel') });
    expect(box.tagName).toBe('TEXTAREA');
    expect(description(box)).toBe(msg('tickets.sendHint', { keys: 'Ctrl Enter' }));
  });

  it('sends with Ctrl/Cmd+Enter, once while the answer is on its way, and stays focused and read-only meanwhile', async () => {
    const sent = held<ReturnType<typeof message>>();
    vi.mocked(replyTicket).mockReturnValue(sent.promise);
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');

    await user.keyboard('{Control>}{Enter}{/Control}');
    await vi.waitFor(() => expect(replyTicket).toHaveBeenCalledTimes(1));
    await user.keyboard('{Meta>}{Enter}{/Meta}');
    await user.click(screen.getByRole('button', { name: msg('tickets.replySending') }));

    expect(replyTicket).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: msg('tickets.replySending') }).getAttribute('aria-busy')).toBe('true');
    expect(replyBox().readOnly).toBe(true);
    expect(replyBox().disabled).toBe(false);
    sent.resolve(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z'));
    await vi.waitFor(() => expect(replyBox().value).toBe(''));
  });

  it('keeps a plain Enter a new line', async () => {
    const { user } = await opened();
    await user.type(replyBox(), 'one{Enter}two');
    expect(replyBox().value).toBe('one\ntwo');
    expect(replyTicket).not.toHaveBeenCalled();
  });

  it('after a reply is sent, clears the box and keeps focus in it', async () => {
    vi.mocked(replyTicket).mockResolvedValue(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z'));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');

    await user.keyboard('{Control>}{Enter}{/Control}');

    await vi.waitFor(() => expect(replyBox().value).toBe(''));
    expect(document.activeElement).toBe(replyBox());
    expect(replyBox().readOnly).toBe(false);
  });

  it('after a reply sent from the button, takes focus to the cleared box', async () => {
    vi.mocked(replyTicket).mockResolvedValue(message(4, 'user', 'Still closes.', '2026-11-02T09:00:00Z'));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');

    await user.click(replyButton());

    await vi.waitFor(() => expect(replyBox().value).toBe(''));
    await vi.waitFor(() => expect(document.activeElement).toBe(replyBox()));
  });

  it('a refusal of the text: at the box, with focus there', async () => {
    vi.mocked(replyTicket).mockRejectedValue(new AccountError('ticket_invalid_body'));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(replyButton());

    await vi.waitFor(() => expect(replyBox().getAttribute('aria-invalid')).toBe('true'));
    expect(document.activeElement).toBe(replyBox());
  });

  it.each([
    ['network_error', msg('state.accountErrorNetwork')],
    ['ticket_limit', msg('errSupport.limit')],
    ['something_new', msg('tickets.replyFailed')],
  ])('%s: once, in the reply form, keeping the text and the box valid', async (code, text) => {
    vi.mocked(replyTicket).mockRejectedValue(new AccountError(code));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(replyButton());

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(text));
    expect(alert.closest('.notice')).toBeTruthy();
    expect(screen.getAllByRole('alert')).toHaveLength(1);
    expect(replyBox().hasAttribute('aria-invalid')).toBe(false);
    expect(replyBox().value).toBe('Still closes.');
  });
});

describe('TicketView: a session gone', () => {
  it('on reading the request: says sign-in is needed and offers it instead of Retry', async () => {
    authState.set('authenticated');
    vi.mocked(getTicket).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = renderDom(TicketView, { number: 7 });

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('state.accountErrorUnauthenticated')));
    const notice = alert.closest('.notice') as HTMLElement;
    expect(within(notice).queryByRole('button', { name: msg('common.retry') })).toBeNull();
    await user.click(within(notice).getByRole('button', { name: msg('tickets.signInAgain') }));
    expect(get(authState)).toBe('unauthenticated');
  });

  it('on a reply: says so in the reply form, with the way to sign in', async () => {
    authState.set('authenticated');
    vi.mocked(replyTicket).mockRejectedValue(new AccountError('unauthenticated'));
    const { user } = await opened();
    await user.type(replyBox(), 'Still closes.');
    await user.click(replyButton());

    const alert = await screen.findByRole('alert');
    await vi.waitFor(() => expect(alert.textContent).toContain(msg('state.accountErrorUnauthenticated')));
    expect(within(alert.closest('.notice') as HTMLElement).getByRole('button', { name: msg('tickets.signInAgain') })).toBeTruthy();
  });
});

describe('TicketView: marking the request resolved', () => {
  it('asks first and sends nothing when the player cancels', async () => {
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.resolve') }));
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.resolveTitle') });
    expect(words(dialog)).toContain(msg('tickets.resolveText', { ref: 'T-7' }));

    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(resolveTicket).not.toHaveBeenCalled();
  });

  it('marks it resolved once confirmed, and says so', async () => {
    const done = held<typeof TICKET>();
    vi.mocked(resolveTicket).mockReturnValue(done.promise);
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.resolve') }));
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.resolveTitle') });

    await user.click(within(dialog).getByRole('button', { name: msg('tickets.resolve') }));
    expect(within(dialog).getByRole('button', { name: msg('tickets.resolveBusy') }).getAttribute('aria-busy')).toBe('true');
    done.resolve({ ...TICKET, status: 'resolved' });

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(resolveTicket).toHaveBeenCalledWith(7);
    expect(words(screen.getByRole('heading', { level: 1 }).parentElement)).toContain(msg('tickets.statusResolved'));
    expect(screen.queryByRole('button', { name: msg('tickets.resolve') })).toBeNull();
    expect(said()).toEqual([['success', msg('tickets.resolved')]]);
  });

  it('says a refusal by its code and keeps the request open', async () => {
    vi.mocked(resolveTicket).mockRejectedValue(new AccountError('network_error'));
    const { user } = await opened();
    await user.click(screen.getByRole('button', { name: msg('tickets.resolve') }));
    const dialog = await screen.findByRole('dialog', { name: msg('tickets.resolveTitle') });

    await user.click(within(dialog).getByRole('button', { name: msg('tickets.resolve') }));

    await vi.waitFor(() => expect(said()).toEqual([['danger', msg('state.accountErrorNetwork')]]));
    expect(words(screen.getByRole('heading', { level: 1 }).parentElement)).toContain(msg('tickets.statusOpen'));
  });
});
